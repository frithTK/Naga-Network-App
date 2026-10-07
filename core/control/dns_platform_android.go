//go:build android

package control

type noopDNSManager struct{}

func NewPlatformDNS() DNSManager {
	return noopDNSManager{}
}

func (noopDNSManager) Configure() (func() error, error) {
	return nil, nil
}
