package sharelink

import (
	"encoding/hex"
	"encoding/json"
	"net"
	"net/url"
	"strconv"
	"strings"
)

func parseURI(raw string) (map[string]any, string, string) {
	scheme, payload, ok := strings.Cut(raw, "://")
	if !ok {
		return nil, "", InvalidLink
	}
	scheme = strings.ToLower(scheme)
	switch scheme {
	case "vmess":
		return parseVMess(payload)
	case "ss":
		return parseShadowsocks(payload)
	case "vless", "trojan", "hysteria2", "hy2", "tuic":
	default:
		return nil, "", UnsupportedProtocol
	}
	u, err := url.Parse(scheme + "://" + payload)
	if err != nil || u.User == nil || u.User.Username() == "" || u.Path != "" && u.Path != "/" {
		return nil, "", InvalidLink
	}
	port, ok := readPort(u.Port(), 443)
	if !ok || !validHost(u.Hostname()) {
		return nil, "", InvalidLink
	}
	q, reason := singleQuery(u.RawQuery)
	if reason != "" {
		return nil, "", reason
	}
	if _, ok := q["fm"]; ok {
		return nil, "", FinalMask
	}
	if scheme == "hy2" {
		scheme = "hysteria2"
	}
	node := outbound(scheme, u.Hostname(), port)
	user := u.User.Username()
	password, hasPassword := u.User.Password()
	switch scheme {
	case "vless", "trojan":
		if hasPassword || scheme == "vless" && !validUUID(user) {
			return nil, "", InvalidLink
		}
		if scheme == "vless" {
			node["uuid"] = strings.ToLower(user)
		} else {
			node["password"] = user
		}
		reason = streamOptions(node, q, scheme == "trojan")
	case "hysteria2":
		if hasPassword {
			user += ":" + password
		}
		node["password"] = user
		reason = hysteriaOptions(node, q)
	case "tuic":
		if !validUUID(user) || !hasPassword {
			return nil, "", InvalidLink
		}
		node["uuid"], node["password"] = strings.ToLower(user), password
		reason = tuicOptions(node, q)
	}
	if reason != "" {
		return nil, "", reason
	}
	return node, u.Fragment, ""
}

func outbound(protocol, host string, port int) map[string]any {
	return map[string]any{"type": protocol, "server": host, "server_port": port}
}

func validHost(host string) bool {
	if net.ParseIP(host) != nil {
		return true
	}
	return host != "" && len(host) <= 253 && !strings.ContainsAny(host, " \t\r\n%/:?#@[]\\")
}

func readPort(raw string, fallback int) (int, bool) {
	if raw == "" && fallback > 0 {
		return fallback, true
	}
	port, err := strconv.Atoi(raw)
	return port, err == nil && port > 0 && port <= 65535
}

func validUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	data, err := hex.DecodeString(strings.ReplaceAll(value, "-", ""))
	return err == nil && len(data) == 16
}

func singleQuery(raw string) (map[string]string, string) {
	items, err := url.ParseQuery(raw)
	if err != nil {
		return nil, InvalidLink
	}
	values := make(map[string]string, len(items))
	for key, entries := range items {
		if len(entries) != 1 || key == "" {
			return nil, InvalidLink
		}
		values[key] = entries[0]
	}
	return values, ""
}

func hasOnly(q map[string]string, allowed string) bool {
	keys := " " + allowed + " "
	for key := range q {
		if strings.ContainsRune(key, ' ') || !strings.Contains(keys, " "+key+" ") {
			return false
		}
	}
	return true
}

func streamOptions(node map[string]any, q map[string]string, defaultTLS bool) string {
	const common = "type security sni fp pbk sid alpn allowInsecure insecure host path serviceName headerType"
	if !hasOnly(q, common+" flow encryption packetEncoding") || q["encryption"] != "" && q["encryption"] != "none" {
		return UnsupportedParameter
	}
	protocol := node["type"].(string)
	security := q["security"]
	if security == "" && defaultTLS {
		security = "tls"
	}
	switch security {
	case "", "none":
		for _, key := range []string{"sni", "fp", "pbk", "sid", "alpn", "allowInsecure", "insecure"} {
			if q[key] != "" {
				return UnsupportedParameter
			}
		}
	case "tls", "reality":
		tls, reason := tlsOptions(q, true)
		if reason != "" {
			return reason
		}
		if security == "reality" {
			key, err := decodeBase64(q["pbk"])
			short, shortErr := hex.DecodeString(q["sid"])
			if err != nil || len(key) != 32 || shortErr != nil || len(short) > 8 || tls["insecure"] == true {
				return InvalidLink
			}
			tls["reality"] = map[string]any{"enabled": true, "public_key": q["pbk"], "short_id": q["sid"]}
			if tls["utls"] == nil {
				tls["utls"] = map[string]any{"enabled": true, "fingerprint": "chrome"}
			}
		} else if q["pbk"] != "" || q["sid"] != "" {
			return UnsupportedParameter
		}
		node["tls"] = tls
	default:
		return UnsupportedParameter
	}
	transport, reason := streamTransport(q)
	if reason != "" {
		return reason
	}
	if transport != nil {
		node["transport"] = transport
	}
	if flow := q["flow"]; flow != "" {
		if protocol != "vless" || flow != "xtls-rprx-vision" || transport != nil || node["tls"] == nil {
			return UnsupportedParameter
		}
		node["flow"] = flow
	}
	if encoding := q["packetEncoding"]; encoding != "" {
		if protocol != "vless" && protocol != "vmess" || encoding != "xudp" && encoding != "packetaddr" {
			return UnsupportedParameter
		}
		node["packet_encoding"] = encoding
	}
	return ""
}

