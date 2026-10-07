package singbox

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"naga.network/core/engine"
)

type fakeProc struct {
	mu    sync.Mutex
	stops int
	kills int
	wait  chan error
}

func newFakeProc() *fakeProc {
	return &fakeProc{wait: make(chan error, 1)}
}

func (p *fakeProc) Wait() error {
	return <-p.wait
}

func (p *fakeProc) SignalStop() error {
	p.mu.Lock()
	p.stops++
	p.mu.Unlock()
	select {
	case p.wait <- nil:
	default:
	}
	return nil
}

func (p *fakeProc) Kill() error {
	p.mu.Lock()
	p.kills++
	p.mu.Unlock()
	select {
	case p.wait <- nil:
	default:
	}
	return nil
}

func TestAdapterMapsUACCancellation(t *testing.T) {
	adapter := Adapter{
		BinaryPath: "sing-box.exe",
		Args:       []string{"run", "-c", "unused.json"},
		Starter: func(context.Context, processRequest) (startedProcess, error) {
			return nil, ErrUACCancelled
		},
	}
	_, err := adapter.Start(context.Background(), []byte(`{"outbounds":[{"type":"direct"}]}`))
	if !errors.Is(err, ErrUACCancelled) {
		t.Fatalf("Start() error = %v, want ErrUACCancelled", err)
	}
	if err.Error() != ErrUACCancelled.Error() {
		t.Fatalf("UAC error text = %q", err.Error())
	}
}

func TestAdapterStopClosesHostOnce(t *testing.T) {
	proc := newFakeProc()
	adapter := Adapter{
		BinaryPath: "sing-box.exe",
		Args:       []string{"run", "-c", "unused.json"},
		Starter: func(context.Context, processRequest) (startedProcess, error) {
			return proc, nil
		},
	}
	runtime, err := adapter.Start(context.Background(), []byte(`{"outbounds":[{"type":"direct"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Stop(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for runtime.Status() != engine.Stopped && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	proc.mu.Lock()
	defer proc.mu.Unlock()
	if proc.stops != 1 {
		t.Fatalf("SignalStop calls = %d, want 1", proc.stops)
	}
	if proc.kills != 0 {
		t.Fatalf("Kill calls = %d, want 0", proc.kills)
	}
}

func TestAdapterDoesNotKeepProcessWhenStarterFails(t *testing.T) {
	adapter := Adapter{
		BinaryPath: "sing-box.exe",
		Args:       []string{"run", "-c", "unused.json"},
		Starter: func(context.Context, processRequest) (startedProcess, error) {
			return nil, errors.New("handshake failed before process")
		},
	}
	runtime, err := adapter.Start(context.Background(), []byte(`{"outbounds":[{"type":"direct"}]}`))
	if err == nil || runtime != nil {
		t.Fatal("Start() must fail without a runtime when the host handshake fails")
	}
}
