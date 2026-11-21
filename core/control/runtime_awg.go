package control

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net"
	goruntime "runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"naga.network/core/androidvpn"
	"naga.network/core/engine"
	awgengine "naga.network/core/engine/amneziawg"
	"naga.network/core/policy"
	"naga.network/core/profile"
	awgcfg "naga.network/core/profile/amneziawg"
)

func (c *RuntimeController) startAmneziaOnlyLocked(stage *string, profileID string, awg []profile.Profile, connectionPolicy policy.ConnectionPolicy, routing policy.RoutingPolicy) (RuntimeSnapshot, error) {
	if len(awg) == 0 {
		c.lastError = "amneziawg profile could not be loaded"
		return c.snapshotLocked(), errors.New(c.lastError)
	}
	*stage = "prepare_profile"
	session := map[string]struct{}{profileID: {}}
	aliases := make(map[string]string)
	candidates := make([]policy.Candidate, 0, len(awg))
	var config []byte
	var lastErr error
	var standby func(profile.Profile) bool
	if mode := selectedModeForProfile(awg, profileID); awgcfg.IsLeafTag(mode) {
		standby = deferUnselectedAWG(profileID, mode)
		// Build the shared AWG-only DNS settings from the selected profile.
		for i, value := range awg {
			if !standby(value) {
				ordered := make([]profile.Profile, 0, len(awg))
				ordered = append(ordered, value)
				ordered = append(ordered, awg[:i]...)
				awg = append(ordered, awg[i+1:]...)
				break
			}
		}
	} else if connectionPolicy.Mode != policy.ConnectionManualStrict {
		standby = func(profile.Profile) bool { return true }
	}
	started := c.startAWGSidecars(stage, awg, standby)
	defer stopUnclaimedAWG(started)
	for i, result := range started {
		started[i].sidecar = nil
		value := awg[i]
		session[value.ID] = struct{}{}
		leaf, sidecar, err := result.leaf, result.sidecar, result.err
		if err != nil {
			lastErr = err
			c.journalAWGSkip(amneziaUnifiedCandidate(value).RuntimeTag, err)
			continue
		}
		host, port, err := parseLoopbackAddr(sidecar.ListenAddr())
		if err != nil {
			lastErr = err
			_ = sidecar.Stop()
			c.journalAWGSkip(amneziaUnifiedCandidate(value).RuntimeTag, err)
			continue
		}
		c.awgSidecars = append(c.awgSidecars, sidecar)
		if config == nil {
			if _, ok := sidecar.(awgengine.HTTPSProber); ok {
				config, err = awgcfg.BuildRuntimeJSONWithSelectedDNS(host, port)
			} else {
				config, err = awgcfg.BuildRuntimeJSON(host, port, leaf.dns)
			}
			if err != nil {
				_ = c.stopSidecarsLocked()
				c.lastError = awgcfg.SafeError(err)
				return c.snapshotLocked(), err
			}
			candidate := amneziaCandidate(value)
			candidates = append(candidates, candidate)
			c.registerAWGProbeLocked(candidate, sidecar)
			addAmneziaAliases(aliases, value.ID, candidate.RuntimeTag, len(awg) == 1 || value.AWGLegacy)
			continue
		}
		runtimeTag := amneziaUnifiedCandidate(value).RuntimeTag
		injected, err := awgcfg.InjectSOCKSLeaf(config, host, port, runtimeTag)
		if err != nil {
			_ = sidecar.Stop()
			c.awgSidecars = c.awgSidecars[:len(c.awgSidecars)-1]
			c.journalAWGSkip(amneziaUnifiedCandidate(value).RuntimeTag, err)
			continue
		}
		config = injected
		candidate := amneziaUnifiedCandidate(value)
		candidates = append(candidates, candidate)
		c.registerAWGProbeLocked(candidate, sidecar)
		addAmneziaAliases(aliases, value.ID, candidate.RuntimeTag, false)
	}
	if config == nil || len(candidates) == 0 {
		_ = c.stopSidecarsLocked()
		mapped := mapAWGStartError(lastErr)
		c.lastError = awgcfg.SafeError(mapped)
		if errors.Is(mapped, awgengine.ErrTunnelUnavailable) {
			c.lastError = mapped.Error()
		}
		return c.snapshotLocked(), mapped
	}
	c.sessionIDs = session
	snapshot, err := c.finishStartLocked(
		stage,
		profileID,
		config,
		aliases,
		candidates,
		profile.EngineAmneziaWG,
		awgFirstProbeTimeout,
		true,
		connectionPolicy,
		routing,
	)
	if err != nil {
		_ = c.stopSidecarsLocked()
		c.sessionIDs = nil
		return snapshot, err
	}
	return snapshot, nil
}

