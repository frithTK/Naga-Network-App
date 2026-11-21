package network

import (
	"context"
	"sync"
	"testing"
	"time"

	"naga.network/core/policy"
)

func TestWatcherDebouncesBurstToOneChange(t *testing.T) {
	var mu sync.Mutex
	current := policy.NetworkWiFi
	var got []policy.NetworkClass

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go Watcher{
		Poll:     8 * time.Millisecond,
		Debounce: 40 * time.Millisecond,
		Class: func(context.Context) policy.NetworkClass {
			mu.Lock()
			defer mu.Unlock()
			return current
		},
		OnChange: func(_, to policy.NetworkClass) {
			mu.Lock()
			got = append(got, to)
			mu.Unlock()
		},
	}.Run(ctx)

	time.Sleep(12 * time.Millisecond)
	mu.Lock()
	current = policy.NetworkEthernet
	mu.Unlock()
	time.Sleep(10 * time.Millisecond)
	mu.Lock()
	current = policy.NetworkWiFi
	mu.Unlock()
	time.Sleep(10 * time.Millisecond)
	mu.Lock()
	current = policy.NetworkEthernet
	mu.Unlock()
	time.Sleep(80 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 || got[0] != policy.NetworkEthernet {
		t.Fatalf("changes = %#v, want a single ethernet event", got)
	}
}

func TestWatcherIgnoresUnknownClass(t *testing.T) {
	var mu sync.Mutex
	current := policy.NetworkWiFi
	changes := 0

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go Watcher{
		Poll:     8 * time.Millisecond,
		Debounce: 20 * time.Millisecond,
		Class: func(context.Context) policy.NetworkClass {
			mu.Lock()
			defer mu.Unlock()
			return current
		},
		OnChange: func(_, _ policy.NetworkClass) {
			mu.Lock()
			changes++
			mu.Unlock()
		},
	}.Run(ctx)

	time.Sleep(12 * time.Millisecond)
	mu.Lock()
	current = policy.NetworkUnknown
	mu.Unlock()
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if changes != 0 {
		t.Fatalf("unknown class triggered %d changes", changes)
	}
}

func TestWatcherEmitsWiFiIdentityChange(t *testing.T) {
	var mu sync.Mutex
	current := Observation{Class: policy.NetworkWiFi, ID: "wifi-one"}
	first := make(chan struct{}, 1)
	got := make(chan Observation, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go Watcher{
		Poll: 5 * time.Millisecond, Debounce: 15 * time.Millisecond,
		Observe: func(context.Context) Observation {
			mu.Lock()
			defer mu.Unlock()
			select {
			case first <- struct{}{}:
			default:
			}
			return current
		},
		OnObservation: func(_, next Observation) { got <- next },
	}.Run(ctx)
	select {
	case <-first:
	case <-time.After(time.Second):
		t.Fatal("first network was not sampled")
	}
	mu.Lock()
	current = Observation{Class: policy.NetworkWiFi, ID: "wifi-two"}
	mu.Unlock()
	select {
	case next := <-got:
		if next.Class != policy.NetworkWiFi || next.ID != "wifi-two" {
			t.Fatalf("observation: %#v", next)
		}
	case <-time.After(time.Second):
		t.Fatal("same-class Wi-Fi transition was not reported")
	}
}
