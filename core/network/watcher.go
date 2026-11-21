package network

import (
	"context"
	"time"

	"naga.network/core/policy"
)

type Observation struct {
	Class policy.NetworkClass
	ID    string
}

// Watcher emits a network change after an observation has been stable for
// Debounce. Observers with an ID can detect transitions within one class.
type Watcher struct {
	Poll          time.Duration
	Debounce      time.Duration
	Class         func(context.Context) policy.NetworkClass
	OnChange      func(from, to policy.NetworkClass)
	Observe       func(context.Context) Observation
	OnObservation func(from, to Observation)
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
	if (w.Class == nil && w.Observe == nil) || (w.OnChange == nil && w.OnObservation == nil) {
		return
	}

	ticker := time.NewTicker(poll)
	defer ticker.Stop()

	emitted := Observation{Class: policy.NetworkUnknown}
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

	read := func() Observation {
		readCtx, cancel := context.WithTimeout(ctx, poll)
		defer cancel()
		if w.Observe != nil {
			return w.Observe(readCtx)
		}
		return Observation{Class: w.Class(readCtx)}
	}

	flush := func() {
		timerActive = false
		if pending == emitted || pending.Class == policy.NetworkUnknown || pending.Class == "" {
			return
		}
		from := emitted
		emitted = pending
		if w.OnChange != nil && from.Class != emitted.Class {
			w.OnChange(from.Class, emitted.Class)
		}
		if w.OnObservation != nil {
			w.OnObservation(from, emitted)
		}
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
