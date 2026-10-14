// Package ruleset downloads remote sing-box rule-sets into local cache files
// so the VPN runtime never fetches them over the stock Go HTTPS stack.
package ruleset

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	DefaultColdWait = 8 * time.Second
	srsMagic        = "SRS"
)

// Fetcher downloads rule-set bodies. subscription.Client implements this
// with the same uTLS transport and URL restrictions as profile fetch.
type Fetcher interface {
	FetchBytes(ctx context.Context, rawURL string) ([]byte, error)
}

type RemoteSet struct {
	Tag            string
	URL            string
	Format         string
	UpdateInterval time.Duration
}

type Stats struct {
	Fetched int
	Cache   int
	Skipped int
}

type Resolver struct {
	Dir      string
	Fetcher  Fetcher
	ColdWait time.Duration
	Now      func() time.Time
	mu       sync.Mutex
	inflight map[string]*fetchResult
	slots    chan struct{}
}

func NewResolver(dir string, fetcher Fetcher) *Resolver {
	return &Resolver{Dir: dir, Fetcher: fetcher, ColdWait: DefaultColdWait}
}

func CachePath(dir, rawURL string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(rawURL)))
	return filepath.Join(dir, hex.EncodeToString(sum[:]))
}

// CachePathForFormat prevents a binary declaration from replacing the source
// file another live configuration is watching at the same URL.
func CachePathForFormat(dir, rawURL, format string) string {
	if format == "binary" {
		return CachePath(dir, strings.TrimSpace(rawURL)+"\x00binary")
	}
	return CachePath(dir, rawURL)
}

func ListRemote(config []byte) ([]RemoteSet, error) {
	var document map[string]any
	if err := json.Unmarshal(config, &document); err != nil {
		return nil, fmt.Errorf("decode rule-set config: %w", err)
	}
	route, _ := document["route"].(map[string]any)
	if route == nil {
		return nil, nil
	}
	rawSets, _ := route["rule_set"].([]any)
	if len(rawSets) > 2048 {
		return nil, errors.New("too many remote rule sets")
	}
	out := make([]RemoteSet, 0, len(rawSets))
	for _, raw := range rawSets {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if kind, _ := item["type"].(string); kind != "remote" {
			continue
		}
		tag, _ := item["tag"].(string)
		rawURL, _ := item["url"].(string)
		rawURL = strings.TrimSpace(rawURL)
		if tag == "" || rawURL == "" {
			continue
		}
		interval := 24 * time.Hour
		if raw, ok := item["update_interval"].(string); ok && raw != "" {
			var err error
			interval, err = ruleSetInterval(raw)
			if err != nil {
				return nil, err
			}
		}
		out = append(out, RemoteSet{
			Tag:            tag,
			URL:            rawURL,
			Format:         ruleSetFormat(item, rawURL),
			UpdateInterval: interval,
		})
	}
	return out, nil
}

func ApplyLocal(config []byte, paths map[string]string) ([]byte, error) {
	if len(paths) == 0 {
		return config, nil
	}
	var document map[string]any
	if err := json.Unmarshal(config, &document); err != nil {
		return nil, fmt.Errorf("decode rule-set config: %w", err)
	}
	route, _ := document["route"].(map[string]any)
	if route == nil {
		return config, nil
	}
	rawSets, _ := route["rule_set"].([]any)
	changed := false
	for i, raw := range rawSets {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if kind, _ := item["type"].(string); kind != "remote" {
			continue
		}
		tag, _ := item["tag"].(string)
		path, ok := paths[tag]
		if !ok || path == "" {
			continue
		}
		abs := path
		if !filepath.IsAbs(abs) {
			resolved, err := filepath.Abs(abs)
			if err != nil {
				continue
			}
			abs = resolved
		}
		format := ruleSetFormat(item, "")
		item["type"] = "local"
		item["path"] = abs
		item["format"] = format
		delete(item, "url")
		delete(item, "download_detour")
		delete(item, "update_interval")
		rawSets[i] = item
		changed = true
	}
	if !changed {
		return config, nil
	}
	route["rule_set"] = rawSets
	out, err := json.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("encode local rule-set config: %w", err)
	}
	return out, nil
}

