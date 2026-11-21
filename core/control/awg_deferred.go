package control

import (
	"context"
	"errors"
	"sync"

	awgengine "naga.network/core/engine/amneziawg"
	"naga.network/core/profile"
	awgcfg "naga.network/core/profile/amneziawg"
)

// Each standby keeps its loopback address stable for sing-box. Activation is
// reserved for select/probe operations, not traffic to an inactive outbound.
type deferredAWG struct {
	engine awgengine.PreparedSidecar
	cfg    awgcfg.Config
	life   context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
	ready  bool
}

func newDeferredAWG(ctx context.Context, sidecar awgengine.PreparedSidecar, cfg awgcfg.Config) *deferredAWG {
	life, cancel := context.WithCancel(ctx)
	return &deferredAWG{engine: sidecar, cfg: cfg, life: life, cancel: cancel}
}

func (s *deferredAWG) Start(ctx context.Context, _ awgcfg.Config) error {
	return s.engine.Prepare(ctx)
}

func (s *deferredAWG) ListenAddr() string { return s.engine.ListenAddr() }

func (s *deferredAWG) Ready(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.life.Err() != nil {
		return awgengine.ErrStopped
	}
	if err := context.Cause(ctx); err != nil {
		return err
	}
	if s.ready {
		return nil
	}
	ctx, cancel := context.WithTimeoutCause(ctx, handshakeReadyTimeout, awgengine.ErrHandshakeTimeout)
	defer cancel()
	stop := context.AfterFunc(s.life, cancel)
	defer stop()
	if err := s.engine.Start(ctx, s.cfg); err != nil {
		return err
	}
	if err := s.engine.Ready(ctx); err != nil {
		s.engine.Deactivate()
		if ctx.Err() != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
			return context.Cause(ctx)
		}
		return err
	}
	s.ready = true
	return nil
}

func (s *deferredAWG) ProbeHTTPS(ctx context.Context, target string) (int, error) {
	return s.engine.ProbeHTTPS(ctx, target)
}

func (s *deferredAWG) Stop() error {
	s.cancel()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.engine.Stop()
}

func deferUnselectedAWG(profileID, mode string) func(profile.Profile) bool {
	return func(value profile.Profile) bool {
		return value.ID != profileID || !awgSelectionMatches(value, mode)
	}
}

func (c *RuntimeController) activateStandbyAWGLocked(target string, standby *deferredAWG) error {
	c.operationMu.Lock()
	if c.stopPending != 0 || c.startCancel != nil {
		c.operationMu.Unlock()
		return errors.New("runtime transition already in progress")
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.startCancel = cancel
	c.operationMu.Unlock()
	defer func() {
		c.operationMu.Lock()
		c.startCancel = nil
		c.operationMu.Unlock()
		cancel()
	}()
	runtime := c.runtime
	c.mu.Unlock()
	err := standby.Ready(ctx)
	c.mu.Lock()
	if ctx.Err() != nil {
		return context.Cause(ctx)
	}
	if runtime != c.runtime || c.awgProbers[target] != standby {
		return ErrRuntimeNotConnected
	}
	return err
}
