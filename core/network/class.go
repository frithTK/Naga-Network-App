// Package network provides platform-facing network context detection.
package network

import (
	"context"
	"os/exec"
	"strings"

	"naga.network/core/policy"
)

// LinuxClass asks NetworkManager for the active physical connection. It is
// deliberately best-effort: environments without nmcli use unknown rather
// than guessing from the VPN/TUN interface. Device and connection tables
// are merged so LTE/WWAN reported only as a connection type still wins.
func LinuxClass(ctx context.Context) policy.NetworkClass {
	return preferPhysicalClass(
		nmcliClass(ctx, ParseNetworkManagerOutput, "device"),
		nmcliClass(ctx, ParseNetworkManagerConnections, "connection", "show", "--active"),
	)
}

func nmcliClass(ctx context.Context, parse func(string) policy.NetworkClass, args ...string) policy.NetworkClass {
	command := exec.CommandContext(ctx, "nmcli", append([]string{"-t", "-f", "TYPE,STATE"}, args...)...)
	output, err := command.Output()
	if err != nil {
		return policy.NetworkUnknown
	}
	return parse(string(output))
}

func preferPhysicalClass(classes ...policy.NetworkClass) policy.NetworkClass {
	best := policy.NetworkUnknown
	for _, class := range classes {
		if class == policy.NetworkCellular {
			return policy.NetworkCellular
		}
		if best == policy.NetworkUnknown && (class == policy.NetworkWiFi || class == policy.NetworkEthernet) {
			best = class
		}
	}
	return best
}

func parseNMTypeState(output string, connectedStates ...string) policy.NetworkClass {
	allowed := make(map[string]struct{}, len(connectedStates))
	for _, state := range connectedStates {
		allowed[state] = struct{}{}
	}
	best := policy.NetworkUnknown
	for _, line := range strings.Split(output, "\n") {
		fields := strings.SplitN(strings.TrimSpace(line), ":", 2)
		if len(fields) != 2 {
			continue
		}
		if _, ok := allowed[strings.ToLower(strings.TrimSpace(fields[1]))]; !ok {
			continue
		}
		switch networkClassFromNMType(fields[0]) {
		case policy.NetworkCellular:
			return policy.NetworkCellular
		case policy.NetworkWiFi:
			if best == policy.NetworkUnknown {
				best = policy.NetworkWiFi
			}
		case policy.NetworkEthernet:
			if best == policy.NetworkUnknown {
				best = policy.NetworkEthernet
			}
		}
	}
	return best
}

func networkClassFromNMType(raw string) policy.NetworkClass {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "gsm", "cdma", "wwan", "lte", "5g", "modem", "cellular":
		return policy.NetworkCellular
	case "wifi", "wifi-p2p", "802-11-wireless":
		return policy.NetworkWiFi
	case "ethernet", "802-3-ethernet":
		return policy.NetworkEthernet
	default:
		return policy.NetworkUnknown
	}
}

// ParseNetworkManagerOutput parses nmcli device TYPE:STATE rows. Connected
// cellular is preferred over Wi-Fi/Ethernet when a modem and another
// interface coexist.
func ParseNetworkManagerOutput(output string) policy.NetworkClass {
	return parseNMTypeState(output, "connected")
}

// ParseNetworkManagerConnections parses nmcli connection TYPE:STATE rows
// from `connection show --active`. VPN/TUN types are ignored.
func ParseNetworkManagerConnections(output string) policy.NetworkClass {
	return parseNMTypeState(output, "activated", "connected")
}