func (c *RuntimeController) startUnifiedLocked(stage *string, profileID string, singBox, awg []profile.Profile, connectionPolicy policy.ConnectionPolicy, routing policy.RoutingPolicy) (RuntimeSnapshot, error) {
	*stage = "prepare_profile"
	// The selected AWG leaf belongs to the envelope, not to its extracted
	// sing-box graph. Keep the persisted choice for the later candidate probe.
	prepared := append([]profile.Profile(nil), singBox...)
	selectedAWG := false
	for i := range prepared {
		if !awgcfg.IsLeafTag(prepared[i].SelectedMode) {
			continue
		}
		if envelopeAmneziaSelectedForProfiles(awg, prepared[i].ID, prepared[i].SelectedMode) {
			if prepared[i].ID == profileID {
				selectedAWG = true
			}
			prepared[i].SelectedMode = ""
		}
	}
	config, aliases, candidates, err := prepareSingBoxRuntime(prepared, profileID, c.networkClass())
	if err != nil {
		c.lastError = safeRuntimeError(err)
		return c.snapshotLocked(), err
	}
	session := map[string]struct{}{profileID: {}}
	for _, value := range singBox {
		session[value.ID] = struct{}{}
	}
	injectedAWG := 0
	selectedAWGReady := false
	var selectedAWGError error
	var standby func(profile.Profile) bool
	if mode := selectedModeForProfile(singBox, profileID); concreteProfileSelection(mode) {
		standby = deferUnselectedAWG(profileID, mode)
	} else if connectionPolicy.Mode != policy.ConnectionManualStrict {
		standby = func(profile.Profile) bool { return true }
	}
	started := c.startAWGSidecars(stage, awg, standby)
	defer stopUnclaimedAWG(started)
	for i, result := range started {
		started[i].sidecar = nil
		value := awg[i]
		isSelected := selectedAWG && value.ID == profileID && awgSelectionMatches(value, selectedModeForProfile(singBox, profileID))
		sidecar, startErr := result.sidecar, result.err
		if startErr != nil {
			if isSelected {
				selectedAWGError = startErr
			}
			c.journalAWGSkip(amneziaUnifiedCandidate(value).RuntimeTag, startErr)
			continue
		}
		host, port, addrErr := parseLoopbackAddr(sidecar.ListenAddr())
		if addrErr != nil {
			if isSelected {
				selectedAWGError = addrErr
			}
			_ = sidecar.Stop()
			c.journalAWGSkip(amneziaUnifiedCandidate(value).RuntimeTag, addrErr)
			continue
		}
		runtimeTag := amneziaUnifiedCandidate(value).RuntimeTag
		injected, injectErr := awgcfg.InjectSOCKSLeaf(config, host, port, runtimeTag)
		if injectErr != nil {
			if isSelected {
				selectedAWGError = injectErr
			}
			_ = sidecar.Stop()
			c.journalAWGSkip(amneziaUnifiedCandidate(value).RuntimeTag, injectErr)
			continue
		}
		config = injected
		c.awgSidecars = append(c.awgSidecars, sidecar)
		candidate := amneziaUnifiedCandidate(value)
		candidates = append(candidates, candidate)
		c.registerAWGProbeLocked(candidate, sidecar)
		addAmneziaAliases(aliases, value.ID, candidate.RuntimeTag, value.AWGLegacy)
		session[value.ID] = struct{}{}
		injectedAWG++
		if isSelected {
			selectedAWGReady = true
		}
	}
	if selectedAWG && !selectedAWGReady {
		_ = c.stopSidecarsLocked()
		if selectedAWGError == nil {
			selectedAWGError = errors.New("Выбранный узел AmneziaWG недоступен")
		}
		c.lastError = awgcfg.SafeError(selectedAWGError)
		return c.snapshotLocked(), selectedAWGError
	}
	if injectedAWG > 0 && !hasNonAmneziaCandidate(candidates) {
		requested, ok := profileByID(singBox, profileID)
		if !ok {
			requested = singBox[0]
		}
		candidates = append(profile.IdentityLeafCandidates(requested), candidates...)
	}
	if injectedAWG == 1 {
		for _, candidate := range candidates {
			if candidate.Protocol == "AmneziaWG" && candidate.SourceTag == awgcfg.LeafTag {
				aliases[awgcfg.LeafTag] = candidate.RuntimeTag
				break
			}
		}
	}
	c.sessionIDs = session
	retry := injectedAWG > 0
	snapshot, err := c.finishStartLocked(
		stage,
		profileID,
		config,
		aliases,
		candidates,
		profile.EngineSingBox,
		outboundProbeTimeout,
		retry,
		connectionPolicy,
		routing,
	)
	if err != nil {
		_ = c.stopSidecarsLocked()
		c.sessionIDs = nil
		return snapshot, err
	}
	return snapshot, nil
}

