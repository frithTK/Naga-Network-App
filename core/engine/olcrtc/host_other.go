//go:build !windows

package olcrtc

import (
	"context"
	"net"
)

// StartElevatedTunnel is only implemented on Windows.
func StartElevatedTunnel(ctx context.Context, hevPath, configPath string, roomIPs []net.IP) (TunnelHost, error) {
	return startElevatedUnsupported(ctx, hevPath, configPath, roomIPs)
}

// RunOlcHost is only implemented on Windows.
func RunOlcHost(string) error {
	return errOlcHostUnsupported
}
