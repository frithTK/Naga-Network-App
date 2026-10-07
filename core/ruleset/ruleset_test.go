package ruleset

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type stubFetcher struct {
	mu      sync.Mutex
	calls   int
	body    []byte
	err     error
	block   chan struct{}
	seenURL string
}

func (s *stubFetcher) FetchBytes(_ context.Context, rawURL string) ([]byte, error) {
	s.mu.Lock()
	s.calls++
	s.seenURL = rawURL
	block := s.block
	s.mu.Unlock()
	if block != nil {
		<-block
	}
	if s.err != nil {
		return nil, s.err
	}
	return append([]byte(nil), s.body...), nil
}

func (s *stubFetcher) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func sampleConfig(format, rawURL string) []byte {
	return []byte(`{"route":{"rule_set":[{"tag":"sub-ru","type":"remote","format":"` + format + `","url":"` + rawURL + `","download_detour":"direct"}],"rules":[{"rule_set":"sub-ru","outbound":"direct"}]}}`)
}

func TestListRemote(t *testing.T) {
	sets, err := ListRemote(sampleConfig("source", "https://example.invalid/ru.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(sets) != 1 || sets[0].Tag != "sub-ru" || sets[0].Format != "source" {
		t.Fatalf("sets = %#v", sets)
	}
}

func TestListRemoteInfersBinaryFromSRSSuffix(t *testing.T) {
	config := []byte(`{"route":{"rule_set":[{"tag":"geo","type":"remote","url":"https://example.invalid/geo.srs"}]}}`)
	sets, err := ListRemote(config)
	if err != nil {
		t.Fatal(err)
	}
	if len(sets) != 1 || sets[0].Format != "binary" {
		t.Fatalf("sets = %#v", sets)
	}
}

func TestMaterializeRewritesRemoteToLocal(t *testing.T) {
	dir := t.TempDir()
	fetcher := &stubFetcher{body: []byte(`{"version":1,"rules":[{"domain_suffix":[".example"]}]}`)}
	resolver := NewResolver(dir, fetcher)
	out, stats, err := resolver.Materialize(context.Background(), sampleConfig("source", "https://example.invalid/ru.json"))
	if err != nil {
		t.Fatal(err)
	}
	if stats.Fetched != 1 || stats.Cache != 0 {
		t.Fatalf("stats = %#v", stats)
	}
	var document map[string]any
	if err := json.Unmarshal(out, &document); err != nil {
		t.Fatal(err)
	}
	item := document["route"].(map[string]any)["rule_set"].([]any)[0].(map[string]any)
	if item["type"] != "local" {
		t.Fatalf("type = %#v", item["type"])
	}
	path, _ := item["path"].(string)
	if !filepath.IsAbs(path) {
		t.Fatalf("path is not absolute: %q", path)
	}
	if _, ok := item["url"]; ok {
		t.Fatal("url must be removed")
	}
	if _, ok := item["download_detour"]; ok {
		t.Fatal("download_detour must be removed")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("cache mode = %o", info.Mode().Perm())
	}
}

func TestMaterializeCacheHitDoesNotFetch(t *testing.T) {
	dir := t.TempDir()
	rawURL := "https://example.invalid/ru.json"
	body := []byte(`{"version":1,"rules":[{"domain_suffix":[".cached"]}]}`)
	if err := os.WriteFile(CachePath(dir, rawURL), body, 0o600); err != nil {
		t.Fatal(err)
	}
	fetcher := &stubFetcher{body: []byte(`{"version":1,"rules":[]}`), block: make(chan struct{})}
	resolver := NewResolver(dir, fetcher)
	started := time.Now()
	out, stats, err := resolver.Materialize(context.Background(), sampleConfig("source", rawURL))
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("cache hit blocked connect")
	}
	if stats.Cache != 1 || stats.Fetched != 0 || fetcher.callCount() != 0 {
		t.Fatalf("stats = %#v calls = %d", stats, fetcher.callCount())
	}
	if !strings.Contains(string(out), `"type":"local"`) {
		t.Fatalf("out = %s", out)
	}
}

func TestMaterializeColdMissDoesNotBlockConnect(t *testing.T) {
	dir := t.TempDir()
	block := make(chan struct{})
	t.Cleanup(func() { close(block) })
	fetcher := &stubFetcher{
		body:  []byte(`{"version":1,"rules":[{"domain_suffix":[".late"]}]}`),
		block: block,
	}
	resolver := NewResolver(dir, fetcher)
	resolver.ColdWait = 40 * time.Millisecond
	config := sampleConfig("source", "https://example.invalid/ru.json")
	started := time.Now()
	out, _, err := resolver.Materialize(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(started) > 400*time.Millisecond {
		t.Fatal("cold miss blocked connect")
	}
	if string(out) != string(config) {
		t.Fatal("timed-out fetch must fail-open")
	}
}

func TestMaterializeFailOpenOnFetchError(t *testing.T) {
	dir := t.TempDir()
	fetcher := &stubFetcher{err: errors.New("network down")}
	resolver := NewResolver(dir, fetcher)
	resolver.ColdWait = 50 * time.Millisecond
	config := sampleConfig("source", "https://example.invalid/ru.json")
	out, stats, err := resolver.Materialize(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Skipped != 1 {
		t.Fatalf("stats = %#v", stats)
	}
	if string(out) != string(config) {
		t.Fatalf("fail-open must keep remotes, got %s", out)
	}
}

func TestMaterializeRejectsInvalidPayload(t *testing.T) {
	dir := t.TempDir()
	fetcher := &stubFetcher{body: []byte("not-json")}
	resolver := NewResolver(dir, fetcher)
	resolver.ColdWait = 50 * time.Millisecond
	config := sampleConfig("source", "https://example.invalid/ru.json")
	out, stats, err := resolver.Materialize(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Skipped != 1 || stats.Fetched != 0 {
		t.Fatalf("stats = %#v", stats)
	}
	if string(out) != string(config) {
		t.Fatal("invalid payload must not be applied")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("invalid payload was cached: %v", entries)
	}
}

func TestMaterializeColdWaitDoesNotBlockOnCache(t *testing.T) {
	var fetches atomic.Int32
	dir := t.TempDir()
	resolver := NewResolver(dir, fetchFunc(func(context.Context, string) ([]byte, error) {
		fetches.Add(1)
		time.Sleep(30 * time.Millisecond)
		return []byte(`{"version":1,"rules":[{"domain_suffix":[".ok"]}]}`), nil
	}))
	resolver.ColdWait = 2 * time.Second
	_, stats, err := resolver.Materialize(context.Background(), sampleConfig("source", "https://example.invalid/ru.json"))
	if err != nil {
		t.Fatal(err)
	}
	if stats.Fetched != 1 || fetches.Load() != 1 {
		t.Fatalf("stats = %#v fetches = %d", stats, fetches.Load())
	}
}

type fetchFunc func(context.Context, string) ([]byte, error)

func (f fetchFunc) FetchBytes(ctx context.Context, rawURL string) ([]byte, error) {
	return f(ctx, rawURL)
}
