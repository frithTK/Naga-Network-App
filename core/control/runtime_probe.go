package control

import (
	"context"
	"errors"
	goruntime "runtime"
	"strings"
	"sync"
	"time"

	"naga.network/core/engine"
	"naga.network/core/policy"
	"naga.network/core/profile"
)

const (
	ProbeUnknown ProbeStatus = "unknown"
	ProbeHealthy ProbeStatus = "healthy"
	ProbeFailed  ProbeStatus = "failed"
)

type ProbeStatus string

type ProbeCacheEntry struct {
	LatencyMS int         `json:"latency_ms,omitempty"`
	Status    ProbeStatus `json:"status"`
	CheckedAt time.Time   `json:"checked_at"`
}

var (
	ErrRuntimeNotConnected = errors.New("runtime is not connected")
	ErrNoProbeProfiles     = errors.New("нет сохранённых профилей для проверки задержки")
	ErrProbeInProgress     = errors.New("проверка задержки уже выполняется")
)

const (
	probeConcurrency     = 8
	nodeListProbeTimeout = 8 * time.Second
	quicProbeTimeout     = 12 * time.Second
	nodeListProbeBudget  = 180 * time.Second
)

func (c *RuntimeController) resetProbeStateLocked() {
	c.probeCache = nil
	c.runtimeCandidates = nil
	c.cooldownUntil = nil
	c.failoverMessage = ""
	c.activeRuntimeTag = ""
	c.healthFailCount = 0
	c.awgSkipReasons = nil
	c.inspect = nil
	c.startingOverlay = false
}

func (c *RuntimeController) recordProbeLocked(result candidateProbeResult) {
	key := strings.TrimSpace(result.candidate.RuntimeTag)
	if key == "" {
		return
	}
	if c.probeCache == nil {
		c.probeCache = make(map[string]ProbeCacheEntry)
	}
	entry := ProbeCacheEntry{CheckedAt: time.Now().UTC(), Status: ProbeFailed}
	if result.err == nil && result.latency > 0 {
		entry.Status = ProbeHealthy
		entry.LatencyMS = result.latency
	}
	c.probeCache[key] = entry
}

func (c *RuntimeController) ProbeSnapshot() map[string]ProbeCacheEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	return cloneProbeCache(c.probeCache)
}

func cloneProbeCache(src map[string]ProbeCacheEntry) map[string]ProbeCacheEntry {
	if len(src) == 0 {
		return map[string]ProbeCacheEntry{}
	}
	out := make(map[string]ProbeCacheEntry, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
}

func lookupProbeCache(cache map[string]ProbeCacheEntry, keys ...string) (ProbeCacheEntry, bool) {
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" || key == "::" {
			continue
		}
		if entry, ok := cache[key]; ok {
			return entry, true
		}
	}
	return ProbeCacheEntry{}, false
}

func probeCacheForCandidate(cache map[string]ProbeCacheEntry, candidate policy.Candidate) (ProbeCacheEntry, bool) {
	return lookupProbeCache(
		cache,
		candidate.RuntimeTag,
		candidate.ProfileID+"::"+candidate.SourceTag,
		candidate.SourceTag,
	)
}

func (c *RuntimeController) activeProbeEntryLocked() (ProbeCacheEntry, bool) {
	if candidate, ok := c.candidateByRuntimeTagLocked(c.activeRuntimeTag); ok {
		if entry, found := probeCacheForCandidate(c.probeCache, candidate); found {
			return entry, true
		}
	}
	return lookupProbeCache(c.probeCache, c.activeRuntimeTag)
}

func (c *RuntimeController) candidateByRuntimeTagLocked(tag string) (policy.Candidate, bool) {
	if tag == "" {
		return policy.Candidate{}, false
	}
	for _, candidate := range c.runtimeCandidates {
		if candidate.RuntimeTag == tag {
			return candidate, true
		}
	}
	return policy.Candidate{}, false
}

func (c *RuntimeController) candidateByDisplayedNodeLocked(node string) (policy.Candidate, bool) {
	node = strings.TrimSpace(node)
	if node == "" {
		return policy.Candidate{}, false
	}
	if candidate, ok := c.candidateByRuntimeTagLocked(node); ok {
		return candidate, true
	}
	source := sourceTagFromRuntime(node)
	for _, candidate := range c.runtimeCandidates {
		if candidate.SourceTag == node || candidate.SourceTag == source || sourceTagFromRuntime(candidate.RuntimeTag) == node {
			return candidate, true
		}
	}
	return policy.Candidate{}, false
}

