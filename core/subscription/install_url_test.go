package subscription

import (
	"strings"
	"testing"
)

func TestResolveFetchURLAcceptsHTTPS(t *testing.T) {
	got, err := ResolveFetchURL(" https://nagavpn.example/bundles/nagavpn/uuid.json ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://nagavpn.example/bundles/nagavpn/uuid.json" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveFetchURLUnwrapsDeepLink(t *testing.T) {
	raw := "nagavpn://install-config?url=https%3A%2F%2Fnagavpn.example%2Fbundles%2Fnagavpn%2Fuuid.json"
	got, err := ResolveFetchURL(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://nagavpn.example/bundles/nagavpn/uuid.json" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveFetchURLRejectsPrivateHost(t *testing.T) {
	if _, err := ResolveFetchURL("https://127.0.0.1/local.json"); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("private host error = %v", err)
	}
}

func TestResolveFetchURLRejectsNonHTTPSInner(t *testing.T) {
	if _, err := ResolveFetchURL("nagavpn://install-config?url=http://example.invalid/profile.json"); err == nil {
		t.Fatal("expected error")
	}
}

func TestResolveFetchURLRejectsUnknownScheme(t *testing.T) {
	if _, err := ResolveFetchURL("karing://install-config?url=https://example.invalid/a.json"); err == nil {
		t.Fatal("expected error")
	}
}
