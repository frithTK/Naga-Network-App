//go:build !windows

package singbox

import (
	"context"
	"testing"
	"time"

	"naga.network/core/engine"
)

func TestAdapterStartsAndStopsProcess(t *testing.T) {
	adapter := Adapter{BinaryPath: "/bin/sh", Args: []string{"-c", "sleep 10"}}
	runtime, err := adapter.Start(context.Background(), []byte(`{"outbounds":[{"type":"direct"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.Status() != engine.Connected {
		t.Fatalf("unexpected status: %s", runtime.Status())
	}
	if err := runtime.Stop(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for runtime.Status() != engine.Stopped && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if runtime.Status() != engine.Stopped {
		t.Fatalf("runtime did not stop: %s", runtime.Status())
	}
}

func TestAdapterReportsUnexpectedProcessExit(t *testing.T) {
	adapter := Adapter{BinaryPath: "/bin/sh", Args: []string{"-c", "exit 7"}}
	runtime, err := adapter.Start(context.Background(), []byte(`{"outbounds":[{"type":"direct"}]}`))
	if err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(time.Second)
	for runtime.Status() != engine.Failed && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if runtime.Status() != engine.Failed {
		t.Fatalf("expected failed runtime, got %s", runtime.Status())
	}
	reporter, ok := runtime.(engine.FailureReporter)
	if !ok || reporter.Failure() == nil {
		t.Fatal("runtime did not expose the process failure")
	}
}