func (c *RuntimeController) applyProbeNodeLocked(snapshot *RuntimeSnapshot) {
	if snapshot == nil || c.activeRuntimeTag == "" {
		return
	}
	if candidate, ok := c.candidateByRuntimeTagLocked(c.activeRuntimeTag); ok {
		snapshot.ActiveNode = candidate.SourceTag
		if candidate.Country != "" {
			snapshot.ActiveCountry = candidate.Country
		}
		if candidate.Protocol != "" {
			snapshot.ActiveProtocol = candidate.Protocol
		}
		return
	}
	snapshot.ActiveNode = sourceTagFromRuntime(c.activeRuntimeTag)
}

func clashLeafIsConcrete(details engine.RuntimeDetails) bool {
	node := strings.TrimSpace(details.Node)
	if node == "" {
		return false
	}
	switch strings.ToLower(node) {
	case "auto", "mode", "select", "manual", strings.ToLower(profile.UnifiedSelectorTag):
		return false
	}
	switch strings.ToLower(strings.TrimSpace(details.Protocol)) {
	case "urltest", "selector", "direct", "block", "dns":
		return false
	}
	if details.Status == "checking" || details.Status == "unavailable" {
		return false
	}
	return details.Status == "active" || details.Protocol != ""
}

func (c *RuntimeController) liveCandidatesLocked(candidates []policy.Candidate) []policy.Candidate {
	if len(c.cooldownUntil) == 0 {
		return candidates
	}
	now := time.Now()
	live := make([]policy.Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		until, cooling := c.cooldownUntil[candidate.RuntimeTag]
		if cooling && now.Before(until) {
			continue
		}
		live = append(live, candidate)
	}
	if len(live) == 0 {
		return candidates
	}
	return live
}

func (c *RuntimeController) failThreshold() int {
	if c.HealthFailThreshold > 0 {
		return c.HealthFailThreshold
	}
	return policy.HealthFailThreshold
}

func (c *RuntimeController) healthInterval() time.Duration {
	if c.HealthInterval > 0 {
		return c.HealthInterval
	}
	return policy.HealthProbeInterval
}

func (c *RuntimeController) stopIdleProbeLocked() {
	if c.idleProbeRuntime != nil {
		_ = c.idleProbeRuntime.Stop()
		c.idleProbeRuntime = nil
	}
	for _, sidecar := range c.idleAWGSidecars {
		if sidecar != nil {
			_ = sidecar.Stop()
		}
	}
	c.idleAWGSidecars = nil
	c.idleProbing = false
}

func (c *RuntimeController) connectedProbeContextLocked() (engine.Runtime, []policy.Candidate, map[string]string, bool) {
	if c.runtime == nil || c.runtime.Status() != engine.Connected {
		return nil, nil, nil, false
	}
	return c.runtime, append([]policy.Candidate(nil), c.runtimeCandidates...), cloneStringMap(c.runtimeAliases), true
}

// ProbeNodes measures every candidate in parallel. When VPN is already
// connected it uses the live Clash API. When VPN is off it starts a
// temporary sing-box without TUN, runs the same HTTP generate_204 checks,
// then stops the process so the system VPN stays down. An active olcRTC
// tunnel is measured through its SOCKS port instead of sing-box.
//
// Android cannot run that idle sing-box: the untrusted app cannot bind
// netlink route sockets, and stock sing-box panics. Ping there needs an
// already running VPN (Clash API) or an olcRTC session.
func (c *RuntimeController) ProbeNodes(parent context.Context) error {
	c.mu.Lock()
	olcOn := c.olcProc != nil
	if runtime, candidates, aliases, ok := c.connectedProbeContextLocked(); ok {
		c.mu.Unlock()
		if all := c.allListCandidates(); len(all) > 0 {
			candidates = all
		}
		err := c.probeWithRuntime(parent, runtime, candidates, aliases)
		if olcOn {
			_, _ = c.probeOlcRTC(parent)
		}
		return err
	}
	if olcOn {
		c.mu.Unlock()
		_, err := c.probeOlcRTC(parent)
		return err
	}
	if c.idleProbing {
		c.mu.Unlock()
		return ErrProbeInProgress
	}
	if !idleSingBoxProbeSupported() {
		c.mu.Unlock()
		olcRan, olcErr := c.probeOlcRTC(parent)
		if olcRan {
			return olcErr
		}
		return ErrRuntimeNotConnected
	}
	c.idleProbing = true
	c.mu.Unlock()

	err := c.probeIdle(parent)
	c.mu.Lock()
	c.stopIdleProbeLocked()
	c.mu.Unlock()
	olcRan, olcErr := c.probeOlcRTC(parent)
	if errors.Is(err, ErrNoProbeProfiles) && !olcRan {
		return ErrNoProbeProfiles
	}
	if err == nil || errors.Is(err, ErrNoProbeProfiles) {
		return olcErr
	}
	return err
}

