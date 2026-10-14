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
	awgcfg "naga.network/core/profile/amneziawg"
)

const (
	ProbeUnknown ProbeStatus = "unknown"
	ProbeHealthy ProbeStatus = "healthy"
	ProbeFailed  ProbeStatus = "failed"
)

type ProbeStatus string

type ProbeCacheEntry struct {
	LatencyMS  int         `json:"latency_ms,omitempty"`
	Status     ProbeStatus `json:"status"`
	CheckedAt  time.Time   `json:"checked_at"`
	ConfigHash string      `json:"-"`
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
	finish := c.beginProbeTransition()
	finish()
	c.probeCache = nil
	c.runtimeConfigHashes = nil
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
	entry := ProbeCacheEntry{CheckedAt: time.Now().UTC(), Status: ProbeFailed, ConfigHash: c.runtimeConfigHashes[result.candidate.ProfileID]}
	if result.err == nil && result.latency > 0 {
		entry.Status = ProbeHealthy
		entry.LatencyMS = result.latency
	}
	c.probeCache[key] = entry
	if result.candidate.ProfileID != "" && result.candidate.SourceTag != "" {
		c.probeCache[result.candidate.ProfileID+"::"+result.candidate.SourceTag] = entry
	}
}

func (c *RuntimeController) ProbeSnapshot() map[string]ProbeCacheEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	cache := cloneProbeCache(c.probeCache)
	c.probeMu.Lock()
	defer c.probeMu.Unlock()
	for key, entry := range c.manualProbeCache {
		if previous, ok := cache[key]; !ok || entry.CheckedAt.After(previous.CheckedAt) {
			cache[key] = entry
		}
	}
	return cache
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
	if candidate.Type == "olcrtc" {
		return lookupProbeCache(cache, olcProbeKey(candidate.ProfileID, candidate.SourceTag))
	}
	return lookupProbeCache(
		cache,
		candidate.ProfileID+"::"+candidate.SourceTag,
		candidate.RuntimeTag,
		candidate.SourceTag,
	)
}

