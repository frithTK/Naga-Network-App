package control

import (
	"errors"
	"net"
	"runtime"
	"strings"
	"testing"

	olcprofile "naga.network/core/profile/olcrtc"
)

type recordingExec struct {
	calls []string
	out   map[string]string
	fail  string
}

func (f *recordingExec) Run(name string, args ...string) error {
	key := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, key)
	if f.fail != "" && strings.Contains(key, f.fail) {
		return errors.New("bypass failed")
	}
	return nil
}

func (f *recordingExec) Output(name string, args ...string) ([]byte, error) {
	key := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, key)
	return []byte(f.out[key]), nil
}

func TestPinOlcRoomRestoresOnBypassFailure(t *testing.T) {
	execer := &recordingExec{out: map[string]string{}}
	if runtime.GOOS == "windows" {
		execer.out["route print -4"] = "0.0.0.0 0.0.0.0 192.168.1.1 192.168.1.10 25\n"
		execer.fail = "route add 203.0.113.10"
	} else {
		execer.out["ip -4 route show default"] = "default via 192.168.1.1 dev wlan0 proto dhcp\n"
		execer.out["ip -6 route show default"] = ""
		execer.out["resolvectl dns wlan0"] = ""
		execer.fail = "203.0.113.10/32"
	}
	_, err := pinOlcRoom(execer, &olcprofile.Config{
		Profiles: []olcprofile.Profile{{Room: "203.0.113.10"}},
	})
	if err == nil {
		t.Fatal("expected bypass failure")
	}
	joined := strings.Join(execer.calls, "\n")
	if runtime.GOOS == "windows" {
		if !strings.Contains(joined, "route delete 203.0.113.10 mask 255.255.255.255") {
			t.Fatalf("windows restore missing:\n%s", joined)
		}
		return
	}
	if !strings.Contains(joined, "ip route del 203.0.113.10/32") {
		t.Fatalf("linux restore missing:\n%s", joined)
	}
}

func TestRoomAddressesKeepsLiteralIPv4(t *testing.T) {
	ips := roomAddresses(&olcprofile.Config{
		Profiles: []olcprofile.Profile{{Room: "203.0.113.10"}},
	})
	if len(ips) != 1 || ips[0].Equal(net.ParseIP("203.0.113.10")) == false {
		t.Fatalf("ips = %v", ips)
	}
}
