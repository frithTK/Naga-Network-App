//go:build windows

package singbox

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"naga.network/core/winhelper"
)

const (
	windowsHostConnectTimeout = 3 * time.Minute
	windowsHostReadyTimeout   = 30 * time.Second

	seeMaskNoCloseProcess = 0x00000040
	seeMaskNoAsync        = 0x00000100
	pipeAccessDuplex      = 0x00000003
	fileFlagFirstInstance = 0x00080000
	pipeRejectRemote      = 0x00000008
	createNoWindow        = 0x08000000
	swHide                = 0
)

var (
	shell32             = windows.NewLazySystemDLL("shell32.dll")
	procShellExecuteExW = shell32.NewProc("ShellExecuteExW")
)

type shellExecuteInfo struct {
	CbSize         uint32
	FMask          uint32
	Hwnd           windows.HWND
	LpVerb         *uint16
	LpFile         *uint16
	LpParameters   *uint16
	LpDirectory    *uint16
	NShow          int32
	HInstApp       windows.Handle
	LpIDList       uintptr
	LpClass        *uint16
	HkeyClass      windows.Handle
	DwHotKey       uint32
	HIconOrMonitor windows.Handle
	HProcess       windows.Handle
}

func defaultReadyTimeout() time.Duration {
	return 30 * time.Second
}

func startPlatformProcess(ctx context.Context, req processRequest) (startedProcess, error) {
	if len(req.Args) > 0 && req.Args[0] != "run" {
		return startCommandProcess(ctx, req)
	}
	return startElevatedHost(ctx, req)
}

