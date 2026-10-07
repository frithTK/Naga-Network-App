package amneziawg

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/amnezia-vpn/amneziawg-go/v3/conn"
	"github.com/amnezia-vpn/amneziawg-go/v3/device"
	awgtun "github.com/amnezia-vpn/amneziawg-go/v3/tun"
	wgtun "golang.zx2c4.com/wireguard/tun"
	wgnetstack "golang.zx2c4.com/wireguard/tun/netstack"

	awgcfg "naga.network/core/profile/amneziawg"
)

type netstackSidecar struct {
	mu        sync.Mutex
	dev       *device.Device
	tnet      *wgnetstack.Net
	server    *socksServer
	addr      string
	keepalive bool
}

func NewNetstackSidecar() Sidecar {
	return &netstackSidecar{}
}

func (s *netstackSidecar) Start(_ context.Context, cfg awgcfg.Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dev != nil {
		return fmt.Errorf("amneziawg sidecar already started")
	}
	addrs, err := interfaceAddrs(cfg)
	if err != nil {
		return err
	}
	dns := dnsAddrs(cfg)
	mtu := cfg.MTU
	if mtu <= 0 {
		mtu = 1280
	}
	tun, tnet, err := wgnetstack.CreateNetTUN(addrs, dns, mtu)
	if err != nil {
		return fmt.Errorf("amneziawg netstack: %w", err)
	}
	dev := device.NewDevice(bridgeTUN{dev: tun}, conn.NewDefaultBind(), device.NewLogger(device.LogLevelError, "awg "))
	uapi, err := ipcRequest(cfg)
	if err != nil {
		dev.Close()
		_ = tun.Close()
		return err
	}
	if err := dev.IpcSet(uapi); err != nil {
		dev.Close()
		_ = tun.Close()
		return fmt.Errorf("amneziawg configure: %w", err)
	}
	if err := dev.Up(); err != nil {
		dev.Close()
		_ = tun.Close()
		return fmt.Errorf("amneziawg start: %w", err)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		dev.Close()
		return err
	}
	s.dev = dev
	s.tnet = tnet
	s.server = serveSOCKS5(listener, tnet.Dial)
	s.addr = s.server.Addr()
	s.keepalive = peerHasKeepalive(cfg)
	return nil
}

func (s *netstackSidecar) ListenAddr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.addr
}

func (s *netstackSidecar) Ready(ctx context.Context) error {
	s.mu.Lock()
	addr := s.addr
	keepalive := s.keepalive
	s.mu.Unlock()
	if addr == "" {
		return ErrNotReady
	}
	if err := waitSOCKS(ctx, addr); err != nil {
		return err
	}
	if keepalive {
		return s.waitHandshake(ctx)
	}
	s.kick(ctx)
	_, _ = s.handshakeEstablished()
	return nil
}

func (s *netstackSidecar) waitHandshake(ctx context.Context) error {
	deadline := time.Now().Add(handshakeWaitTimeout)
	if ready, ok := ctx.Deadline(); ok && ready.Before(deadline) {
		deadline = ready
	}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		ok, err := s.handshakeEstablished()
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		if time.Now().After(deadline) {
			return ErrHandshakeTimeout
		}
		select {
		case <-ctx.Done():
			return ErrHandshakeTimeout
		case <-ticker.C:
		}
	}
}

func (s *netstackSidecar) handshakeEstablished() (bool, error) {
	s.mu.Lock()
	dev := s.dev
	s.mu.Unlock()
	if dev == nil {
		return false, ErrStopped
	}
	dump, err := dev.IpcGet()
	if err != nil {
		return false, nil
	}
	return lastHandshakeUnix(dump) > 0, nil
}

func (s *netstackSidecar) kick(ctx context.Context) {
	s.mu.Lock()
	tnet := s.tnet
	s.mu.Unlock()
	if tnet == nil {
		return
	}
	dctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	conn, err := tnet.DialContext(dctx, "tcp", "1.1.1.1:443")
	if err == nil {
		_ = conn.Close()
	}
}

func lastHandshakeUnix(dump string) int64 {
	for _, line := range strings.Split(dump, "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if strings.TrimSpace(key) != "last_handshake_time_sec" {
			continue
		}
		n, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		if err != nil {
			return 0
		}
		return n
	}
	return 0
}

func peerHasKeepalive(cfg awgcfg.Config) bool {
	for _, peer := range cfg.Peers {
		if peer.PersistentKeepalive > 0 {
			return true
		}
	}
	return false
}

func (s *netstackSidecar) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result error
	if s.server != nil {
		result = s.server.Close()
		s.server = nil
	}
	if s.dev != nil {
		s.dev.Close()
		s.dev = nil
	}
	s.tnet = nil
	s.addr = ""
	return result
}

// bridgeTUN adapts wireguard-go's userspace TUN to amneziawg-go's Device.
type bridgeTUN struct {
	dev wgtun.Device
}

func (t bridgeTUN) File() *os.File { return t.dev.File() }

func (t bridgeTUN) Read(bufs [][]byte, sizes []int, offset int) (int, error) {
	return t.dev.Read(bufs, sizes, offset)
}

func (t bridgeTUN) Write(bufs [][]byte, offset int) (int, error) {
	return t.dev.Write(bufs, offset)
}

func (t bridgeTUN) MTU() (int, error) { return t.dev.MTU() }

func (t bridgeTUN) Name() (string, error) { return t.dev.Name() }

func (t bridgeTUN) Close() error { return t.dev.Close() }

func (t bridgeTUN) BatchSize() int { return t.dev.BatchSize() }

func (t bridgeTUN) Events() <-chan awgtun.Event {
	in := t.dev.Events()
	out := make(chan awgtun.Event)
	go func() {
		defer close(out)
		for ev := range in {
			out <- awgtun.Event(ev)
		}
	}()
	return out
}

func interfaceAddrs(cfg awgcfg.Config) ([]netip.Addr, error) {
	out := make([]netip.Addr, 0, len(cfg.Addresses))
	for _, raw := range cfg.Addresses {
		prefix, err := netip.ParsePrefix(raw)
		if err != nil {
			addr, addrErr := netip.ParseAddr(raw)
			if addrErr != nil {
				return nil, fmt.Errorf("amneziawg interface address is invalid")
			}
			out = append(out, addr)
			continue
		}
		out = append(out, prefix.Addr())
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("amneziawg interface address is invalid")
	}
	return out, nil
}

func dnsAddrs(cfg awgcfg.Config) []netip.Addr {
	out := make([]netip.Addr, 0, len(cfg.DNS))
	for _, raw := range cfg.DNS {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		addr, err := netip.ParseAddr(raw)
		if err != nil {
			continue
		}
		out = append(out, addr)
	}
	return out
}