func streamTransport(q map[string]string) (map[string]any, string) {
	kind := q["type"]
	if q["headerType"] != "" && q["headerType"] != "none" {
		return nil, UnsupportedParameter
	}
	switch kind {
	case "", "tcp", "raw":
		if q["host"] != "" || q["path"] != "" || q["serviceName"] != "" {
			return nil, UnsupportedParameter
		}
		return nil, ""
	case "ws", "httpupgrade", "http", "h2":
		if q["serviceName"] != "" {
			return nil, UnsupportedParameter
		}
		if kind == "h2" {
			kind = "http"
		}
		result := map[string]any{"type": kind}
		if q["path"] != "" {
			result["path"] = q["path"]
		}
		if host := q["host"]; host != "" {
			switch kind {
			case "ws":
				result["headers"] = map[string]string{"Host": host}
			case "http":
				result["host"] = strings.Split(host, ",")
			default:
				result["host"] = host
			}
		}
		return result, ""
	case "grpc":
		if q["host"] != "" || q["path"] != "" {
			return nil, UnsupportedParameter
		}
		return map[string]any{"type": "grpc", "service_name": q["serviceName"]}, ""
	default:
		return nil, UnsupportedParameter
	}
}

func tlsOptions(q map[string]string, allowUTLS bool) (map[string]any, string) {
	tls := map[string]any{"enabled": true}
	if q["sni"] != "" {
		tls["server_name"] = q["sni"]
	}
	if q["alpn"] != "" {
		tls["alpn"] = strings.Split(q["alpn"], ",")
	}
	for _, key := range []string{"insecure", "allowInsecure"} {
		switch q[key] {
		case "", "0", "false":
		case "1", "true":
			tls["insecure"] = true
		default:
			return nil, InvalidLink
		}
	}
	if fp := strings.ToLower(q["fp"]); fp != "" {
		if !allowUTLS {
			return nil, UnsupportedFingerprint
		}
		switch fp {
		case "chrome", "firefox", "edge", "safari", "ios", "android", "360", "qq", "random", "randomized", "hellochrome_120":
			tls["utls"] = map[string]any{"enabled": true, "fingerprint": fp}
		default:
			return nil, UnsupportedFingerprint
		}
	}
	return tls, ""
}

func hysteriaOptions(node map[string]any, q map[string]string) string {
	if !hasOnly(q, "sni alpn insecure allowInsecure obfs obfs-password") {
		return UnsupportedParameter
	}
	tls, reason := tlsOptions(q, false)
	if reason != "" {
		return reason
	}
	node["tls"] = tls
	if q["obfs"] == "" && q["obfs-password"] == "" {
		return ""
	}
	if q["obfs"] != "salamander" || q["obfs-password"] == "" {
		return UnsupportedParameter
	}
	node["obfs"] = map[string]any{"type": "salamander", "password": q["obfs-password"]}
	return ""
}

func tuicOptions(node map[string]any, q map[string]string) string {
	if !hasOnly(q, "sni alpn insecure allowInsecure congestion_control udp_relay_mode zero_rtt_handshake") {
		return UnsupportedParameter
	}
	tls, reason := tlsOptions(q, false)
	if reason != "" {
		return reason
	}
	node["tls"] = tls
	switch q["congestion_control"] {
	case "":
	case "cubic", "new_reno", "bbr":
		node["congestion_control"] = q["congestion_control"]
	default:
		return UnsupportedParameter
	}
	switch q["udp_relay_mode"] {
	case "":
	case "native", "quic":
		node["udp_relay_mode"] = q["udp_relay_mode"]
	default:
		return UnsupportedParameter
	}
	switch q["zero_rtt_handshake"] {
	case "", "0", "false":
	case "1", "true":
		node["zero_rtt_handshake"] = true
	default:
		return InvalidLink
	}
	return ""
}

