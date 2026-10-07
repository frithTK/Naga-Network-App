package singbox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
)

// ErrUACCancelled is returned when the user dismisses the elevation prompt.
var ErrUACCancelled = errors.New("запуск отменён, права администратора не выданы")

type startedProcess interface {
	Wait() error
	SignalStop() error
	Kill() error
}

type processRequest struct {
	Binary     string
	Args       []string
	Dir        string
	Env        []string
	ConfigPath string
	LogPath    string
	Stdout     io.Writer
	Stderr     io.Writer
}

type processStarter func(context.Context, processRequest) (startedProcess, error)

type cmdProcess struct {
	cmd *exec.Cmd
}

func (p *cmdProcess) Wait() error {
	if p == nil || p.cmd == nil {
		return errors.New("sing-box process is missing")
	}
	return p.cmd.Wait()
}

func (p *cmdProcess) SignalStop() error {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	err := p.cmd.Process.Signal(os.Interrupt)
	if errors.Is(err, os.ErrProcessDone) {
		return nil
	}
	return err
}

func (p *cmdProcess) Kill() error {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	err := p.cmd.Process.Kill()
	if errors.Is(err, os.ErrProcessDone) {
		return nil
	}
	return err
}

func startCommandProcess(ctx context.Context, req processRequest) (*cmdProcess, error) {
	if req.Binary == "" {
		return nil, ErrBinaryNotConfigured
	}
	command := exec.CommandContext(ctx, req.Binary, req.Args...)
	command.Dir = req.Dir
	if len(req.Env) > 0 {
		command.Env = append(os.Environ(), req.Env...)
	}
	command.Stdout = req.Stdout
	command.Stderr = req.Stderr
	if err := command.Start(); err != nil {
		return nil, err
	}
	return &cmdProcess{cmd: command}, nil
}

func (a Adapter) launch(ctx context.Context, req processRequest) (startedProcess, error) {
	if a.Starter != nil {
		return a.Starter(ctx, req)
	}
	return startPlatformProcess(ctx, req)
}

func (a Adapter) readyTimeout() time.Duration {
	if a.ReadyTimeout > 0 {
		return a.ReadyTimeout
	}
	return defaultReadyTimeout()
}

func commandPID(proc startedProcess) int {
	cmd, ok := proc.(*cmdProcess)
	if !ok || cmd == nil || cmd.cmd == nil || cmd.cmd.Process == nil {
		return 0
	}
	return cmd.cmd.Process.Pid
}

func wrapStartError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrUACCancelled) {
		return ErrUACCancelled
	}
	return fmt.Errorf("start sing-box: %w", err)
}
