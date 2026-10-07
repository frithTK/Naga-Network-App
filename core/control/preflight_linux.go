//go:build linux

package control

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// LinuxTUNPreflight verifies the device and rejects a second active TUN.
// Multiple TUNs can technically coexist, but allowing two full-tunnel
// runtimes to alter routes and nftables at once is unsafe and non-deterministic.
type LinuxTUNPreflight struct {
	Run     func(context.Context, string, ...string) ([]byte, error)
	OpenTUN func() error
}

func NewLinuxTUNPreflight() *LinuxTUNPreflight {
	return &LinuxTUNPreflight{}
}

func (p *LinuxTUNPreflight) CheckTUN() error {
	openTUN := p.OpenTUN
	if openTUN == nil {
		openTUN = openLinuxTUN
	}
	if err := openTUN(); err != nil {
		return fmt.Errorf("TUN недоступен: %w", err)
	}

	run := p.Run
	if run == nil {
		run = runLinuxPreflightCommand
	}
	output, err := run(context.Background(), "ip", "-o", "link", "show", "type", "tun")
	if err != nil {
		return fmt.Errorf("не удалось проверить активные TUN-интерфейсы: %w", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimSuffix(fields[1], ":")
		name = strings.TrimSuffix(name, "@NONE")
		if name != "" {
			return fmt.Errorf("другой TUN-интерфейс уже активен: %s; отключите другой VPN", name)
		}
	}
	return nil
}

func openLinuxTUN() error {
	device, err := os.OpenFile("/dev/net/tun", os.O_RDWR, 0)
	if err != nil {
		return err
	}
	return device.Close()
}

func runLinuxPreflightCommand(parent context.Context, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}
