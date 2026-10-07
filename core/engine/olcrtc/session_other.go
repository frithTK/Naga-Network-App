//go:build !linux && !windows

package olcrtc

import "errors"

var errRoutingUnsupported = errors.New("olcRTC routing is not supported on this platform")

func CaptureLink(Execer) (LinkState, error) {
	return LinkState{}, errRoutingUnsupported
}

func BypassRoom(Execer, LinkState) error {
	return errRoutingUnsupported
}

func EngageDefault(Execer, LinkState) error {
	return errRoutingUnsupported
}

func RestoreLink(Execer, LinkState) error {
	return nil
}
