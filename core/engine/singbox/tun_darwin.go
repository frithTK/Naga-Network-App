//go:build darwin && !ios

package singbox

import "naga.network/core/diagnostics"

func linuxTUNInterfaceNames([]byte) ([]string, error)                 { return nil, nil }
func configureLinuxTUNInterfaces([]string, *diagnostics.Logger) error { return nil }

func adaptTUNForPlatform(config []byte) ([]byte, error) {
	return prepareDarwinConfig(config)
}

// The helper publishes a session-owned supplemental resolver pointing into
// utun. It never changes the Wi-Fi service's DNS or binds localhost:53.
func shouldPrepareSystemResolver() bool { return false }
