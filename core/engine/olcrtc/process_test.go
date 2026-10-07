package olcrtc

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestVersionAndReady(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell fixture")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "olcrtc")
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = version ]; then echo " + PinnedCommit + "; exit 0; fi\n" +
		"exec python3 - <<'PY'\n" +
		"import socket\n" +
		"s=socket.socket(); s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)\n" +
		"s.bind(('127.0.0.1', 10808)); s.listen(8)\n" +
		"while True:\n" +
		"    c,_=s.accept(); c.close()\n" +
		"PY\n"
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "olcrtc.version"), []byte(PinnedCommit+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd, err := Start(ctx, bin, filepath.Join(dir, "client.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	defer cmd.Stop()
	readyCtx, readyCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer readyCancel()
	if err := cmd.Ready(readyCtx); err != nil {
		t.Fatal(err)
	}
	conn, err := net.Dial("tcp", "127.0.0.1:10808")
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	if cmd.ExitReason() != ExitOther {
		t.Fatalf("reason = %s", cmd.ExitReason())
	}
}

func TestCheckVersionRejectsEmpty(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "olcrtc")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := CheckVersion(context.Background(), bin); err != ErrVersion {
		t.Fatalf("err = %v", err)
	}
}

func TestClassifyLineUsesPinnedMarkers(t *testing.T) {
	if got := classifyLine("link: empty room", ExitOther); got != ExitEmptyRoom {
		t.Fatalf("empty = %s", got)
	}
	if got := classifyLine("handshake: key does not match the peer", ExitOther); got != ExitBadKey {
		t.Fatalf("key = %s", got)
	}
	if got := classifyLine("socks ready", ExitOther); got != ExitOther {
		t.Fatalf("other = %s", got)
	}
}

func TestDiscoverMissing(t *testing.T) {
	t.Setenv("NAGA_OLCRTC_PATH", "")
	if _, err := DiscoverBinary(filepath.Join(t.TempDir(), "missing")); err != ErrBinaryNotFound {
		t.Fatalf("err = %v", err)
	}
}

func TestDiscoverBinaryFindsSibling(t *testing.T) {
	dir := t.TempDir()
	name := "olcrtc"
	if runtime.GOOS == "windows" {
		name = "olcrtc.exe"
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("x"), 0o700); err != nil {
		t.Fatal(err)
	}
	old := executablePath
	executablePath = func() (string, error) {
		return filepath.Join(dir, "naga-control.exe"), nil
	}
	t.Cleanup(func() { executablePath = old })
	t.Setenv("NAGA_OLCRTC_PATH", "")
	t.Setenv("NAGA_NATIVE_LIB_DIR", "")
	t.Setenv("NAGA_OLCRTC_VERSION_FILE", "")
	t.Setenv("NAGA_DATA_DIR", "")
	got, err := DiscoverBinary("")
	if err != nil {
		t.Fatal(err)
	}
	if got != path {
		t.Fatalf("got %q want %q", got, path)
	}
	if BinaryReady("") {
		t.Fatal("missing olcrtc.version must not be ready")
	}
	if err := os.WriteFile(filepath.Join(dir, "olcrtc.version"), []byte(PinnedCommit+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !BinaryReady("") {
		t.Fatal("pinned sibling should be ready")
	}
}
