//go:build windows

package main

import "testing"

func TestShouldReplaceControl(t *testing.T) {
	cases := []struct {
		name                    string
		healthy, ready, bundled bool
		want                    bool
	}{
		{name: "stale without runtime", healthy: true, ready: false, bundled: true, want: true},
		{name: "already ready", healthy: true, ready: true, bundled: true, want: false},
		{name: "not running", healthy: false, ready: false, bundled: true, want: false},
		{name: "bundle missing files", healthy: true, ready: false, bundled: false, want: false},
		{name: "foreign skipped by caller", healthy: false, ready: false, bundled: true, want: false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got := shouldReplaceControl(test.healthy, test.ready, test.bundled)
			if got != test.want {
				t.Fatalf("shouldReplaceControl(%v,%v,%v) = %v, want %v", test.healthy, test.ready, test.bundled, got, test.want)
			}
		})
	}
}

func TestNboPort(t *testing.T) {
	if got := nboPort(0x3d22); got != 8765 {
		t.Fatalf("nboPort = %d, want 8765", got)
	}
}

func TestParseNagaHealth(t *testing.T) {
	payload, ours := parseNagaHealth([]byte(`{"service":"naga-control","status":"ok","runtime_ready":true}`))
	if !ours || !payload.RuntimeReady {
		t.Fatalf("payload = %#v ours = %v", payload, ours)
	}
	_, ours = parseNagaHealth([]byte(`{"service":"other"}`))
	if ours {
		t.Fatal("foreign health must not match")
	}
}
