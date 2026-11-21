package amneziawg

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/amnezia-vpn/amneziawg-go/v3/conn"
	"github.com/amnezia-vpn/amneziawg-go/v3/device"
	"golang.org/x/net/proxy"
	wgnetstack "golang.zx2c4.com/wireguard/tun/netstack"
	awgcfg "naga.network/core/profile/amneziawg"
)

func readNetstackFixture(t *testing.T) awgcfg.Config {
	t.Helper()
	raw, err := os.ReadFile(profileTestdataPath(t, "awg31.valid.conf"))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := awgcfg.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Peers[0].Endpoint = "127.0.0.1:1"
	cfg.Peers[0].PersistentKeepalive = 0
	return cfg
}

func TestNetstackCanRestartAfterDeviceConfigurationError(t *testing.T) {
	cfg := readNetstackFixture(t)
	sidecar := NewNetstackSidecar()
	defer sidecar.Stop()
	cfg.Peers[0].AllowedIPs = []string{"invalid prefix"}
	if err := sidecar.Start(t.Context(), cfg); !errors.Is(err, ErrDeviceConfiguration) {
		t.Fatalf("Start with invalid prefix = %v", err)
	}
	if sidecar.ListenAddr() != "" {
		t.Fatal("failed sidecar published a SOCKS address")
	}
	cfg.Peers[0].AllowedIPs = []string{"10.8.0.0/24"}
	if err := sidecar.Start(t.Context(), cfg); err != nil {
		t.Fatalf("Start after failed configuration: %v", err)
	}
	ctx, cancel := context.WithTimeoutCause(t.Context(), 100*time.Millisecond, ErrHandshakeTimeout)
	defer cancel()
	if err := sidecar.Ready(ctx); !errors.Is(err, ErrHandshakeTimeout) {
		t.Fatalf("Ready without a peer or keepalive = %v", err)
	}
}

func TestNetstackSendsHTTPViaLocalPeerWithHostnameAndNoKeepalive(t *testing.T) {
	cfg := readNetstackFixture(t)
	clientPrivate, clientPublic := testKeyPair(t)
	serverPrivate, serverPublic := testKeyPair(t)
	tun, network, err := wgnetstack.CreateNetTUN([]netip.Addr{netip.MustParseAddr("10.8.0.1")}, nil, 1280)
	if err != nil {
		t.Fatal(err)
	}
	server := device.NewDevice(bridgeTUN{dev: tun}, conn.NewDefaultBind(), device.NewLogger(device.LogLevelSilent, ""))
	defer server.Close()
	serverCfg := cfg
	serverCfg.PrivateKey = serverPrivate
	serverCfg.Peers = []awgcfg.Peer{{PublicKey: clientPublic, AllowedIPs: []string{"10.8.0.2/32"}}}
	uapi, err := ipcRequest(serverCfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := server.IpcSet(uapi); err != nil {
		t.Fatal(err)
	}
	if err := server.Up(); err != nil {
		t.Fatal(err)
	}
	dump, err := server.IpcGet()
	if err != nil {
		t.Fatal(err)
	}
	_, portAndConfig, ok := strings.Cut(dump, "\nlisten_port=")
	if !ok {
		t.Fatal("server did not report its port")
	}
	port, _, _ := strings.Cut(portAndConfig, "\n")
	listener, err := network.ListenTCP(&net.TCPAddr{IP: net.IPv4(10, 8, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "through-awg")
	})}
	defer httpServer.Close()
	go httpServer.Serve(listener)

	cfg.PrivateKey = clientPrivate
	cfg.Peers = []awgcfg.Peer{{PublicKey: serverPublic, Endpoint: net.JoinHostPort("localhost", port), AllowedIPs: []string{"10.8.0.1/32"}}}
	sidecar := NewNetstackSidecar()
	defer sidecar.Stop()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := sidecar.Start(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	if err := sidecar.Ready(ctx); err != nil {
		t.Fatal(err)
	}
	dialer, err := proxy.SOCKS5("tcp", sidecar.ListenAddr(), nil, proxy.Direct)
	if err != nil {
		t.Fatal(err)
	}
	transport := &http.Transport{DialContext: dialer.(proxy.ContextDialer).DialContext}
	defer transport.CloseIdleConnections()
	client := http.Client{Transport: transport, Timeout: 3 * time.Second}
	response, err := client.Get("http://" + listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || string(body) != "through-awg" {
		t.Fatalf("tunneled data = %q, %v", body, err)
	}
	if !strings.HasPrefix(cfg.Peers[0].Endpoint, "localhost:") {
		t.Fatal("Start modified the saved endpoint")
	}
}

func testKeyPair(t *testing.T) (private, public string) {
	t.Helper()
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(key.Bytes()), base64.StdEncoding.EncodeToString(key.PublicKey().Bytes())
}

type fixedEndpointResolver []netip.Addr

func (r fixedEndpointResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return r, nil
}

func TestEndpointResolutionKeepsAlternatesAndOriginalProfile(t *testing.T) {
	cfg := readNetstackFixture(t)
	cfg.Peers[0].Endpoint = "awg.example.invalid:51820"
	resolved, err := resolveEndpoints(t.Context(), cfg, fixedEndpointResolver{
		netip.MustParseAddr("2001:db8::2"),
		netip.MustParseAddr("2001:db8::2"),
		netip.MustParseAddr("192.0.2.2"),
		netip.MustParseAddr("192.0.2.3"),
	})
	if err != nil || len(resolved) != 2 {
		t.Fatalf("resolve returned %d choices: %v", len(resolved), err)
	}
	if resolved[0].Peers[0].Endpoint != "[2001:db8::2]:51820" ||
		resolved[1].Peers[0].Endpoint != "192.0.2.2:51820" || cfg.Peers[0].Endpoint != "awg.example.invalid:51820" {
		t.Fatal("resolved endpoints did not preserve address order or original profile")
	}
}

func TestConflictingAWGBindDoesNotReplaceRunningSidecar(t *testing.T) {
	cfg := readNetstackFixture(t)
	first, second := NewNetstackSidecar(), NewNetstackSidecar()
	defer first.Stop()
	defer second.Stop()
	if err := first.Start(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	if err := second.Start(t.Context(), cfg); !errors.Is(err, ErrBindingInUse) {
		t.Fatalf("overlapping AWG keys: %v", err)
	}
	key, err := base64.StdEncoding.DecodeString(cfg.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	key[0] ^= 1 // X25519 masks this bit when deriving the public key.
	cfg.PrivateKey = base64.StdEncoding.EncodeToString(key)
	if err := second.Start(t.Context(), cfg); !errors.Is(err, ErrBindingInUse) {
		t.Fatalf("equivalent AWG keys: %v", err)
	}
	if first.ListenAddr() == "" || second.ListenAddr() != "" {
		t.Fatal("conflicting Start changed the running SOCKS listener")
	}
	if err := first.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := second.Start(t.Context(), cfg); err != nil {
		t.Fatalf("AWG binding was not released: %v", err)
	}
}
