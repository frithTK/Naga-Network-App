package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestReadTokenFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "control.token")
	if err := os.WriteFile(path, []byte("  token-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	token, err := readTokenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if token != "token-value" {
		t.Fatalf("token = %q", token)
	}
}

func TestReadTokenFileRejectsEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "control.token")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readTokenFile(path); err == nil {
		t.Fatal("empty token file was accepted")
	}
}

func TestReadTokenFileRejectsBroadPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permission bits are not meaningful on Windows")
	}
	path := filepath.Join(t.TempDir(), "control.token")
	if err := os.WriteFile(path, []byte("token-value"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readTokenFile(path); err == nil {
		t.Fatal("broadly readable token file was accepted")
	}
}

func TestReadTokenFileAcceptsWindowsDefaultMode(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("covers NTFS default file mode")
	}
	path := filepath.Join(t.TempDir(), "control.token")
	if err := os.WriteFile(path, []byte("token-value"), 0o644); err != nil {
		t.Fatal(err)
	}
	token, err := readTokenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if token != "token-value" {
		t.Fatalf("token = %q", token)
	}
}

func TestReadTokenFileRejectsMissingFile(t *testing.T) {
	if _, err := readTokenFile(filepath.Join(t.TempDir(), "missing.token")); err == nil {
		t.Fatal("missing token file was accepted")
	}
}
