//go:build linux && !android

package olcrtc

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestPolicyRoutingBaselineInNetworkNamespace(t *testing.T) {
	if os.Getenv("NAGA_TEST_POLICY_GUARD_NAMESPACE") != "1" {
		if _, err := exec.LookPath("ip"); err != nil {
			t.Skip("iproute2 is unavailable")
		}
		if err := exec.Command("unshare", "--user", "--map-root-user", "--net", "true").Run(); err != nil {
			t.Skip("unprivileged network namespaces are unavailable")
		}
		binary, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("unshare", "--user", "--map-root-user", "--net", binary, "-test.run=^TestPolicyRoutingBaselineInNetworkNamespace$")
		cmd.Env = append(os.Environ(), "NAGA_TEST_POLICY_GUARD_NAMESPACE=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("isolated rule check: %v\n%s", err, output)
		}
		return
	}
	execer := namespaceExec{}
	if err := CheckPolicyRoutingBaseline(execer); err != nil {
		t.Fatalf("fresh namespace: %v", err)
	}
	if err := execer.Run("ip", "-4", "rule", "add", "priority", "9000", "lookup", "2022"); err != nil {
		t.Fatal(err)
	}
	if err := CheckPolicyRoutingBaseline(execer); !errors.Is(err, ErrPolicyRoutingConflict) {
		t.Fatalf("foreign policy rule was not detected: %v", err)
	}
}

func TestMarkedRouteLifecycleInNetworkNamespace(t *testing.T) {
	if os.Getenv("NAGA_TEST_MARKED_ROUTE_NAMESPACE") != "1" {
		if _, err := exec.LookPath("ip"); err != nil {
			t.Skip("iproute2 is unavailable")
		}
		if err := exec.Command("unshare", "--user", "--map-root-user", "--net", "true").Run(); err != nil {
			t.Skip("unprivileged network namespaces are unavailable")
		}
		binary, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("unshare", "--user", "--map-root-user", "--net", binary, "-test.run=^TestMarkedRouteLifecycleInNetworkNamespace$")
		cmd.Env = append(os.Environ(), "NAGA_TEST_MARKED_ROUTE_NAMESPACE=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("isolated marked route: %v\n%s", err, output)
		}
		return
	}
	execer := namespaceExec{}
	if err := execer.Run("ip", "link", "set", "lo", "up"); err != nil {
		t.Fatal(err)
	}
	state, err := InstallMarkedRoute(execer, 0x5a, 26001, 12001, DefaultRoute{Gateway: "127.0.0.1", Device: "lo"})
	if err != nil {
		t.Fatal(err)
	}
	output, err := execer.Output("ip", "-4", "route", "get", "198.18.0.2", "mark", "0x5a")
	if err != nil || !strings.Contains(string(output), "dev lo") {
		t.Fatalf("marked packet did not use protected route: %s %v", output, err)
	}
	if _, err := InstallMarkedRoute(execer, 0x5b, 26002, 12002, DefaultRoute{Gateway: "127.0.0.1", Device: "lo"}); !errors.Is(err, ErrPolicyRoutingConflict) {
		t.Fatalf("simultaneous reservation not refused: %v", err)
	}
	if err := state.Remove(execer); err != nil {
		t.Fatal(err)
	}
	if err := CheckPolicyRoutingBaseline(execer); err != nil {
		t.Fatalf("policy rules not restored: %v", err)
	}
	if err := state.Remove(execer); err != nil {
		t.Fatalf("repeated cleanup: %v", err)
	}
	if err := execer.Run("ip", "-4", "route", "add", "blackhole", "default", "table", "26001"); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallMarkedRoute(execer, 0x5a, 26001, 12001, DefaultRoute{Gateway: "127.0.0.1", Device: "lo"}); !errors.Is(err, ErrPolicyRoutingConflict) {
		t.Fatalf("occupied routing table not refused: %v", err)
	}
	if err := execer.Run("ip", "-4", "route", "del", "blackhole", "default", "table", "26001"); err != nil {
		t.Fatal(err)
	}
	state, err = InstallMarkedRoute(execer, 0x5a, 26001, 12001, DefaultRoute{Gateway: "127.0.0.1", Device: "lo"})
	if err != nil {
		t.Fatal(err)
	}
	if err := execer.Run("ip", "-4", "route", "replace", "default", "via", "127.0.0.1", "dev", "lo", "proto", "99", "table", "26001"); err != nil {
		t.Fatal(err)
	}
	if err := state.Remove(execer); err == nil || !state.uncertain {
		t.Fatal("a replaced route must leave cleanup unconfirmed")
	}
	output, err = execer.Output("ip", "-j", "-4", "route", "show", "table", "26001")
	if err != nil || !strings.Contains(string(output), `"dst":"default"`) {
		t.Fatalf("a replacement route was removed: %s %v", output, err)
	}
}