func (c *RuntimeController) activeProbeEntryLocked() (ProbeCacheEntry, bool) {
	c.probeMu.Lock()
	defer c.probeMu.Unlock()
	var health, manual ProbeCacheEntry
	var healthOK, manualOK bool
	if c.activeEngine == profile.EngineOlcRTC {
		key := olcProbeKey(c.profileID, c.activeRuntimeTag)
		health, healthOK = lookupProbeCache(c.probeCache, key)
		manual, manualOK = lookupProbeCache(c.manualProbeCache, key)
	} else if candidate, ok := c.candidateByRuntimeTagLocked(c.activeRuntimeTag); ok {
		health, healthOK = probeCacheForCandidate(c.probeCache, candidate)
		manual, manualOK = probeCacheForCandidate(c.manualProbeCache, candidate)
	} else {
		health, healthOK = lookupProbeCache(c.probeCache, c.profileID+"::"+c.activeRuntimeTag, c.activeRuntimeTag)
		manual, manualOK = lookupProbeCache(c.manualProbeCache, c.profileID+"::"+c.activeRuntimeTag, c.activeRuntimeTag)
	}
	if manualOK && (!healthOK || manual.CheckedAt.After(health.CheckedAt)) {
		return manual, true
	}
	return health, healthOK
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

func (c *RuntimeController) connectedProbeContextLocked() (engine.Runtime, []policy.Candidate, map[string]string, bool) {
	if c.runtime == nil || c.runtime.Status() != engine.Connected {
		return nil, nil, nil, false
	}
	return c.runtime, append([]policy.Candidate(nil), c.runtimeCandidates...), cloneStringMap(c.runtimeAliases), true
}

// ProbeNodes measures every candidate in parallel. When VPN is already
// connected it uses the live Clash API. When VPN is off it starts a
// temporary sing-box without TUN, runs the same HTTPS generate_204 checks,
// then stops the process so the system VPN stays down. An active olcRTC
// tunnel is measured through its SOCKS port instead of sing-box.
//
// Android cannot run that idle sing-box: the untrusted app cannot bind
// netlink route sockets, and stock sing-box panics. Ping there needs an
// already running VPN (Clash API) or an olcRTC session.
func (c *RuntimeController) ProbeNodes(parent context.Context) error {
	job, _, err := c.startProbeJob()
	if err != nil {
		return err
	}
	select {
	case <-job.done:
		return job.err
	case <-parent.Done():
		_, _ = c.CancelProbeJob(job.snapshot.JobID)
		<-job.done
		return parent.Err()
	}
}

func (c *RuntimeController) runProbeNodes(parent context.Context) error {
	if err := parent.Err(); err != nil {
		return err
	}
	type olcResult struct {
		ran bool
		err error
	}
	olc := make(chan olcResult, 1)
	go func() { ran, err := c.probeOlcRTC(parent); olc <- olcResult{ran, err} }()
	err := c.runSingBoxProbeNodes(parent)
	result := <-olc
	if parent.Err() != nil {
		return parent.Err()
	}
	if errors.Is(err, ErrNoProbeProfiles) || errors.Is(err, ErrRuntimeNotConnected) {
		if result.ran {
			return result.err
		}
	}
	if err != nil {
		return err
	}
	return result.err
}

func (c *RuntimeController) runSingBoxProbeNodes(parent context.Context) error {
	if err := parent.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	if runtime, candidates, aliases, ok := c.connectedProbeContextLocked(); ok {
		hashes := cloneStringMap(c.runtimeConfigHashes)
		c.mu.Unlock()
		currentHashes := map[string]string{}
		if values, err := c.Store.List(); err == nil {
			currentHashes = probeProfileHashes(values)
		}
		if all := c.allListCandidates(); len(all) > 0 {
			candidates = nil
			for _, candidate := range all {
				if candidate.Type == "olcrtc" {
					continue
				}
				if _, loaded := aliases[candidate.ProfileID+"::"+candidate.SourceTag]; !loaded ||
					(hashes[candidate.ProfileID] != "" && hashes[candidate.ProfileID] != currentHashes[candidate.ProfileID]) {
					c.publishProbePhase(parent, candidate, "skipped", "runtime_requires_reload")
					continue
				}
				candidates = append(candidates, candidate)
			}
		}
		if len(candidates) == 0 {
			return nil
		}
		return c.probeWithRuntime(parent, runtime, candidates, aliases)
	}
	c.mu.Unlock()
	if !idleSingBoxProbeSupported() {
		for _, candidate := range c.allListCandidates() {
			if candidate.Type != "olcrtc" {
				c.publishProbePhase(parent, candidate, "skipped", "vpn_required")
			}
		}
		return ErrRuntimeNotConnected
	}
	return c.probeIdle(parent)
}

func idleSingBoxProbeSupported() bool {
	return goruntime.GOOS != "android" && goruntime.GOOS != "ios"
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
	// Standard transports start independently of slow AWG handshakes.
	awgDone := make(chan error, 1)
	go func() { awgDone <- c.probeIdleAWG(parent, awg, len(singBox) > 0) }()
	var standardErr error
	if len(candidates) > 0 && config != nil {
		config, standardErr = profile.PrepareLatencyProbeConfig(config)
		if standardErr == nil {
			standardErr = c.probeOwnedRuntime(parent, config, candidates, aliases)
		}
	}
	awgErr := <-awgDone
	if parent.Err() != nil {
		return parent.Err()
	}
	if standardErr != nil {
		return standardErr
	}
	return awgErr
}

func (c *RuntimeController) probeOwnedRuntime(ctx context.Context, config []byte, candidates []policy.Candidate, aliases map[string]string) (resultErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	config = c.materializeIdleRuleSets(ctx, config)
	var runtime engine.Runtime
	var err error
	if starter, ok := c.Adapter.(interface {
		StartProbe(context.Context, []byte) (engine.Runtime, error)
	}); ok {
		runtime, err = starter.StartProbe(ctx, config)
	} else {
		runtime, err = c.Adapter.Start(ctx, config)
	}
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, c.stopOwnedProbeResource(ctx, runtime.Stop)) }()
	return c.probeWithRuntime(ctx, runtime, candidates, aliases)
}

