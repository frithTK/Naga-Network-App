package amneziawg

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/amnezia-vpn/amneziawg-go/v3/device"
	awgtun "github.com/amnezia-vpn/amneziawg-go/v3/tun"
	wgtun "golang.zx2c4.com/wireguard/tun"
	wgnetstack "golang.zx2c4.com/wireguard/tun/netstack"

	awgcfg "naga.network/core/profile/amneziawg"
)

type netstackSidecar struct {
	mu         sync.Mutex
	dev        *device.Device
	tnet       *wgnetstack.Net
	server     *socksServer
	addr       string
	publicKeys []device.NoisePublicKey
	dnsRelay   string
	release    func()
	stopWatch  context.CancelFunc
	resolver   endpointResolver
	alternates []awgcfg.Config
}

func NewNetstackSidecar() Sidecar {
	return &netstackSidecar{}
}

func (s *netstackSidecar) Start(ctx context.Context, cfg awgcfg.Config) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := awgcfg.Validate(cfg); err != nil {
		return err
	}
	if err := awgcfg.ValidateRuntime(cfg); err != nil {
		return err
	}
	resolveCtx, cancelResolve := context.WithTimeoutCause(ctx, endpointResolveTimeout, ErrEndpointDNS)
	resolver := s.resolver
	if resolver == nil {
		resolver = platformEndpointResolver()
	}
	resolved, err := resolveEndpoints(resolveCtx, cfg, resolver)
	cancelResolve()
	if err != nil {
		return err
	}
	return s.startResolved(ctx, resolved, "")
}

func (s *netstackSidecar) startResolved(ctx context.Context, choices []awgcfg.Config, existingListener string) error {
	var lastErr error
	for i, cfg := range choices {
		if err := s.startDevice(ctx, cfg, choices[i+1:], existingListener); err != nil {
			lastErr = err
			if errors.Is(err, ErrUplinkUnavailable) {
				continue
			}
			return err
		}
		return nil
	}
	return lastErr
}

