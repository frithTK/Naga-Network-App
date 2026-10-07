package singbox

import (
	"encoding/json"
	"fmt"

	"naga.network/core/profile"
)

const (
	androidMixedListen = "127.0.0.1"
	androidDNSTag      = "naga-dns"
	androidDNSBackup   = "naga-dns-backup"
)

// stripTUNInboundsEnsureMixed removes tun inbounds from a runtime config and
// keeps a loopback mixed inbound for hev-socks5-tunnel. The stored profile is
// not modified by the caller.
func stripTUNInboundsEnsureMixed(config []byte) ([]byte, error) {
	var document map[string]any
	if err := json.Unmarshal(config, &document); err != nil {
		return nil, fmt.Errorf("decode Android TUN config: %w", err)
	}
	raw, _ := document["inbounds"].([]any)
	kept := make([]any, 0, len(raw))
	for _, item := range raw {
		inbound, ok := item.(map[string]any)
		if !ok {
			kept = append(kept, item)
			continue
		}
		if inboundType, _ := inbound["type"].(string); inboundType == "tun" {
			continue
		}
		kept = append(kept, inbound)
	}
	document["inbounds"] = kept
	profile.EnsureLoopbackProxyInbound(document)
	ensureAndroidRuntimeDNS(document)
	stripAndroidUnsupportedRouteFeatures(document)
	encoded, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode Android mixed config: %w", err)
	}
	return encoded, nil
}

// ensureAndroidRuntimeDNS gives mixed-mode sing-box a remote resolver. Naga
// envelopes often omit dns; Android has no stub on [::1]:53, so start probes
// fail with HTTP 503 and the session is torn down.
func ensureAndroidRuntimeDNS(document map[string]any) {
	dns, _ := document["dns"].(map[string]any)
	if dns == nil {
		dns = map[string]any{}
		document["dns"] = dns
	}
	servers, _ := dns["servers"].([]any)
	if !androidDNSHasRemoteServer(servers) {
		// No detour: sing-box 1.13 rejects DNS detour to an empty direct
		// outbound. The process is excluded from VpnService, so UDP to
		// 1.1.1.1 uses the physical network.
		servers = append(servers,
			map[string]any{
				"type":   "udp",
				"tag":    androidDNSTag,
				"server": "1.1.1.1",
			},
			map[string]any{
				"type":   "udp",
				"tag":    androidDNSBackup,
				"server": "8.8.8.8",
			},
		)
		dns["servers"] = servers
		if _, ok := dns["final"]; !ok {
			dns["final"] = androidDNSTag
		}
	}
	stripEmptyDirectDNSDetours(document, dns)
	if _, ok := dns["strategy"]; !ok {
		dns["strategy"] = "prefer_ipv4"
	}
	route, _ := document["route"].(map[string]any)
	if route == nil {
		route = map[string]any{}
		document["route"] = route
	}
	if _, exists := route["default_domain_resolver"]; !exists {
		final, _ := dns["final"].(string)
		if final == "" {
			final = firstDNSServerTag(servers)
		}
		if final != "" {
			route["default_domain_resolver"] = final
		}
	}
	route["auto_detect_interface"] = false
}

func stripAndroidUnsupportedRouteFeatures(document map[string]any) {
	route, _ := document["route"].(map[string]any)
	if route == nil {
		route = map[string]any{}
		document["route"] = route
	}
	route["find_process"] = false
	if rules, ok := route["rules"].([]any); ok {
		route["rules"] = stripAndroidUnsupportedRules(rules)
	}
	dns, _ := document["dns"].(map[string]any)
	if dns != nil {
		if rules, ok := dns["rules"].([]any); ok {
			dns["rules"] = stripAndroidUnsupportedRules(rules)
		}
	}
}

var androidUnsupportedRuleKeys = map[string]struct{}{
	"process_name":              {},
	"process_path":              {},
	"process_path_regex":        {},
	"package_name":              {},
	"user":                      {},
	"uid":                       {},
	"wifi_ssid":                 {},
	"wifi_bssid":                {},
	"network_type":              {},
	"network_is_expensive":      {},
	"network_is_constrained":    {},
	"interface_address":         {},
	"default_interface_address": {},
}

func stripAndroidUnsupportedRules(rules []any) []any {
	kept := make([]any, 0, len(rules))
	for _, item := range rules {
		rule, ok := item.(map[string]any)
		if !ok {
			kept = append(kept, item)
			continue
		}
		if kind, _ := rule["type"].(string); kind == "logical" {
			if nested, ok := rule["rules"].([]any); ok {
				rule["rules"] = stripAndroidUnsupportedRules(nested)
			}
			kept = append(kept, rule)
			continue
		}
		hadUnsupported := false
		for key := range androidUnsupportedRuleKeys {
			if _, exists := rule[key]; exists {
				delete(rule, key)
				hadUnsupported = true
			}
		}
		if hadUnsupported && !androidRuleHasMatcher(rule) {
			continue
		}
		kept = append(kept, rule)
	}
	return kept
}

func androidRuleHasMatcher(rule map[string]any) bool {
	for key := range rule {
		switch key {
		case "outbound", "action", "invert", "type", "mode", "method":
			continue
		default:
			return true
		}
	}
	return false
}

func stripEmptyDirectDNSDetours(document map[string]any, dns map[string]any) {
	emptyDirect := map[string]struct{}{}
	rawOutbounds, _ := document["outbounds"].([]any)
	for _, item := range rawOutbounds {
		outbound, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if outboundType, _ := outbound["type"].(string); outboundType != "direct" {
			continue
		}
		if androidDirectOutboundIsEmpty(outbound) {
			tag, _ := outbound["tag"].(string)
			if tag == "" {
				tag = "direct"
			}
			emptyDirect[tag] = struct{}{}
		}
	}
	if len(emptyDirect) == 0 {
		return
	}
	servers, _ := dns["servers"].([]any)
	for _, item := range servers {
		server, ok := item.(map[string]any)
		if !ok {
			continue
		}
		detour, _ := server["detour"].(string)
		if _, drop := emptyDirect[detour]; drop {
			delete(server, "detour")
		}
	}
}

func androidDirectOutboundIsEmpty(outbound map[string]any) bool {
	for key := range outbound {
		switch key {
		case "type", "tag":
			continue
		default:
			return false
		}
	}
	return true
}

func androidDNSHasRemoteServer(servers []any) bool {
	for _, item := range servers {
		server, ok := item.(map[string]any)
		if !ok {
			continue
		}
		kind, _ := server["type"].(string)
		switch kind {
		case "udp", "tcp", "tls", "https", "h3", "quic", "http":
			if host, _ := server["server"].(string); host != "" && host != "local" {
				return true
			}
		}
		if address, _ := server["address"].(string); address != "" && address != "local" && address != "localhost" {
			return true
		}
	}
	return false
}
