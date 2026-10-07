package amneziawg

import (
	"context"
	"errors"
	"io"
	"net"
	"time"

	awgcfg "naga.network/core/profile/amneziawg"
)

var (
	ErrNotReady          = errors.New("amneziawg sidecar is not ready")
	ErrStopped           = errors.New("amneziawg sidecar is stopped")
	ErrHandshakeTimeout  = errors.New("amneziawg tunnel handshake timed out")
	ErrTunnelUnavailable = errors.New("AmneziaWG не установил туннель")
)

// Sidecar terminates AmneziaWG in userspace and exposes SOCKS5 on loopback.
type Sidecar interface {
	Start(ctx context.Context, cfg awgcfg.Config) error
	ListenAddr() string
	Ready(ctx context.Context) error
	Stop() error
}

type Factory func() Sidecar

func DefaultFactory() Factory {
	return func() Sidecar {
		return NewNetstackSidecar()
	}
}

func waitTCP(ctx context.Context, addr string) error {
	deadline := time.Now().Add(8 * time.Second)
	if ready, ok := ctx.Deadline(); ok && ready.Before(deadline) {
		deadline = ready
	}
	var last error
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 300*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		last = err
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	if last == nil {
		last = ErrNotReady
	}
	return last
}

const handshakeWaitTimeout = 12 * time.Second

func waitSOCKS(ctx context.Context, addr string) error {
	if err := waitTCP(ctx, addr); err != nil {
		return err
	}
	dialer := net.Dialer{Timeout: 400 * time.Millisecond}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	if _, err := conn.Write([]byte{5, 1, 0}); err != nil {
		return err
	}
	resp := make([]byte, 2)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return err
	}
	if resp[0] != 5 || resp[1] != 0 {
		return ErrNotReady
	}
	return nil
}
