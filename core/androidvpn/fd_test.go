package androidvpn

import "strings"
import "testing"

func TestFDConfigYAMLOmitsNamedTUN(t *testing.T) {
	raw := FDConfigYAML(1500, "172.19.0.1", "127.0.0.1", 2080)
	if strings.Contains(raw, "naga-olc0") {
		t.Fatalf("named tun leaked: %s", raw)
	}
	if !strings.Contains(raw, "ipv4: 172.19.0.1") {
		t.Fatalf("ipv4: %s", raw)
	}
	if !strings.Contains(raw, "port: 2080") {
		t.Fatalf("port: %s", raw)
	}
}
