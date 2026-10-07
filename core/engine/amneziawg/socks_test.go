package amneziawg

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/proxy"

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
}

func TestLoopbackSidecarUDPAssociate(t *testing.T) {
	echo, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echo.Close()
	go func() {
		buf := make([]byte, 2048)
		for {
			n, addr, err := echo.ReadFrom(buf)
			if err != nil {
				return
			}
			_, _ = echo.WriteTo(buf[:n], addr)
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
	payload := []byte("ping-awg")
	packet := encodeSOCKSUDP(echoAddr.IP.String(), echoAddr.Port, payload)
	if _, err := udpConn.WriteTo(packet, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: bindPort}); err != nil {
		t.Fatal(err)
	}
	_ = udpConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 2048)
	n, _, err := udpConn.ReadFrom(buf)
	if err != nil {
		t.Fatal(err)
	}
	_, _, data, ok := parseSOCKSUDP(buf[:n])
	if !ok || string(data) != "ping-awg" {
		t.Fatalf("udp echo = %q ok=%v", data, ok)
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
