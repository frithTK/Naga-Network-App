//go:build ios

package singbox

import "naga.network/core/diagnostics"

func linuxTUNInterfaceNames([]byte) ([]string, error) {
	return nil, nil
}

func configureLinuxTUNInterfaces([]string, *diagnostics.Logger) error {
	return nil
}

func adaptTUNForPlatform(config []byte) ([]byte, error) {
	// PacketTunnel owns the system interface. The runtime config must not
	// inject Linux naga-tun0 or Android hev mixed mode.
	return config, nil
}

func shouldPrepareSystemResolver() bool {
	return false
}
