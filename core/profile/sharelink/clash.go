package sharelink

import (
	"bytes"
	"errors"
	"io"
	"strings"

	"go.yaml.in/yaml/v3"
)

// ParseClash imports inline proxy definitions. The caller presents the
// proxy-only limitation because Clash routing and DNS have their own semantics.
func ParseClash(raw []byte) (Result, bool, error) {
	looksLikeClash := false
	for line := range bytes.SplitSeq(raw, []byte{'\n'}) {
		key, _, _ := bytes.Cut(line, []byte{':'})
		switch string(bytes.TrimSuffix(key, []byte{'\r'})) {
		case "proxies", "proxy-providers", "proxy-groups":
			looksLikeClash = true
		}
		if looksLikeClash {
			break
		}
	}
	if !looksLikeClash {
		return Result{}, false, nil
	}
	var doc map[string]any
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	if decoder.Decode(&doc) != nil || decoder.Decode(new(any)) != io.EOF {
		return Result{}, true, errors.New("subscription contains invalid Clash YAML")
	}
	proxies, ok := doc["proxies"].([]any)
	if !ok && doc["proxies"] != nil {
		return Result{}, true, errors.New("subscription contains invalid Clash YAML")
	}
	if len(proxies) > maxLinks {
		return Result{}, true, errors.New("subscription has too many proxy links")
	}
	result := newResult("clash_yaml")
	result.Summary.ProxyOnly = len(doc) > 1
	seen := make(map[string]bool)
	for _, item := range proxies {
		node, label, reason := parseClashProxy(item)
		result.add(node, label, reason, seen)
	}
	result.finish()
	return result, true, nil
}