type awgLeaf struct {
	dns []string
}

type awgStartResult struct {
	leaf    awgLeaf
	sidecar awgengine.Sidecar
	err     error
}

func stopUnclaimedAWG(results []awgStartResult) {
	for _, result := range results {
		if result.sidecar != nil {
			_ = result.sidecar.Stop()
		}
	}
}

// Prepare independent tunnels concurrently, but retain profile order for
// deterministic aliases and selection. A slow peer must not add its full
// handshake timeout to every other peer's startup time.
func (c *RuntimeController) startAWGSidecars(stage *string, values []profile.Profile, standby func(profile.Profile) bool) []awgStartResult {
	results := make([]awgStartResult, len(values))
	life := c.startupContextLocked()
	ctx, cancel := context.WithTimeout(life, 20*time.Second)
	defer cancel()
	*stage = "sidecar_start"
	const batchSize = 4
	for first := 0; first < len(values); first += batchSize {
		last := min(first+batchSize, len(values))
		var wg sync.WaitGroup
		for i := first; i < last; i++ {
			// Factories are called in order and need not be concurrency-safe.
			sidecar := c.newAWGSidecar()
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				localStage := ""
				var leaf awgLeaf
				var started awgengine.Sidecar
				var err error
				prepared, canDefer := sidecar.(awgengine.PreparedSidecar)
				if standby != nil && standby(values[i]) && canDefer &&
					(goruntime.GOOS == "linux" || goruntime.GOOS == "android") {
					leaf, started, err = prepareAWGStandby(ctx, life, values[i], prepared)
				} else {
					leaf, started, err = c.startAWGSidecarWith(ctx, &localStage, values[i], sidecar)
				}
				results[i] = awgStartResult{leaf: leaf, sidecar: started, err: err}
			}(i)
		}
		wg.Wait()
	}
	return results
}

func prepareAWGStandby(ctx, life context.Context, value profile.Profile, sidecar awgengine.PreparedSidecar) (awgLeaf, awgengine.Sidecar, error) {
	cfg, err := awgcfg.Parse(value.Config)
	if err == nil {
		err = awgcfg.ValidateRuntime(cfg)
	}
	if err != nil {
		return awgLeaf{}, nil, err
	}
	standby := newDeferredAWG(life, sidecar, cfg)
	if err := standby.Start(ctx, cfg); err != nil {
		_ = standby.Stop()
		return awgLeaf{}, nil, err
	}
	return awgLeaf{dns: cfg.DNS}, standby, nil
}
func (c *RuntimeController) startAWGSidecar(stage *string, value profile.Profile) (awgLeaf, awgengine.Sidecar, error) {
	return c.startAWGSidecarContext(c.startupContextLocked(), stage, value)
}

func (c *RuntimeController) startAWGSidecarContext(ctx context.Context, stage *string, value profile.Profile) (awgLeaf, awgengine.Sidecar, error) {
	return c.startAWGSidecarWith(ctx, stage, value, c.newAWGSidecar())
}

