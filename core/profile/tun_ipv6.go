package profile

import (
	"bytes"
	"net/netip"
	"os"
)

const runtimeTUNIPv6 = "fd3e:6e61:6761:1::1/126"

func linuxIPv6Enabled() bool {
	for _, iface := range []string{"all", "default"} {
		value, err := os.ReadFile("/proc/sys/net/ipv6/conf/" + iface + "/disable_ipv6")
		if err != nil || !bytes.Equal(bytes.TrimSpace(value), []byte("0")) {
			return false
		}
	}
	return true
}

// sing-tun installs IPv6 auto-routes only when the inbound has an IPv6
// address. Give an IPv4-only auto-routed tunnel a ULA on dual-stack hosts.
// Existing IPv6 addresses and the profile's routing rules are preserved.
func ensureTUNIPv6(inbound map[string]any) {
	if inbound["auto_route"] != true {
		return
	}
	var addresses []any
	switch value := inbound["address"].(type) {
	case []any:
		addresses = value
	case string:
		addresses = []any{value}
	default:
		return
	}
	if len(addresses) == 0 {
		return
	}
	for _, value := range addresses {
		text, ok := value.(string)
		if !ok {
			return
		}
		prefix, err := netip.ParsePrefix(text)
		if err != nil || prefix.Addr().Is6() {
			return
		}
	}
	inbound["address"] = append(addresses, runtimeTUNIPv6)
}
