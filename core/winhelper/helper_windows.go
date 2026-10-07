//go:build windows

package winhelper

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	createNoWindow        = 0x08000000
	pipeAccessDuplex      = 0x00000003
	fileFlagFirstInstance = 0x00080000
	pipeRejectRemote      = 0x00000008
	helperReadyTimeout    = 20 * time.Second
	modePing              = "ping"
	modeQuit              = "quit"
	swHide                = 0
)

var (
	errHelperQuit        = errors.New("elevated helper quit")
	kernel32             = windows.NewLazySystemDLL("kernel32.dll")
	user32               = windows.NewLazySystemDLL("user32.dll")
	procGetConsoleWindow = kernel32.NewProc("GetConsoleWindow")
	procShowWindow       = user32.NewProc("ShowWindow")
)

func Install() error {
	if !processElevated() {
		return ErrNotElevated
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate naga-control: %w", err)
	}
	exe, err = filepath.Abs(exe)
	if err != nil {
		return err
	}
	sid, err := currentUserSID()
	if err != nil {
		return err
	}
	xmlPath, err := writeTaskXML(TaskDefinition(exe, sid))
	if err != nil {
		return err
	}
	defer os.Remove(xmlPath)
	if err := runSchtasks("/Create", "/TN", TaskName, "/XML", xmlPath, "/F"); err != nil {
		return fmt.Errorf("register elevated helper: %w", err)
	}
	_ = runSchtasks("/Run", "/TN", TaskName)
	return nil
}

func Uninstall() error {
	_ = requestSpawn(modeQuit, "")
	if err := runSchtasks("/Delete", "/TN", TaskName, "/F"); err != nil {
		if helperInstalled() {
			return fmt.Errorf("remove elevated helper: %w", err)
		}
	}
	return nil
}

func EnsureRunning() {
	if helperReachable() {
		return
	}
	if !helperInstalled() {
		return
	}
	_ = runSchtasks("/Run", "/TN", TaskName)
}

