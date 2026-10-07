//go:build linux && !android

package olcrtc

import "strings"

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
func BypassRoom(exec Execer, state LinkState) error {
	for _, ip := range state.RoomIPs {
		if ip == nil || ip.IsLoopback() {
			continue
		}
		if err := exec.Run("ip", "route", "replace", ip.String()+"/32", "via", state.Route.Gateway, "dev", state.Route.Device); err != nil {
			return err
		}
	}
	return nil
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
	for _, ip := range state.RoomIPs {
		if ip == nil {
			continue
		}
		_ = exec.Run("ip", "route", "del", ip.String()+"/32")
	}
	return restoreError(result)
}
