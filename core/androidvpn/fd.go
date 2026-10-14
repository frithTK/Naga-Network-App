package androidvpn

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
)

const (
	SingBoxIPv4 = "172.19.0.1"
	SingBoxPref = 30
	OlcIPv4     = "198.18.0.1"
	OlcPref     = 32
	MTU         = 1500
	DNS         = "1.1.1.1"
	SocksHost   = "127.0.0.1"
	SingBoxPort = 2080
	OlcPort     = 10808
)

var (
	opMu      sync.Mutex
	mu        sync.Mutex
	tunFD     *os.File
	hevBridge *Bridge
)

func SetTunFD(fd int) error {
	if fd < 0 {
		return fmt.Errorf("invalid tun fd")
	}
	opMu.Lock()
	defer opMu.Unlock()
	stopHev()
	mu.Lock()
	defer mu.Unlock()
	if tunFD != nil {
		_ = tunFD.Close()
		tunFD = nil
	}
	tunFD = os.NewFile(uintptr(fd), "vpn-tun")
	if tunFD == nil {
		return fmt.Errorf("tun fd is not usable")
	}
	return nil
}

func TunFile() *os.File {
	mu.Lock()
	defer mu.Unlock()
	return tunFD
}

func CloseTun() {
	opMu.Lock()
	defer opMu.Unlock()
	stopHev()
	mu.Lock()
	defer mu.Unlock()
	if tunFD != nil {
		_ = tunFD.Close()
		tunFD = nil
	}
}

func FDConfigYAML(mtu int, ipv4, socksHost string, socksPort int) string {
	if mtu <= 0 {
		mtu = MTU
	}
	if ipv4 == "" {
		ipv4 = SingBoxIPv4
	}
	if socksHost == "" {
		socksHost = SocksHost
	}
	if socksPort <= 0 {
		socksPort = SingBoxPort
	}
	return fmt.Sprintf(`tunnel:
  mtu: %d
  ipv4: %s
socks5:
  port: %d
  address: %s
  udp: 'udp'
misc:
  log-level: warn
`, mtu, ipv4, socksPort, socksHost)
}

// Bridge owns the single Wait call for one subprocess and its inherited TUN.
// Completion from a replaced session can never close a new session's FD.
type Bridge struct {
	cmd      *exec.Cmd
	tun      *os.File
	done     chan struct{}
	expected bool
	failure  error
}

func (b *Bridge) Done() <-chan struct{} { return b.done }
func (b *Bridge) Failure() error {
	mu.Lock()
	defer mu.Unlock()
	return b.failure
}
func CurrentBridge() *Bridge {
	mu.Lock()
	defer mu.Unlock()
	return hevBridge
}

func StartHev(path, configPath string) error {
	opMu.Lock()
	defer opMu.Unlock()
	stopHev()
	mu.Lock()
	defer mu.Unlock()
	if tunFD == nil {
		return fmt.Errorf("VPN interface is not ready")
	}
	cmd := exec.Command(path, configPath)
	cmd.ExtraFiles = []*os.File{tunFD}
	cmd.SysProcAttr = hevProcAttr()
	if err := cmd.Start(); err != nil {
		return err
	}
	bridge := &Bridge{cmd: cmd, tun: tunFD, done: make(chan struct{})}
	hevBridge = bridge
	go func() {
		err := cmd.Wait()
		mu.Lock()
		defer mu.Unlock()
		if !bridge.expected {
			if err == nil {
				err = fmt.Errorf("bridge exited unexpectedly")
			}
			bridge.failure = fmt.Errorf("VPN packet bridge failed: %w", err)
			if hevBridge == bridge && tunFD == bridge.tun {
				_ = tunFD.Close()
				tunFD = nil
			}
		}
		close(bridge.done)
	}()
	return nil
}

func StopHev() {
	opMu.Lock()
	defer opMu.Unlock()
	stopHev()
}

// Called with opMu held. Waiting never holds mu: the Wait owner needs it to
// publish the exit and release the old TUN before a replacement is installed.
func stopHev() {
	mu.Lock()
	bridge := hevBridge
	if bridge == nil {
		mu.Unlock()
		return
	}
	bridge.expected = true
	mu.Unlock()
	_ = bridge.cmd.Process.Kill()
	<-bridge.done
	mu.Lock()
	if hevBridge == bridge {
		hevBridge = nil
	}
	mu.Unlock()
}