func (r *Resolver) Materialize(ctx context.Context, config []byte) ([]byte, Stats, error) {
	var stats Stats
	if r == nil {
		return config, stats, nil
	}
	remotes, err := ListRemote(config)
	if err != nil {
		return config, stats, err
	}
	if len(remotes) == 0 {
		return config, stats, nil
	}
	if err := os.MkdirAll(r.Dir, 0o700); err != nil {
		stats.Skipped = len(remotes)
		return config, stats, err
	}

	paths := make(map[string]string, len(remotes))
	var mu sync.Mutex
	var wg sync.WaitGroup
	misses := 0
	wait := r.ColdWait
	if wait <= 0 {
		wait = DefaultColdWait
	}

	for _, remote := range remotes {
		cache := CachePathForFormat(r.Dir, remote.URL, remote.Format)
		if remote.Format == "binary" {
			if _, err := os.Stat(cache); errors.Is(err, os.ErrNotExist) {
				legacy := CachePath(r.Dir, remote.URL)
				if body, ok := readCache(legacy); ok && validPayload(body, "binary") {
					if info, err := os.Stat(legacy); err == nil {
						if err = writeCache(cache, body); err == nil {
							_ = os.Chtimes(cache, info.ModTime(), info.ModTime())
						}
					}
				}
			}
		}
		cached, cachedOK := readCache(cache)
		cacheUsable := cachedOK && validPayload(cached, remote.Format)
		if cacheUsable {
			abs, err := filepath.Abs(cache)
			if err != nil {
				mu.Lock()
				stats.Skipped++
				mu.Unlock()
				continue
			}
			mu.Lock()
			paths[remote.Tag] = abs
			stats.Cache++
			mu.Unlock()
			if r.fresh(cache, remote.UpdateInterval) {
				continue
			}
		}
		misses++
		if r.Fetcher == nil {
			mu.Lock()
			stats.Skipped++
			mu.Unlock()
			continue
		}
		wg.Add(1)
		go func(remote RemoteSet, cache string) {
			defer wg.Done()
			body, err := r.fetch(ctx, remote, cache)
			if err != nil || !validPayload(body, remote.Format) {
				mu.Lock()
				stats.Skipped++
				mu.Unlock()
				return
			}
			if !validPayload(body, remote.Format) {
				mu.Lock()
				stats.Skipped++
				mu.Unlock()
				return
			}
			abs, err := filepath.Abs(cache)
			if err != nil {
				mu.Lock()
				stats.Skipped++
				mu.Unlock()
				return
			}
			mu.Lock()
			paths[remote.Tag] = abs
			stats.Fetched++
			mu.Unlock()
		}(remote, cache)
	}

	if misses > 0 {
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-done:
		case <-timer.C:
		case <-ctx.Done():
		}
	}

	// Workers may still be warming the cache after ColdWait. Neither config
	// transformation nor the returned stats may share their mutable result.
	mu.Lock()
	resolved := make(map[string]string, len(paths))
	for tag, path := range paths {
		resolved[tag] = path
	}
	snapshot := stats
	mu.Unlock()
	out, err := ApplyLocal(config, resolved)
	if err != nil {
		return config, snapshot, err
	}
	return out, snapshot, nil
}

type fetchResult struct {
	done chan struct{}
	body []byte
	err  error
}

func ruleSetInterval(raw string) (time.Duration, error) {
	if strings.HasSuffix(raw, "d") {
		days, err := strconv.ParseFloat(strings.TrimSuffix(raw, "d"), 64)
		if err != nil || math.IsNaN(days) || math.IsInf(days, 0) || days <= 0 || days > 365 {
			return 0, errors.New("invalid rule-set update interval")
		}
		return time.Duration(days * float64(24*time.Hour)), nil
	}
	duration, err := time.ParseDuration(raw)
	if err != nil || duration <= 0 || duration > 365*24*time.Hour {
		return 0, errors.New("invalid rule-set update interval")
	}
	return duration, nil
}

