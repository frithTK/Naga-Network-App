package control

// SystemProxyManager applies a desktop system proxy and returns a function
// that restores the user's previous settings.
type SystemProxyManager interface {
	Configure(host string, port int) (restore func() error, err error)
}
