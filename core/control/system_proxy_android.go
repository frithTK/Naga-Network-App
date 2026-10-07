//go:build android

package control

func NewSystemProxyManager() SystemProxyManager { return nil }

func clearStuckOlcProxy() {}
