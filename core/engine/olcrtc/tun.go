package olcrtc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	TunnelIface = "naga-olc0"
	TunnelIPv4  = "198.18.0.1"
	TunnelMTU   = 8500
	windowsMTU  = 1500
	hevConfig   = "hev.yml"
)

var ErrTunnelBinary = fmt.Errorf("hev-socks5-tunnel binary was not found")

var tunnelExecutablePath = os.Executable

func tunnelNames() []string {
	if runtime.GOOS == "windows" {
		return []string{"hev-socks5-tunnel.exe", "hev-socks5-tunnel"}
	}
	return []string{"hev-socks5-tunnel"}
}

func siblingTunnel() string {
	exe, err := tunnelExecutablePath()
	if err != nil {
		return ""
	}
	dir := filepath.Dir(exe)
	for _, name := range tunnelNames() {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

func tunnelMTU() int {
	if runtime.GOOS == "windows" {
		return windowsMTU
	}
	return TunnelMTU
}

// DiscoverTunnel finds hev-socks5-tunnel next to an explicit path, via
// NAGA_HEV_PATH, beside the current executable, then on PATH.
func DiscoverTunnel(explicit string) (string, error) {
	if value := strings.TrimSpace(explicit); value != "" {
		if _, err := os.Stat(value); err != nil {
			return "", ErrTunnelBinary
		}
		return value, nil
	}
	if value := strings.TrimSpace(os.Getenv("NAGA_HEV_PATH")); value != "" {
		if _, err := os.Stat(value); err != nil {
			return "", ErrTunnelBinary
		}
		return value, nil
	}
	if sibling := siblingTunnel(); sibling != "" {
		return sibling, nil
	}
	for _, name := range tunnelNames() {
		path, err := exec.LookPath(name)
		if err == nil {
			return path, nil
		}
	}
	return "", ErrTunnelBinary
}

// TunnelReady is true when hev-socks5-tunnel is next to the control-plane or on PATH.
func TunnelReady(explicit string) bool {
	_, err := DiscoverTunnel(explicit)
	return err == nil
}

// TunnelConfig is the hev-socks5-tunnel file. Routes and DNS are applied by
// Naga, not by hev.
func TunnelConfig() string {
	return fmt.Sprintf(`tunnel:
  name: %s
  mtu: %d
  ipv4: %s
socks5:
  port: %d
  address: %s
  udp: 'udp'
misc:
  log-level: warn
`, TunnelIface, tunnelMTU(), TunnelIPv4, 10808, "127.0.0.1")
}

// WriteTunnelConfig stores the hev config next to the olcrtc YAML.
func WriteTunnelConfig(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, hevConfig)
	if err := os.WriteFile(path, []byte(TunnelConfig()), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// WaitForIface returns when the tunnel adapter exists or carries TunnelIPv4.
func WaitForIface(ctx context.Context, name string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		if TunnelIfacePresent(name) {
			return nil
		}
		select {
		case <-ctx.Done():
			return errors.New("tunnel interface was not created")
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// TunnelIfacePresent reports whether the adapter name or 198.18.0.1 is up.
func TunnelIfacePresent(name string) bool {
	ifaces, err := net.Interfaces()
	if err != nil {
		return false
	}
	want := strings.ToLower(strings.TrimSpace(name))
	for _, iface := range ifaces {
		got := strings.ToLower(iface.Name)
		if want != "" && (got == want || strings.Contains(got, want)) {
			return true
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			text := addr.String()
			if text == TunnelIPv4 || strings.HasPrefix(text, TunnelIPv4+"/") {
				return true
			}
		}
	}
	return false
}
