package olcrtc

import (
	"fmt"
	"net"
	"strings"
)

// Execer runs host commands. Tests substitute a recorder.
type Execer interface {
	Run(name string, args ...string) error
	Output(name string, args ...string) ([]byte, error)
}

// LinkState is what stop must undo. Room IPs are not logged by the caller.
type LinkState struct {
	Route    DefaultRoute
	IPv6     []string
	RoomIPs  []net.IP
	DNSLink  string
	DNSSaved string
}

func dnsServers(text string) []string {
	var servers []string
	for _, field := range strings.Fields(text) {
		if net.ParseIP(field) != nil {
			servers = append(servers, field)
		}
	}
	return servers
}

func restoreError(result error) error {
	if result != nil {
		return fmt.Errorf("restore link: %w", result)
	}
	return nil
}
