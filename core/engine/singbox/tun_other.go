//go:build !linux && !windows

package singbox

import "naga.network/core/diagnostics"

func linuxTUNInterfaceNames([]byte) ([]string, error) {
	return nil, nil
}

func configureLinuxTUNInterfaces([]string, *diagnostics.Logger) error {
	return nil
}

func adaptTUNForPlatform(config []byte) ([]byte, error) {
	return config, nil
}

func shouldPrepareSystemResolver() bool {
	return true
}
