//go:build windows

package control

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// WindowsTUNPreflight only checks that the Windows runtime files sit next to
// naga-control. Opening a Wintun adapter here would fail without the elevated
// helper and would hide a portable UAC prompt on Connect.
type WindowsTUNPreflight struct {
	Dir string
}

func NewLinuxTUNPreflight() TUNPreflight {
	return &WindowsTUNPreflight{}
}

func (p *WindowsTUNPreflight) CheckTUN() error {
	dir := ""
	if p != nil {
		dir = p.Dir
	}
	if dir == "" {
		exe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("не удалось определить каталог naga-control: %w", err)
		}
		dir = filepath.Dir(exe)
	}
	if _, err := os.Stat(filepath.Join(dir, "sing-box.exe")); err != nil {
		return errors.New("рядом с naga-control нет sing-box.exe")
	}
	if _, err := os.Stat(filepath.Join(dir, "wintun.dll")); err != nil {
		return errors.New("рядом с naga-control нет wintun.dll")
	}
	return nil
}
