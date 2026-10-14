package ruleset

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestExpiredCacheRefreshAndOfflineFallback(t *testing.T) {
	for _, offline := range []bool{false, true} {
		t.Run(map[bool]string{false: "refresh", true: "offline"}[offline], func(t *testing.T) {
			dir := t.TempDir()
			url := "https://example.invalid/ru.json"
			path := CachePath(dir, url)
			old := []byte(`{"version":1,"rules":[{"domain":"old.example"}]}`)
			if err := os.WriteFile(path, old, 0600); err != nil {
				t.Fatal(err)
			}
			past := time.Now().Add(-30 * 24 * time.Hour)
			if err := os.Chtimes(path, past, past); err != nil {
				t.Fatal(err)
			}
			f := &stubFetcher{body: []byte(`{"version":1,"rules":[{"domain":"new.example"}]}`)}
			if offline {
				f.err = errors.New("offline")
			}
			r := NewResolver(dir, f)
			out, _, err := r.Materialize(context.Background(), sampleConfig("source", url))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(out), `"type":"local"`) || f.callCount() != 1 {
				t.Fatal("no refresh or lost offline fallback")
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			want := "new.example"
			if offline {
				want = "old.example"
			}
			if !strings.Contains(string(got), want) {
				t.Fatalf("cache = %s", got)
			}
		})
	}
}
func TestConcurrentMaterializeSharesOneFetch(t *testing.T) {
	f := &stubFetcher{body: []byte(`{"version":1,"rules":[]}`), block: make(chan struct{})}
	r := NewResolver(t.TempDir(), f)
	r.ColdWait = time.Second
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := r.Materialize(context.Background(), sampleConfig("source", "https://example.invalid/shared.json"))
			if err != nil {
				t.Error(err)
			}
		}()
	}
	deadline := time.Now().Add(time.Second)
	for f.callCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	close(f.block)
	wg.Wait()
	if f.callCount() != 1 {
		t.Fatalf("fetches=%d", f.callCount())
	}
}
func TestRuleSetIntervalDefaultsAndDayUnit(t *testing.T) {
	for _, raw := range []string{"1d", "24h", "2h30m"} {
		if _, err := ruleSetInterval(raw); err != nil {
			t.Fatal(raw, err)
		}
	}
	for _, raw := range []string{"-1d", "0h", "366d", "NaNd", "1e100d"} {
		if duration, err := ruleSetInterval(raw); err == nil && duration > 0 {
			t.Fatalf("accepted %s", raw)
		}
	}
	r := NewResolver(t.TempDir(), nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() { r.Watch(ctx, sampleConfig("source", "https://example.invalid/a.json")); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("watch ignored cancellation")
	}
}

func TestCacheFormatsAreSeparateAndLegacyBinaryRemainsUsable(t *testing.T) {
	dir := t.TempDir()
	url := "https://example.invalid/shared"
	if CachePathForFormat(dir, url, "source") == CachePathForFormat(dir, url, "binary") {
		t.Fatal("cache format collision")
	}
	if err := os.WriteFile(CachePath(dir, url), []byte("SRS\x01legacy"), 0600); err != nil {
		t.Fatal(err)
	}
	r := NewResolver(dir, &stubFetcher{err: errors.New("offline")})
	out, stats, err := r.Materialize(context.Background(), sampleConfig("binary", url))
	if err != nil || stats.Cache != 1 || !strings.Contains(string(out), `"type":"local"`) {
		t.Fatalf("lost legacy cache: %v %v %s", stats, err, out)
	}
}
func TestSourcePayloadRequiresRuleSetEnvelope(t *testing.T) {
	for _, raw := range []string{`{}`, `[]`, `{"version":99,"rules":[]}`, `{"version":1,"rules":null}`, `{"version":1,"rules":[true]}`} {
		if validPayload([]byte(raw), "source") {
			t.Fatalf("accepted %s", raw)
		}
	}
}
