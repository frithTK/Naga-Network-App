//go:build linux && !android

package control

import (
	"context"
	"errors"
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
		{"org.gnome.system.proxy", "mode", "'manual'"},
		{"org.gnome.system.proxy.http", "host", "''"},
		{"org.gnome.system.proxy.http", "port", "0"},
		{"org.gnome.system.proxy.https", "host", "''"},
		{"org.gnome.system.proxy.https", "port", "0"},
		{"org.gnome.system.proxy.socks", "host", strconv.Quote(host)},
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
	applied := 0
	restore := func() error {
		// Restore the snapshot only while the settings we changed still belong
		// to this session. A user's replacement must remain a coherent whole.
		for _, item := range keys[:applied] {
			current, err := run("get", item.schema, item.key)
			if err != nil {
				return fmt.Errorf("read desktop proxy before restore: %w", err)
			}
			if proxyValue(current) != proxyValue(item.value) {
				return nil
			}
		}
		var result error
		for index, item := range keys[:applied] {
			if _, err := run("set", item.schema, item.key, previous[index]); err != nil {
				result = errors.Join(result, fmt.Errorf("restore desktop proxy settings: %w", err))
			}
		}
		return result
	}
	for index := range keys {
		item := &keys[index]
		if item.key == "port" {
			item.value = "uint32 " + item.value
		}
		if _, err := run("set", item.schema, item.key, item.value); err != nil {
			return nil, errors.Join(fmt.Errorf("enable desktop proxy: %w", err), restore())
		}
		applied++
	}
	return restore, nil
}

// gsettings serializes strings with single or double quotes and ports with
// a type annotation. Compare values rather than serialization spelling.
func proxyValue(value string) string {
	value = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(value), "uint32 "))
	if unquoted, err := strconv.Unquote(value); err == nil {
		return unquoted
	}
	if len(value) >= 2 && value[0] == '\'' && value[len(value)-1] == '\'' {
		return value[1 : len(value)-1]
	}
	return value
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
