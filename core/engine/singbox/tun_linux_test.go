//go:build linux

package singbox

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLinuxTUNInterfaceNames(t *testing.T) {
	config := []byte(`{"inbounds":[
    {"type":"tun","interface_name":"naga-tun0"},
    {"type":"mixed","interface_name":"ignored"},
    {"type":"tun","interface_name":"naga-tun0"}
  ]}`)
	names, err := linuxTUNInterfaceNames(config)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "naga-tun0" {
		t.Fatalf("TUN interface names = %#v", names)
	}
}

func TestLinuxTUNInterfaceNamesRejectsUnsafeName(t *testing.T) {
	_, err := linuxTUNInterfaceNames([]byte(`{"inbounds":[{"type":"tun","interface_name":"../all"}]}`))
	if err == nil {
		t.Fatal("unsafe TUN interface name was accepted")
	}
}

func TestSetLooseReversePathFilter(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "naga-tun0")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "rp_filter")
	if err := os.WriteFile(path, []byte("1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := setLooseReversePathFilter(root, "naga-tun0"); err != nil {
		t.Fatal(err)
	}
	value, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(value) != "2\n" {
		t.Fatalf("rp_filter = %q, want 2", value)
	}
}