func parseClashProxy(raw any) (map[string]any, string, string) {
	proxy, ok := raw.(map[string]any)
	if !ok {
		return nil, "", InvalidLink
	}
	text := func(key string) string {
		value, _ := proxy[key].(string)
		return value
	}
	protocol, host := text("type"), text("server")
	port, ok := proxy["port"].(int)
	if !ok {
		port, ok = readPort(text("port"), 0)
	}
	if !ok || port <= 0 || port > 65535 || !validHost(host) {
		return nil, "", InvalidLink
	}
	node := outbound(protocol, host, port)
	q := make(map[string]string)
	allowed := "name type server port udp"
	if skip, ok := proxy["skip-cert-verify"].(bool); ok && skip {
		q["insecure"] = "1"
	}
	if list, valid := proxy["alpn"].([]any); valid {
		alpn := make([]string, 0, len(list))
		for _, value := range list {
			part, ok := value.(string)
			if !ok || strings.ContainsRune(part, ',') {
				return nil, "", InvalidLink
			}
			alpn = append(alpn, part)
		}
		q["alpn"] = strings.Join(alpn, ",")
	} else if proxy["alpn"] != nil {
		return nil, "", InvalidLink
	}
	var reason string
	switch protocol {
	case "vless", "vmess", "trojan":
		allowed += " uuid password alterId cipher tls skip-cert-verify network servername sni client-fingerprint reality-opts grpc-opts ws-opts alpn flow packet-encoding"
		q["type"], q["sni"], q["fp"] = text("network"), text("servername"), text("client-fingerprint")
		if q["sni"] == "" {
			q["sni"] = text("sni")
		}
		if proxy["tls"] == true || protocol == "trojan" {
			q["security"] = "tls"
		}
		if reality := proxy["reality-opts"]; reality != nil {
			fields, valid := clashOptions(reality, "public-key short-id")
			if !valid {
				return nil, "", UnsupportedParameter
			}
			q["security"] = "reality"
			q["pbk"], _ = fields["public-key"].(string)
			q["sid"], _ = fields["short-id"].(string)
			if q["pbk"] == "" {
				return nil, "", InvalidLink
			}
		}
		q["flow"], q["packetEncoding"] = text("flow"), text("packet-encoding")
		if reason = clashTransport(proxy, q); reason != "" {
			return nil, "", reason
		}
		if protocol == "trojan" {
			node["password"] = text("password")
			if node["password"] == "" || proxy["uuid"] != nil || proxy["alterId"] != nil || proxy["cipher"] != nil {
				return nil, "", InvalidLink
			}
		} else {
			if !validUUID(text("uuid")) || proxy["password"] != nil {
				return nil, "", InvalidLink
			}
			node["uuid"] = strings.ToLower(text("uuid"))
			if protocol == "vmess" {
				alter, _ := proxy["alterId"].(int)
				if alter < 0 || alter > 65535 {
					return nil, "", InvalidLink
				}
				node["alter_id"] = alter
				cipher := text("cipher")
				if cipher == "" {
					cipher = "auto"
				}
				switch cipher {
				case "auto", "aes-128-gcm", "chacha20-poly1305", "none", "zero":
					node["security"] = cipher
				default:
					return nil, "", UnsupportedParameter
				}
			} else if proxy["alterId"] != nil || proxy["cipher"] != nil {
				return nil, "", UnsupportedParameter
			}
		}
		reason = streamOptions(node, q, protocol == "trojan")
	case "hysteria2", "tuic":
		allowed += " password sni alpn skip-cert-verify"
		if text("password") == "" {
			return nil, "", InvalidLink
		}
		node["password"] = text("password")
		q["sni"] = text("sni")
		if protocol == "hysteria2" {
			allowed += " obfs obfs-password"
			q["obfs"], q["obfs-password"] = text("obfs"), text("obfs-password")
			reason = hysteriaOptions(node, q)
		} else {
			allowed += " uuid congestion-controller udp-relay-mode reduce-rtt"
			if !validUUID(text("uuid")) {
				return nil, "", InvalidLink
			}
			node["uuid"] = strings.ToLower(text("uuid"))
			q["congestion_control"], q["udp_relay_mode"] = text("congestion-controller"), text("udp-relay-mode")
			if proxy["reduce-rtt"] == true {
				q["zero_rtt_handshake"] = "1"
			}
			reason = tuicOptions(node, q)
		}
	case "ss":
		allowed += " cipher password"
		switch text("cipher") {
		case "aes-128-gcm", "aes-192-gcm", "aes-256-gcm", "chacha20-ietf-poly1305", "xchacha20-ietf-poly1305",
			"2022-blake3-aes-128-gcm", "2022-blake3-aes-256-gcm", "2022-blake3-chacha20-poly1305":
			node["type"], node["method"], node["password"] = "shadowsocks", text("cipher"), text("password")
			if text("password") == "" {
				return nil, "", InvalidLink
			}
		default:
			return nil, "", UnsupportedParameter
		}
	default:
		return nil, "", UnsupportedProtocol
	}
	if reason != "" {
		return nil, "", reason
	}
	if _, ok := clashOptions(proxy, allowed); !ok {
		return nil, "", UnsupportedParameter
	}
	for _, key := range []string{"tls", "skip-cert-verify", "reduce-rtt", "udp"} {
		if value := proxy[key]; value != nil {
			if _, ok := value.(bool); !ok {
				return nil, "", InvalidLink
			}
		}
	}
	if proxy["udp"] == false {
		if protocol == "hysteria2" || protocol == "tuic" {
			return nil, "", UnsupportedParameter
		}
		node["network"] = "tcp"
	}
	return node, text("name"), ""
}

func clashTransport(proxy map[string]any, q map[string]string) string {
	if raw := proxy["grpc-opts"]; raw != nil {
		options, ok := clashOptions(raw, "grpc-service-name")
		if !ok || q["type"] != "grpc" {
			return UnsupportedParameter
		}
		q["serviceName"], _ = options["grpc-service-name"].(string)
	}
	if raw := proxy["ws-opts"]; raw != nil {
		options, ok := clashOptions(raw, "path headers")
		if !ok || q["type"] != "ws" {
			return UnsupportedParameter
		}
		q["path"], _ = options["path"].(string)
		if headers := options["headers"]; headers != nil {
			fields, ok := clashOptions(headers, "Host host")
			if !ok || len(fields) != 1 {
				return UnsupportedParameter
			}
			for _, value := range fields {
				q["host"], _ = value.(string)
			}
		}
	}
	return ""
}

func clashOptions(raw any, allowed string) (map[string]any, bool) {
	fields, ok := raw.(map[string]any)
	if !ok {
		return nil, false
	}
	keys := " " + allowed + " "
	for key := range fields {
		if strings.ContainsRune(key, ' ') || !strings.Contains(keys, " "+key+" ") {
			return nil, false
		}
	}
	return fields, true
}
