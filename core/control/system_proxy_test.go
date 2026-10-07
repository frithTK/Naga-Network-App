//go:build linux && !android

package control

import (
	"reflect"
	"testing"
)

func TestGSettingsSystemProxyConfiguresAndRestores(t *testing.T) {
	var calls [][]string
	manager := &GSettingsSystemProxy{
		run: func(args ...string) (string, error) {
			calls = append(calls, append([]string(nil), args...))
			if args[0] == "get" {
				return "previous", nil
			}
			return "", nil
		},
	}

	restore, err := manager.Configure("127.0.0.1", 2080)
	if err != nil {
		t.Fatal(err)
	}
	if err := restore(); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 21 {
		t.Fatalf("gsettings calls = %#v", calls)
	}
	if !reflect.DeepEqual(calls[14], []string{"set", "org.gnome.system.proxy", "mode", "previous"}) {
		t.Fatalf("first restore call = %#v", calls[14])
	}
	if !reflect.DeepEqual(calls[16], []string{"set", "org.gnome.system.proxy.http", "port", "previous"}) {
		t.Fatalf("port restore call = %#v", calls[16])
	}
}
