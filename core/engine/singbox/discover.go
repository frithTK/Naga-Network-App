package singbox

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

var ErrBinaryNotFound = errors.New("sing-box binary was not found")

var executablePath = os.Executable

func DiscoverBinary(explicit string) (string, error) {
	if value := strings.TrimSpace(explicit); value != "" {
		if _, err := os.Stat(value); err != nil {
			return "", ErrBinaryNotFound
		}
		return value, nil
	}
	if value := strings.TrimSpace(os.Getenv("NAGA_SINGBOX_PATH")); value != "" {
		if _, err := os.Stat(value); err != nil {
			return "", ErrBinaryNotFound
		}
		return value, nil
	}
	if path := nativeLibCandidate(singBoxNames()); path != "" {
		return path, nil
	}
	if sibling := siblingSingBox(); sibling != "" {
		return sibling, nil
	}
	for _, name := range singBoxNames() {
		path, err := exec.LookPath(name)
		if err == nil {
			return path, nil
		}
	}
	return "", ErrBinaryNotFound
}

// DiscoverControlRuntime finds the sing-box used by naga-control. On Windows
// PATH is ignored: wintun.dll is loaded from the directory of sing-box.exe,
// so both files must sit next to naga-control.exe (or next to an explicit path).
func DiscoverControlRuntime(explicit string) (string, error) {
	if value := strings.TrimSpace(explicit); value != "" {
		if _, err := os.Stat(value); err != nil {
			return "", ErrBinaryNotFound
		}
		if err := requireWindowsRuntimeDLL(value); err != nil {
			return "", err
		}
		return value, nil
	}
	if runtime.GOOS == "windows" {
		sibling := siblingSingBox()
		if sibling == "" {
			return "", ErrBinaryNotFound
		}
		if err := requireWindowsRuntimeDLL(sibling); err != nil {
			return "", err
		}
		return sibling, nil
	}
	return DiscoverBinary("")
}

func siblingSingBox() string {
	exe, err := executablePath()
	if err != nil {
		return ""
	}
	dir := filepath.Dir(exe)
	for _, name := range singBoxNames() {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

func singBoxNames() []string {
	if runtime.GOOS == "windows" {
		return []string{"sing-box.exe", "sing-box"}
	}
	if runtime.GOOS == "android" {
		return []string{"libsingbox.so", "sing-box"}
	}
	return []string{"sing-box"}
}

func nativeLibCandidate(names []string) string {
	dir := strings.TrimSpace(os.Getenv("NAGA_NATIVE_LIB_DIR"))
	if dir == "" {
		return ""
	}
	for _, name := range names {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

func requireWindowsRuntimeDLL(singBox string) error {
	if runtime.GOOS != "windows" {
		return nil
	}
	dll := filepath.Join(filepath.Dir(singBox), "wintun.dll")
	if _, err := os.Stat(dll); err != nil {
		return ErrBinaryNotFound
	}
	return nil
}
