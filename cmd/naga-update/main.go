//go:build windows

package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

func main() {
	waitPID := flag.Int("wait-pid", 0, "exit after this process ends")
	zipPath := flag.String("zip", "", "portable zip to extract")
	dest := flag.String("to", "", "install directory")
	launch := flag.String("launch", "NagaNetwork.exe", "exe to start after extract")
	flag.Parse()
	if err := run(*waitPID, *zipPath, *dest, *launch); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func run(waitPID int, zipPath, dest, launch string) error {
	if zipPath == "" || dest == "" {
		return fmt.Errorf("нужны --zip и --to")
	}
	if waitPID > 0 {
		if err := waitForPID(waitPID, 2*time.Minute); err != nil {
			return err
		}
		time.Sleep(500 * time.Millisecond)
	}
	_ = exec.Command("taskkill", "/IM", "naga-control.exe", "/F").Run()
	time.Sleep(400 * time.Millisecond)
	if err := extractZip(zipPath, dest); err != nil {
		return err
	}
	exe := launch
	if !filepath.IsAbs(exe) {
		exe = filepath.Join(dest, launch)
	}
	cmd := exec.Command(exe)
	cmd.Dir = dest
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: false}
	return cmd.Start()
}

func waitForPID(pid int, timeout time.Duration) error {
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(handle)
	event, err := windows.WaitForSingleObject(handle, uint32(timeout.Milliseconds()))
	if err != nil {
		return fmt.Errorf("ожидание процесса %d: %w", pid, err)
	}
	if event == windows.WAIT_TIMEOUT {
		return fmt.Errorf("процесс %d не завершился вовремя", pid)
	}
	return nil
}
