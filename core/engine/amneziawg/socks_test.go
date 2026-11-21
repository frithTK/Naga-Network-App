package amneziawg

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/amnezia-vpn/amneziawg-go/v3/conn"
	"github.com/amnezia-vpn/amneziawg-go/v3/device"
	"golang.org/x/net/proxy"
	wgnetstack "golang.zx2c4.com/wireguard/tun/netstack"

	awgcfg "naga.network/core/profile/amneziawg"
)

func TestLoopbackSidecarSOCKSConnect(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "pong")
	}))
	defer backend.Close()

	sidecar := NewLoopbackSidecar()
	if err := sidecar.Start(context.Background(), awgcfg.Config{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sidecar.Stop() })
	if err := sidecar.Ready(context.Background()); err != nil {
		t.Fatal(err)
	}

	dialer, err := proxy.SOCKS5("tcp", sidecar.ListenAddr(), nil, proxy.Direct)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, network, addr string) (net.Conn, error) {
				return dialer.Dial(network, addr)
			},
		},
	}
	resp, err := client.Get(backend.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "pong" {
		t.Fatalf("body = %q", body)
	}
}

func TestIpcRequestUsesHexKeysAndOmitsSecretsFromErrors(t *testing.T) {
	raw, err := os.ReadFile(profileTestdataPath(t, "awg31.valid.conf"))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := awgcfg.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	request, err := ipcRequest(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(request, cfg.PrivateKey) {
		t.Fatal("ipc request contains base64 private key")
	}
	if !strings.Contains(request, "jc=4") || !strings.Contains(request, "private_key=") {
		t.Fatalf("ipc request = %s", request)
	}

	_, err = ipcRequest(awgcfg.Config{PrivateKey: "not-a-valid-key"})
	if err == nil {
		t.Fatal("expected invalid key error")
	}
	if strings.Contains(err.Error(), "not-a-valid-key") {
		t.Fatalf("error leaked key material: %v", err)
	}
}

func TestIpcRequestIncludesS3AndHeaderProtection(t *testing.T) {
	raw, err := os.ReadFile(profileTestdataPath(t, "awg31.s3.conf"))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := awgcfg.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	request, err := ipcRequest(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"s3=10",
		"s4=16",
		"header_protection_key=",
		"content_padding_addition=10-64",
		"random_trailers=true",
		"disable_cookies=true",
	} {
		if !strings.Contains(request, want) {
			t.Fatalf("ipc missing %q: %s", want, request)
		}
	}
	if strings.Contains(request, cfg.HeaderProtectionKey) {
		t.Fatal("ipc leaked base64 header protection key")
	}
}

func TestAWGTimingRangesReachDeviceUAPI(t *testing.T) {
	raw, err := os.ReadFile(profileTestdataPath(t, "awg31.valid.conf"))
	if err != nil {
		t.Fatal(err)
	}
	input := strings.Replace(string(raw), "[Interface]", "[Interface]\nRekeyAfterTime = 20-30\nRekeyTimeout = 3\nRejectAfterTime = 90-120\nKeepaliveTimeout = 8-10\nMaxHandshakeAttempts = 5-9", 1)
	input = strings.Replace(input, "PersistentKeepalive = 25", "PersistentKeepalive = 22-30", 1)
	cfg, err := awgcfg.Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	request, err := ipcRequest(cfg)
	if err != nil {
		t.Fatal(err)
	}
	tun, _, err := wgnetstack.CreateNetTUN([]netip.Addr{netip.MustParseAddr("10.8.0.2")}, nil, 1280)
	if err != nil {
		t.Fatal(err)
	}
	dev := device.NewDevice(bridgeTUN{dev: tun}, conn.NewDefaultBind(), device.NewLogger(device.LogLevelSilent, ""))
	defer dev.Close()
	if err := dev.IpcSet(request); err != nil {
		t.Fatalf("device rejected generated configuration: %v", err)
	}
	dump, err := dev.IpcGet()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"rekey_after_time=20-30", "rekey_timeout=3", "reject_after_time=90-120", "keepalive_timeout=8-10", "max_handshake_attempts=5-9", "persistent_keepalive_interval=22-30"} {
		if !strings.Contains(dump, want+"\n") {
			t.Errorf("device UAPI missing %s", want)
		}
	}
}

