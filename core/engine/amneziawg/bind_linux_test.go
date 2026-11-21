//go:build linux && !android

package amneziawg

import (
	"context"
	"net/netip"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/amnezia-vpn/amneziawg-go/v3/conn"
	"golang.org/x/sys/unix"
)

func TestLinuxAWGBindKeepsUplinkUnderNagaPolicyRoutes(t *testing.T) {
	const marker = "NAGA_TEST_AWG_ROUTE_NAMESPACE"
	if os.Getenv(marker) != "1" {
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
		cmd := exec.CommandContext(t.Context(), "unshare", "--user", "--map-root-user", "--net", binary, "-test.run=^TestLinuxAWGBindKeepsUplinkUnderNagaPolicyRoutes$")
		cmd.Env = append(os.Environ(), marker+"=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("isolated AWG route: %v\n%s", err, output)
		}
		return
	}
	for _, command := range [][]string{
		{"link", "set", "lo", "up"},
		{"link", "add", "uplink", "type", "dummy"},
		{"addr", "add", "192.0.2.1/24", "dev", "uplink"},
		{"link", "set", "uplink", "up"},
		{"route", "add", "default", "dev", "uplink"},
		{"link", "add", "naga-tun0", "type", "dummy"},
		{"addr", "add", "198.51.100.1/24", "dev", "naga-tun0"},
		{"link", "set", "naga-tun0", "up"},
		{"route", "add", "default", "dev", "naga-tun0", "table", "2022"},
		{"rule", "add", "priority", "9000", "lookup", "2022"},
	} {
		if output, err := exec.CommandContext(t.Context(), "ip", command...).CombinedOutput(); err != nil {
			t.Fatalf("ip %v: %v\n%s", command, err, output)
		}
	}
	peer := netip.MustParseAddrPort("203.0.113.5:51820")
	routes, err := readLinuxRoutes(t.Context(), "-4", "get", peer.Addr().String())
	if err != nil || firstLinuxDevice(routes) != "naga-tun0" {
		t.Fatalf("ordinary traffic was not captured: %+v, %v", routes, err)
	}
	cfg := readNetstackFixture(t)
	cfg.Peers[0].Endpoint = peer.String()
	bind, err := newPlatformBind(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := bind.Open(0); err != nil {
		t.Fatal(err)
	}
	defer bind.Close()
	fd, err := bind.(*linuxUplinkBind).Bind.(conn.PeekLookAtSocketFd).PeekLookAtSocketFd4()
	if err != nil {
		t.Fatal(err)
	}
	if err := unix.Connect(fd, &unix.SockaddrInet4{Port: int(peer.Port()), Addr: peer.Addr().As4()}); err != nil {
		t.Fatal(err)
	}
	source, err := unix.Getsockname(fd)
	if err != nil {
		t.Fatal(err)
	}
	if addr, ok := source.(*unix.SockaddrInet4); !ok || addr.Addr != [4]byte{192, 0, 2, 1} {
		t.Fatalf("AWG socket selected the wrong source: %+v", source)
	}
	routes, err = readLinuxRoutes(t.Context(), "-4", "get", peer.Addr().String())
	if err != nil || firstLinuxDevice(routes) != "naga-tun0" {
		t.Fatalf("socket binding changed ordinary routes: %+v, %v", routes, err)
	}

	// A re-association can keep the same network class while changing
	// its outgoing interface. This must wake the socket-level watcher.
	for _, command := range [][]string{
		{"link", "add", "replacement", "type", "dummy"},
		{"addr", "add", "192.0.2.2/24", "dev", "replacement"},
		{"link", "set", "replacement", "up"},
	} {
		if output, err := exec.CommandContext(t.Context(), "ip", command...).CombinedOutput(); err != nil {
			t.Fatalf("ip %v: %v\n%s", command, err, output)
		}
	}
	watch, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	changed := make(chan string, 1)
	device := bind.(*linuxUplinkBind)
	go device.watchUplink(watch, func() error {
		device.mu.Lock()
		name := device.v4
		device.mu.Unlock()
		select {
		case changed <- name:
		default:
		}
		return nil
	}, func() error { return nil })

	for {
		if output, err := exec.CommandContext(watch, "ip", "-4", "route", "replace", "default", "dev", "replacement").CombinedOutput(); err != nil {
			t.Fatalf("change uplink: %v\n%s", err, output)
		}
		select {
		case name := <-changed:
			if name != "replacement" {
				t.Fatalf("route watcher picked %q", name)
			}
			return
		case <-watch.Done():
			t.Fatal("route watcher did not observe the new uplink")
		case <-time.After(100 * time.Millisecond):
		}
	}
}
