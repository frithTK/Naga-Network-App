package amneziawg

import (
	"context"
	"errors"
	"net"
	"sync"

	awgcfg "naga.network/core/profile/amneziawg"
)

// LoopbackSidecar is a test double: SOCKS CONNECT dials the host network.
type LoopbackSidecar struct {
	mu      sync.Mutex
	server  *socksServer
	addr    string
	started bool
}

func NewLoopbackSidecar() *LoopbackSidecar {
	return &LoopbackSidecar{}
}

func (s *LoopbackSidecar) Start(_ context.Context, _ awgcfg.Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return errors.New("amneziawg sidecar already started")
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	s.server = serveSOCKS5(listener, net.Dial)
	s.addr = s.server.Addr()
	s.started = true
	return nil
}

func (s *LoopbackSidecar) ListenAddr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.addr
}

func (s *LoopbackSidecar) Ready(ctx context.Context) error {
	s.mu.Lock()
	addr := s.addr
	started := s.started
	s.mu.Unlock()
	if !started || addr == "" {
		return ErrNotReady
	}
	return waitSOCKS(ctx, addr)
}

func (s *LoopbackSidecar) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.started = false
	s.addr = ""
	if s.server == nil {
		return nil
	}
	err := s.server.Close()
	s.server = nil
	return err
}

// FailingSidecar starts then reports not ready, for rollback tests.
type FailingSidecar struct{}

func (FailingSidecar) Start(context.Context, awgcfg.Config) error {
	return errors.New("amneziawg sidecar failed")
}
func (FailingSidecar) ListenAddr() string          { return "" }
func (FailingSidecar) Ready(context.Context) error { return ErrNotReady }
func (FailingSidecar) Stop() error                 { return nil }

// NoHandshakeSidecar starts SOCKS but Ready fails, so Adapter.Start is skipped.
type NoHandshakeSidecar struct {
	inner *LoopbackSidecar
}

func (s *NoHandshakeSidecar) Start(ctx context.Context, cfg awgcfg.Config) error {
	if s.inner == nil {
		s.inner = NewLoopbackSidecar()
	}
	return s.inner.Start(ctx, cfg)
}

func (s *NoHandshakeSidecar) ListenAddr() string {
	if s.inner == nil {
		return ""
	}
	return s.inner.ListenAddr()
}

func (s *NoHandshakeSidecar) Ready(context.Context) error {
	return ErrHandshakeTimeout
}

func (s *NoHandshakeSidecar) Stop() error {
	if s.inner == nil {
		return nil
	}
	return s.inner.Stop()
}
