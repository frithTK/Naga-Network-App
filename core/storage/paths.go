package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const appDataDirName = "naga-network"

// DefaultRoot is the per-user data directory: token, profiles, logs.
// NAGA_DATA_DIR overrides the platform default.
func DefaultRoot() (string, error) {
	if value := strings.TrimSpace(os.Getenv("NAGA_DATA_DIR")); value != "" {
		return value, nil
	}
	if runtime.GOOS == "windows" {
		base := strings.TrimSpace(os.Getenv("LOCALAPPDATA"))
		if base == "" {
			var err error
			base, err = os.UserConfigDir()
			if err != nil {
				return "", fmt.Errorf("resolve Windows app data directory: %w", err)
			}
		}
		return filepath.Join(base, appDataDirName), nil
	}
	base := strings.TrimSpace(os.Getenv("XDG_DATA_HOME"))
	if base == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		base = filepath.Join(userHome, ".local", "share")
	}
	return filepath.Join(base, appDataDirName), nil
}

// DefaultTokenPath is control.token inside DefaultRoot.
func DefaultTokenPath() (string, error) {
	root, err := DefaultRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "control.token"), nil
}
