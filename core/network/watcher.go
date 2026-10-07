package network

import (
	"context"
	"time"

	"naga.network/core/policy"
)

// Watcher polls the current network class and emits a single change after
// the class has been stable for Debounce. Unknown is used as the initial
// baseline so the first non-unknown reading is treated as a change.
type Watcher struct {
	Poll     time.Duration
	Debounce time.Duration
	Class    func(context.Context) policy.NetworkClass
	OnChange func(from, to policy.NetworkClass)
}

// Run blocks until the context is cancelled.
func (w Watcher) Run(ctx context.Context) {
	poll := w.Poll
	if poll <= 0 {
		poll = policy.NetworkPollInterval
	}
	debounce := w.Debounce
	if debounce <= 0 {
		debounce = policy.NetworkChangeDebounce
	}
	if w.Class == nil || w.OnChange == nil {
		return
	}

	ticker := time.NewTicker(poll)
	defer ticker.Stop()

	emitted := policy.NetworkUnknown
	pending := emitted
	armed := false
	timer := time.NewTimer(debounce)
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timerActive := false

	read := func() policy.NetworkClass {
		readCtx, cancel := context.WithTimeout(ctx, poll)
		defer cancel()
		return w.Class(readCtx)
	}

	flush := func() {
		timerActive = false
		if pending == emitted || pending == policy.NetworkUnknown {
			return
		}
		from := emitted
		emitted = pending
		w.OnChange(from, emitted)
	}

	for {
		select {
		case <-ctx.Done():
			if !timer.Stop() && timerActive {
				select {
				case <-timer.C:
				default:
				}
			}
			return
		case <-ticker.C:
			next := read()
			if !armed {
				emitted = next
				pending = next
				armed = true
				continue
			}
			if next == pending {
				continue
			}
			pending = next
			if timerActive {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
			}
			timer.Reset(debounce)
			timerActive = true
		case <-timer.C:
			flush()
		}
	}
}
