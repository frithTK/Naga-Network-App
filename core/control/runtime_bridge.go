package control

import (
	"context"
	"errors"
	"time"

	"naga.network/core/androidvpn"
	"naga.network/core/engine"
)

type bridgeRuntime struct {
	engine.Runtime
	bridge *androidvpn.Bridge
}

func (r *bridgeRuntime) Status() engine.Status {
	if r.bridge != nil && r.bridge.Failure() != nil {
		return engine.Failed
	}
	return r.Runtime.Status()
}
func (r *bridgeRuntime) Failure() error {
	if r.bridge != nil {
		if err := r.bridge.Failure(); err != nil {
			return err
		}
	}
	if reporter, ok := r.Runtime.(engine.FailureReporter); ok {
		return reporter.Failure()
	}
	return nil
}
func (r *bridgeRuntime) Select(ctx context.Context, tag string) error {
	if switcher, ok := r.Runtime.(engine.SelectorSwitcher); ok {
		return switcher.Select(ctx, tag)
	}
	return errors.New("runtime does not support node selection")
}
func (r *bridgeRuntime) Probe(ctx context.Context, tag string, timeout time.Duration) (int, error) {
	if prober, ok := r.Runtime.(engine.OutboundProber); ok {
		return prober.Probe(ctx, tag, timeout)
	}
	return 0, errors.New("runtime does not support node probes")
}

// ProbeOnce preserves the manual list's single-shot contract across the TUN wrapper.
func (r *bridgeRuntime) ProbeOnce(ctx context.Context, tag string, timeout time.Duration) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if r.bridge != nil {
		if err := r.bridge.Failure(); err != nil {
			return 0, err
		}
	}
	if status := r.Runtime.Status(); status == engine.Stopped || status == engine.Failed {
		return 0, ErrRuntimeNotConnected
	}
	return probeOutboundOnce(r.Runtime, ctx, tag, timeout)
}
func (r *bridgeRuntime) Details() engine.RuntimeDetails {
	if reporter, ok := r.Runtime.(engine.DetailReporter); ok {
		return reporter.Details()
	}
	return engine.RuntimeDetails{}
}