func Spawn(mode, pipe string) error {
	if err := ValidateSpawn(mode, pipe); err != nil {
		return err
	}
	if err := requestSpawn(mode, pipe); err == nil {
		return nil
	}
	if !helperInstalled() {
		return ErrHelperUnavailable
	}
	if err := runSchtasks("/Run", "/TN", TaskName); err != nil && !helperReachable() {
		return fmt.Errorf("%w: %v", ErrHelperUnavailable, err)
	}
	deadline := time.Now().Add(helperReadyTimeout)
	var last error
	for time.Now().Before(deadline) {
		last = requestSpawn(mode, pipe)
		if last == nil {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	if last == nil {
		last = ErrHelperUnavailable
	}
	return fmt.Errorf("%w: %v", ErrHelperUnavailable, last)
}

func Run() error {
	if !processElevated() {
		return ErrNotElevated
	}
	hideConsole()
	mutexName, err := windows.UTF16PtrFromString(MutexName)
	if err != nil {
		return err
	}
	mutex, err := windows.CreateMutex(nil, true, mutexName)
	if err != nil {
		if mutex != 0 {
			_ = windows.CloseHandle(mutex)
		}
		if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
			return nil
		}
		return err
	}
	defer windows.CloseHandle(mutex)

	self, err := os.Executable()
	if err != nil {
		return err
	}
	for {
		if err := serveOnce(self); err != nil {
			if errors.Is(err, errHelperQuit) {
				return nil
			}
			return err
		}
	}
}

func serveOnce(self string) error {
	listener, err := listenHelperPipe()
	if err != nil {
		return err
	}
	if err := windows.ConnectNamedPipe(listener, nil); err != nil && !isPipeAlreadyConnected(err) {
		_ = windows.CloseHandle(listener)
		return err
	}
	conn := os.NewFile(uintptr(listener), `\\.\pipe\`+PipeName)
	defer conn.Close()

	var req SpawnRequest
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		return nil
	}
	switch req.Mode {
	case modePing:
		return json.NewEncoder(conn).Encode(SpawnReply{OK: true})
	case modeQuit:
		_ = json.NewEncoder(conn).Encode(SpawnReply{OK: true})
		return errHelperQuit
	}
	if err := spawnHost(self, req.Mode, req.Pipe); err != nil {
		_ = json.NewEncoder(conn).Encode(SpawnReply{Error: err.Error()})
		return nil
	}
	return json.NewEncoder(conn).Encode(SpawnReply{OK: true})
}

func spawnHost(self, mode, pipe string) error {
	if err := ValidateSpawn(mode, pipe); err != nil {
		return err
	}
	flag, ok := HostFlag(mode)
	if !ok {
		return ErrInvalidSpawn
	}
	cmd := exec.Command(self, flag, pipe)
	cmd.Dir = filepath.Dir(self)
	hideCommand(cmd)
	return cmd.Start()
}

func requestSpawn(mode, pipe string) error {
	conn, err := openHelperPipe()
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := json.NewEncoder(conn).Encode(SpawnRequest{Mode: mode, Pipe: pipe}); err != nil {
		return err
	}
	var reply SpawnReply
	if err := json.NewDecoder(conn).Decode(&reply); err != nil {
		return err
	}
	if !reply.OK {
		message := reply.Error
		if message == "" {
			message = "elevated helper refused spawn"
		}
		return errors.New(message)
	}
	return nil
}

func helperReachable() bool {
	return requestSpawn(modePing, "") == nil
}

func helperInstalled() bool {
	cmd := exec.Command("schtasks", "/Query", "/TN", TaskName)
	hideCommand(cmd)
	return cmd.Run() == nil
}

func runSchtasks(args ...string) error {
	cmd := exec.Command("schtasks", args...)
	hideCommand(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		message := string(bytes.TrimSpace(out))
		if message == "" {
			return err
		}
		return fmt.Errorf("%s", message)
	}
	return nil
}

func hideCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}

func writeTaskXML(body string) (string, error) {
	file, err := os.CreateTemp("", "naga-helper-*.xml")
	if err != nil {
		return "", err
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		os.Remove(path)
		return "", err
	}
	if err := os.WriteFile(path, utf16LE(body), 0o600); err != nil {
		os.Remove(path)
		return "", err
	}
	return path, nil
}

func utf16LE(s string) []byte {
	u := utf16.Encode([]rune(s))
	buf := make([]byte, 2+len(u)*2)
	buf[0], buf[1] = 0xFF, 0xFE
	for i, r := range u {
		binary.LittleEndian.PutUint16(buf[2+i*2:], r)
	}
	return buf
}

func currentUserSID() (string, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", err
	}
	return user.User.Sid.String(), nil
}

func currentUserSecurityDescriptor() (*windows.SECURITY_DESCRIPTOR, error) {
	sid, err := currentUserSID()
	if err != nil {
		return nil, err
	}
	return windows.SecurityDescriptorFromString("D:P(A;;GA;;;" + sid + ")")
}

func listenHelperPipe() (windows.Handle, error) {
	sd, err := currentUserSecurityDescriptor()
	if err != nil {
		return 0, err
	}
	name, err := windows.UTF16PtrFromString(`\\.\pipe\` + PipeName)
	if err != nil {
		return 0, err
	}
	sa := windows.SecurityAttributes{
		Length:             uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		SecurityDescriptor: sd,
	}
	return windows.CreateNamedPipe(
		name,
		pipeAccessDuplex|fileFlagFirstInstance,
		windows.PIPE_TYPE_BYTE|windows.PIPE_READMODE_BYTE|windows.PIPE_WAIT|pipeRejectRemote,
		1,
		65536,
		65536,
		0,
		&sa,
	)
}

func openHelperPipe() (*os.File, error) {
	name, err := windows.UTF16PtrFromString(`\\.\pipe\` + PipeName)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(
		name,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		0,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(handle), `\\.\pipe\`+PipeName), nil
}

func processElevated() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}

func isPipeAlreadyConnected(err error) bool {
	if err == nil {
		return true
	}
	return errors.Is(err, windows.ERROR_PIPE_CONNECTED) || errnoFrom(err) == windows.ERROR_PIPE_CONNECTED
}

func errnoFrom(err error) windows.Errno {
	var errno windows.Errno
	if errors.As(err, &errno) {
		return errno
	}
	var syserr syscall.Errno
	if errors.As(err, &syserr) {
		return windows.Errno(syserr)
	}
	return 0
}

func hideConsole() {
	hwnd, _, _ := procGetConsoleWindow.Call()
	if hwnd != 0 {
		_, _, _ = procShowWindow.Call(hwnd, uintptr(swHide))
	}
}

func DoneEventName(pipe string) string {
	return `Local\NagaHostDone-` + pipe
}

func CreateDoneEvent(pipe string) (windows.Handle, error) {
	if !ValidPipeName(pipe) {
		return 0, ErrInvalidSpawn
	}
	sd, err := currentUserSecurityDescriptor()
	if err != nil {
		return 0, err
	}
	name, err := windows.UTF16PtrFromString(DoneEventName(pipe))
	if err != nil {
		return 0, err
	}
	sa := windows.SecurityAttributes{
		Length:             uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		SecurityDescriptor: sd,
	}
	handle, err := windows.CreateEvent(&sa, 1, 0, name)
	if handle != 0 && errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		_ = windows.ResetEvent(handle)
		return handle, nil
	}
	return handle, err
}

func SignalDoneEvent(handle windows.Handle) {
	if handle != 0 {
		_ = windows.SetEvent(handle)
	}
}

func WaitDoneEvent(pipe string, timeout time.Duration) error {
	if !ValidPipeName(pipe) {
		return nil
	}
	name, err := windows.UTF16PtrFromString(DoneEventName(pipe))
	if err != nil {
		return nil
	}
	handle, err := windows.OpenEvent(windows.SYNCHRONIZE, false, name)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(handle)
	ms := uint32(windows.INFINITE)
	if timeout > 0 {
		ms = uint32(timeout / time.Millisecond)
	}
	_, err = windows.WaitForSingleObject(handle, ms)
	return err
}
