package control

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"naga.network/core/profile"
)

// DNSManager temporarily points the host resolver at the runtime DNS server.
// The returned function must restore the resolver state before the runtime is
// stopped, while its virtual interface is still available.
type DNSManager interface {
	Configure() (restore func() error, err error)
}

// ResolvedDNSManager integrates a sing-box TUN runtime with systemd-resolved.
// sing-box listens on loopback:53 and resolved is directed there through a
// per-link runtime configuration. The settings disappear on revert or when
// the TUN link is removed.
type ResolvedDNSManager struct {
	Interface string
	Server    string
	run       func(args ...string) error
}

func NewResolvedDNSManager() *ResolvedDNSManager {
	return &ResolvedDNSManager{Interface: profile.RuntimeTUNName, Server: "127.0.0.1"}
}

func (m *ResolvedDNSManager) Configure() (func() error, error) {
	if m == nil {
		return nil, nil
	}
	if strings.TrimSpace(m.Interface) == "" || strings.TrimSpace(m.Server) == "" {
		return nil, fmt.Errorf("system DNS configuration is incomplete")
	}
	run := m.run
	if run == nil {
		if _, err := exec.LookPath("resolvectl"); err != nil {
			return nil, fmt.Errorf("resolvectl is unavailable: %w", err)
		}
		run = runResolvectl
	}

	if err := run("dns", m.Interface, m.Server); err != nil {
		return nil, fmt.Errorf("set system DNS server: %w", err)
	}
	if err := run("domain", m.Interface, "~."); err != nil {
		_ = run("revert", m.Interface)
		return nil, fmt.Errorf("set system DNS route: %w", err)
	}
	if err := run("default-route", m.Interface, "yes"); err != nil {
		_ = run("revert", m.Interface)
		return nil, fmt.Errorf("set system DNS default route: %w", err)
	}
	_ = run("flush-caches")

	return func() error {
		err := run("revert", m.Interface)
		if err != nil && strings.Contains(strings.ToLower(err.Error()), "not found") {
			return nil
		}
		return err
	}, nil
}

func runResolvectl(args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "resolvectl", args...)
	output, err := command.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message != "" {
			return fmt.Errorf("%w: %s", err, message)
		}
		return err
	}
	return nil
}
