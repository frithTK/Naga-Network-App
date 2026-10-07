//go:build linux

package olcrtc

import (
	"net"
	"strings"
	"testing"
)

type fakeExec struct {
	calls []string
	out   map[string]string
}

func (f *fakeExec) Run(name string, args ...string) error {
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	return nil
}

func (f *fakeExec) Output(name string, args ...string) ([]byte, error) {
	key := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, key)
	return []byte(f.out[key]), nil
}

func TestBypassPinsRoomViaCurrentGateway(t *testing.T) {
	exec := &fakeExec{out: map[string]string{
		"ip -4 route show default": "default via 192.168.1.1 dev wlan0 proto dhcp\n",
		"ip -6 route show default": "default via fe80::1 dev wlan0 proto ra metric 600\n",
		"resolvectl dns wlan0":     "Link 3 (wlan0): 192.168.1.1\n",
	}}
	state, err := CaptureLink(exec)
	if err != nil {
		t.Fatal(err)
	}
	state.RoomIPs = []net.IP{net.ParseIP("203.0.113.10")}
	if err := BypassRoom(exec, state); err != nil {
		t.Fatal(err)
	}
	if err := EngageDefault(exec, state); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(exec.calls, "\n")
	if !strings.Contains(joined, "ip route replace 203.0.113.10/32 via 192.168.1.1 dev wlan0") {
		t.Fatalf("bypass missing:\n%s", joined)
	}
	if strings.Contains(joined, "127.0.0.1") {
		t.Fatalf("loopback was pinned:\n%s", joined)
	}
	if !strings.Contains(joined, "ip route replace default dev naga-olc0") {
		t.Fatalf("default route missing:\n%s", joined)
	}
	if !strings.Contains(joined, "ip -6 route del default via fe80::1 dev wlan0 proto ra metric 600") {
		t.Fatalf("ipv6 default not removed:\n%s", joined)
	}
	if err := RestoreLink(exec, state); err != nil {
		t.Fatal(err)
	}
	joined = strings.Join(exec.calls, "\n")
	if !strings.Contains(joined, "ip route replace default via 192.168.1.1 dev wlan0 proto dhcp") {
		t.Fatalf("restore missing:\n%s", joined)
	}
}
