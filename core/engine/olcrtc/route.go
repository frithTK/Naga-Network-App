package olcrtc

import (
	"bufio"
	"fmt"
	"net"
	"strings"
)

// DefaultRoute is the IPv4 default route that must be restored on stop.
type DefaultRoute struct {
	Gateway string
	Device  string
	Line    string
}

// ParseDefaultRoute reads one `ip -4 route show default` line.
func ParseDefaultRoute(text string) (DefaultRoute, error) {
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || fields[0] != "default" || fields[1] != "via" {
			continue
		}
		route := DefaultRoute{Gateway: fields[2], Line: strings.TrimSpace(line)}
		for i := 0; i < len(fields)-1; i++ {
			if fields[i] == "dev" {
				route.Device = fields[i+1]
			}
		}
		if route.Device == "" {
			continue
		}
		return route, nil
	}
	return DefaultRoute{}, fmt.Errorf("ipv4 default route was not found")
}

// ParseWindowsDefaultRoute reads the IPv4 0.0.0.0/0 row from `route print -4`.
func ParseWindowsDefaultRoute(text string) (DefaultRoute, error) {
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		if fields[0] != "0.0.0.0" || fields[1] != "0.0.0.0" {
			continue
		}
		gateway := fields[2]
		if strings.EqualFold(gateway, "On-link") || net.ParseIP(gateway) == nil {
			continue
		}
		route := DefaultRoute{Gateway: gateway, Line: strings.TrimSpace(line)}
		if len(fields) > 3 {
			route.Device = fields[3]
		}
		return route, nil
	}
	return DefaultRoute{}, fmt.Errorf("ipv4 default route was not found")
}

// ParseIPv6Defaults returns full lines of `ip -6 route show default`.
func ParseIPv6Defaults(text string) []string {
	var lines []string
	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "default") {
			lines = append(lines, line)
		}
	}
	return lines
}

// TrafficRates converts two interface counter samples into bytes per second.
// tx is host-to-tunnel (upload) and rx is tunnel-to-host (download). A
// counter that went backwards yields a zero rate.
func TrafficRates(prevTx, prevRx, tx, rx int64, seconds int64) (upload, download int64) {
	if seconds <= 0 {
		return 0, 0
	}
	if tx >= prevTx {
		upload = (tx - prevTx) / seconds
	}
	if rx >= prevRx {
		download = (rx - prevRx) / seconds
	}
	return upload, download
}