func idleSingBoxProbeSupported() bool {
	return goruntime.GOOS != "android"
}

func (c *RuntimeController) probeIdle(parent context.Context) error {
	if c.Store == nil || c.Adapter == nil {
		return errors.New("runtime is not configured")
	}
	values, err := c.Store.List()
	if err != nil {
		return err
	}
	now := nowUTC()
	active := make([]profile.Profile, 0, len(values))
	for _, value := range values {
		if value.Subscription.Expired(now) {
			continue
		}
		active = append(active, value)
	}
	singBox := profile.SingBoxProfiles(active)
	awg := profile.AmneziaWGProfiles(active)
	if len(singBox) == 0 && len(awg) == 0 {
		return ErrNoProbeProfiles
	}
	aliases := make(map[string]string)
	var candidates []policy.Candidate
	var config []byte
	if len(singBox) > 0 {
		network := c.networkClass()
		merged, mergeErr := profile.MergeProfilesForNetwork(singBox, string(network))
		if mergeErr != nil {
			return mergeErr
		}
		for _, candidate := range merged.Candidates {
			aliases[candidate.ProfileID+"::"+candidate.SourceTag] = candidate.RuntimeTag
			aliases[candidate.RuntimeTag] = candidate.RuntimeTag
		}
		candidates = policy.RankCandidates(network, merged.Candidates)
		config, err = profile.ApplyTrafficMode(merged.Config, policy.TrafficSystemProxy)
		if err != nil {
			return err
		}
	}
	config, candidates, err = c.attachIdleAmneziaWG(config, aliases, candidates, awg, len(singBox) > 0)
	if err != nil && config == nil {
		return err
	}
	if config == nil || len(candidates) == 0 {
		return ErrNoProbeProfiles
	}
	config = c.materializeIdleRuleSets(parent, config)
	runtime, err := c.Adapter.Start(parent, config)
	if err != nil {
		return err
	}
	c.mu.Lock()
	if live, liveCandidates, liveAliases, ok := c.connectedProbeContextLocked(); ok {
		c.mu.Unlock()
		_ = runtime.Stop()
		return c.probeWithRuntime(parent, live, liveCandidates, liveAliases)
	}
	c.idleProbeRuntime = runtime
	c.mu.Unlock()
	return c.probeWithRuntime(parent, runtime, candidates, aliases)
}

