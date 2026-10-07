package storage

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestDefaultRootUsesNagaDataDir(t *testing.T) {
	root := filepath.Join(t.TempDir(), "custom-data")
	t.Setenv("NAGA_DATA_DIR", root)
	got, err := DefaultRoot()
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Fatalf("DefaultRoot() = %q, want %q", got, root)
	}
	token, err := DefaultTokenPath()
	if err != nil {
		t.Fatal(err)
	}
	wantToken := filepath.Join(root, "control.token")
	if token != wantToken {
		t.Fatalf("DefaultTokenPath() = %q, want %q", token, wantToken)
	}
}

func TestDefaultRootUsesXDGOnUnix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix XDG layout")
	}
	t.Setenv("NAGA_DATA_DIR", "")
	xdg := filepath.Join(t.TempDir(), "xdg-data")
	t.Setenv("XDG_DATA_HOME", xdg)
	t.Setenv("LOCALAPPDATA", filepath.Join(t.TempDir(), "should-not-use"))
	got, err := DefaultRoot()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(xdg, "naga-network")
	if got != want {
		t.Fatalf("DefaultRoot() = %q, want %q", got, want)
	}
}

func TestDefaultRootUsesLocalAppDataOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows LOCALAPPDATA layout")
	}
	t.Setenv("NAGA_DATA_DIR", "")
	local := filepath.Join(t.TempDir(), "local-app-data")
	t.Setenv("LOCALAPPDATA", local)
	got, err := DefaultRoot()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(local, "naga-network")
	if got != want {
		t.Fatalf("DefaultRoot() = %q, want %q", got, want)
	}
}

func TestNewDefaultUsesProfilesUnderRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "data")
	t.Setenv("NAGA_DATA_DIR", root)
	store, err := NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "profiles")
	if store.Root != want {
		t.Fatalf("store.Root = %q, want %q", store.Root, want)
	}
}
