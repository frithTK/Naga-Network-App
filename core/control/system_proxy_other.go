//go:build !linux && !windows

package control

func NewSystemProxyManager() SystemProxyManager { return nil }

func clearStuckOlcProxy() {}