func (c *RuntimeController) probeWithRuntime(parent context.Context, runtime engine.Runtime, candidates []policy.Candidate, aliases map[string]string) error {
	if _, ok := runtime.(engine.OutboundProber); !ok {
		return nil
	}
	if len(candidates) == 0 {
		candidates = c.unifiedProbeCandidates()
	}
	if len(candidates) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(parent, nodeListProbeBudget)
	defer cancel()

	results := make(chan candidateProbeResult, len(candidates))
	jobs := make(chan policy.Candidate)
	workers := probeConcurrency
	if workers > len(candidates) {
		workers = len(candidates)
	}
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for candidate := range jobs {
				timeout := probeTimeoutFor(candidate, nodeListProbeTimeout)
				tag := clashProbeTag(candidate, aliases)
				latency, err := probeOutboundOnce(runtime, ctx, tag, timeout)
				results <- candidateProbeResult{candidate: candidate, latency: latency, err: err}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, candidate := range candidates {
			select {
			case jobs <- candidate:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		wg.Wait()
		close(results)
	}()

	collected := make([]candidateProbeResult, 0, len(candidates))
	for result := range results {
		collected = append(collected, result)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	seen := make(map[string]struct{}, len(collected))
	for _, result := range collected {
		c.recordProbeLocked(result)
		if id := strings.TrimSpace(result.candidate.ID); id != "" {
			seen[id] = struct{}{}
		}
	}
	for _, candidate := range candidates {
		id := strings.TrimSpace(candidate.ID)
		if id == "" {
			id = strings.TrimSpace(candidate.RuntimeTag)
		}
		if _, ok := seen[id]; ok {
			continue
		}
		c.recordProbeLocked(candidateProbeResult{candidate: candidate, err: ctx.Err()})
	}
	return nil
}

func probeOutboundOnce(runtime engine.Runtime, ctx context.Context, outbound string, timeout time.Duration) (int, error) {
	type once interface {
		ProbeOnce(context.Context, string, time.Duration) (int, error)
	}
	if prober, ok := runtime.(once); ok {
		return prober.ProbeOnce(ctx, outbound, timeout)
	}
	if prober, ok := runtime.(engine.OutboundProber); ok {
		return prober.Probe(ctx, outbound, timeout)
	}
	return 0, errors.New("runtime does not support outbound probes")
}

func (c *RuntimeController) allListCandidates() []policy.Candidate {
	if c.Store == nil {
		return nil
	}
	values, err := c.Store.List()
	if err != nil || len(values) == 0 {
		return nil
	}
	now := nowUTC()
	active := make([]profile.Profile, 0, len(values))
	for _, value := range values {
		if value.Subscription.Expired(now) {
			continue
		}
		active = append(active, value)
	}
	singBox := profile.SingBoxProfiles(active)
	awg := profile.AmneziaWGProfiles(active)
	candidates := make([]policy.Candidate, 0)
	if len(singBox) > 0 {
		merged, mergeErr := profile.MergeProfilesForNetwork(singBox, string(c.networkClass()))
		if mergeErr == nil {
			candidates = append(candidates, merged.Candidates...)
		}
	}
	if len(awg) > 0 {
		candidates = append(candidates, amneziaUnifiedCandidates(awg, len(singBox) > 0)...)
	}
	candidates = append(candidates, olcRTCUnifiedCandidates(active)...)
	return candidates
}

func (c *RuntimeController) unifiedProbeCandidates() []policy.Candidate {
	if c.Store == nil {
		return nil
	}
	values, err := c.Store.List()
	if err != nil || len(values) == 0 {
		return nil
	}
	active := make([]profile.Profile, 0, len(values))
	now := nowUTC()
	for _, value := range values {
		if value.Subscription.Expired(now) {
			continue
		}
		active = append(active, value)
	}
	active = profile.SingBoxProfiles(active)
	if len(active) == 0 {
		return nil
	}
	merged, err := profile.MergeProfilesForNetwork(active, string(c.networkClass()))
	if err != nil {
		return nil
	}
	return policy.RankCandidates(c.networkClass(), merged.Candidates)
}

func clashProbeTag(candidate policy.Candidate, aliases map[string]string) string {
	keys := []string{
		strings.TrimSpace(candidate.RuntimeTag),
		strings.TrimSpace(candidate.ProfileID) + "::" + strings.TrimSpace(candidate.SourceTag),
		strings.TrimSpace(candidate.SourceTag),
	}
	for _, key := range keys {
		if key == "" || key == "::" {
			continue
		}
		if alias, ok := aliases[key]; ok && alias != "" {
			return alias
		}
	}
	if tag := strings.TrimSpace(candidate.RuntimeTag); tag != "" {
		return tag
	}
	return strings.TrimSpace(candidate.SourceTag)
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
}

// SelectByPolicy re-runs RankCandidates + probe groups without tearing TUN.
func (c *RuntimeController) SelectByPolicy() (RuntimeSnapshot, error) {
	return c.reselectByPolicy(context.Background())
}

func (c *RuntimeController) reselectByPolicy(parent context.Context) (RuntimeSnapshot, error) {
	if c.Store == nil {
		return c.Status(), errors.New("runtime is not configured")
	}
	connectionPolicy, err := c.Store.LoadConnectionPolicy()
	if err != nil {
		return c.Status(), err
	}

	c.mu.Lock()
	if c.runtime == nil || !c.IsRuntimeActiveLocked() {
		snap := c.snapshotLocked()
		c.mu.Unlock()
		return snap, ErrRuntimeNotConnected
	}
	network := c.effectiveNetworkClassLocked(connectionPolicy.NetworkClass)
	connectionPolicy.NetworkClass = network
	ranked := policy.RankCandidates(network, c.runtimeCandidates)
	ranked = c.liveCandidatesLocked(ranked)
	if connectionPolicy.Mode == policy.ConnectionManualStrict && c.Store != nil && c.profileID != "" {
		if value, err := c.Store.Load(c.profileID); err == nil {
			ranked = candidatesForSelection(ranked, value.SelectedMode)
		}
	}
	runtime := c.runtime
	c.mu.Unlock()

	if len(ranked) == 0 {
		return c.Status(), nil
	}

	outcome, probeErr := probeCandidateGroups(parent, c.Journal, runtime, ranked, connectionPolicy, outboundProbeTimeout)

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.runtime != runtime || !c.IsRuntimeActiveLocked() {
		return c.snapshotLocked(), nil
	}
	if outcome.skip {
		c.lastError = ""
		return c.snapshotLocked(), nil
	}
	if err := c.applyProbeSelectionLocked(parent, runtime, outcome); err != nil {
		if probeErr != nil {
			err = probeErr
		}
		c.lastError = safeRuntimeError(err)
		return c.snapshotLocked(), err
	}
	c.lastError = ""
	return c.snapshotLocked(), nil
}

func (c *RuntimeController) persistObservedNetworkClass(next policy.NetworkClass) {
	if c.Store == nil || next == policy.NetworkUnknown {
		return
	}
	if err := c.Store.PatchConnectionPolicy(func(current *policy.ConnectionPolicy) error {
		current.NetworkClass = next
		return nil
	}); err != nil && c.Journal != nil {
		c.Journal.Warn("runtime", "network_class_persist_failed", "Не удалось сохранить класс сети, VPN остаётся подключённым", map[string]any{
			"error": err.Error(),
		})
	}
}

// OnNetworkClass is invoked after the watcher debounce. Unknown does not
// trigger a reselect. TUN is never torn down. A connected session re-runs
// the same auto/manual probe groups as a periodic health tick.
func (c *RuntimeController) OnNetworkClass(next policy.NetworkClass) (RuntimeSnapshot, error) {
	if next == policy.NetworkUnknown {
		return c.Status(), nil
	}
	c.mu.Lock()
	if c.lastObservedClass == next {
		snap := c.snapshotLocked()
		c.mu.Unlock()
		return snap, nil
	}
	previous := c.lastObservedClass
	c.lastObservedClass = next
	running := c.runtime != nil && c.IsRuntimeActiveLocked()
	c.mu.Unlock()

	c.persistObservedNetworkClass(next)
	if !running {
		return c.Status(), nil
	}
	if c.Journal != nil {
		c.Journal.Info("runtime", "network_class_changed", "Класс сети изменился, выбираем узел заново", map[string]any{
			"from": string(previous),
			"to":   string(next),
		})
	}
	return c.reselectByPolicy(context.Background())
}

func (c *RuntimeController) monitorActiveHealth(ctx context.Context) {
	ticker := time.NewTicker(c.healthInterval())
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.tickActiveHealth(ctx)
		}
	}
}

func (c *RuntimeController) tickActiveHealth(parent context.Context) {
	c.mu.Lock()
	if c.runtime == nil || c.runtime.Status() != engine.Connected {
		c.mu.Unlock()
		return
	}
	mode := c.connectionMode
	c.mu.Unlock()
	if mode == "" {
		mode = policy.ConnectionAuto
	}
	if mode == policy.ConnectionAuto {
		_, _ = c.reselectByPolicy(parent)
		return
	}
	c.probeActiveLeaf(parent)
}

func (c *RuntimeController) probeActiveLeaf(parent context.Context) {
	c.mu.Lock()
	if c.runtime == nil || c.runtime.Status() != engine.Connected || c.activeRuntimeTag == "" {
		c.mu.Unlock()
		return
	}
	runtime := c.runtime
	tag := c.activeRuntimeTag
	candidate, _ := c.candidateByRuntimeTagLocked(tag)
	c.mu.Unlock()

	prober, ok := runtime.(engine.OutboundProber)
	if !ok {
		return
	}
	latency, err := prober.Probe(parent, tag, outboundProbeTimeout)

	c.mu.Lock()
	if c.runtime != runtime || c.activeRuntimeTag != tag {
		c.mu.Unlock()
		return
	}
	c.recordProbeLocked(candidateProbeResult{candidate: candidate, latency: latency, err: err})
	if err == nil && latency > 0 {
		c.healthFailCount = 0
		c.mu.Unlock()
		return
	}
	c.healthFailCount++
	if c.healthFailCount < c.failThreshold() {
		c.mu.Unlock()
		return
	}
	if c.connectionMode == policy.ConnectionManualStrict {
		c.lastError = "выбранный узел не отвечает"
		c.mu.Unlock()
		return
	}
	if c.cooldownUntil == nil {
		c.cooldownUntil = make(map[string]time.Time)
	}
	c.cooldownUntil[tag] = time.Now().Add(policy.HealthCooldown)
	if c.Journal != nil {
		c.Journal.Warn("runtime", "active_node_unhealthy", "Активный VPN-узел не отвечает, переключаемся", map[string]any{
			"node": candidate.SourceTag,
		})
	}
	c.mu.Unlock()
	_, _ = c.reselectByPolicy(parent)
}
