//go:build android

package olcrtc

// CaptureLink is a no-op: VpnService owns routes and DNS.
func CaptureLink(Execer) (LinkState, error) {
	return LinkState{}, nil
}

func BypassRoom(Execer, LinkState) error {
	return nil
}

func EngageDefault(Execer, LinkState) error {
	return nil
}

func RestoreLink(Execer, LinkState) error {
	return nil
}
