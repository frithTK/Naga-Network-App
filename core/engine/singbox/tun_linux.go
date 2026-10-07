//go:build linux && !android

package singbox

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"naga.network/core/diagnostics"
)

const linuxIPv4ConfRoot = "/proc/sys/net/ipv4/conf"

func adaptTUNForPlatform(config []byte) ([]byte, error) {
	return config, nil
}

// linuxTUNInterfaceNames returns only interfaces owned by TUN inbounds in the
// temporary runtime config. Linux interface names cannot contain a slash, but
// validate that again before constructing a procfs path from profile data.
func linuxTUNInterfaceNames(config []byte) ([]string, error) {
	var document map[string]any
	if err := json.Unmarshal(config, &document); err != nil {
		return nil, fmt.Errorf("decode Linux TUN config: %w", err)
	}
	names := make([]string, 0, 1)
	seen := make(map[string]struct{})
	for _, rawInbound := range inbounds(document) {
		inbound, ok := rawInbound.(map[string]any)
		if !ok {
			continue
		}
		if inboundType, _ := inbound["type"].(string); inboundType != "tun" {
			continue
		}
		name, _ := inbound["interface_name"].(string)
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if filepath.Base(name) != name || name == "." || strings.ContainsRune(name, '\x00') {
			return nil, fmt.Errorf("unsafe Linux TUN interface name %q", name)
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return names, nil
}

// configureLinuxTUNInterfaces switches reverse-path filtering to loose mode
// after sing-box has created the interface. Strict mode is incompatible with
// asymmetric policy routing: replies injected through TUN may be discarded
// because the main-table reverse route points at the physical interface.
//
// Only the per-interface setting is changed. It disappears together with the
// ephemeral TUN interface, so no global sysctl needs to be restored on stop.
func configureLinuxTUNInterfaces(names []string, journal *diagnostics.Logger) error {
	for _, name := range names {
		var err error
		for attempt := 0; attempt < 20; attempt++ {
			err = setLooseReversePathFilter(linuxIPv4ConfRoot, name)
			if err == nil {
				break
			}
			if !errors.Is(err, os.ErrNotExist) {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		if err != nil {
			return fmt.Errorf("configure Linux TUN %s: %w", name, err)
		}
		if journal != nil {
			journal.Info("sing-box", "tun_route_compatibility_applied", "Применена совместимость маршрутизации Linux TUN", map[string]any{
				"interface": name,
				"rp_filter": "loose",
				"stack":     "gvisor",
			})
		}
	}
	return nil
}

func setLooseReversePathFilter(root, name string) error {
	if filepath.Base(name) != name || name == "" || name == "." || strings.ContainsRune(name, '\x00') {
		return fmt.Errorf("unsafe interface name %q", name)
	}
	path := filepath.Join(root, name, "rp_filter")
	current, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(current)) == "2" {
		return nil
	}
	if err := os.WriteFile(path, []byte("2\n"), 0); err != nil {
		return err
	}
	updated, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(updated)) != "2" {
		return fmt.Errorf("rp_filter remained %q", strings.TrimSpace(string(updated)))
	}
	return nil
}

func shouldPrepareSystemResolver() bool {
	return true
}
