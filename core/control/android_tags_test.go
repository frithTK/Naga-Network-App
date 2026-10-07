package control

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestAndroidBuildDropsLinuxHostFiles(t *testing.T) {
	cmd := exec.Command("go", "list", "-f", "{{.GoFiles}}", ".")
	cmd.Env = append(os.Environ(), "GOOS=android", "GOARCH=arm64", "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list: %v\n%s", err, out)
	}
	files := string(out)
	for _, name := range []string{
		"preflight_android.go",
		"dns_platform_android.go",
		"apps_android.go",
		"system_proxy_android.go",
		"runtime_hevfd.go",
	} {
		if !strings.Contains(files, name) {
			t.Fatalf("expected %s in Android build, got %s", name, files)
		}
	}
	for _, name := range []string{
		"preflight_linux.go",
		"dns_platform_linux.go",
		"apps_linux.go",
		"system_proxy_linux.go",
	} {
		if strings.Contains(files, name) {
			t.Fatalf("%s leaked into Android build: %s", name, files)
		}
	}
}
