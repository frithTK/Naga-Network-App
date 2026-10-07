package olcrtc

import (
	"runtime"
	"strings"
	"testing"
)

func TestParseWindowsDefaultRoute(t *testing.T) {
	text := `
===========================================================================
Interface List
 12...aa bb cc ........ Intel(R) Wi-Fi
===========================================================================
IPv4 Route Table
===========================================================================
Active Routes:
Network Destination        Netmask          Gateway       Interface  Metric
          0.0.0.0          0.0.0.0      192.168.1.1     192.168.1.10     25
        127.0.0.0        255.0.0.0         On-link         127.0.0.1    331
`
	route, err := ParseWindowsDefaultRoute(text)
	if err != nil {
		t.Fatal(err)
	}
	if route.Gateway != "192.168.1.1" || route.Device != "192.168.1.10" {
		t.Fatalf("route = %+v", route)
	}
	if _, err := ParseWindowsDefaultRoute("On-link only\n"); err == nil {
		t.Fatal("expected error")
	}
}

func TestTunnelConfigUsesPlatformMTU(t *testing.T) {
	body := TunnelConfig()
	if runtime.GOOS == "windows" {
		if !strings.Contains(body, "mtu: 1500") {
			t.Fatalf("windows mtu missing:\n%s", body)
		}
		return
	}
	if !strings.Contains(body, "mtu: 8500") {
		t.Fatalf("linux mtu missing:\n%s", body)
	}
}

func TestDiscoverTunnelMissing(t *testing.T) {
	t.Setenv("NAGA_HEV_PATH", "")
	old := tunnelExecutablePath
	tunnelExecutablePath = func() (string, error) {
		return "/no/such/naga-control", nil
	}
	defer func() { tunnelExecutablePath = old }()
	if _, err := DiscoverTunnel("/no/such/hev-socks5-tunnel"); err != ErrTunnelBinary {
		t.Fatalf("err = %v", err)
	}
}

func TestParseDefaultRoute(t *testing.T) {
	route, err := ParseDefaultRoute("default via 192.168.1.1 dev wlan0 proto dhcp src 192.168.1.130 metric 600\n")
	if err != nil {
		t.Fatal(err)
	}
	if route.Gateway != "192.168.1.1" || route.Device != "wlan0" {
		t.Fatalf("route = %+v", route)
	}
	if _, err := ParseDefaultRoute("not a route\n"); err == nil {
		t.Fatal("expected error")
	}
}

func TestTrafficRates(t *testing.T) {
	upload, download := TrafficRates(100, 1000, 150, 1600, 2)
	if upload != 25 || download != 300 {
		t.Fatalf("rates = %d %d", upload, download)
	}
	if up, down := TrafficRates(20, 20, 10, 10, 1); up != 0 || down != 0 {
		t.Fatalf("counter reset = %d %d", up, down)
	}
}

func TestTunnelConfigPointsAtNagaSOCKS(t *testing.T) {
	body := TunnelConfig()
	for _, want := range []string{
		"name: naga-olc0",
		"ipv4: 198.18.0.1",
		"port: 10808",
		"address: 127.0.0.1",
		"udp: 'udp'",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in\n%s", want, body)
		}
	}
}
