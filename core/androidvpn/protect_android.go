//go:build android

package androidvpn

// ProtectSocket is set by the Android host before starting the local control.
// VpnService owns the callback and checks that the VPN permission is active.
var ProtectSocket func(fd int) bool
