package singbox

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDiscoverBinaryUsesExplicitPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sing-box")
	if err := os.WriteFile(path, []byte("binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	got, err := DiscoverBinary(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != path {
		t.Fatalf("path = %q, want %q", got, path)
	}
}

func TestDiscoverControlRuntimeRequiresWindowsWintun(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("wintun is a Windows runtime file")
	}
	dir := t.TempDir()
	box := filepath.Join(dir, "sing-box.exe")
	if err := os.WriteFile(box, []byte("binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := DiscoverControlRuntime(box); err == nil {
		t.Fatal("expected missing wintun.dll to fail")
	}
	if err := os.WriteFile(filepath.Join(dir, "wintun.dll"), []byte("dll"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := DiscoverControlRuntime(box)
	if err != nil {
		t.Fatal(err)
	}
	if got != box {
		t.Fatalf("path = %q, want %q", got, box)
	}
}

func TestDiscoverBinaryUsesSiblingOfExecutable(t *testing.T) {
	dir := t.TempDir()
	sibling := filepath.Join(dir, singBoxNames()[0])
	if err := os.WriteFile(sibling, []byte("binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	fakeExe := filepath.Join(dir, "naga-control")
	previous := executablePath
	executablePath = func() (string, error) { return fakeExe, nil }
	t.Cleanup(func() { executablePath = previous })
	t.Setenv("NAGA_SINGBOX_PATH", "")

	got, err := DiscoverBinary("")
	if err != nil {
		t.Fatal(err)
	}
	if got != sibling {
		t.Fatalf("path = %q, want %q", got, sibling)
	}
}
