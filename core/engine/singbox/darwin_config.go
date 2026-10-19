package singbox

import (
	"encoding/json"
	"fmt"
)

// Kept platform-independent so platform adaptation is covered on Linux too.
// The privileged helper validates again and chooses the actual utun number.
func prepareDarwinConfig(config []byte) ([]byte, error) {
	var doc map[string]any
	if err := json.Unmarshal(config, &doc); err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, fmt.Errorf("VPN config must be a JSON object")
	}
	// User-mode probes and proxy runtimes do not cross the privilege boundary.
	if !hasTUNInbound(doc) {
		return config, nil
	}
	for _, raw := range inbounds(doc) {
		in, ok := raw.(map[string]any)
		if !ok || in["type"] != "tun" {
			continue
		}
		in["interface_name"] = "utun123"
		in["address"] = []string{"172.19.0.1/30", "fdfe:dcba:9876::1/126"}
		in["auto_route"] = false // Only the helper owns route changes.
		in["stack"] = "gvisor"
		in["mtu"] = 1500
		for _, key := range []string{"strict_route", "auto_redirect", "iproute2_table_index", "iproute2_rule_index", "inet4_address", "inet6_address"} {
			delete(in, key)
		}
	}
	// Cache/UI paths belong to user-mode probes only, never the root engine.
	delete(doc, "experimental")
	delete(doc, "log")
	if _, ok := doc["outbounds"]; !ok {
		return nil, fmt.Errorf("VPN config has no outbounds")
	}
	return json.Marshal(doc)
}