type namespaceExec struct{}

func (namespaceExec) Run(name string, args ...string) error {
	output, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w: %s", name, err, output)
	}
	return nil
}

func (namespaceExec) Output(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

func TestBypassPreservesExistingRouteInNetworkNamespace(t *testing.T) {
	if os.Getenv("NAGA_TEST_OLC_ROUTE_NAMESPACE") != "1" {
		if _, err := exec.LookPath("ip"); err != nil {
			t.Skip("iproute2 is unavailable")
		}
		if err := exec.Command("unshare", "--user", "--map-root-user", "--net", "true").Run(); err != nil {
			t.Skip("unprivileged network namespaces are unavailable")
		}
		binary, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("unshare", "--user", "--map-root-user", "--net", binary, "-test.run=^TestBypassPreservesExistingRouteInNetworkNamespace$")
		cmd.Env = append(os.Environ(), "NAGA_TEST_OLC_ROUTE_NAMESPACE=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("isolated route test: %v\n%s", err, output)
		}
		return
	}
	execer := namespaceExec{}
	for _, args := range [][]string{
		{"link", "set", "lo", "up"},
		{"route", "add", "203.0.113.11/32", "dev", "lo", "proto", "99"},
	} {
		if err := execer.Run("ip", args...); err != nil {
			t.Fatal(err)
		}
	}
	state := LinkState{
		Route:   DefaultRoute{Gateway: "127.0.0.1", Device: "lo"},
		RoomIPs: []net.IP{net.ParseIP("203.0.113.10"), net.ParseIP("203.0.113.11")},
	}
	if err := BypassRoom(execer, &state); err == nil {
		t.Fatal("existing route should cause an add conflict")
	}
	if err := RemoveRoomBypasses(execer, state); err != nil {
		t.Fatal(err)
	}
	existing, err := execer.Output("ip", "route", "show", "203.0.113.11/32")
	if err != nil || !strings.Contains(string(existing), "203.0.113.11") {
		t.Fatalf("existing route was changed: %s, %v", existing, err)
	}
	owned, err := execer.Output("ip", "route", "show", "203.0.113.10/32")
	if err != nil || len(strings.TrimSpace(string(owned))) != 0 {
		t.Fatalf("owned route was not removed: %s, %v", owned, err)
	}
	// A route replaced by another actor after creation must not be deleted.
	state.RoomIPs = []net.IP{net.ParseIP("203.0.113.12")}
	state.OwnedRoomIPs = nil
	if err := BypassRoom(execer, &state); err != nil {
		t.Fatal(err)
	}
	if err := execer.Run("ip", "route", "replace", "203.0.113.12/32", "dev", "lo", "proto", "99"); err != nil {
		t.Fatal(err)
	}
	if err := RemoveRoomBypasses(execer, state); err == nil {
		t.Fatal("replaced route must not be reported as removed")
	}
	replaced, err := execer.Output("ip", "route", "show", "203.0.113.12/32")
	if err != nil || !strings.Contains(string(replaced), "203.0.113.12") {
		t.Fatalf("replacement route was removed: %s, %v", replaced, err)
	}
}
