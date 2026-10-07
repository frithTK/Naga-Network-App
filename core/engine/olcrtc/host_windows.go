//go:build windows

package olcrtc

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
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

type sysExec struct{}

func (sysExec) Run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	configureCommand(cmd)
	return cmd.Run()
}

func (sysExec) Output(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	configureCommand(cmd)
	return cmd.Output()
}

type olcHostProcess struct {
	conn   *os.File
	handle windows.Handle
	cmd    *exec.Cmd
	pipe   string
}

func (p *olcHostProcess) Stop() error {
	if p == nil {
		return nil
	}
	_ = p.closeConn()
	return p.waitHost()
}

func (p *olcHostProcess) closeConn() error {
	if p == nil || p.conn == nil {
		return nil
	}
	err := p.conn.Close()
	p.conn = nil
	return err
}

func (p *olcHostProcess) waitHost() error {
	defer p.closeHandle()
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
		return fmt.Errorf("wait elevated olcRTC host: %d", event)
	}
	var code uint32
	if err := windows.GetExitCodeProcess(p.handle, &code); err != nil {
		return err
	}
	if code == 0 {
		return nil
	}
	return fmt.Errorf("elevated olcRTC host exited with status %d", code)
}

func (p *olcHostProcess) closeHandle() {
	if p == nil || p.handle == 0 || p.cmd != nil {
		return
	}
	_ = windows.CloseHandle(p.handle)
	p.handle = 0
}

func (p *olcHostProcess) kill() {
	_ = p.closeConn()
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
		return
	}
	if p.handle != 0 {
		_ = windows.TerminateProcess(p.handle, 1)
	}
}

