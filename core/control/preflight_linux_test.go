//go:build linux && !android

package control

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestLinuxTUNPreflightRejectsExistingTUN(t *testing.T) {
	preflight := &LinuxTUNPreflight{
		OpenTUN: func() error { return nil },
		Run: func(context.Context, string, ...string) ([]byte, error) {
			return []byte("6: Meta: <POINTOPOINT,UP> mtu 4064\n"), nil
		},
	}
	if err := preflight.CheckTUN(); err == nil || !strings.Contains(err.Error(), "Meta") {
		t.Fatalf("CheckTUN() error = %v, want Meta conflict", err)
	}
}

func TestLinuxTUNPreflightPropagatesIPFailure(t *testing.T) {
	preflight := &LinuxTUNPreflight{
		OpenTUN: func() error { return nil },
		Run: func(context.Context, string, ...string) ([]byte, error) {
			return []byte("netlink unavailable"), errors.New("exit status 1")
		},
	}
	if err := preflight.CheckTUN(); err == nil || !strings.Contains(err.Error(), "проверить") {
		t.Fatalf("CheckTUN() error = %v, want ip failure", err)
	}
}
