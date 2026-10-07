//go:build linux

package control

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverProcessesUsesExeBasenameNotComm(t *testing.T) {
	root := t.TempDir()
	writeProcess(t, root, "4242", 1000, "Name:\ttelegram-deskto\nUid:\t1000\t1000\t1000\t1000\n", "/opt/Telegram/telegram-desktop")
	writeProcess(t, root, "7", 0, "Name:\tinit\nUid:\t0\t0\t0\t0\n", "/sbin/init")
	writeProcess(t, root, "4243", 1000, "Name:\ttelegram-deskto\nUid:\t1000\t1000\t1000\t1000\n", "/opt/Telegram/telegram-desktop")

	got := discoverProcesses(root, 1000, os.Readlink, os.ReadFile)
	if len(got) != 1 {
		t.Fatalf("discovered = %#v, want one unique telegram-desktop", got)
	}
	if got[0].Process != "telegram-desktop" {
		t.Fatalf("process = %q, want telegram-desktop (not 15-char comm)", got[0].Process)
	}
	if got[0].ProcessPath != "/opt/Telegram/telegram-desktop" {
		t.Fatalf("process_path = %q", got[0].ProcessPath)
	}
}

func TestDiscoverProcessesReturnsEmptyWhenProcMissing(t *testing.T) {
	got := discoverProcesses(filepath.Join(t.TempDir(), "missing"), 1000, os.Readlink, os.ReadFile)
	if got == nil || len(got) != 0 {
		t.Fatalf("missing /proc = %#v, want empty slice", got)
	}
}

func writeProcess(t *testing.T, root, pid string, uid int, status, exe string) {
	t.Helper()
	dir := filepath.Join(root, pid)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "status"), []byte(status), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(exe, filepath.Join(dir, "exe")); err != nil {
		t.Fatal(err)
	}
	_ = uid
}
