//go:build linux

package control

func NewPlatformDNS() DNSManager {
	return NewResolvedDNSManager()
}
