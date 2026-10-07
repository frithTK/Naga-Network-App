// Package ruleset downloads remote sing-box rule-sets into local cache files
// so the VPN runtime never fetches them over the stock Go HTTPS stack.
package ruleset

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
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
	Tag    string
	URL    string
	Format string
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
}

func NewResolver(dir string, fetcher Fetcher) *Resolver {
	return &Resolver{Dir: dir, Fetcher: fetcher, ColdWait: DefaultColdWait}
}

func CachePath(dir, rawURL string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(rawURL)))
	return filepath.Join(dir, hex.EncodeToString(sum[:]))
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
		out = append(out, RemoteSet{
			Tag:    tag,
			URL:    rawURL,
			Format: ruleSetFormat(item, rawURL),
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
		cache := CachePath(r.Dir, remote.URL)
		if cached, ok := readCache(cache); ok && validPayload(cached, remote.Format) {
			abs, err := filepath.Abs(cache)
			if err != nil {
				stats.Skipped++
				continue
			}
			paths[remote.Tag] = abs
			stats.Cache++
			continue
		}
		misses++
		if r.Fetcher == nil {
			stats.Skipped++
			continue
		}
		wg.Add(1)
		go func(remote RemoteSet, cache string) {
			defer wg.Done()
			body, err := r.Fetcher.FetchBytes(ctx, remote.URL)
			if err != nil || !validPayload(body, remote.Format) {
				mu.Lock()
				stats.Skipped++
				mu.Unlock()
				return
			}
			if err := writeCache(cache, body); err != nil {
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

	out, err := ApplyLocal(config, paths)
	if err != nil {
		return config, stats, err
	}
	return out, stats, nil
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
	if len(body) == 0 {
		return false
	}
	if format == "source" {
		return json.Valid(body)
	}
	return len(body) >= 4 && bytes.HasPrefix(body, []byte(srsMagic))
}

func readCache(path string) ([]byte, bool) {
	body, err := os.ReadFile(path)
	if err != nil || len(body) == 0 {
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