func (s *netstackSidecar) startDevice(ctx context.Context, resolved awgcfg.Config, alternates []awgcfg.Config, existingListener string) error {
	bind, err := newPlatformBind(ctx, resolved)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dev != nil {
		return fmt.Errorf("amneziawg sidecar already started")
	}
	if existingListener != "" && s.addr != existingListener {
		return ErrStopped
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	uapi, err := ipcRequest(resolved)
	if err != nil {
		return err
	}
	publicKeys := make([]device.NoisePublicKey, 0, len(resolved.Peers))
	for _, peer := range resolved.Peers {
		key, err := keyToHex(peer.PublicKey)
		if err != nil {
			return ErrDeviceConfiguration
		}
		var publicKey device.NoisePublicKey
		if _, err := hex.Decode(publicKey[:], []byte(key)); err != nil {
			return ErrDeviceConfiguration
		}
		publicKeys = append(publicKeys, publicKey)
	}
	addrs, err := interfaceAddrs(resolved)
	if err != nil {
		return err
	}
	dns := dnsAddrs(resolved)
	if len(dns) == 0 {
		dns = append(dns, netip.MustParseAddr("1.1.1.1"))
	}
	mtu := resolved.MTU
	if mtu <= 0 {
		mtu = 1280
	}
	release, err := reserveBinding(resolved)
	if err != nil {
		return err
	}
	started := false
	defer func() {
		if !started {
			release()
		}
	}()
	tun, tnet, err := wgnetstack.CreateNetTUN(addrs, dns, mtu)
	if err != nil {
		return fmt.Errorf("amneziawg netstack: %w", err)
	}
	// The device owns tun from here. Its Close also closes the underlying TUN.
	dev := device.NewDevice(bridgeTUN{dev: tun}, bind, device.NewLogger(device.LogLevelSilent, ""))
	defer func() {
		if !started {
			dev.Close()
		}
	}()
	if err := dev.IpcSet(uapi); err != nil {
		return ErrDeviceConfiguration
	}
	if err := dev.Up(); err != nil {
		return fmt.Errorf("amneziawg start: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.prepareLocked(ctx); err != nil {
		return err
	}
	s.dev = dev
	s.tnet = tnet
	s.publicKeys = publicKeys
	s.alternates = alternates
	s.dnsRelay = netip.AddrPortFrom(dns[0], 53).String()
	s.release = release
	if monitor, ok := bind.(interface {
		watchUplink(context.Context, func() error, func() error)
	}); ok {
		watch, cancel := context.WithCancel(context.Background())
		s.stopWatch = cancel
		go monitor.watchUplink(watch, dev.BindUpdate, dev.BindClose)
	}
	started = true
	return nil
}

func (s *netstackSidecar) Prepare(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.prepareLocked(ctx)
}

func (s *netstackSidecar) prepareLocked(ctx context.Context) error {
	if s.server != nil {
		return nil
	}
	var listenerConfig net.ListenConfig
	listener, err := listenerConfig.Listen(ctx, "tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		_ = listener.Close()
		return err
	}
	s.server = serveSOCKS5(listener, s.dial)
	s.addr = s.server.Addr()
	return nil
}

func (s *netstackSidecar) dial(ctx context.Context, network, addr string) (net.Conn, error) {
	s.mu.Lock()
	tnet := s.tnet
	if network == "udp" && addr == awgcfg.DNSRelayAddr {
		addr = s.dnsRelay
	}
	s.mu.Unlock()
	if tnet == nil {
		return nil, ErrNotReady
	}
	return tnet.DialContext(ctx, network, addr)
}

func (s *netstackSidecar) ListenAddr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.addr
}

func (s *netstackSidecar) Ready(ctx context.Context) error {
	if err := context.Cause(ctx); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeoutCause(ctx, handshakeWaitTimeout, ErrHandshakeTimeout)
	defer cancel()
	for {
		s.mu.Lock()
		addr, dev, alternates := s.addr, s.dev, s.alternates
		s.mu.Unlock()
		if dev == nil || addr == "" {
			return ErrNotReady
		}
		attempt := ctx
		stopAttempt := func() {}
		if len(alternates) > 0 {
			deadline, _ := ctx.Deadline()
			attempt, stopAttempt = context.WithTimeoutCause(ctx, time.Until(deadline)/time.Duration(len(alternates)+1), ErrHandshakeTimeout)
		}
		err := s.waitHandshake(attempt, dev, addr)
		stopAttempt()
		if !errors.Is(err, ErrHandshakeTimeout) || len(alternates) == 0 || ctx.Err() != nil {
			return err
		}
		s.mu.Lock()
		if s.dev != dev {
			s.mu.Unlock()
			return ErrStopped
		}
		s.deactivateLocked()
		s.mu.Unlock()
		if err := s.startResolved(ctx, alternates, addr); err != nil {
			return err
		}
	}
}

func (s *netstackSidecar) waitHandshake(ctx context.Context, dev *device.Device, addr string) error {
	s.mu.Lock()
	if s.dev != dev {
		s.mu.Unlock()
		return ErrStopped
	}
	for _, key := range s.publicKeys {
		if peer := dev.LookupPeer(key); peer != nil {
			// Queue a protocol keepalive to trigger the ordinary handshake
			// timers without sending a test TCP request to an unrelated IP.
			peer.SendKeepalive()
		}
	}
	s.mu.Unlock()
	if err := waitSOCKS(ctx, addr); err != nil {
		if ctx.Err() != nil {
			return context.Cause(ctx)
		}
		return err
	}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := context.Cause(ctx); err != nil {
			return err
		}
		ok, err := s.handshakeEstablished(dev)
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-ticker.C:
		}
	}
}

func (s *netstackSidecar) handshakeEstablished(dev *device.Device) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dev != dev {
		return false, ErrStopped
	}
	dump, err := dev.IpcGet()
	if err != nil {
		return false, ErrDeviceConfiguration
	}
	return lastHandshakeUnix(dump) > 0, nil
}

func lastHandshakeUnix(dump string) int64 {
	var latest int64
	for _, line := range strings.Split(dump, "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if strings.TrimSpace(key) != "last_handshake_time_sec" {
			continue
		}
		n, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		if err == nil && n > latest {
			latest = n
		}
	}
	return latest
}

func (s *netstackSidecar) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result error
	if s.server != nil {
		result = s.server.Close()
		s.server = nil
	}
	s.deactivateLocked()
	s.addr = ""
	return result
}

func (s *netstackSidecar) Deactivate() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deactivateLocked()
}

func (s *netstackSidecar) deactivateLocked() {
	if s.stopWatch != nil {
		s.stopWatch()
		s.stopWatch = nil
	}
	if s.dev != nil {
		s.dev.Close()
		s.dev = nil
	}
	if s.release != nil {
		s.release()
		s.release = nil
	}
	s.tnet = nil
	s.dnsRelay = ""
	s.publicKeys = nil
	s.alternates = nil
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
		host, _, err := net.SplitHostPort(raw)
		if err == nil {
			raw = host
		}
		addr, err := netip.ParseAddr(raw)
		if err != nil {
			continue
		}
		out = append(out, addr)
	}
	return out
}