func (c *RuntimeController) probeIdleAWG(ctx context.Context, values []profile.Profile, namespaced bool) error {
	jobs := make(chan profile.Profile)
	var wg sync.WaitGroup
	// Keep native sidecar/process concurrency conservative until stand measurements.
	for i := 0; i < 1 && i < len(values); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for value := range jobs {
				if ctx.Err() != nil {
					return
				}
				c.probeOneIdleAWG(ctx, value, namespaced)
			}
		}()
	}
	for _, value := range values {
		select {
		case jobs <- value:
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return ctx.Err()
		}
	}
	close(jobs)
	wg.Wait()
	return ctx.Err()
}

func (c *RuntimeController) probeOneIdleAWG(ctx context.Context, value profile.Profile, namespaced bool) {
	candidate := amneziaCandidate(value)
	if namespaced {
		candidate = amneziaUnifiedCandidate(value)
	}
	c.publishProbePhase(ctx, candidate, "preparing", "")
	stage := "idle_amneziawg"
	leaf, sidecar, err := c.startAWGSidecarContext(ctx, &stage, value)
	if err != nil {
		c.publishProbeResult(ctx, candidateProbeResult{candidate: candidate, err: err})
		return
	}
	defer c.stopOwnedProbeResource(ctx, sidecar.Stop)
	host, port, err := parseLoopbackAddr(sidecar.ListenAddr())
	var config []byte
	if err == nil {
		config, err = awgcfg.BuildRuntimeJSON(host, port, leaf.dns)
	}
	if err == nil && namespaced {
		config, err = awgcfg.InjectSOCKSLeaf(config, host, port, candidate.RuntimeTag)
	}
	if err == nil {
		config, err = profile.PrepareLatencyProbeConfig(config)
	}
	if err == nil {
		err = c.probeOwnedRuntime(ctx, config, []policy.Candidate{candidate}, map[string]string{candidate.RuntimeTag: candidate.RuntimeTag})
	}
	if err != nil {
		c.publishProbeResult(ctx, candidateProbeResult{candidate: candidate, err: err})
	}
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
	workers := c.manualProbeConcurrency()
	if job := probeJobFromContext(parent); job != nil {
		workers = job.snapshot.Concurrency.SingBox
	}
	if workers > len(candidates) {
		workers = len(candidates)
	}
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for candidate := range jobs {
				if ctx.Err() != nil {
					return
				}
				c.publishProbePhase(ctx, candidate, "checking", "")
				timeout := probeTimeoutFor(candidate, nodeListProbeTimeout)
				tag := clashProbeTag(candidate, aliases)
				probeCtx, probeCancel := context.WithTimeout(ctx, timeout)
				latency, err := probeOutboundOnce(runtime, probeCtx, tag, timeout)
				probeCancel()
				results <- candidateProbeResult{candidate: candidate, latency: latency, err: err}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, candidate := range fairProbeQueue(candidates) {
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

	for result := range results {
		c.publishProbeResult(ctx, result)
	}
	return ctx.Err()
}

func (c *RuntimeController) manualProbeConcurrency() int {
	limit := maxManualProbeConcurrency()
	value := c.ManualProbeConcurrency
	if value <= 0 {
		value = probeConcurrency
	}
	if value > limit {
		value = limit
	}
	return value
}

func maxManualProbeConcurrency() int {
	if goruntime.GOOS == "android" || goruntime.GOOS == "ios" {
		return 10
	}
	return 20
}

// Alternate profiles so a long list cannot fill every slot before another
// subscription gets its first measurement.
func fairProbeQueue(candidates []policy.Candidate) []policy.Candidate {
	groups := map[string][]policy.Candidate{}
	var order []string
	for _, candidate := range candidates {
		if _, exists := groups[candidate.ProfileID]; !exists {
			order = append(order, candidate.ProfileID)
		}
		groups[candidate.ProfileID] = append(groups[candidate.ProfileID], candidate)
	}
	queue := make([]policy.Candidate, 0, len(candidates))
	for len(order) > 0 {
		next := order[:0]
		for _, id := range order {
			group := groups[id]
			queue = append(queue, group[0])
			if len(group) > 1 {
				groups[id] = group[1:]
				next = append(next, id)
			}
		}
		order = next
	}
	return queue
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
