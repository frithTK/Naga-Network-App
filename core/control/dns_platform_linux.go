//go:build linux && !android

package control

func NewPlatformDNS() DNSManager {
	return NewResolvedDNSManager()
}
