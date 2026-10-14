package ruleset

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestMaterializeConcurrentCacheHitsAndFetch(t *testing.T) {
	dir := t.TempDir()
	body := []byte(`{"version":1,"rules":[]}`)
	const cachedURL = "https://example.invalid/cached.json"
	if err := os.WriteFile(CachePath(dir, cachedURL), body, 0o600); err != nil {
		t.Fatal(err)
	}
	sets := []map[string]any{{"type": "remote", "tag": "download", "format": "source", "url": "https://example.invalid/fresh.json"}}
	for i := range 500 {
		sets = append(sets, map[string]any{"type": "remote", "tag": fmt.Sprintf("cached-%d", i), "format": "source", "url": cachedURL})
	}
	config, err := json.Marshal(map[string]any{"route": map[string]any{"rule_set": sets}})
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	resolver := NewResolver(dir, fetchFunc(func(context.Context, string) ([]byte, error) { close(entered); <-release; return body, nil }))
	done := make(chan error, 1)
	go func() {
		_, stats, err := resolver.Materialize(context.Background(), config)
		if err == nil && (stats.Cache != 500 || stats.Fetched != 1) {
			err = &countMismatch{stats}
		}
		done <- err
	}()
	select {
	case <-entered:
		close(release)
	case <-time.After(5 * time.Second):
		t.Fatal("fetch did not start")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

type countMismatch struct{ stats Stats }

func (e *countMismatch) Error() string {
	return fmt.Sprintf("concurrent cache/fetch counts do not match: %+v", e.stats)
}

func TestMaterializeReturnedStatsStayStableAfterLateFetch(t *testing.T) {
	dir := t.TempDir()
	entered, release := make(chan struct{}), make(chan struct{})
	resolver := NewResolver(dir, fetchFunc(func(context.Context, string) ([]byte, error) {
		close(entered)
		<-release
		return []byte(`{"version":1,"rules":[]}`), nil
	}))
	resolver.ColdWait = 10 * time.Millisecond
	url := "https://example.invalid/late.json"
	config := sampleConfig("source", url)
	out, stats, err := resolver.Materialize(context.Background(), config)
	if err != nil {
		close(release)
		t.Fatal(err)
	}
	<-entered
	close(release)
	if string(out) != string(config) || stats.Fetched != 0 {
		t.Fatalf("timeout snapshot changed: %s %+v", out, stats)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(CachePath(dir, url)); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("late fetch did not populate cache")
		}
		time.Sleep(time.Millisecond)
	}
	if stats.Fetched != 0 {
		t.Fatal("returned stats changed after late fetch")
	}
}
