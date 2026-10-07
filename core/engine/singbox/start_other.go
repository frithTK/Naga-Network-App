//go:build !windows

package singbox

import (
	"context"
	"time"
)

func defaultReadyTimeout() time.Duration {
	return 5 * time.Second
}

func startPlatformProcess(ctx context.Context, req processRequest) (startedProcess, error) {
	return startCommandProcess(ctx, req)
}

func restrictConfigAccess(string, string) error {
	return nil
}

// RunTunHost is only implemented on Windows.
func RunTunHost(string) error {
	return errTunHostUnsupported
}
