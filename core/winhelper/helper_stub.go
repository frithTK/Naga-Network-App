//go:build !windows

package winhelper

func Install() error { return ErrWindowsOnly }

func Uninstall() error { return ErrWindowsOnly }

func Run() error { return ErrWindowsOnly }

func EnsureRunning() {}