func (c *RuntimeController) startAWGSidecarWith(ctx context.Context, stage *string, value profile.Profile, sidecar awgengine.Sidecar) (awgLeaf, awgengine.Sidecar, error) {
	*stage = "parse_amneziawg"
	cfg, err := awgcfg.Parse(value.Config)
	if err != nil {
		return awgLeaf{}, nil, err
	}
	*stage = "sidecar_start"
	if err := sidecar.Start(ctx, cfg); err != nil {
		_ = c.stopOwnedProbeResource(ctx, sidecar.Stop)
		return awgLeaf{}, nil, err
	}
	readyCtx, cancelReady := context.WithTimeoutCause(ctx, handshakeReadyTimeout, awgengine.ErrHandshakeTimeout)
	err = sidecar.Ready(readyCtx)
	if err != nil && readyCtx.Err() != nil {
		err = context.Cause(readyCtx)
	}
	cancelReady()
	if err != nil {
		_ = c.stopOwnedProbeResource(ctx, sidecar.Stop)
		return awgLeaf{}, nil, err
	}
	return awgLeaf{dns: cfg.DNS}, sidecar, nil
}

func prepareSingBoxRuntime(singBox []profile.Profile, requestedID string, networkClass policy.NetworkClass) ([]byte, map[string]string, []policy.Candidate, error) {
	if len(singBox) == 0 {
		return nil, nil, nil, errors.New("no sing-box profiles")
	}
	requested, ok := profileByID(singBox, requestedID)
	if !ok {
		requested = singBox[0]
	}
	mergeable := len(singBox) > 1
	if len(singBox) == 1 && strings.EqualFold(strings.TrimSpace(singBox[0].SelectedMode), "auto") {
		if _, inspectErr := profile.InspectMode(singBox[0].Config); inspectErr == nil {
			mergeable = true
		}
	}
	aliases := make(map[string]string)
	if mergeable {
		merged, err := profile.MergeProfilesForNetwork(singBox, string(networkClass))
		if err != nil {
			return nil, nil, nil, err
		}
		for _, candidate := range merged.Candidates {
			aliases[candidate.ProfileID+"::"+candidate.SourceTag] = candidate.RuntimeTag
			aliases[candidate.RuntimeTag] = candidate.RuntimeTag
		}
		return merged.Config, aliases, policy.RankCandidates(networkClass, merged.Candidates), nil
	}
	config := requested.Config
	aliases = identityUnifiedAliases(requested.ID, requested.Config)
	if requested.SelectedMode != "" && !strings.EqualFold(strings.TrimSpace(requested.SelectedMode), "auto") {
		selected, err := profile.ApplyModeSelection(config, requested.SelectedMode)
		if err != nil {
			return nil, nil, nil, err
		}
		config = selected
	}
	return config, aliases, nil, nil
}

func profileByID(values []profile.Profile, id string) (profile.Profile, bool) {
	for _, value := range values {
		if value.ID == id {
			return value, true
		}
	}
	return profile.Profile{}, false
}