func parseVMess(payload string) (map[string]any, string, string) {
	encoded, fragment, _ := strings.Cut(payload, "#")
	if fragment != "" {
		return nil, "", UnsupportedParameter
	}
	body, err := decodeBase64(encoded)
	if err != nil {
		return nil, "", InvalidLink
	}
	var values map[string]json.RawMessage
	if json.Unmarshal(body, &values) != nil {
		return nil, "", InvalidLink
	}
	fields := make(map[string]string, len(values))
	for key, raw := range values {
		var value string
		if json.Unmarshal(raw, &value) != nil {
			if key != "v" && key != "port" && key != "aid" && key != "allowInsecure" {
				return nil, "", UnsupportedParameter
			}
			value = string(raw)
		}
		fields[key] = value
	}
	if !hasOnly(fields, "v ps add port id aid scy net type host path tls sni alpn fp allowInsecure") ||
		fields["v"] != "" && fields["v"] != "2" {
		return nil, "", UnsupportedParameter
	}
	port, ok := readPort(fields["port"], 0)
	if !ok || !validHost(fields["add"]) || !validUUID(fields["id"]) {
		return nil, "", InvalidLink
	}
	node := outbound("vmess", fields["add"], port)
	node["uuid"] = strings.ToLower(fields["id"])
	var alterID int
	if fields["aid"] != "" {
		alterID, err = strconv.Atoi(fields["aid"])
		if err != nil || alterID < 0 || alterID > 65535 {
			return nil, "", InvalidLink
		}
	}
	node["alter_id"] = alterID
	switch fields["scy"] {
	case "", "auto":
		node["security"] = "auto"
	case "aes-128-gcm", "chacha20-poly1305", "none", "zero":
		node["security"] = fields["scy"]
	default:
		return nil, "", UnsupportedParameter
	}
	q := map[string]string{"type": fields["net"], "headerType": fields["type"], "host": fields["host"], "path": fields["path"],
		"security": fields["tls"], "sni": fields["sni"], "fp": fields["fp"], "alpn": fields["alpn"], "allowInsecure": fields["allowInsecure"]}
	if q["type"] == "grpc" {
		q["serviceName"], q["path"] = q["path"], ""
	}
	if reason := streamOptions(node, q, false); reason != "" {
		return nil, "", reason
	}
	return node, fields["ps"], ""
}

func parseShadowsocks(payload string) (map[string]any, string, string) {
	authority, label, _ := strings.Cut(payload, "#")
	fragment, err := url.PathUnescape(label)
	if err != nil {
		return nil, "", InvalidLink
	}
	if !strings.ContainsRune(authority, '@') {
		encoded, suffix, _ := strings.Cut(authority, "?")
		decoded, err := decodeBase64(encoded)
		if err != nil {
			return nil, "", InvalidLink
		}
		user, host, ok := cutLast(string(decoded), '@')
		if !ok {
			return nil, "", InvalidLink
		}
		method, password, ok := strings.Cut(user, ":")
		if !ok {
			return nil, "", InvalidLink
		}
		authority = url.UserPassword(method, password).String() + "@" + host
		if suffix != "" {
			authority += "?" + suffix
		}
	}
	u, err := url.Parse("ss://" + authority)
	if err != nil || u.User == nil || u.Path != "" && u.Path != "/" || !validHost(u.Hostname()) {
		return nil, "", InvalidLink
	}
	port, ok := readPort(u.Port(), 0)
	if !ok {
		return nil, "", InvalidLink
	}
	method, password := u.User.Username(), ""
	if value, explicit := u.User.Password(); explicit {
		password = value
	} else {
		data, err := decodeBase64(method)
		if err != nil {
			return nil, "", InvalidLink
		}
		method, password, ok = strings.Cut(string(data), ":")
		if !ok {
			return nil, "", InvalidLink
		}
	}
	switch method {
	case "aes-128-gcm", "aes-192-gcm", "aes-256-gcm", "chacha20-ietf-poly1305", "xchacha20-ietf-poly1305",
		"2022-blake3-aes-128-gcm", "2022-blake3-aes-256-gcm", "2022-blake3-chacha20-poly1305":
	default:
		return nil, "", UnsupportedParameter
	}
	if password == "" {
		return nil, "", InvalidLink
	}
	q, reason := singleQuery(u.RawQuery)
	if reason != "" || !hasOnly(q, "plugin") {
		if reason != "" {
			return nil, "", reason
		}
		return nil, "", UnsupportedParameter
	}
	node := outbound("shadowsocks", u.Hostname(), port)
	node["method"], node["password"] = method, password
	if q["plugin"] != "" {
		plugin, options, _ := strings.Cut(q["plugin"], ";")
		if plugin != "v2ray-plugin" && plugin != "obfs-local" {
			return nil, "", UnsupportedParameter
		}
		node["plugin"], node["plugin_opts"] = plugin, options
	}
	return node, fragment, ""
}

func cutLast(value string, delimiter byte) (string, string, bool) {
	index := strings.LastIndexByte(value, delimiter)
	if index < 0 {
		return "", "", false
	}
	return value[:index], value[index+1:], true
}
