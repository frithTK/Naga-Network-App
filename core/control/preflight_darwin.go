//go:build darwin && !ios

package control

// The authenticated helper owns utun allocation and conflict checks. The user
// backend must not probe /dev/net/tun or require root before contacting it.
type darwinTUNPreflight struct{}

func (darwinTUNPreflight) CheckTUN() error { return nil }

func NewLinuxTUNPreflight() TUNPreflight { return darwinTUNPreflight{} }
