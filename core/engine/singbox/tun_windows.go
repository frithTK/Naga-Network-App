//go:build windows

package singbox

import (
	"encoding/json"
	"fmt"

	"naga.network/core/diagnostics"
)

func linuxTUNInterfaceNames([]byte) ([]string, error) {
	return nil, nil
}

func configureLinuxTUNInterfaces([]string, *diagnostics.Logger) error {
	return nil
}

func adaptTUNForPlatform(config []byte) ([]byte, error) {
	var document map[string]any
	if err := json.Unmarshal(config, &document); err != nil {
		return nil, fmt.Errorf("decode Windows TUN config: %w", err)
	}
	rawInbounds, _ := document["inbounds"].([]any)
	for _, raw := range rawInbounds {
		inbound, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if inboundType, _ := inbound["type"].(string); inboundType != "tun" {
			continue
		}
		if autoRoute, _ := inbound["auto_route"].(bool); autoRoute {
			inbound["strict_route"] = true
		}
		// gVisor handles every UDP datagram in userspace. Hysteria2 is QUIC,
		// so that stack caps throughput far below a system TUN. mixed keeps
		// UDP on gVisor, so Windows uses the system stack here. Linux still
		// forces gVisor earlier; this override is Windows-only.
		inbound["stack"] = "system"
		switch mtu := inbound["mtu"].(type) {
		case float64:
			if mtu <= 0 || mtu > 1500 {
				inbound["mtu"] = float64(1500)
			}
		default:
			inbound["mtu"] = float64(1500)
		}
	}
	result, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode Windows TUN config: %w", err)
	}
	return result, nil
}

func shouldPrepareSystemResolver() bool {
	return false
}
