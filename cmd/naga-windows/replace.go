//go:build windows

package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	controlPort              = 8765
	tcpTableOwnerPIDListener = 3
	afINET                   = 2
	mibTCPStateListen        = 2
	replacePortWait          = 5 * time.Second
	errorInsufficientBuffer  = windows.ERROR_INSUFFICIENT_BUFFER
)

var (
	iphlpapi                = windows.NewLazySystemDLL("iphlpapi.dll")
	procGetExtendedTcpTable = iphlpapi.NewProc("GetExtendedTcpTable")
)

func shouldReplaceControl(healthy, runtimeReady, bundleReady bool) bool {
	return healthy && !runtimeReady && bundleReady
}

func bundleHasRuntime(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, "sing-box.exe")); err != nil {
		return false
	}
	if _, err := os.Stat(filepath.Join(dir, "wintun.dll")); err != nil {
		return false
	}
	return true
}

func replaceStaleControl() error {
	pid, err := listenerPID(controlPort)
	if err != nil {
		return err
	}
	if pid == 0 {
		return waitPortFree(controlPort, replacePortWait)
	}
	if int(pid) == os.Getpid() {
		return errors.New("нельзя остановить собственный процесс control-plane")
	}
	name, err := processBaseName(pid)
	if err != nil {
		return fmt.Errorf("не удалось определить процесс на порту 8765: %w", err)
	}
	if !strings.EqualFold(name, "naga-control.exe") {
		return fmt.Errorf("порт 8765 занят процессом %s", name)
	}
	if err := terminatePID(pid); err != nil {
		return fmt.Errorf("не удалось остановить naga-control: %w", err)
	}
	if err := waitPortFree(controlPort, replacePortWait); err != nil {
		return err
	}
	return nil
}

type mibTCPRowOwnerPID struct {
	State      uint32
	LocalAddr  uint32
	LocalPort  uint32
	RemoteAddr uint32
	RemotePort uint32
	OwningPid  uint32
}

func listenerPID(port uint16) (uint32, error) {
	var size uint32
	r1, _, _ := procGetExtendedTcpTable.Call(0, uintptr(unsafe.Pointer(&size)), 0, afINET, tcpTableOwnerPIDListener, 0)
	if r1 != 0 && windows.Errno(r1) != errorInsufficientBuffer && size == 0 {
		return 0, windows.Errno(r1)
	}
	buf := make([]byte, size)
	r1, _, err := procGetExtendedTcpTable.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
		0,
		afINET,
		tcpTableOwnerPIDListener,
		0,
	)
	if r1 != 0 {
		if err != nil && err != windows.ERROR_SUCCESS {
			return 0, err
		}
		return 0, windows.Errno(r1)
	}
	if size < 4 {
		return 0, nil
	}
	count := binary.LittleEndian.Uint32(buf[:4])
	rowSize := unsafe.Sizeof(mibTCPRowOwnerPID{})
	need := 4 + uintptr(count)*rowSize
	if uintptr(len(buf)) < need {
		return 0, errors.New("TCP table is truncated")
	}
	loopback := binary.LittleEndian.Uint32([]byte{127, 0, 0, 1})
	for i := uint32(0); i < count; i++ {
		row := (*mibTCPRowOwnerPID)(unsafe.Pointer(&buf[4+uintptr(i)*rowSize]))
		if nboPort(row.LocalPort) != port {
			continue
		}
		if row.LocalAddr != 0 && row.LocalAddr != loopback {
			continue
		}
		if row.State != 0 && row.State != mibTCPStateListen {
			continue
		}
		return row.OwningPid, nil
	}
	return 0, nil
}

func nboPort(value uint32) uint16 {
	return uint16(value>>8)&0xff | uint16(value&0xff)<<8
}

func processBaseName(pid uint32) (string, error) {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(handle)
	var size uint32 = 512
	buf := make([]uint16, size)
	if err := windows.QueryFullProcessImageName(handle, 0, &buf[0], &size); err != nil {
		return "", err
	}
	return filepath.Base(windows.UTF16ToString(buf)), nil
}

func terminatePID(pid uint32) error {
	handle, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	return windows.TerminateProcess(handle, 1)
}

func waitPortFree(port uint16, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		pid, err := listenerPID(port)
		if err != nil {
			return err
		}
		if pid == 0 {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("порт %d всё ещё занят после остановки naga-control", port)
}
