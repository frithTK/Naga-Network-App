package profile

import (
	"strings"
	"testing"
)

func TestIDFromSourceURLIsStableAndOpaque(t *testing.T) {
	first := IDFromSourceURL("https://example.com/a")
	second := IDFromSourceURL("https://example.com/a")
	other := IDFromSourceURL("https://example.com/b")

	if first != second {
		t.Fatalf("same URL produced different IDs: %q and %q", first, second)
	}
	if first == other {
		t.Fatalf("different URLs produced the same ID: %q", first)
	}
	if first == "https://example.com/a" {
		t.Fatal("profile ID must not contain the source URL")
	}
}

func TestIDFromConfigIsStableAndOpaque(t *testing.T) {
	first := IDFromConfig([]byte(`{"outbounds":[{"type":"direct"}]}`))
	second := IDFromConfig([]byte(`{"outbounds":[{"type":"direct"}]}`))
	other := IDFromConfig([]byte(`{"outbounds":[{"type":"block"}]}`))
	if first != second {
		t.Fatalf("same config produced different IDs: %q and %q", first, second)
	}
	if first == other {
		t.Fatalf("different configs produced the same ID: %q", first)
	}
	if strings.Contains(first, "outbounds") {
		t.Fatal("profile ID must not contain config bytes")
	}
}