// StartElevatedTunnel asks naga-control --olc-host to create hev and move routes.
func StartElevatedTunnel(ctx context.Context, hevPath, configPath string, roomIPs []net.IP) (TunnelHost, error) {
	if strings.TrimSpace(hevPath) == "" || strings.TrimSpace(configPath) == "" {
		return nil, errors.New("olcRTC TUN host request is incomplete")
	}
	self, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("locate naga-control: %w", err)
	}
	id := make([]byte, 8)
	if _, err := rand.Read(id); err != nil {
		return nil, err
	}
	pipeName := "NagaOlc-" + hex.EncodeToString(id)
	pipePath := `\\.\pipe\` + pipeName

	listener, err := listenNagaOlcPipe(pipePath)
	if err != nil {
		return nil, err
	}
	connected := make(chan error, 1)
	go func() {
		connected <- windows.ConnectNamedPipe(listener, nil)
	}()

	proc, err := launchOlcHost(self, pipeName)
	if err != nil {
		_ = windows.CloseHandle(listener)
		return nil, err
	}

	select {
	case <-ctx.Done():
		proc.kill()
		_ = windows.CloseHandle(listener)
		return nil, ctx.Err()
	case err := <-connected:
		if err != nil && !isPipeAlreadyConnected(err) {
			proc.kill()
			_ = windows.CloseHandle(listener)
			return nil, fmt.Errorf("wait for elevated olcRTC host: %w", err)
		}
	case <-time.After(windowsHostConnectTimeout):
		proc.kill()
		_ = windows.CloseHandle(listener)
		return nil, errors.New("elevated olcRTC host did not connect in time")
	}

	proc.conn = os.NewFile(uintptr(listener), pipePath)
	if err := proc.sendStart(olcHostRequest{
		HevPath:    hevPath,
		ConfigPath: configPath,
		RoomIPs:    roomIPStrings(roomIPs),
	}); err != nil {
		proc.kill()
		_ = proc.closeConn()
		return nil, err
	}
	return proc, nil
}

func (p *olcHostProcess) sendStart(req olcHostRequest) error {
	if err := json.NewEncoder(p.conn).Encode(req); err != nil {
		return fmt.Errorf("send olcRTC host request: %w", err)
	}
	type outcome struct {
		reply olcHostReply
		err   error
	}
	done := make(chan outcome, 1)
	go func() {
		var reply olcHostReply
		err := json.NewDecoder(p.conn).Decode(&reply)
		done <- outcome{reply: reply, err: err}
	}()
	select {
	case <-time.After(windowsHostReadyTimeout):
		_ = p.closeConn()
		return errors.New("elevated olcRTC host did not become ready in time")
	case got := <-done:
		if got.err != nil {
			return fmt.Errorf("read olcRTC host reply: %w", got.err)
		}
		if !got.reply.OK {
			message := strings.TrimSpace(got.reply.Error)
			if message == "" {
				message = "elevated olcRTC host failed"
			}
			return errors.New(message)
		}
		return nil
	}
}

func launchOlcHost(self, pipeName string) (*olcHostProcess, error) {
	if processElevated() {
		cmd := exec.Command(self, "--olc-host", pipeName)
		configureCommand(cmd)
		if err := cmd.Start(); err != nil {
			return nil, err
		}
		return &olcHostProcess{cmd: cmd}, nil
	}
	if err := winhelper.Spawn(winhelper.ModeOlc, pipeName); err == nil {
		return &olcHostProcess{pipe: pipeName}, nil
	}
	verb, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return nil, err
	}
	file, err := windows.UTF16PtrFromString(self)
	if err != nil {
		return nil, err
	}
	params, err := windows.UTF16PtrFromString("--olc-host " + pipeName)
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
		return nil, errors.New("elevated olcRTC host handle is missing")
	}
	return &olcHostProcess{handle: info.HProcess}, nil
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
	if errors.As(err, &errno) && uint32(errno) == uint32(windows.ERROR_CANCELLED) {
		return true
	}
	return uint32(errnoFrom(err)) == uint32(windows.ERROR_CANCELLED)
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

func currentUserSID() (string, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", err
	}
	return user.User.Sid.String(), nil
}

func listenNagaOlcPipe(path string) (windows.Handle, error) {
	sid, err := currentUserSID()
	if err != nil {
		return 0, err
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;GA;;;" + sid + ")")
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

func openNagaOlcPipe(path string) (*os.File, error) {
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

// RunOlcHost is the elevated child: hev + routes, then restore on pipe close.
func RunOlcHost(pipeName string) error {
	if strings.TrimSpace(pipeName) == "" {
		return errors.New("olcRTC host pipe name is empty")
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
	conn, err := openNagaOlcPipe(path)
	if err != nil {
		return err
	}
	defer conn.Close()

	var req olcHostRequest
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		return err
	}
	if err := runHostOlcTunnel(conn, req); err != nil {
		_ = json.NewEncoder(conn).Encode(olcHostReply{Error: err.Error()})
		return err
	}
	return nil
}

func runHostOlcTunnel(conn *os.File, req olcHostRequest) error {
	if strings.TrimSpace(req.HevPath) == "" || strings.TrimSpace(req.ConfigPath) == "" {
		return errors.New("olcRTC TUN host request is incomplete")
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	if !winhelper.AllowedBundledBinary(filepath.Dir(self), req.HevPath, "hev-socks5-tunnel.exe") {
		return errors.New("unexpected hev-socks5-tunnel path")
	}
	execer := sysExec{}
	link, err := CaptureLink(execer)
	if err != nil {
		return err
	}
	link.RoomIPs = parseRoomIPs(req.RoomIPs)
	if err := BypassRoom(execer, link); err != nil {
		_ = RestoreLink(execer, link)
		return err
	}
	command := exec.Command(req.HevPath, req.ConfigPath)
	command.Dir = filepath.Dir(req.HevPath)
	configureCommand(command)
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if err := command.Start(); err != nil {
		_ = RestoreLink(execer, link)
		return err
	}
	if job, err := attachKillOnCloseJob(command.Process.Pid); err == nil {
		defer windows.CloseHandle(job)
	}
	waitCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	err = WaitForIface(waitCtx, TunnelIface)
	cancel()
	if err != nil {
		_ = command.Process.Kill()
		_, _ = command.Process.Wait()
		_ = RestoreLink(execer, link)
		return err
	}
	if err := EngageDefault(execer, link); err != nil {
		_ = RestoreLink(execer, link)
		_ = command.Process.Kill()
		_, _ = command.Process.Wait()
		return err
	}

	started := make(chan error, 1)
	go func() {
		started <- command.Wait()
	}()
	if err := json.NewEncoder(conn).Encode(olcHostReply{OK: true}); err != nil {
		_ = RestoreLink(execer, link)
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
		_ = RestoreLink(execer, link)
		_ = command.Process.Kill()
		<-started
		return nil
	case err := <-started:
		_ = RestoreLink(execer, link)
		message := "hev-socks5-tunnel exited"
		if err != nil {
			message = err.Error()
		}
		_ = json.NewEncoder(conn).Encode(olcHostReply{Error: message})
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
