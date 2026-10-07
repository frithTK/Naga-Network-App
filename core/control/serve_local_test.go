package control

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAndroidTempDir(t *testing.T) {
	dir := t.TempDir()
	got, err := androidTempDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "tmp")
	if got != want {
		t.Fatalf("tmp = %q, want %q", got, want)
	}
	info, err := os.Stat(want)
	if err != nil || !info.IsDir() {
		t.Fatalf("tmp dir: %v", err)
	}
}

func TestStartLocalLeavesHostTempDir(t *testing.T) {
	before := os.Getenv("TMPDIR")
	t.Cleanup(func() {
		if before == "" {
			_ = os.Unsetenv("TMPDIR")
			return
		}
		_ = os.Setenv("TMPDIR", before)
	})
	dir := t.TempDir()
	srv, err := StartLocal(LocalOptions{
		Listen:    "127.0.0.1:0",
		DataDir:   dir,
		TokenPath: filepath.Join(dir, "control.token"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Stop)
	if os.Getenv("TMPDIR") != before {
		t.Fatalf("TMPDIR changed on %s: %q -> %q", os.Getenv("GOOS"), before, os.Getenv("TMPDIR"))
	}
}

func TestStartLocalRejectsNonLoopback(t *testing.T) {
	_, err := StartLocal(LocalOptions{Listen: "0.0.0.0:8765", DataDir: t.TempDir()})
	if err == nil {
		t.Fatal("expected non-loopback listen to fail")
	}
}

func TestStartLocalWritesTokenAndServesHealth(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("NAGA_DATA_DIR", dir)
	srv, err := StartLocal(LocalOptions{
		Listen:    "127.0.0.1:0",
		DataDir:   dir,
		TokenPath: filepath.Join(dir, "control.token"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Stop)
	token, err := os.ReadFile(filepath.Join(dir, "control.token"))
	if err != nil {
		t.Fatal(err)
	}
	if len(token) < 16 {
		t.Fatalf("token = %q", token)
	}
	var last error
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		req, err := http.NewRequest(http.MethodGet, "http://"+srv.Server.Addr+"/v1/health", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+srv.Token)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("health status = %d body = %s", resp.StatusCode, body)
			}
			return
		}
		last = err
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("health: %v", last)
}
