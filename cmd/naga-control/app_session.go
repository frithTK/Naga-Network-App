package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"naga.network/core/control"
	"naga.network/core/storage"
	"naga.network/core/version"
)

// Stdout is a private pipe to the parent application, never a log stream.
// Readiness is emitted only after StartLocal has bound its own loopback listener.
func runAppSession(parent io.Reader, readiness io.Writer, singBoxPath string) (err error) {
	defer func() {
		if err != nil {
			_ = json.NewEncoder(readiness).Encode(map[string]any{
				"type": "error", "message": err.Error(),
			})
		}
	}()
	if runtime.GOOS == "darwin" && os.Geteuid() == 0 {
		return fmt.Errorf("application backend must run as the logged-in user")
	}
	root, err := storage.DefaultRoot()
	if err != nil {
		return err
	}
	release, err := lockAppSession(root)
	if err != nil {
		return err
	}
	defer release()
	tokenPath := filepath.Join(root, "control.token")
	if err := validateSessionToken(tokenPath); err != nil {
		return err
	}
	logDir := filepath.Join(root, "logs")
	if runtime.GOOS == "darwin" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		logDir = filepath.Join(home, "Library", "Logs", "Naga Network")
	}
	local, err := control.StartLocal(control.LocalOptions{
		Listen: "127.0.0.1:0", DataDir: root, TokenPath: tokenPath,
		SingBoxPath: singBoxPath, LogDir: logDir, RejectBrowserOrigin: true,
		OnlyExplicitRuntime:         true,
		ConnectionUnavailableReason: "VPN в тестовой сборке macOS ещё недоступен: системная служба не подключена.",
	})
	if err != nil {
		return fmt.Errorf("start application backend: %w", err)
	}
	defer func() {
		// Let in-flight profile/settings writes finish before process exit.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := local.Server.Shutdown(ctx); err != nil {
			_ = local.Server.Close()
		}
		local.Stop()
	}()
	if err := json.NewEncoder(readiness).Encode(map[string]any{
		"type": "ready", "protocol_version": 1, "endpoint": "http://" + local.Server.Addr,
		"token": local.Token, "version": version.Version, "data_dir": root,
	}); err != nil {
		return fmt.Errorf("send application readiness: %w", err)
	}
	closed := make(chan struct{})
	go func() { _, _ = io.Copy(io.Discard, parent); close(closed) }()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)
	select {
	case <-closed:
	case <-stop:
	}
	return nil
}
