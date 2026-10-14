package control

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"naga.network/core/policy"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/proxy"
	engineolc "naga.network/core/engine/olcrtc"
	"naga.network/core/profile"
	olcprofile "naga.network/core/profile/olcrtc"
)

const (
	// Same HTTPS endpoint/metric as the sing-box outbound URL test.
	olcProbeURL       = "https://www.gstatic.com/generate_204"
	olcMeasureTimeout = 24 * time.Second
)

func (c *RuntimeController) probeOlcRTC(ctx context.Context) (bool, error) {
	if probeJobFromContext(ctx) != nil {
		return c.probeOlcRTCJob(ctx)
	}
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
		if skipDetachedOlcProbe(err) {
			return false, nil
		}
		return true, err
	}
	return true, nil
}

func skipDetachedOlcProbe(err error) bool {
	return errors.Is(err, engineolc.ErrBinaryNotFound) || errors.Is(err, engineolc.ErrVersion)
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

// Each detached probe owns a private directory and an ephemeral loopback port.
// Port selection releases the reservation before exec so the child can bind it;
// a failed startup gets one fresh-port retry, bounded by the same job context.
func (c *RuntimeController) measureOlcProfile(ctx context.Context, binary, profileID string, cfg *olcprofile.Config) (int, error) {
	var last error
	for attempt := 0; attempt < 2; attempt++ {
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return 0, err
		}
		port := listener.Addr().(*net.TCPAddr).Port
		_ = listener.Close()
		latency, started, err := c.measureOlcAtPort(ctx, binary, cfg, port)
		if err == nil || started {
			return latency, err
		}
		last = err
	}
	return 0, last
}

func (c *RuntimeController) measureOlcAtPort(ctx context.Context, binary string, cfg *olcprofile.Config, port int) (int, bool, error) {
	built, err := olcprofile.BuildAtPort(cfg, port)
	if err != nil {
		return 0, false, err
	}
	dir, err := os.MkdirTemp("", "naga-olcrtc-probe-")
	if err != nil {
		return 0, false, err
	}
	processOwnsDir := false
	defer func() {
		if !processOwnsDir {
			_ = os.RemoveAll(dir)
		}
	}()
	yamlPath, err := olcprofile.WriteFile(dir, built)
	if err != nil {
		return 0, false, err
	}
	proc, err := engineolc.Start(ctx, binary, yamlPath)
	if err != nil {
		return 0, false, err
	}
	processOwnsDir = true
	defer c.stopOwnedProbeResource(ctx, func() error {
		err := proc.Stop()
		if errors.Is(err, engineolc.ErrStopTimeout) {
			<-proc.Done()
		}
		return errors.Join(err, os.RemoveAll(dir))
	})
	address := net.JoinHostPort(olcprofile.SOCKSHost, strconv.Itoa(port))
	readyCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	err = proc.ReadyAt(readyCtx, address)
	cancel()
	if err != nil {
		return 0, false, err
	}
	measureCtx, cancel := context.WithTimeout(ctx, olcMeasureTimeout)
	defer cancel()
	latency, err := measureSOCKSLatency(measureCtx, address, olcProbeURL)
	return latency, true, err
}

func (c *RuntimeController) probeOlcRTCJob(ctx context.Context) (bool, error) {
	if c.Store == nil {
		return false, errors.New("runtime is not configured")
	}
	values, err := c.Store.List()
	if err != nil {
		return false, err
	}
	c.mu.Lock()
	live, liveID, activeTag := c.olcProc != nil, c.profileID, c.activeRuntimeTag
	c.mu.Unlock()
	var ran bool
	for _, value := range values {
		if ctx.Err() != nil {
			return ran, ctx.Err()
		}
		if value.Subscription.Expired(nowUTC()) {
			continue
		}
		profiles := olcProfiles(value.Config)
		if len(profiles) == 0 {
			continue
		}
		cfg := &olcprofile.Config{Profiles: profiles}
		tags := olcprofile.Tags(cfg.Profiles)
		liveTag := ""
		if live && liveID == value.ID {
			liveTag = olcLiveProbeTag(cfg.Profiles, activeTag, value.SelectedMode)
		}
		binary, binaryErr := engineolc.DiscoverBinary(c.OlcRTCBinary)
		for i, item := range cfg.Profiles {
			if ctx.Err() != nil {
				return ran, ctx.Err()
			}
			candidate := policy.Candidate{ID: olcProbeKey(value.ID, tags[i]), ProfileID: value.ID, SourceTag: tags[i], RuntimeTag: value.ID + "::" + tags[i]}
			if !item.RuntimeAllowed {
				c.publishProbePhase(ctx, candidate, "skipped", "unsupported_transport")
				continue
			}
			if tags[i] == liveTag {
				ran = true
				c.publishProbePhase(ctx, candidate, "checking", "")
				latency, err := measureSOCKSLatency(ctx, net.JoinHostPort(olcprofile.SOCKSHost, fmt.Sprint(olcprofile.SOCKSPort)), olcProbeURL)
				c.publishProbeResult(ctx, candidateProbeResult{candidate: candidate, latency: latency, err: err})
				continue
			}
			if binaryErr != nil {
				c.publishProbePhase(ctx, candidate, "skipped", "engine_unavailable")
				continue
			}
			ran = true
			c.publishProbePhase(ctx, candidate, "preparing", "")
			one := *cfg
			one.Profiles = []olcprofile.Profile{item}
			latency, err := c.measureOlcProfile(ctx, binary, value.ID, &one)
			c.publishProbeResult(ctx, candidateProbeResult{candidate: candidate, latency: latency, err: err})
		}
	}
	return ran, nil
}

func (c *RuntimeController) recordOlcProbe(profileID, tag string, latency int, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.recordOlcProbeLocked(profileID, tag, latency, err)
}

func (c *RuntimeController) recordOlcProbeLocked(profileID, tag string, latency int, err error) {
	entry := ProbeCacheEntry{CheckedAt: time.Now().UTC(), Status: ProbeFailed, ConfigHash: c.runtimeConfigHashes[profileID]}
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
	c.probeCache[olcProbeKey(profileID, tag)] = entry
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
	defer transport.CloseIdleConnections()
	client := &http.Client{Timeout: 12 * time.Second, Transport: transport}
	if probeJobFromContext(ctx) != nil {
		return measureHTTPMethodLatency(ctx, client, rawURL, http.MethodHead)
	}
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
	return measureHTTPMethodLatency(ctx, client, rawURL, http.MethodGet)
}

func measureHTTPMethodLatency(ctx context.Context, client *http.Client, rawURL, method string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
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
