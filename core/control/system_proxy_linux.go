//go:build linux && !android

package control

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// GSettingsSystemProxy integrates with GNOME-compatible desktop settings.
// Other Linux desktops report unsupported instead of silently pretending that
// the proxy was enabled.
type GSettingsSystemProxy struct {
	run func(args ...string) (string, error)
}

func NewSystemProxyManager() SystemProxyManager {
	return &GSettingsSystemProxy{}
}

func (m *GSettingsSystemProxy) Configure(host string, port int) (func() error, error) {
	if strings.TrimSpace(host) == "" || port < 1 || port > 65535 {
		return nil, fmt.Errorf("system proxy address is invalid")
	}
	run := m.run
	if run == nil {
		if _, err := exec.LookPath("gsettings"); err != nil {
			return nil, fmt.Errorf("gsettings is unavailable: %w", err)
		}
		run = runGSettings
	}
	keys := []struct {
		schema string
		key    string
		value  string
	}{
		{"org.gnome.system.proxy", "mode", "manual"},
		{"org.gnome.system.proxy.http", "host", "''"},
		{"org.gnome.system.proxy.http", "port", "0"},
		{"org.gnome.system.proxy.https", "host", "''"},
		{"org.gnome.system.proxy.https", "port", "0"},
		{"org.gnome.system.proxy.socks", "host", host},
		{"org.gnome.system.proxy.socks", "port", strconv.Itoa(port)},
	}
	previous := make([]string, len(keys))
	for index, item := range keys {
		value, err := run("get", item.schema, item.key)
		if err != nil {
			return nil, fmt.Errorf("read desktop proxy settings: %w", err)
		}
		previous[index] = strings.TrimSpace(value)
	}
	if proxyPointsAt(previous, port) {
		previous = []string{"'none'", "''", "uint32 0", "''", "uint32 0", "''", "uint32 0"}
	}
	restore := func() error {
		var result error
		for index, item := range keys {
			if _, err := run("set", item.schema, item.key, previous[index]); err != nil {
				result = fmt.Errorf("restore desktop proxy settings: %w", err)
			}
		}
		return result
	}
	for _, item := range keys {
		value := item.value
		if strings.HasSuffix(item.key, "port") {
			value = "uint32 " + value
		}
		if _, err := run("set", item.schema, item.key, value); err != nil {
			_ = restore()
			return nil, fmt.Errorf("enable desktop proxy: %w", err)
		}
	}
	return restore, nil
}

// proxyPointsAt reports whether the saved desktop proxy already targets the
// olcRTC SOCKS port. That state is a leftover from a crashed session, not the
// user's own settings, so disconnect must turn the proxy off.
func proxyPointsAt(previous []string, port int) bool {
	if len(previous) < 7 {
		return false
	}
	portText := strconv.Itoa(port)
	return strings.Contains(previous[2], portText) || strings.Contains(previous[4], portText) || strings.Contains(previous[6], portText)
}

// clearStuckOlcProxy turns off a desktop proxy left pointing at the olcRTC
// SOCKS port by an earlier crash. Later olcRTC starts do not enable it.
func clearStuckOlcProxy() {
	run := runGSettings
	mode, err := run("get", "org.gnome.system.proxy.socks", "port")
	if err != nil || !strings.Contains(mode, "10808") {
		return
	}
	_, _ = run("set", "org.gnome.system.proxy", "mode", "none")
	for _, schema := range []string{"org.gnome.system.proxy.http", "org.gnome.system.proxy.https", "org.gnome.system.proxy.socks"} {
		_, _ = run("set", schema, "host", "''")
		_, _ = run("set", schema, "port", "uint32 0")
	}
}

func runGSettings(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gsettings", args...)
	if strings.TrimSpace(os.Getenv("DBUS_SESSION_BUS_ADDRESS")) == "" {
		bus := fmt.Sprintf("/run/user/%d/bus", os.Getuid())
		if _, err := os.Stat(bus); err == nil {
			cmd.Env = append(os.Environ(),
				"DBUS_SESSION_BUS_ADDRESS=unix:path="+bus,
				"XDG_RUNTIME_DIR=/run/user/"+strconv.Itoa(os.Getuid()),
			)
		}
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message != "" {
			return "", fmt.Errorf("%w: %s", err, message)
		}
		return "", err
	}
	return string(output), nil
}
