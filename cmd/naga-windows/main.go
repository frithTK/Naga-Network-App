//go:build windows

package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"naga.network/core/storage"
	"naga.network/core/winhelper"
)

const (
	controlListen  = "127.0.0.1:8765"
	controlHealth  = "http://127.0.0.1:8765/v1/health"
	createNoWindow = 0x08000000
)

func main() {
	if err := run(); err != nil {
		showError(err.Error())
		os.Exit(1)
	}
}

func run() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("не удалось определить путь лаунчера: %w", err)
	}
	root := filepath.Dir(exe)
	controlPath := filepath.Join(root, "naga-control.exe")
	uiPath := filepath.Join(root, "naga_network.exe")
	if _, err := os.Stat(controlPath); err != nil {
		return errors.New("рядом с лаунчером нет naga-control.exe")
	}
	if _, err := os.Stat(uiPath); err != nil {
		return errors.New("рядом с лаунчером нет naga_network.exe")
	}
	registerProtocol(exe)

	dataDir, err := storage.DefaultRoot()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "logs"), 0o700); err != nil {
		return fmt.Errorf("не удалось создать каталог данных: %w", err)
	}
	tokenPath := filepath.Join(dataDir, "control.token")
	token, err := ensureToken(tokenPath)
	if err != nil {
		return err
	}
	if err := os.Setenv("NAGA_DATA_DIR", dataDir); err != nil {
		return err
	}
	if err := os.Setenv("NAGA_CONTROL_TOKEN_FILE", tokenPath); err != nil {
		return err
	}

	healthy, runtimeReady, err := inspectControl(token)
	if err != nil {
		return err
	}
	if shouldReplaceControl(healthy, runtimeReady, bundleHasRuntime(root)) {
		if err := replaceStaleControl(); err != nil {
			return err
		}
		healthy = false
	}
	if !healthy {
		if err := startControl(controlPath, root, dataDir, tokenPath); err != nil {
			return err
		}
		if err := waitHealthy(token, 10*time.Second); err != nil {
			return err
		}
	}
	winhelper.EnsureRunning()

	ui := exec.Command(uiPath, os.Args[1:]...)
	ui.Dir = root
	ui.Env = os.Environ()
	return ui.Run()
}

func registerProtocol(launcher string) {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Classes\nagavpn`, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer key.Close()
	_ = key.SetStringValue("", "URL:Naga Network")
	_ = key.SetStringValue("URL Protocol", "")
	cmd, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Classes\nagavpn\shell\open\command`, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer cmd.Close()
	_ = cmd.SetStringValue("", `"`+launcher+`" "%1"`)
}

func ensureToken(path string) (string, error) {
	if data, err := os.ReadFile(path); err == nil {
		token := strings.TrimSpace(string(data))
		if token != "" {
			return token, nil
		}
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("не удалось создать control token: %w", err)
	}
	token := hex.EncodeToString(raw)
	if err := os.WriteFile(path, []byte(token+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("не удалось записать control token: %w", err)
	}
	return token, nil
}

func startControl(bin, dir, dataDir, tokenPath string) error {
	logPath := filepath.Join(dataDir, "logs", "naga-control.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("не удалось открыть лог control-plane: %w", err)
	}
	cmd := exec.Command(bin, "-listen", controlListen, "-production", "-token-file", tokenPath)
	cmd.Dir = dir
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return fmt.Errorf("не удалось запустить naga-control: %w", err)
	}
	go func() {
		_ = cmd.Wait()
		_ = logFile.Close()
	}()
	return nil
}

func waitHealthy(token string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var last error
	for time.Now().Before(deadline) {
		ok, _, err := inspectControl(token)
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		last = errors.New("control-plane не ответил")
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("naga-control не запустился: %w", last)
}

func inspectControl(token string) (bool, bool, error) {
	req, err := http.NewRequest(http.MethodGet, controlHealth, nil)
	if err != nil {
		return false, false, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{Timeout: 800 * time.Millisecond}
	resp, err := client.Do(req)
	if err != nil {
		return false, false, nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	payload, ours := parseNagaHealth(body)
	if resp.StatusCode == http.StatusOK && ours {
		return true, payload.RuntimeReady, nil
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return false, false, errors.New("порт 8765 занят, но это не наш control-plane")
	}
	if ours {
		return false, false, nil
	}
	return false, false, errors.New("порт 8765 занят другим приложением")
}

type nagaHealth struct {
	Service      string `json:"service"`
	Status       string `json:"status"`
	RuntimeReady bool   `json:"runtime_ready"`
}

func parseNagaHealth(body []byte) (nagaHealth, bool) {
	var payload nagaHealth
	if json.Unmarshal(body, &payload) != nil {
		return nagaHealth{}, false
	}
	return payload, payload.Service == "naga-control"
}

func showError(message string) {
	text, err := windows.UTF16PtrFromString(message)
	if err != nil {
		return
	}
	title, err := windows.UTF16PtrFromString("Naga Network")
	if err != nil {
		return
	}
	_, _ = windows.MessageBox(0, text, title, windows.MB_OK|windows.MB_ICONERROR)
}
