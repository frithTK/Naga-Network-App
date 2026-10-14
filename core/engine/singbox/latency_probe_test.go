package singbox

import (
	"context"
	"os"
	"path/filepath"
	goruntime "runtime"
	"testing"
	"time"

	"naga.network/core/engine"
	"naga.network/core/profile"
)

// Opt-in integration check uses the packaged binary and only loopback API
// traffic. No subscription credentials or external probe requests are needed.
func TestLatencyProbeNativeStartIsolation(t *testing.T) {
	binary := os.Getenv("NAGA_TEST_SINGBOX")
	if binary == "" {
		t.Skip("set NAGA_TEST_SINGBOX to check the native packaged engine")
	}
	config := []byte(`{"inbounds":[{"type":"mixed","listen":"127.0.0.1","listen_port":2080}],"outbounds":[{"type":"urltest","tag":"auto","outbounds":["leaf"],"url":"https://www.gstatic.com/generate_204","interval":"3m","tolerance":50},{"type":"socks","tag":"leaf","server":"127.0.0.1","server_port":65535}],"route":{"final":"auto","rule_set":[{"tag":"unused","type":"remote","format":"binary","url":"https://example.invalid/unneeded.srs"}]}}`)
	minimal, err := profile.PrepareLatencyProbeConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	liveDir := t.TempDir()
	livePath := filepath.Join(liveDir, "config.json")
	if err := os.WriteFile(livePath, []byte("live session sentinel"), 0600); err != nil {
		t.Fatal(err)
	}
	adapter := Adapter{BinaryPath: binary, ConfigDir: liveDir}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := adapter.Validate(ctx, minimal); err != nil {
		t.Fatal(err)
	}
	runtime, err := adapter.StartProbe(ctx, minimal)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Stop()
	if runtime.Status() != engine.Connected {
		t.Fatal("probe engine not ready")
	}
	ownedDir := runtime.(*processRuntime).configDir
	if ownedDir == liveDir {
		t.Fatal("probe shares live config directory")
	}
	if err := runtime.Stop(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ownedDir); !os.IsNotExist(err) {
		t.Fatal("temporary probe directory remains", err)
	}
	if data, err := os.ReadFile(livePath); err != nil || string(data) != "live session sentinel" {
		t.Fatal("live configuration overwritten", err)
	}
	if goruntime.GOOS == "linux" {
		before, err := os.ReadDir("/proc/self/fd")
		if err != nil {
			t.Fatal(err)
		}
		goroutines := goruntime.NumGoroutine()
		for i := 0; i < 5; i++ {
			probe, err := adapter.StartProbe(ctx, minimal)
			if err != nil {
				t.Fatal(err)
			}
			dir := probe.(*processRuntime).configDir
			if err := probe.Stop(); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(dir); !os.IsNotExist(err) {
				t.Fatal("probe directory remains after repeat", err)
			}
		}
		deadline := time.Now().Add(2 * time.Second)
		for {
			after, err := os.ReadDir("/proc/self/fd")
			if err != nil {
				t.Fatal(err)
			}
			if len(after) <= len(before)+2 && goruntime.NumGoroutine() <= goroutines+3 {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("repeat leaks resources: FD %d -> %d; goroutines %d -> %d", len(before), len(after), goroutines, goruntime.NumGoroutine())
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}
