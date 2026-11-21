//go:build linux && !android

package olcrtc

import (
	"errors"
	"net"
	"strings"
)

// A dedicated protocol value lets cleanup match the exact route we created.
const roomRouteProtocol = "186"

// CaptureLink reads the current IPv4 default route and IPv6 default lines.
func CaptureLink(exec Execer) (LinkState, error) {
	raw, err := exec.Output("ip", "-4", "route", "show", "default")
	if err != nil {
		return LinkState{}, err
	}
	route, err := ParseDefaultRoute(string(raw))
	if err != nil {
		return LinkState{}, err
	}
	v6, err := exec.Output("ip", "-6", "route", "show", "default")
	if err != nil {
		return LinkState{}, err
	}
	dns, _ := exec.Output("resolvectl", "dns", route.Device)
	return LinkState{
		Route:    route,
		IPv6:     ParseIPv6Defaults(string(v6)),
		DNSLink:  route.Device,
		DNSSaved: strings.TrimSpace(string(dns)),
	}, nil
}

// BypassRoom pins each address to the current gateway so olcrtc can still
// reach the room after the default route moves into the tunnel.
func BypassRoom(exec Execer, state *LinkState) error {
	hasIPv4, hasIPv6 := false, false
	for _, ip := range state.RoomIPs {
		if ip == nil || ip.IsLoopback() {
			continue
		}
		ip4 := ip.To4()
		if ip4 == nil {
			hasIPv6 = ip.To16() != nil || hasIPv6
			continue
		}
		hasIPv4 = true
		if err := exec.Run("ip", "route", "add", ip4.String()+"/32", "via", state.Route.Gateway, "dev", state.Route.Device, "proto", roomRouteProtocol); err != nil {
			return err
		}
		state.OwnedRoomIPs = append(state.OwnedRoomIPs, append(net.IP(nil), ip4...))
	}
	if hasIPv6 && !hasIPv4 {
		return errors.New("olcRTC: no IPv4 room address is available for the current tunnel")
	}
	return nil
}

// RemoveRoomBypasses removes only exact routes created by BypassRoom.
func RemoveRoomBypasses(exec Execer, state LinkState) error {
	var result error
	for _, ip := range state.OwnedRoomIPs {
		if ip4 := ip.To4(); ip4 != nil {
			result = errors.Join(result, exec.Run("ip", "route", "del", ip4.String()+"/32", "via", state.Route.Gateway, "dev", state.Route.Device, "proto", roomRouteProtocol))
		}
	}
	return result
}

// EngageDefault moves the default route to the tunnel, drops IPv6 defaults,
// and points resolved at 1.1.1.1 through the tunnel interface.
func EngageDefault(exec Execer, state LinkState) error {
	if err := exec.Run("ip", "route", "replace", "default", "dev", TunnelIface); err != nil {
		return err
	}
	for _, line := range state.IPv6 {
		fields := strings.Fields(line)
		args := append([]string{"-6", "route", "del"}, fields...)
		if err := exec.Run("ip", args...); err != nil {
			return err
		}
	}
	if err := exec.Run("resolvectl", "dns", TunnelIface, "1.1.1.1"); err != nil {
		return err
	}
	return exec.Run("resolvectl", "domain", TunnelIface, "~.")
}

// RestoreLink puts back the saved default routes and DNS, then deletes the
// room bypass. It is safe to call after the tunnel device has already gone.
func RestoreLink(exec Execer, state LinkState) error {
	var result error
	if state.Route.Line != "" {
		fields := strings.Fields(state.Route.Line)
		args := append([]string{"route", "replace"}, fields...)
		if err := exec.Run("ip", args...); err != nil {
			result = err
		}
	}
	for _, line := range state.IPv6 {
		fields := strings.Fields(line)
		args := append([]string{"-6", "route", "replace"}, fields...)
		if err := exec.Run("ip", args...); err != nil && result == nil {
			result = err
		}
	}
	if state.DNSLink != "" && state.DNSSaved != "" {
		args := []string{"dns", state.DNSLink}
		args = append(args, dnsServers(state.DNSSaved)...)
		if len(args) > 2 {
			if err := exec.Run("resolvectl", args...); err != nil && result == nil {
				result = err
			}
		}
	}
	if err := exec.Run("resolvectl", "revert", TunnelIface); err != nil && result == nil {
		result = err
	}
	return restoreError(errors.Join(result, RemoveRoomBypasses(exec, state)))
}
