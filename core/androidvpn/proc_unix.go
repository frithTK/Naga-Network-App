//go:build unix

package androidvpn

import "syscall"

func hevProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