func TestAWGRejectsInvalidTimingAndKeepaliveRanges(t *testing.T) {
	raw, err := os.ReadFile(profileTestdataPath(t, "awg31.valid.conf"))
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"", "30-20", "1-2-3", "-1", "0-4294967295", "secret-value"} {
		input := strings.Replace(string(raw), "[Interface]", "[Interface]\nRekeyAfterTime = "+value, 1)
		_, err := awgcfg.Parse([]byte(input))
		if err == nil {
			t.Errorf("invalid timing range accepted")
		} else if value == "secret-value" && strings.Contains(err.Error(), value) {
			t.Error("error exposed input")
		}
	}
	for _, value := range []string{"30-20", "22-65536", "1-2-3"} {
		input := strings.Replace(string(raw), "PersistentKeepalive = 25", "PersistentKeepalive = "+value, 1)
		if _, err := awgcfg.Parse([]byte(input)); err == nil {
			t.Errorf("invalid keepalive range accepted: %s", value)
		}
	}
}

func TestIpcRequestOmitsJ1ITimeUnknownToV3(t *testing.T) {
	raw, err := os.ReadFile(profileTestdataPath(t, "awg31.j1.conf"))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := awgcfg.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	request, err := ipcRequest(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(request, "i1=") {
		t.Fatalf("ipc missing i1: %s", request)
	}
	if strings.Contains(request, "j1=") || strings.Contains(request, "itime=") {
		t.Fatalf("v3 UAPI must not receive j1/itime: %s", request)
	}
	sidecar := NewNetstackSidecar()
	if err := sidecar.Start(context.Background(), cfg); err == nil || !strings.Contains(err.Error(), "J1") {
		t.Fatal("runtime must reject unsupported fields instead of silently dropping them")
	}
}

func TestLoopbackSidecarUDPAssociate(t *testing.T) {
	echo, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echo.Close()
	silent, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer silent.Close()
	go func() {
		buf := make([]byte, 2048)
		for {
			n, addr, err := echo.ReadFrom(buf)
			if err != nil {
				return
			}
			_, _ = echo.WriteTo(buf[:n], addr)
			_, _ = echo.WriteTo([]byte("second-response"), addr)
		}
	}()

	sidecar := NewLoopbackSidecar()
	if err := sidecar.Start(context.Background(), awgcfg.Config{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sidecar.Stop() })
	if err := sidecar.Ready(context.Background()); err != nil {
		t.Fatal(err)
	}

	tcpConn, err := net.Dial("tcp", sidecar.ListenAddr())
	if err != nil {
		t.Fatal(err)
	}
	defer tcpConn.Close()
	if _, err := tcpConn.Write([]byte{5, 1, 0}); err != nil {
		t.Fatal(err)
	}
	greeting := make([]byte, 2)
	if _, err := io.ReadFull(tcpConn, greeting); err != nil {
		t.Fatal(err)
	}
	echoAddr := echo.LocalAddr().(*net.UDPAddr)
	req := []byte{5, 3, 0, 1, 0, 0, 0, 0, 0, 0}
	if _, err := tcpConn.Write(req); err != nil {
		t.Fatal(err)
	}
	reply := make([]byte, 10)
	if _, err := io.ReadFull(tcpConn, reply); err != nil {
		t.Fatal(err)
	}
	if reply[0] != 5 || reply[1] != 0 {
		t.Fatalf("associate reply = %v", reply)
	}
	bindPort := int(reply[8])<<8 | int(reply[9])
	udpConn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer udpConn.Close()
	relayAddr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: bindPort}
	silentAddr := silent.LocalAddr().(*net.UDPAddr)
	if _, err := udpConn.WriteTo(encodeSOCKSUDP(silentAddr.IP.String(), silentAddr.Port, []byte("no-response")), relayAddr); err != nil {
		t.Fatal(err)
	}
	payload := []byte("ping-awg")
	packet := encodeSOCKSUDP(echoAddr.IP.String(), echoAddr.Port, payload)
	if _, err := udpConn.WriteTo(packet, relayAddr); err != nil {
		t.Fatal(err)
	}
	_ = udpConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 2048)
	for _, expected := range []string{"ping-awg", "second-response"} {
		n, _, err := udpConn.ReadFrom(buf)
		if err != nil {
			t.Fatal(err)
		}
		_, _, data, ok := parseSOCKSUDP(buf[:n])
		if !ok || string(data) != expected {
			t.Fatalf("udp echo = %q ok=%v", data, ok)
		}
	}
}

func profileTestdataPath(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "profile", "amneziawg", "testdata", name)
}
