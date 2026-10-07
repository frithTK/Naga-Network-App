package olcrtc

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

var executablePath = os.Executable

const (
	ExitOther     = "other"
	ExitBadKey    = "bad_key"
	ExitEmptyRoom = "empty_room"
	readyTimeout  = 8 * time.Second
	PinnedCommit  = "7f849e08"
)

var ErrBinaryNotFound = errors.New("olcrtc binary was not found")
var ErrVersion = errors.New("olcrtc binary version does not match the pinned commit")

// Command is the running olcrtc process. Tests substitute a loopback double.
type Command struct {
	cmd    *exec.Cmd
	reason string
	done   chan struct{}
	lines  []string
	onLine func(string)
	mu     sync.Mutex
}

// Start launches olcrtc with the YAML path as its only argument.
func Start(ctx context.Context, binary, yamlPath string) (*Command, error) {
	if strings.TrimSpace(binary) == "" {
		return nil, ErrBinaryNotFound
	}
	if err := CheckVersion(ctx, binary); err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, binary, yamlPath)
	configureCommand(cmd)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	running := &Command{cmd: cmd, reason: ExitOther, done: make(chan struct{})}
	go running.watch(stderr)
	return running, nil
}

func (c *Command) note(line string) {
	c.mu.Lock()
	c.lines = append(c.lines, line)
	if len(c.lines) > 32 {
		c.lines = c.lines[len(c.lines)-32:]
	}
	fn := c.onLine
	c.mu.Unlock()
	if fn != nil {
		fn(line)
	}
}

// SetLineHandler receives each stderr line. The caller redacts secrets.
func (c *Command) SetLineHandler(fn func(string)) {
	c.mu.Lock()
	c.onLine = fn
	c.mu.Unlock()
}

const (
	markerEmptyRoom = "empty room"
	markerBadKey    = "key does not match the peer"
)

func classifyLine(line, current string) string {
	switch {
	case strings.Contains(line, markerBadKey):
		return ExitBadKey
	case strings.Contains(line, markerEmptyRoom):
		return ExitEmptyRoom
	default:
		return current
	}
}

func (c *Command) watch(stderr io.Reader) {
	defer close(c.done)
	scanner := bufio.NewScanner(stderr)
	scanner.Buffer(make([]byte, 0, 64*1024), 64*1024)
	for scanner.Scan() {
		line := scanner.Text()
		c.note(line)
		c.mu.Lock()
		c.reason = classifyLine(line, c.reason)
		c.mu.Unlock()
	}
	_ = c.cmd.Wait()
}

// Ready waits until 127.0.0.1:10808 accepts a connection.
func (c *Command) Ready(ctx context.Context) error {
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), readyTimeout)
		defer cancel()
	}
	dialer := net.Dialer{}
	var last error
	for {
		conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort("127.0.0.1", "10808"))
		if err == nil {
			_ = conn.Close()
			return nil
		}
		last = err
		select {
		case <-ctx.Done():
			if last != nil {
				return last
			}
			return ctx.Err()
		case <-c.done:
			return errors.New("olcrtc exited before the socks port was ready")
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// Stop ends the process and waits for it to leave.
func (c *Command) Stop() error {
	if c == nil || c.cmd == nil || c.cmd.Process == nil {
		return nil
	}
	if runtime.GOOS == "windows" {
		_ = c.cmd.Process.Kill()
	} else {
		_ = c.cmd.Process.Signal(os.Interrupt)
		select {
		case <-c.done:
			return nil
		case <-time.After(2 * time.Second):
			_ = c.cmd.Process.Kill()
		}
	}
	select {
	case <-c.done:
	case <-time.After(2 * time.Second):
	}
	return nil
}

// ExitReason is other until exit markers are confirmed against the pinned binary.
func (c *Command) ExitReason() string {
	if c == nil {
		return ExitOther
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.reason == "" {
		return ExitOther
	}
	return c.reason
}

// CheckVersion reads olcrtc.version beside the binary. The upstream CLI has
// no version subcommand: its only argument is a YAML path.
func CheckVersion(ctx context.Context, binary string) error {
	_ = ctx
	if strings.TrimSpace(binary) == "" {
		return ErrBinaryNotFound
	}
	path := filepath.Join(filepath.Dir(binary), "olcrtc.version")
	data, err := os.ReadFile(path)
	text := strings.TrimSpace(string(data))
	if err != nil || !strings.Contains(text, PinnedCommit) {
		return ErrVersion
	}
	return nil
}

// BinaryReady is true when the pinned olcrtc binary sits next to the
// control-plane (or on PATH) and olcrtc.version matches PinnedCommit.
func BinaryReady(explicit string) bool {
	path, err := DiscoverBinary(explicit)
	if err != nil {
		return false
	}
	return CheckVersion(context.Background(), path) == nil
}

func binaryNames() []string {
	if runtime.GOOS == "windows" {
		return []string{"olcrtc.exe", "olcrtc"}
	}
	return []string{"olcrtc"}
}

func siblingBinary() string {
	exe, err := executablePath()
	if err != nil {
		return ""
	}
	dir := filepath.Dir(exe)
	for _, name := range binaryNames() {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// DiscoverBinary looks next to the current executable, then on PATH.
func DiscoverBinary(explicit string) (string, error) {
	if value := strings.TrimSpace(explicit); value != "" {
		if _, err := os.Stat(value); err != nil {
			return "", ErrBinaryNotFound
		}
		return value, nil
	}
	if value := strings.TrimSpace(os.Getenv("NAGA_OLCRTC_PATH")); value != "" {
		if _, err := os.Stat(value); err != nil {
			return "", ErrBinaryNotFound
		}
		return value, nil
	}
	if sibling := siblingBinary(); sibling != "" {
		return sibling, nil
	}
	for _, name := range binaryNames() {
		path, err := exec.LookPath(name)
		if err == nil {
			return path, nil
		}
	}
	return "", ErrBinaryNotFound
}
