//go:build darwin || linux

package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAppSessionChild(t *testing.T) {
	if os.Getenv("NAGA_APP_SESSION_TEST_CHILD") != "1" {
		return
	}
	if err := runAppSession(os.Stdin, os.Stdout, ""); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

type sessionTestFrame struct {
	Type            string
	Endpoint        string
	Token           string
	Version         string
	ProtocolVersion int `json:"protocol_version"`
	Message         string
}

func startSessionTestChild(t *testing.T, root string) (*exec.Cmd, io.WriteCloser, sessionTestFrame) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestAppSessionChild$")
	child.Env = append(os.Environ(), "NAGA_APP_SESSION_TEST_CHILD=1", "NAGA_DATA_DIR="+root)
	input, err := child.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = input.Close(); _ = child.Process.Kill(); _ = child.Wait() })
	var frame sessionTestFrame
	if err := json.NewDecoder(io.LimitReader(output, 16*1024)).Decode(&frame); err != nil {
		t.Fatalf("read session frame: %v", err)
	}
	return child, input, frame
}

func TestAppSessionOwnsEndpointAuthLockAndParentLifetime(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Пользователь с пробелами")
	child, input, frame := startSessionTestChild(t, root)
	if frame.Type != "ready" || frame.ProtocolVersion != 1 || len(frame.Token) != 64 ||
		!strings.HasPrefix(frame.Endpoint, "http://127.0.0.1:") {
		t.Fatal("invalid readiness contract")
	}
	client := &http.Client{Timeout: 2 * time.Second}
	for _, tc := range []struct {
		token, origin string
		want          int
	}{
		{"", "", http.StatusUnauthorized},
		{"wrong-token", "", http.StatusUnauthorized},
		{frame.Token, "https://untrusted.example", http.StatusForbidden},
		{frame.Token, "", http.StatusOK},
	} {
		req, _ := http.NewRequest(http.MethodGet, frame.Endpoint+"/v1/health", nil)
		if tc.token != "" {
			req.Header.Set("Authorization", "Bearer "+tc.token)
		}
		if tc.origin != "" {
			req.Header.Set("Origin", tc.origin)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != tc.want {
			t.Fatalf("health status = %d, want %d", resp.StatusCode, tc.want)
		}
	}
	req, _ := http.NewRequest(http.MethodPost, frame.Endpoint+"/v1/runtime/start", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+frame.Token)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatal("unfinished VPN must be unavailable before runtime mutation")
	}
	second, _, rejected := startSessionTestChild(t, root)
	if rejected.Type != "error" || !strings.Contains(rejected.Message, "already using") {
		t.Fatal("second backend must not adopt the first endpoint")
	}
	if err := second.Wait(); err == nil {
		t.Fatal("second backend must fail")
	}
	if err := input.Close(); err != nil {
		t.Fatal(err)
	}
	if err := child.Wait(); err != nil {
		t.Fatalf("parent EOF cleanup: %v", err)
	}
	if _, err := client.Get(frame.Endpoint + "/v1/health"); err == nil {
		t.Fatal("listener survived parent EOF")
	}
	third, nextInput, restarted := startSessionTestChild(t, root)
	if restarted.Type != "ready" || restarted.Token != frame.Token {
		t.Fatal("restart must retain owner token and release the session lock")
	}
	nextInput.Close()
	if err := third.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestAppSessionRejectsPublicAndSymlinkTokens(t *testing.T) {
	for _, symlink := range []bool{false, true} {
		root := t.TempDir()
		token := filepath.Join(root, "control.token")
		if symlink {
			target := filepath.Join(t.TempDir(), "outside.token")
			if err := os.WriteFile(target, []byte("outside"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, token); err != nil {
				t.Fatal(err)
			}
		} else {
			if err := os.WriteFile(token, []byte("public"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		child, _, frame := startSessionTestChild(t, root)
		if frame.Type != "error" {
			t.Fatal("unsafe token was accepted")
		}
		if err := child.Wait(); err == nil {
			t.Fatal("unsafe token must fail startup")
		}
	}
}
