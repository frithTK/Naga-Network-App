package control

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/net/proxy"
	engineolc "naga.network/core/engine/olcrtc"
	"naga.network/core/profile"
	olcprofile "naga.network/core/profile/olcrtc"
)

const (
	// HTTP generate_204; HTTPS through SOCKS would inflate the displayed ping.
	olcProbeURL       = "http://www.gstatic.com/generate_204"
	olcMeasureTimeout = 24 * time.Second
)

func (c *RuntimeController) probeOlcRTC(ctx context.Context) (bool, error) {
	if c.Store == nil {
		return false, errors.New("runtime is not configured")
	}
	values, err := c.Store.List()
	if err != nil {
		return false, err
	}
	var target profile.Profile
	var found bool
	for _, value := range values {
		if profile.RuntimeOlcRTC(value.Config) != nil || olcprofile.IsDocument(value.Config) {
			target = value
			found = true
			break
		}
	}
	if !found {
		return false, nil
	}
	cfg := profile.RuntimeOlcRTC(target.Config)
	if cfg == nil {
		cfg, err = olcprofile.ParseDocument(target.Config)
		if err != nil {
			return true, err
		}
	}
	c.mu.Lock()
	live := c.olcProc != nil
	activeTag := c.activeRuntimeTag
	c.mu.Unlock()
	if live {
		tag := olcLiveProbeTag(cfg.Profiles, activeTag, target.SelectedMode)
		latency, err := measureSOCKSLatency(ctx, net.JoinHostPort(olcprofile.SOCKSHost, fmt.Sprint(olcprofile.SOCKSPort)), olcProbeURL)
		if tag != "" {
			c.recordOlcProbe(target.ID, tag, latency, err)
		}
		return true, err
	}
	if err := c.probeOlcRTCDetached(ctx, target, cfg); err != nil {
		return true, err
	}
	return true, nil
}

// olcLiveProbeTag is the list row that should receive a measurement of the
// already running tunnel. A shared failover session has no single row.
func olcLiveProbeTag(profiles []olcprofile.Profile, activeTag, selected string) string {
	tags := olcprofile.Tags(profiles)
	activeTag = strings.TrimSpace(activeTag)
	for _, tag := range tags {
		if tag == activeTag {
			return tag
		}
	}
	if index, ok := olcprofile.Index(profiles, selected); ok && index >= 0 {
		return tags[index]
	}
	if len(tags) == 1 {
		return tags[0]
	}
	return ""
}

func (c *RuntimeController) probeOlcRTCDetached(ctx context.Context, value profile.Profile, cfg *olcprofile.Config) error {
	if cfg == nil || len(cfg.Profiles) == 0 {
		return errors.New("olcrtc mode is not available")
	}
	binary, err := engineolc.DiscoverBinary(c.OlcRTCBinary)
	if err != nil {
		return err
	}
	tags := olcprofile.Tags(cfg.Profiles)
	attempted := 0
	for i, item := range cfg.Profiles {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !item.RuntimeAllowed {
			continue
		}
		attempted++
		one := *cfg
		one.Profiles = []olcprofile.Profile{item}
		latency, measureErr := c.measureOlcProfile(ctx, binary, value.ID, &one)
		c.recordOlcProbe(value.ID, tags[i], latency, measureErr)
	}
	if attempted == 0 {
		return errors.New("olcrtc mode is not available")
	}
	return nil
}

func (c *RuntimeController) measureOlcProfile(ctx context.Context, binary, profileID string, cfg *olcprofile.Config) (int, error) {
	built, err := olcprofile.Build(cfg)
	if err != nil {
		return 0, err
	}
	dir := filepath.Join(olcprofile.ConfigDir(c.Store.Root, profileID), "probe")
	yamlPath, err := olcprofile.WriteFile(dir, built)
	if err != nil {
		return 0, err
	}
	proc, err := engineolc.Start(ctx, binary, yamlPath)
	if err != nil {
		return 0, err
	}
	defer proc.Stop()
	readyCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	err = proc.Ready(readyCtx)
	cancel()
	if err != nil {
		return 0, err
	}
	measureCtx, cancel := context.WithTimeout(ctx, olcMeasureTimeout)
	defer cancel()
	return measureSOCKSLatency(measureCtx, net.JoinHostPort(olcprofile.SOCKSHost, fmt.Sprint(olcprofile.SOCKSPort)), olcProbeURL)
}

func (c *RuntimeController) recordOlcProbe(profileID, tag string, latency int, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry := ProbeCacheEntry{CheckedAt: time.Now().UTC(), Status: ProbeFailed}
	if err == nil && latency > 0 {
		entry.Status = ProbeHealthy
		entry.LatencyMS = latency
		c.olcLatency = latency
	}
	if c.probeCache == nil {
		c.probeCache = map[string]ProbeCacheEntry{}
	}
	if strings.TrimSpace(tag) == "" {
		tag = olcprofile.NodeTag
	}
	c.probeCache[profileID+"::"+tag] = entry
	c.probeCache[tag] = entry
}

func measureSOCKSLatency(ctx context.Context, socksAddr, rawURL string) (int, error) {
	dialer, err := proxy.SOCKS5("tcp", socksAddr, nil, proxy.Direct)
	if err != nil {
		return 0, err
	}
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if contextDialer, ok := dialer.(proxy.ContextDialer); ok {
				return contextDialer.DialContext(ctx, network, addr)
			}
			return dialer.Dial(network, addr)
		},
	}
	client := &http.Client{Timeout: 12 * time.Second, Transport: transport}
	return measureWarmHTTPLatency(ctx, client, rawURL)
}

// measureWarmHTTPLatency issues two GETs on the same client so the second
// shot can reuse a keep-alive connection. Latency is the second success.
// First failure fails the node; a cancelled parent or second failure keeps
// the first; otherwise the second is returned even when it is larger.
func measureWarmHTTPLatency(ctx context.Context, client *http.Client, rawURL string) (int, error) {
	first, err := measureHTTPLatency(ctx, client, rawURL)
	if err != nil {
		return 0, err
	}
	if ctx.Err() != nil {
		return first, nil
	}
	second, err := measureHTTPLatency(ctx, client, rawURL)
	if err != nil || second <= 0 {
		return first, nil
	}
	return second, nil
}

func measureHTTPLatency(ctx context.Context, client *http.Client, rawURL string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, err
	}
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 400 {
		return 0, fmt.Errorf("probe status %d", resp.StatusCode)
	}
	elapsed := time.Since(start)
	if elapsed < time.Millisecond {
		return 1, nil
	}
	return int(elapsed.Milliseconds()), nil
}
