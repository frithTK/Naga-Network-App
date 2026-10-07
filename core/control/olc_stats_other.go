//go:build !linux && !windows

package control

import "errors"

func readPlatformIfaceBytes(string) (tx, rx int64, err error) {
	return 0, 0, errors.New("olcRTC interface counters are not available")
}
