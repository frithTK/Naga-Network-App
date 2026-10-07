//go:build windows

package control

import (
	"errors"
	"strings"
	"testing"
)

type memoryProxyStore struct {
	current proxySettings
	writes  []proxySettings
	failAt  int
}

func (s *memoryProxyStore) snapshot() (proxySettings, error) {
	return s.current, nil
}

func (s *memoryProxyStore) apply(value proxySettings) error {
	s.writes = append(s.writes, value)
	if s.failAt > 0 && len(s.writes) == s.failAt {
		return errors.New("write failed")
	}
	s.current = value
	return nil
}

type memoryProxyNotify struct {
	calls  []proxySettings
	failAt int
}

func (n *memoryProxyNotify) apply(value proxySettings) error {
	n.calls = append(n.calls, value)
	if n.failAt > 0 && len(n.calls) == n.failAt {
		return errors.New("notify failed")
	}
	return nil
}

func TestWinINETSystemProxyConfiguresAndRestores(t *testing.T) {
	store := &memoryProxyStore{current: proxySettings{
		override:   "*.internal",
		pac:        "http://pac.invalid/proxy.pac",
		pacPresent: true,
	}}
	notify := &memoryProxyNotify{}
	manager := &WinINETSystemProxy{store: store, notify: notify}

	restore, err := manager.Configure("127.0.0.1", 2080)
	if err != nil {
		t.Fatal(err)
	}
	if store.current.enable != 1 || !strings.Contains(store.current.server, "127.0.0.1:2080") {
		t.Fatalf("enabled settings = %#v", store.current)
	}
	if store.current.pacPresent {
		t.Fatal("PAC must be cleared while Naga proxy is active")
	}
	if err := restore(); err != nil {
		t.Fatal(err)
	}
	if store.current.enable != 0 || store.current.pac != "http://pac.invalid/proxy.pac" || !store.current.pacPresent {
		t.Fatalf("restored settings = %#v", store.current)
	}
	if len(notify.calls) < 2 {
		t.Fatalf("notify calls = %#v", notify.calls)
	}
}

func TestWinINETSystemProxyRollsBackWriteError(t *testing.T) {
	store := &memoryProxyStore{failAt: 1}
	notify := &memoryProxyNotify{}
	manager := &WinINETSystemProxy{store: store, notify: notify}
	if _, err := manager.Configure("127.0.0.1", 2080); err == nil {
		t.Fatal("expected write error")
	}
	if store.current.enable != 0 {
		t.Fatalf("rolled back settings = %#v", store.current)
	}
}

func TestWinINETSystemProxyRollsBackNotifyError(t *testing.T) {
	store := &memoryProxyStore{}
	notify := &memoryProxyNotify{failAt: 1}
	manager := &WinINETSystemProxy{store: store, notify: notify}
	if _, err := manager.Configure("127.0.0.1", 2080); err == nil {
		t.Fatal("expected notify error")
	}
	if store.current.enable != 0 {
		t.Fatalf("rolled back settings = %#v", store.current)
	}
	if len(notify.calls) < 2 {
		t.Fatalf("notify must retry previous settings, got %#v", notify.calls)
	}
}

func TestMergeProxyOverrideKeepsUserExclusions(t *testing.T) {
	got := mergeProxyOverride("*.corp;<local>", "localhost", "127.0.0.1", "<local>")
	if !strings.Contains(got, "*.corp") || !strings.Contains(got, "localhost") || !strings.Contains(got, "127.0.0.1") {
		t.Fatalf("override = %q", got)
	}
	if strings.Count(strings.ToLower(got), "<local>") != 1 {
		t.Fatalf("duplicate <local> in %q", got)
	}
}

func TestWinINETSystemProxyRejectsInvalidAddress(t *testing.T) {
	store := &memoryProxyStore{current: proxySettings{enable: 1, server: "keep"}}
	manager := &WinINETSystemProxy{store: store, notify: &memoryProxyNotify{}}
	if _, err := manager.Configure("", 2080); err == nil {
		t.Fatal("expected invalid address")
	}
	if store.current.server != "keep" {
		t.Fatalf("invalid address wrote %#v", store.current)
	}
}