func (c *RuntimeController) finishStartLocked(
	stage *string,
	profileID string,
	config []byte,
	runtimeAliases map[string]string,
	runtimeCandidates []policy.Candidate,
	engineType profile.Engine,
	probeTimeout time.Duration,
	retryProbe bool,
	connectionPolicy policy.ConnectionPolicy,
	routing policy.RoutingPolicy,
) (RuntimeSnapshot, error) {
	*stage = "connection_policy"
	connectionPolicy.NetworkClass = c.effectiveNetworkClassLocked(connectionPolicy.NetworkClass)
	config, err := profile.ApplyConnectionPolicy(config, connectionPolicy)
	if err != nil {
		c.lastError = safeRuntimeError(err)
		return c.snapshotLocked(), err
	}
	*stage = "traffic_mode"
	config, err = profile.ApplyTrafficMode(config, connectionPolicy.TrafficMode)
	if err != nil {
		c.lastError = safeRuntimeError(err)
		return c.snapshotLocked(), err
	}
	if connectionPolicy.TrafficMode == policy.TrafficTUN && c.TUN != nil {
		*stage = "tun_preflight"
		if err := c.TUN.CheckTUN(); err != nil {
			c.lastError = safeRuntimeError(err)
			return c.snapshotLocked(), err
		}
	}
	*stage = "routing_policy"
	config, err = profile.ApplyRoutingPolicy(config, routing)
	if err != nil {
		c.lastError = safeRuntimeError(err)
		return c.snapshotLocked(), err
	}
	ruleSetSource := append([]byte(nil), config...)
	config, cancelled := c.materializeProviderRuleSets(stage, config, routing)
	if cancelled {
		c.lastError = "start cancelled"
		return c.snapshotLocked(), errors.New("start cancelled")
	}
	*stage = "engine_start"
	lifecycleContext, cancel := context.WithCancel(c.startupContextLocked())
	runtime, effectiveConfig, err := c.startEngineWithRuleSetRetry(lifecycleContext, config, ruleSetSource)
	if err != nil {
		cancel()
		c.lastError = safeRuntimeError(err)
		return c.snapshotLocked(), err
	}
	pendingInspect, inspectErr := inspectRuntimeConfig(effectiveConfig)
	if goruntime.GOOS == "android" && connectionPolicy.TrafficMode == policy.TrafficTUN {
		*stage = "android_hev"
		if _, err := c.startAndroidHevLocked(profileID, androidvpn.SingBoxIPv4, androidvpn.SingBoxPort); err != nil {
			cancel()
			_ = runtime.Stop()
			androidvpn.StopHev()
			c.lastError = safeRuntimeError(err)
			return c.snapshotLocked(), err
		}
	}
	if err := lifecycleContext.Err(); err != nil {
		return c.abortStartLocked(runtime, cancel, err)
	}
	if goruntime.GOOS == "android" && connectionPolicy.TrafficMode == policy.TrafficTUN {
		runtime = &bridgeRuntime{Runtime: runtime, bridge: c.androidBridge}
	}
	c.runtime = runtime
	c.runtimeCancel = cancel
	c.profileID = profileID
	c.runtimeAliases = runtimeAliases
	c.runtimeCandidates = append([]policy.Candidate(nil), runtimeCandidates...)
	c.lastObservedClass = c.networkClass()
	if c.Store != nil && profileID != "" {
		if value, err := c.Store.Load(profileID); err == nil && concreteProfileSelection(value.SelectedMode) {
			country, protocol := selectionLabels(value.Config, value.SelectedMode)
			c.pinManualSelectionLocked(value.SelectedMode, country, protocol)
			connectionPolicy.Mode = policy.ConnectionManualStrict
			selection := value.SelectedMode
			if alias, ok := runtimeAliases[profileID+"::"+selection]; ok {
				selection = alias
			}
			runtimeCandidates = candidatesForSelection(runtimeCandidates, selection)
		}
	}
	c.connectionMode = connectionPolicy.Mode
	c.activeEngine = engineType
	c.startingOverlay = true
	c.inspect = nil
	c.lastError = ""
	if len(runtimeCandidates) > 0 {
		*stage = "node_health"
		err := c.probeFirstStartLocked(lifecycleContext, runtime, runtimeCandidates, connectionPolicy, probeTimeout, retryProbe)
		if errors.Is(err, errStartAborted) {
			return c.snapshotLocked(), nil
		}
		if err != nil {
			return c.abortStartLocked(runtime, cancel, err)
		}
	}
	if c.activeRuntimeTag == "" && len(runtimeCandidates) == 1 {
		c.activeRuntimeTag = runtimeCandidates[0].RuntimeTag
	}
	if err := lifecycleContext.Err(); err != nil {
		return c.abortStartLocked(runtime, cancel, err)
	}
	var restoreDNS func() error
	var restoreProxy func() error
	if connectionPolicy.TrafficMode == policy.TrafficSystemProxy {
		*stage = "system_proxy"
		if c.SystemProxy == nil {
			return c.abortStartLocked(runtime, cancel, errors.New("system proxy is not supported on this platform"))
		}
		restoreProxy, err = c.SystemProxy.Configure("127.0.0.1", profile.SystemProxyPort)
		c.proxyRestore = restoreProxy
		if err != nil {
			return c.abortStartLocked(runtime, cancel, err)
		}
	} else if c.DNS != nil {
		*stage = "dns"
		restoreDNS, err = c.DNS.Configure()
		c.dnsRestore = restoreDNS
		if err != nil {
			return c.abortStartLocked(runtime, cancel, err)
		}
	}
	c.dnsRestore = restoreDNS
	c.proxyRestore = restoreProxy
	c.startingOverlay = false
	if inspectErr == nil {
		inspectCopy := pendingInspect
		c.inspect = &inspectCopy
	}
	c.startRecoveryMonitorLocked(profileID, runtime)
	if routing.ProviderRules && c.RuleSets != nil {
		go c.RuleSets.Watch(c.recoveryContext, ruleSetSource)
	}
	*stage = "complete"
	return c.snapshotLocked(), nil
}