func (r *Resolver) fresh(path string, interval time.Duration) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	now := time.Now()
	if r.Now != nil {
		now = r.Now()
	}
	age := now.Sub(info.ModTime())
	return age >= 0 && age < interval
}

func (r *Resolver) fetch(ctx context.Context, remote RemoteSet, cache string) ([]byte, error) {
	// Formats sharing a URL must not overwrite one another's cached payload.
	key := remote.URL + "\x00" + remote.Format
	r.mu.Lock()
	if r.inflight == nil {
		r.inflight = make(map[string]*fetchResult)
		r.slots = make(chan struct{}, 8)
	}
	if result := r.inflight[key]; result != nil {
		r.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-result.done:
			return result.body, result.err
		}
	}
	result := &fetchResult{done: make(chan struct{})}
	r.inflight[key] = result
	slots := r.slots
	r.mu.Unlock()
	defer func() { r.mu.Lock(); delete(r.inflight, key); close(result.done); r.mu.Unlock() }()
	select {
	case slots <- struct{}{}:
	case <-ctx.Done():
		result.err = ctx.Err()
		return nil, result.err
	}
	defer func() { <-slots }()
	// Another request may have filled the cache while this one waited.
	if body, ok := readCache(cache); ok && validPayload(body, remote.Format) && r.fresh(cache, remote.UpdateInterval) {
		result.body = body
		return body, nil
	}
	result.body, result.err = r.Fetcher.FetchBytes(ctx, remote.URL)
	if result.err == nil && !validPayload(result.body, remote.Format) {
		result.err = errors.New("invalid rule-set payload")
	}
	if result.err == nil {
		result.err = ctx.Err()
	}
	if result.err == nil {
		result.err = writeCache(cache, result.body)
	}
	return result.body, result.err
}

// Watch refreshes the original remote definitions while the runtime owns ctx.
// sing-box watches local rule-set replacements; no reconnect is necessary.
func (r *Resolver) Watch(ctx context.Context, config []byte) {
	remotes, err := ListRemote(config)
	if err != nil || len(remotes) == 0 {
		return
	}
	interval := time.Minute
	for _, remote := range remotes {
		if remote.UpdateInterval < interval {
			interval = remote.UpdateInterval
		}
	}
	// Avoid polling storms from hostile provider intervals.
	if interval < time.Minute {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		refreshCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		_, _, _ = r.Materialize(refreshCtx, config)
		cancel()
	}
}

func ruleSetFormat(item map[string]any, rawURL string) string {
	if format, _ := item["format"].(string); format == "binary" || format == "source" {
		return format
	}
	path := rawURL
	if parsed, err := url.Parse(rawURL); err == nil && parsed.Path != "" {
		path = parsed.Path
	}
	if strings.HasSuffix(strings.ToLower(path), ".srs") {
		return "binary"
	}
	return "source"
}

func validPayload(body []byte, format string) bool {
	if len(body) == 0 || len(body) > 8<<20 {
		return false
	}
	if format == "source" {
		var document struct {
			Version int               `json:"version"`
			Rules   []json.RawMessage `json:"rules"`
		}
		if json.Unmarshal(body, &document) != nil || document.Version < 1 || document.Version > 5 || document.Rules == nil {
			return false
		}
		for _, raw := range document.Rules {
			if len(raw) == 0 || raw[0] != '{' {
				return false
			}
		}
		return true
	}
	return len(body) >= 4 && bytes.HasPrefix(body, []byte(srsMagic))
}

func readCache(path string) ([]byte, bool) {
	file, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, (8<<20)+1))
	if err != nil || len(body) == 0 || len(body) > 8<<20 {
		return nil, false
	}
	return body, true
}

func writeCache(path string, body []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".ruleset-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