func startElevatedHost(ctx context.Context, req processRequest) (startedProcess, error) {
	self, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("locate naga-control: %w", err)
	}
	id := make([]byte, 8)
	if _, err := rand.Read(id); err != nil {
		return nil, err
	}
	pipeName := "NagaTun-" + hex.EncodeToString(id)
	pipePath := `\\.\pipe\` + pipeName

	listener, err := listenNagaPipe(pipePath)
	if err != nil {
		return nil, err
	}
	connected := make(chan error, 1)
	go func() {
		connected <- windows.ConnectNamedPipe(listener, nil)
	}()

	proc, err := launchTunHost(self, pipeName)
	if err != nil {
		_ = windows.CloseHandle(listener)
		return nil, err
	}

	select {
	case <-ctx.Done():
		_ = proc.closeHost()
		_ = windows.CloseHandle(listener)
		return nil, ctx.Err()
	case err := <-connected:
		if err != nil && !isPipeAlreadyConnected(err) {
			_ = proc.closeHost()
			_ = windows.CloseHandle(listener)
			return nil, fmt.Errorf("wait for elevated TUN host: %w", err)
		}
	case <-time.After(windowsHostConnectTimeout):
		_ = proc.closeHost()
		_ = windows.CloseHandle(listener)
		return nil, errors.New("elevated TUN host did not connect in time")
	}

	proc.conn = os.NewFile(uintptr(listener), pipePath)
	if err := proc.sendStart(req); err != nil {
		_ = proc.Kill()
		_ = proc.closeHost()
		if proc.conn != nil {
			_ = proc.conn.Close()
		}
		return nil, err
	}
	return proc, nil
}

func launchTunHost(self, pipeName string) (*hostProcess, error) {
	if processElevated() {
		cmd := exec.Command(self, "--tun-host", pipeName)
		cmd.SysProcAttr = &syscall.SysProcAttr{
			HideWindow:    true,
			CreationFlags: createNoWindow,
		}
		if err := cmd.Start(); err != nil {
			return nil, err
		}
		return &hostProcess{cmd: cmd}, nil
	}
	if err := winhelper.Spawn(winhelper.ModeTun, pipeName); err == nil {
		return &hostProcess{pipe: pipeName}, nil
	}
	verb, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return nil, err
	}
	file, err := windows.UTF16PtrFromString(self)
	if err != nil {
		return nil, err
	}
	params, err := windows.UTF16PtrFromString("--tun-host " + pipeName)
	if err != nil {
		return nil, err
	}
	info := shellExecuteInfo{
		FMask:        seeMaskNoCloseProcess | seeMaskNoAsync,
		LpVerb:       verb,
		LpFile:       file,
		LpParameters: params,
		NShow:        swHide,
	}
	info.CbSize = uint32(unsafe.Sizeof(info))
	r1, _, errno := procShellExecuteExW.Call(uintptr(unsafe.Pointer(&info)))
	if r1 == 0 {
		if isCancelled(errno) {
			return nil, ErrUACCancelled
		}
		if errno != nil && errno != windows.ERROR_SUCCESS {
			return nil, errno
		}
		return nil, errors.New("не удалось запросить права администратора")
	}
	if info.HProcess == 0 {
		return nil, errors.New("elevated TUN host handle is missing")
	}
	return &hostProcess{handle: info.HProcess}, nil
}

func processElevated() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}

func isCancelled(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, windows.ERROR_CANCELLED) {
		return true
	}
	var errno syscall.Errno
	if errors.As(err, &errno) && errno == errorCancelledErrno() {
		return true
	}
	return uint32(errnoFrom(err)) == uint32(windows.ERROR_CANCELLED)
}

func errorCancelledErrno() syscall.Errno {
	return syscall.Errno(windows.ERROR_CANCELLED)
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

func isPipeAlreadyConnected(err error) bool {
	if err == nil {
		return true
	}
	return errors.Is(err, windows.ERROR_PIPE_CONNECTED) || errnoFrom(err) == windows.ERROR_PIPE_CONNECTED
}

func listenNagaPipe(path string) (windows.Handle, error) {
	sd, err := currentUserSecurityDescriptor()
	if err != nil {
		return 0, err
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	sa := windows.SecurityAttributes{
		Length:             uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		SecurityDescriptor: sd,
	}
	handle, err := windows.CreateNamedPipe(
		name,
		pipeAccessDuplex|fileFlagFirstInstance,
		windows.PIPE_TYPE_BYTE|windows.PIPE_READMODE_BYTE|windows.PIPE_WAIT|pipeRejectRemote,
		1,
		65536,
		65536,
		0,
		&sa,
	)
	if err != nil {
		return 0, err
	}
	return handle, nil
}

type hostProcess struct {
	conn    *os.File
	handle  windows.Handle
	cmd     *exec.Cmd
	decoder *json.Decoder
	pipe    string
}

func (p *hostProcess) sendStart(req processRequest) error {
	payload := tunHostRequest{
		ConfigPath: req.ConfigPath,
		BinaryPath: req.Binary,
		LogPath:    req.LogPath,
		Env:        req.Env,
	}
	if err := json.NewEncoder(p.conn).Encode(payload); err != nil {
		return fmt.Errorf("send TUN host request: %w", err)
	}
	p.decoder = json.NewDecoder(p.conn)
	type outcome struct {
		reply tunHostReply
		err   error
	}
	done := make(chan outcome, 1)
	go func() {
		var reply tunHostReply
		err := p.decoder.Decode(&reply)
		done <- outcome{reply: reply, err: err}
	}()
	select {
	case <-time.After(windowsHostReadyTimeout):
		_ = p.conn.Close()
		return errors.New("elevated TUN host did not become ready in time")
	case got := <-done:
		if got.err != nil {
			return fmt.Errorf("read TUN host reply: %w", got.err)
		}
		if !got.reply.OK {
			message := strings.TrimSpace(got.reply.Error)
			if message == "" {
				message = "elevated TUN host failed"
			}
			return errors.New(message)
		}
		return nil
	}
}

func (p *hostProcess) Wait() error {
	defer func() {
		_ = p.closeHost()
		if p.conn != nil {
			_ = p.conn.Close()
		}
	}()
	var replyErr error
	if p.decoder != nil {
		var reply tunHostReply
		decodeErr := p.decoder.Decode(&reply)
		replyErr = tunHostReplyError(reply, decodeErr)
		if decodeErr != nil {
			replyErr = nil
		}
	}
	procErr := p.waitHost()
	if replyErr != nil {
		return replyErr
	}
	return procErr
}

func (p *hostProcess) waitHost() error {
	if p.cmd != nil {
		return p.cmd.Wait()
	}
	if p.handle == 0 {
		if p.pipe != "" {
			return winhelper.WaitDoneEvent(p.pipe, 8*time.Second)
		}
		return nil
	}
	event, err := windows.WaitForSingleObject(p.handle, windows.INFINITE)
	if err != nil {
		return err
	}
	if event != windows.WAIT_OBJECT_0 {
		return fmt.Errorf("wait elevated TUN host: %d", event)
	}
	var code uint32
	if err := windows.GetExitCodeProcess(p.handle, &code); err != nil {
		return err
	}
	if code == 0 {
		return nil
	}
	return fmt.Errorf("elevated TUN host exited with status %d", code)
}

func (p *hostProcess) SignalStop() error {
	if p == nil || p.conn == nil {
		return nil
	}
	return p.conn.Close()
}

func (p *hostProcess) Kill() error {
	return p.SignalStop()
}

func (p *hostProcess) closeHost() error {
	if p == nil || p.handle == 0 || p.cmd != nil {
		return nil
	}
	err := windows.CloseHandle(p.handle)
	p.handle = 0
	return err
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

func restrictPathToCurrentUser(path string) error {
	sid, err := currentUserSID()
	if err != nil {
		return err
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;" + sid + ")")
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		dacl,
		nil,
	)
}

func restrictConfigAccess(dir, file string) error {
	if dir != "" {
		if err := restrictPathToCurrentUser(dir); err != nil {
			return fmt.Errorf("restrict config directory: %w", err)
		}
	}
	if file != "" {
		if err := restrictPathToCurrentUser(file); err != nil {
			return fmt.Errorf("restrict config file: %w", err)
		}
	}
	return nil
}

func RunTunHost(pipeName string) error {
	if strings.TrimSpace(pipeName) == "" {
		return errors.New("tun host pipe name is empty")
	}
	if done, err := winhelper.CreateDoneEvent(pipeName); err == nil && done != 0 {
		defer func() {
			winhelper.SignalDoneEvent(done)
			_ = windows.CloseHandle(done)
		}()
	}
	path := pipeName
	if !strings.HasPrefix(path, `\\.\pipe\`) && !strings.HasPrefix(path, `//./pipe/`) {
		path = `\\.\pipe\` + pipeName
	}
	conn, err := openNagaPipe(path)
	if err != nil {
		return err
	}
	defer conn.Close()

	var req tunHostRequest
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		return err
	}
	if err := runHostSingBox(conn, req); err != nil {
		_ = json.NewEncoder(conn).Encode(tunHostReply{Error: err.Error()})
		return err
	}
	return nil
}

func openNagaPipe(path string) (*os.File, error) {
	name, err := windows.UTF16PtrFromString(path)
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
	return os.NewFile(uintptr(handle), path), nil
}

func runHostSingBox(conn *os.File, req tunHostRequest) error {
	if req.BinaryPath == "" || req.ConfigPath == "" {
		return errors.New("TUN host request is incomplete")
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	if !winhelper.AllowedBundledBinary(filepath.Dir(self), req.BinaryPath, "sing-box.exe") {
		return errors.New("unexpected sing-box path")
	}
	command := exec.Command(req.BinaryPath, "run", "-c", req.ConfigPath)
	command.Dir = filepath.Dir(req.BinaryPath)
	if len(req.Env) > 0 {
		command.Env = append(os.Environ(), req.Env...)
	}
	stderr := newTailBuffer(maxFailureOutputBytes)
	command.Stdout = io.Discard
	command.Stderr = stderr
	if req.LogPath != "" {
		logFile, err := os.OpenFile(req.LogPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return fmt.Errorf("open sing-box log: %w", err)
		}
		defer logFile.Close()
		command.Stdout = logFile
		command.Stderr = io.MultiWriter(stderr, logFile)
	}
	clearStaleWintunAdapter("naga-tun0")
	if err := command.Start(); err != nil {
		return err
	}
	if job, err := attachKillOnCloseJob(command.Process.Pid); err == nil {
		defer windows.CloseHandle(job)
	}
	started := make(chan error, 1)
	go func() {
		started <- command.Wait()
	}()
	select {
	case err := <-started:
		return errors.New(formatTunHostExit(err, stderr.String()))
	case <-time.After(200 * time.Millisecond):
	}
	if err := json.NewEncoder(conn).Encode(tunHostReply{OK: true}); err != nil {
		_ = command.Process.Kill()
		<-started
		return err
	}

	done := make(chan struct{})
	go func() {
		_, _ = bufio.NewReader(conn).ReadByte()
		close(done)
	}()
	select {
	case <-done:
		_ = command.Process.Kill()
		return <-started
	case err := <-started:
		message := formatTunHostExit(err, stderr.String())
		_ = json.NewEncoder(conn).Encode(tunHostReply{Error: message})
		return errors.New(message)
	}
}

func attachKillOnCloseJob(pid int) (windows.Handle, error) {
	if pid <= 0 {
		return 0, errors.New("pid is missing")
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	ret, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	)
	if ret == 0 {
		_ = windows.CloseHandle(job)
		if err != nil {
			return 0, err
		}
		return 0, errors.New("SetInformationJobObject failed")
	}
	process, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE,
		false,
		uint32(pid),
	)
	if err != nil {
		_ = windows.CloseHandle(job)
		return 0, err
	}
	defer windows.CloseHandle(process)
	if err := windows.AssignProcessToJobObject(job, process); err != nil {
		_ = windows.CloseHandle(job)
		return 0, err
	}
	return job, nil
}

func clearStaleWintunAdapter(name string) {
	script, ok := staleWintunCleanupScript(name)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
	_ = cmd.Run()
}
