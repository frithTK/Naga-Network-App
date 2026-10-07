package control

import (
	"errors"
	"reflect"
	"testing"
)

func TestResolvedDNSManagerConfiguresAndRestores(t *testing.T) {
	var calls [][]string
	manager := &ResolvedDNSManager{
		Interface: "tun0",
		Server:    "127.0.0.1",
		run: func(args ...string) error {
			calls = append(calls, append([]string(nil), args...))
			return nil
		},
	}

	restore, err := manager.Configure()
	if err != nil {
		t.Fatal(err)
	}
	if err := restore(); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"dns", "tun0", "127.0.0.1"},
		{"domain", "tun0", "~."},
		{"default-route", "tun0", "yes"},
		{"flush-caches"},
		{"revert", "tun0"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("resolvectl calls = %#v, want %#v", calls, want)
	}
}

func TestResolvedDNSManagerRevertsAfterConfigurationFailure(t *testing.T) {
	var calls [][]string
	manager := &ResolvedDNSManager{
		Interface: "tun0",
		Server:    "127.0.0.1",
		run: func(args ...string) error {
			calls = append(calls, append([]string(nil), args...))
			if len(args) > 0 && args[0] == "domain" {
				return errors.New("denied")
			}
			return nil
		},
	}

	if _, err := manager.Configure(); err == nil {
		t.Fatal("Configure() unexpectedly succeeded")
	}
	want := [][]string{
		{"dns", "tun0", "127.0.0.1"},
		{"domain", "tun0", "~."},
		{"revert", "tun0"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("resolvectl calls = %#v, want %#v", calls, want)
	}
}
