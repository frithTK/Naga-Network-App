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
	mu     sync.Mutex
	tunFD  *os.File
	hevCmd *exec.Cmd
)

func SetTunFD(fd int) error {
	if fd < 0 {
		return fmt.Errorf("invalid tun fd")
	}
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
	mu.Lock()
	defer mu.Unlock()
	stopHevLocked()
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

func StartHev(path, configPath string) error {
	mu.Lock()
	defer mu.Unlock()
	if tunFD == nil {
		return fmt.Errorf("VPN interface is not ready")
	}
	stopHevLocked()
	cmd := exec.Command(path, configPath)
	cmd.ExtraFiles = []*os.File{tunFD}
	cmd.SysProcAttr = hevProcAttr()
	if err := cmd.Start(); err != nil {
		return err
	}
	hevCmd = cmd
	go func() { _, _ = cmd.Process.Wait() }()
	return nil
}

func StopHev() {
	mu.Lock()
	defer mu.Unlock()
	stopHevLocked()
}

func stopHevLocked() {
	if hevCmd == nil || hevCmd.Process == nil {
		hevCmd = nil
		return
	}
	_ = hevCmd.Process.Kill()
	_, _ = hevCmd.Process.Wait()
	hevCmd = nil
}
