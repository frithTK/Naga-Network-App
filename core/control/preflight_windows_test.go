//go:build windows

package control

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsTUNPreflightRequiresSingBox(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "wintun.dll"), []byte("dll"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := (&WindowsTUNPreflight{Dir: dir}).CheckTUN()
	if err == nil || !strings.Contains(err.Error(), "sing-box.exe") {
		t.Fatalf("CheckTUN() error = %v, want missing sing-box.exe", err)
	}
}

func TestWindowsTUNPreflightRequiresWintun(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sing-box.exe"), []byte("exe"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := (&WindowsTUNPreflight{Dir: dir}).CheckTUN()
	if err == nil || !strings.Contains(err.Error(), "wintun.dll") {
		t.Fatalf("CheckTUN() error = %v, want missing wintun.dll", err)
	}
}

func TestWindowsTUNPreflightAcceptsSiblingFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sing-box.exe"), []byte("exe"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "wintun.dll"), []byte("dll"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := (&WindowsTUNPreflight{Dir: dir}).CheckTUN(); err != nil {
		t.Fatal(err)
	}
}
