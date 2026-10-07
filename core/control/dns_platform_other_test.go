//go:build !linux

package control

import "testing"

func TestPlatformDNSIsNoop(t *testing.T) {
	restore, err := NewPlatformDNS().Configure()
	if err != nil {
		t.Fatal(err)
	}
	if restore != nil {
		t.Fatal("Windows DNS manager must not return a restore hook")
	}
}
