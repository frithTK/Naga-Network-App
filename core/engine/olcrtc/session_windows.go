//go:build windows

package olcrtc

import (
	"errors"
	"strings"
)

var errNoWindowsGateway = errors.New("ipv4 default gateway was not found")

// CaptureLink reads the current IPv4 default route from `route print`.
func CaptureLink(exec Execer) (LinkState, error) {
	raw, err := exec.Output("route", "print", "-4")
	if err != nil {
		return LinkState{}, err
	}
	route, err := ParseWindowsDefaultRoute(string(raw))
	if err != nil {
		return LinkState{}, err
	}
	return LinkState{Route: route}, nil
}

// BypassRoom pins each IPv4 room address through the current gateway.
func BypassRoom(exec Execer, state LinkState) error {
	if strings.TrimSpace(state.Route.Gateway) == "" {
		return errNoWindowsGateway
	}
	for _, ip := range state.RoomIPs {
		if ip == nil || ip.IsLoopback() {
			continue
		}
		ip4 := ip.To4()
		if ip4 == nil {
			continue
		}
		if err := exec.Run("route", "add", ip4.String(), "mask", "255.255.255.255", state.Route.Gateway); err != nil {
			return err
		}
	}
	return nil
}

// EngageDefault adds a metric-1 default via the hev adapter address.
func EngageDefault(exec Execer, state LinkState) error {
	_ = state
	err := exec.Run("route", "add", "0.0.0.0", "mask", "0.0.0.0", TunnelIPv4, "metric", "1")
	if err == nil {
		return nil
	}
	return exec.Run("route", "change", "0.0.0.0", "mask", "0.0.0.0", TunnelIPv4)
}

// RestoreLink removes the tunnel default and room pins, then puts back the
// saved gateway. IPv6 defaults are left untouched so a failed undo cannot
// drop native IPv6.
func RestoreLink(exec Execer, state LinkState) error {
	_ = exec.Run("route", "delete", "0.0.0.0", "mask", "0.0.0.0", TunnelIPv4)
	if gw := strings.TrimSpace(state.Route.Gateway); gw != "" {
		if err := exec.Run("route", "add", "0.0.0.0", "mask", "0.0.0.0", gw); err != nil {
			_ = exec.Run("route", "change", "0.0.0.0", "mask", "0.0.0.0", gw)
		}
	}
	for _, ip := range state.RoomIPs {
		if ip == nil {
			continue
		}
		ip4 := ip.To4()
		if ip4 == nil {
			continue
		}
		_ = exec.Run("route", "delete", ip4.String(), "mask", "255.255.255.255")
	}
	return nil
}