var errStartAborted = errors.New("runtime start aborted")

func (c *RuntimeController) probeFirstStartLocked(
	parent context.Context,
	runtime engine.Runtime,
	candidates []policy.Candidate,
	connectionPolicy policy.ConnectionPolicy,
	timeout time.Duration,
	retry bool,
) error {
	// AWG already checks its independent reserve target inside this attempt.
	for _, candidate := range candidates {
		if c.awgProbers[candidate.RuntimeTag] != nil {
			retry = false
			break
		}
	}
	err := c.probeFirstStartAttemptLocked(parent, runtime, candidates, connectionPolicy, timeout)
	if err != nil && retry && !errors.Is(err, errStartAborted) && c.runtime == runtime {
		err = c.probeFirstStartAttemptLocked(parent, runtime, candidates, connectionPolicy, timeout)
	}
	return err
}

func (c *RuntimeController) probeFirstStartAttemptLocked(
	parent context.Context,
	runtime engine.Runtime,
	candidates []policy.Candidate,
	connectionPolicy policy.ConnectionPolicy,
	timeout time.Duration,
) error {
	awgProbers := maps.Clone(c.awgProbers)
	c.mu.Unlock()
	probeContext, cancel := context.WithTimeout(parent, 30*time.Second)
	outcome, probeErr := probeCandidateGroups(probeContext, c.Journal, runtime, candidates, connectionPolicy, timeout, awgProbers)
	cancel()
	c.mu.Lock()
	if c.runtime != runtime {
		return errStartAborted
	}
	if outcome.skip {
		return nil
	}
	if connectionPolicy.Mode == policy.ConnectionManualStrict && parent.Err() == nil &&
		len(candidates) == 1 && outcome.selected == nil && len(outcome.recorded) == 1 {
		result := outcome.recorded[0]
		if result.source == awgProbeSource && isAWGHTTPSFailure(result.err) {
			c.recordProbeLocked(result)
			return c.selectProbedCandidateLocked(parent, runtime, result.candidate)
		}
	}
	if applyErr := c.applyProbeSelectionLocked(parent, runtime, outcome); applyErr != nil {
		if probeErr != nil {
			return probeErr
		}
		return applyErr
	}
	return probeErr
}

func (c *RuntimeController) abortStartLocked(runtime engine.Runtime, cancel context.CancelFunc, err error) (RuntimeSnapshot, error) {
	if c.runtime == runtime {
		err = errors.Join(err, c.restoreDNSLocked())
	}
	_ = runtime.Stop()
	cancel()
	if c.runtime == runtime {
		c.runtime = nil
		c.runtimeCancel = nil
		c.profileID = ""
		c.runtimeAliases = nil
		c.sessionIDs = nil
		c.activeEngine = ""
	}
	c.resetProbeStateLocked()
	c.lastError = safeRuntimeError(err)
	return c.snapshotLocked(), err
}

func (c *RuntimeController) newAWGSidecar() awgengine.Sidecar {
	c.awgFactoryMu.Lock()
	defer c.awgFactoryMu.Unlock()
	if c.AWGFactory != nil {
		return c.AWGFactory()
	}
	return awgengine.DefaultFactory()()
}

func (c *RuntimeController) stopSidecarsLocked() error {
	var result error
	for _, sidecar := range c.awgSidecars {
		if sidecar == nil {
			continue
		}
		result = errors.Join(result, sidecar.Stop())
	}
	c.awgSidecars = nil
	c.awgProbers = nil
	if c.runtime == nil {
		c.activeEngine = ""
	}
	return result
}

func (c *RuntimeController) sessionContainsLocked(profileID string) bool {
	if profileID == "" {
		return false
	}
	if c.profileID == profileID {
		return true
	}
	_, ok := c.sessionIDs[profileID]
	return ok
}

