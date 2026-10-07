//go:build windows

package olcrtc

import (
	"net"
	"strings"
	"testing"
)

type fakeExec struct {
	calls []string
	out   map[string]string
	fail  string
}

func (f *fakeExec) Run(name string, args ...string) error {
	key := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, key)
	if f.fail != "" && strings.Contains(key, f.fail) {
		return errNoWindowsGateway
	}
	return nil
}

func (f *fakeExec) Output(name string, args ...string) ([]byte, error) {
	key := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, key)
	return []byte(f.out[key]), nil
}

func TestWindowsBypassPinsRoomViaCurrentGateway(t *testing.T) {
	exec := &fakeExec{out: map[string]string{
		"route print -4": "Network Destination        Netmask          Gateway       Interface  Metric\n          0.0.0.0          0.0.0.0      192.168.1.1     192.168.1.10     25\n",
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
	if !strings.Contains(joined, "route add 203.0.113.10 mask 255.255.255.255 192.168.1.1") {
		t.Fatalf("bypass missing:\n%s", joined)
	}
	if !strings.Contains(joined, "route add 0.0.0.0 mask 0.0.0.0 198.18.0.1 metric 1") {
		t.Fatalf("default route missing:\n%s", joined)
	}
	if err := RestoreLink(exec, state); err != nil {
		t.Fatal(err)
	}
	joined = strings.Join(exec.calls, "\n")
	if !strings.Contains(joined, "route delete 0.0.0.0 mask 0.0.0.0 198.18.0.1") {
		t.Fatalf("restore missing:\n%s", joined)
	}
	if !strings.Contains(joined, "route delete 203.0.113.10 mask 255.255.255.255") {
		t.Fatalf("bypass restore missing:\n%s", joined)
	}
}

func TestWindowsRestoreAfterBypassFailure(t *testing.T) {
	exec := &fakeExec{
		out: map[string]string{
			"route print -4": "0.0.0.0 0.0.0.0 192.168.1.1 192.168.1.10 25\n",
		},
		fail: "route add 203.0.113.10",
	}
	state, err := CaptureLink(exec)
	if err != nil {
		t.Fatal(err)
	}
	state.RoomIPs = []net.IP{net.ParseIP("203.0.113.10")}
	if err := BypassRoom(exec, state); err == nil {
		t.Fatal("expected bypass failure")
	}
	_ = RestoreLink(exec, state)
	joined := strings.Join(exec.calls, "\n")
	if !strings.Contains(joined, "route delete 203.0.113.10 mask 255.255.255.255") {
		t.Fatalf("failed bypass was not undone:\n%s", joined)
	}
}
