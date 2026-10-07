package singbox

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAndroidBuildUsesTunAndroidNotLinux(t *testing.T) {
	cmd := exec.Command("go", "list", "-f", "{{.GoFiles}}", ".")
	cmd.Env = append(os.Environ(), "GOOS=android", "GOARCH=arm64", "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list: %v\n%s", err, out)
	}
	files := string(out)
	if !strings.Contains(files, "tun_android.go") {
		t.Fatalf("expected tun_android.go in Android build, got %s", files)
	}
	if strings.Contains(files, "tun_linux.go") {
		t.Fatalf("tun_linux.go leaked into Android build: %s", files)
	}
}

func TestDiscoverBinaryUsesNativeLibDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, singBoxNames()[0])
	if err := os.WriteFile(path, []byte("binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NAGA_NATIVE_LIB_DIR", dir)
	t.Setenv("NAGA_SINGBOX_PATH", "")
	got, err := DiscoverBinary("")
	if err != nil {
		t.Fatal(err)
	}
	if got != path {
		t.Fatalf("path = %q, want %q", got, path)
	}
}