func (c *RuntimeController) knownRuntimeTargetLocked(target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	if _, ok := c.runtimeAliases[target]; ok {
		return true
	}
	for _, alias := range c.runtimeAliases {
		if alias == target {
			return true
		}
	}
	for _, candidate := range c.runtimeCandidates {
		if candidate.RuntimeTag == target || candidate.SourceTag == target || candidate.ID == target {
			return true
		}
	}
	return awgcfg.IsLeafTag(target)
}

func (c *RuntimeController) activeStoredProfilesLocked() []profile.Profile {
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
	return active
}

func (c *RuntimeController) journalAWGSkip(profileID string, err error) {
	if profileID != "" && err != nil {
		if c.awgSkipReasons == nil {
			c.awgSkipReasons = make(map[string]error)
		}
		c.awgSkipReasons[profileID] = err
	}
	if c.Journal == nil {
		return
	}
	c.Journal.Warn("runtime", "amneziawg_sidecar_skipped", awgcfg.SafeError(err), map[string]any{
		"profile_id": profileID,
	})
}

func (c *RuntimeController) resolveAmneziaSelectLocked(profileID, target string) (string, bool) {
	if target != awgcfg.SelectorTag && !awgcfg.IsLeafTag(target) {
		return "", false
	}
	owner := strings.TrimSpace(profileID)
	if owner == "" {
		owner = profileIDFromRuntimeTag(target)
	}
	keys := []string{target}
	if owner != "" {
		keys = append([]string{owner + "::" + target}, keys...)
	}
	if target == awgcfg.LeafTag || target == awgcfg.SelectorTag {
		keys = append(keys, awgcfg.LeafTag, awgcfg.SelectorTag)
		if owner != "" {
			keys = append(keys, amneziaRuntimeTag(owner))
		}
	}
	for _, key := range keys {
		if alias, ok := c.runtimeAliases[key]; ok {
			return alias, true
		}
	}
	if target != awgcfg.LeafTag && target != awgcfg.SelectorTag && !strings.EqualFold(target, amneziaRuntimeTag(owner)) {
		return "", false
	}
	matches := make([]string, 0, 1)
	seen := map[string]struct{}{}
	for _, candidate := range c.runtimeCandidates {
		if !strings.EqualFold(candidate.Protocol, "AmneziaWG") && !awgcfg.IsLeafTag(candidate.RuntimeTag) {
			continue
		}
		if candidate.SourceTag != awgcfg.LeafTag {
			continue
		}
		if owner != "" && candidate.ProfileID != owner {
			continue
		}
		if _, ok := seen[candidate.RuntimeTag]; ok {
			continue
		}
		seen[candidate.RuntimeTag] = struct{}{}
		matches = append(matches, candidate.RuntimeTag)
	}
	if len(matches) == 1 {
		return matches[0], true
	}
	if strings.Contains(target, "::") {
		return target, true
	}
	return "", false
}

func (c *RuntimeController) amneziaSelectErrorLocked(profileID, requested, runtimeTarget string) error {
	if !awgcfg.IsLeafTag(requested) && requested != awgcfg.SelectorTag && !awgcfg.IsLeafTag(runtimeTarget) {
		return nil
	}
	if runtimeTarget != "" {
		if _, ok := c.runtimeAliases[runtimeTarget]; ok {
			return nil
		}
		for _, candidate := range c.runtimeCandidates {
			if candidate.RuntimeTag == runtimeTarget || candidate.RuntimeTag == requested {
				return nil
			}
		}
	}
	keys := []string{runtimeTarget, requested, strings.TrimSpace(profileID) + "::" + requested}
	for _, key := range keys {
		if err, ok := c.awgSkipReasons[key]; ok {
			return fmt.Errorf("AmneziaWG недоступен: %s", awgcfg.SafeError(err))
		}
	}
	if len(c.awgSkipReasons) == 0 {
		return nil
	}
	return errors.New("AmneziaWG: выбранный узел недоступен")
}

func profileIDFromRuntimeTag(tag string) string {
	tag = strings.TrimSpace(tag)
	separator := strings.LastIndex(tag, "::")
	if separator <= 0 {
		return ""
	}
	if !strings.EqualFold(tag[separator+2:], awgcfg.LeafTag) {
		return ""
	}
	return tag[:separator]
}

