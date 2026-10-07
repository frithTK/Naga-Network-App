package amneziawg

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
)

const (
	SelectorTag = "Mode"
	LeafTag     = "AmneziaWG"
	DirectTag   = "direct"
	dnsTag      = "dns-bootstrap"
)

// IsLeafTag reports whether tag is the AmneziaWG leaf or a namespaced
// runtime tag for that leaf (profileID::AmneziaWG).
func IsLeafTag(tag string) bool {
	tag = strings.TrimSpace(tag)
	if strings.EqualFold(tag, LeafTag) {
		return true
	}
	if separator := strings.LastIndex(tag, "::"); separator >= 0 {
		return strings.EqualFold(tag[separator+2:], LeafTag)
	}
	return false
}

// SocksOutbound is the runtime-only SOCKS5 leaf that fronts the userspace
// sidecar. The source AmneziaWG conf is never modified.
func SocksOutbound(host string, port int, tag string) map[string]any {
	host = strings.TrimSpace(host)
	if host == "" {
		host = "127.0.0.1"
	}
	if strings.TrimSpace(tag) == "" {
		tag = LeafTag
	}
	return map[string]any{
		"type":        "socks",
		"tag":         tag,
		"server":      host,
		"server_port": port,
		"version":     "5",
	}
}

// BuildRuntimeJSON returns a temporary sing-box config that sends TUN traffic
// to a loopback SOCKS5 sidecar. The source AmneziaWG conf is never modified.
func BuildRuntimeJSON(socksHost string, socksPort int, dnsServers []string) ([]byte, error) {
	host := strings.TrimSpace(socksHost)
	if host == "" {
		host = "127.0.0.1"
	}
	if socksPort <= 0 || socksPort > 65535 {
		return nil, fmt.Errorf("amneziawg socks port is invalid")
	}
	document := map[string]any{
		"dns": dnsSection(dnsServers, LeafTag),
		"inbounds": []any{
			map[string]any{
				"type":           "tun",
				"tag":            "tun-in",
				"interface_name": "naga-tun0",
				"address":        []any{"172.19.0.1/30"},
				"mtu":            1400,
				"auto_route":     true,
				"strict_route":   false,
				"stack":          "gvisor",
			},
		},
		"outbounds": []any{
			map[string]any{
				"type":      "selector",
				"tag":       SelectorTag,
				"outbounds": []any{LeafTag},
				"default":   LeafTag,
			},
			SocksOutbound(host, socksPort, LeafTag),
			map[string]any{
				"type": "direct",
				"tag":  DirectTag,
			},
		},
		"route": map[string]any{
			"final": SelectorTag,
		},
	}
	return json.MarshalIndent(document, "", "  ")
}

func dnsSection(servers []string, detour string) map[string]any {
	return map[string]any{
		"servers": []any{
			map[string]any{
				"type":   "udp",
				"tag":    dnsTag,
				"server": firstDNSServer(servers),
				"detour": detour,
			},
		},
		"final": dnsTag,
	}
}

func firstDNSServer(servers []string) string {
	for _, raw := range servers {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if host, _, err := net.SplitHostPort(raw); err == nil && host != "" {
			return host
		}
		return raw
	}
	return "1.1.1.1"
}