func mapAWGStartError(err error) error {
	if err == nil {
		return awgengine.ErrTunnelUnavailable
	}
	if errors.Is(err, awgengine.ErrHandshakeTimeout) || errors.Is(err, awgengine.ErrNotReady) {
		return awgengine.ErrTunnelUnavailable
	}
	return err
}

func hasNonAmneziaCandidate(candidates []policy.Candidate) bool {
	for _, candidate := range candidates {
		if !strings.EqualFold(candidate.Protocol, "AmneziaWG") && !strings.EqualFold(candidate.Type, "amneziawg") {
			return true
		}
	}
	return false
}

func probeTimeoutFor(candidate policy.Candidate, fallback time.Duration) time.Duration {
	if strings.EqualFold(candidate.Protocol, "AmneziaWG") || strings.EqualFold(candidate.Type, "amneziawg") || awgcfg.IsLeafTag(candidate.RuntimeTag) {
		return awgFirstProbeTimeout
	}
	if candidateNeedsLongProbe(candidate) {
		return quicProbeTimeout
	}
	if fallback > 0 {
		return fallback
	}
	return outboundProbeTimeout
}

func candidateNeedsLongProbe(candidate policy.Candidate) bool {
	protocol := strings.ToLower(strings.TrimSpace(candidate.Protocol))
	kind := strings.ToLower(strings.TrimSpace(candidate.Type))
	switch protocol {
	case "tuic", "hysteria2", "hysteria":
		return true
	}
	switch kind {
	case "tuic", "hysteria2", "hysteria":
		return true
	}
	return false
}

func amneziaCandidate(value profile.Profile) policy.Candidate {
	source := awgSourceTag(value)
	return policy.Candidate{
		ID:         value.ID + "::" + source,
		ProfileID:  value.ID,
		SourceTag:  source,
		RuntimeTag: source,
		Country:    value.AWGCountry,
		Protocol:   profile.ProtocolFromType("amneziawg"),
		Type:       "amneziawg",
	}
}

func amneziaUnifiedCandidate(value profile.Profile) policy.Candidate {
	source := awgSourceTag(value)
	runtimeTag := value.ID + "::" + source
	return policy.Candidate{
		ID:         runtimeTag,
		ProfileID:  value.ID,
		SourceTag:  source,
		RuntimeTag: runtimeTag,
		Country:    value.AWGCountry,
		Protocol:   profile.ProtocolFromType("amneziawg"),
		Type:       "amneziawg",
	}
}

func amneziaRuntimeTag(profileID string) string {
	return profileID + "::" + awgcfg.LeafTag
}

func addAmneziaAliases(aliases map[string]string, profileID, runtimeTag string, mapBareLeaf bool) {
	aliases[runtimeTag] = runtimeTag
	if mapBareLeaf {
		aliases[profileID+"::"+awgcfg.LeafTag] = runtimeTag
		aliases[awgcfg.LeafTag] = runtimeTag
		aliases[awgcfg.SelectorTag] = runtimeTag
	}
}

func awgSourceTag(value profile.Profile) string {
	if value.AWGTag != "" {
		return value.AWGTag
	}
	return awgcfg.LeafTag
}

func awgSelectionMatches(value profile.Profile, mode string) bool {
	return mode == awgSourceTag(value) || (mode == awgcfg.LeafTag && (value.AWGLegacy || value.AWGTag == ""))
}

func envelopeAmneziaSelectedForProfiles(values []profile.Profile, id, mode string) bool {
	for _, value := range values {
		if value.ID == id && awgSelectionMatches(value, mode) {
			return true
		}
	}
	return false
}

func selectedModeForProfile(values []profile.Profile, id string) string {
	if value, ok := profileByID(values, id); ok {
		return value.SelectedMode
	}
	return ""
}

func parseLoopbackAddr(addr string) (string, int, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return "", 0, fmt.Errorf("amneziawg socks address is invalid")
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		return "", 0, fmt.Errorf("amneziawg socks port is invalid")
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return host, port, nil
}

const handshakeReadyTimeout = 12 * time.Second
