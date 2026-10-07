package control

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"naga.network/core/diagnostics"
	"naga.network/core/engine"
	awgengine "naga.network/core/engine/amneziawg"
	engineolc "naga.network/core/engine/olcrtc"
	"naga.network/core/policy"
	"naga.network/core/profile"
	awgcfg "naga.network/core/profile/amneziawg"
	olcprofile "naga.network/core/profile/olcrtc"
	"naga.network/core/ruleset"
	"naga.network/core/storage"
)

var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-9;]*[[:alpha:]]`)

type RuntimeController struct {
	Store    *storage.Store
	Adapter  engine.Adapter
	DNS      DNSManager
	TUN      TUNPreflight
	Journal  *diagnostics.Logger
	RuleSets *ruleset.Resolver

	mu              sync.Mutex
	cacheMu         sync.Mutex
	cachedSnapshot  RuntimeSnapshot
	startInProgress atomic.Bool
	runtime         engine.Runtime
	runtimeCancel   context.CancelFunc
	recoveryCancel  context.CancelFunc
	profileID       string
	runtimeAliases  map[string]string
	lastError       string
	dnsRestore      func() error
	proxyRestore    func() error
	SystemProxy     SystemProxyManager
	// NetworkClass is injected by the platform runtime. Keeping it as a
	// callback lets Linux and mobile implementations update the policy without
	// making the domain layer depend on a particular network manager.
	NetworkClass func() policy.NetworkClass
	// HealthInterval and HealthFailThreshold override policy defaults in tests.
	HealthInterval      time.Duration
	HealthFailThreshold int
	// AWGFactory builds the userspace AmneziaWG sidecar. Tests inject a
	// loopback double; production uses netstack + SOCKS5.
	AWGFactory awgengine.Factory

	probeCache        map[string]ProbeCacheEntry
	runtimeCandidates []policy.Candidate
	cooldownUntil     map[string]time.Time
	failoverMessage   string
	activeRuntimeTag  string
	healthFailCount   int
	lastObservedClass policy.NetworkClass
	awgSidecars       []awgengine.Sidecar
	olcProc           *engineolc.Command
	olcHev            *exec.Cmd
	olcHost           engineolc.TunnelHost
	olcLink           engineolc.LinkState
	olcLinkOn         bool
	olcStatsCancel    context.CancelFunc
	olcUpload         int64
	olcDownload       int64
	olcUploadRate     int64
	olcDownloadRate   int64
	olcStatsAt        time.Time
	olcStatsOK        bool
	olcLatency        int
	OlcRTCBinary      string
	HevBinary         string
	awgSkipReasons    map[string]error
	sessionIDs        map[string]struct{}
	activeEngine      profile.Engine
	idleProbeRuntime  engine.Runtime
	idleAWGSidecars   []awgengine.Sidecar
	idleProbing       bool
	connectionMode    policy.ConnectionMode
	startingOverlay   bool
	inspect           *RuntimeInspect
	startOwner        bool
	startAborted      bool
}

const (
	recoveryAttempts     = 3
	recoveryBackoff      = 500 * time.Millisecond
	outboundProbeTimeout = 4 * time.Second
	awgFirstProbeTimeout = 8 * time.Second
)

type RuntimeSnapshot struct {
	Status            string `json:"status"`
	Engine            string `json:"engine,omitempty"`
	ProfileID         string `json:"profile_id,omitempty"`
	Error             string `json:"error,omitempty"`
	UploadBytes       int64  `json:"upload_bytes"`
	DownloadBytes     int64  `json:"download_bytes"`
	UploadRateBytes   int64  `json:"upload_rate_bytes"`
	DownloadRateBytes int64  `json:"download_rate_bytes"`
	SessionStartedAt  string `json:"session_started_at,omitempty"`
	LastTrafficUpdate string `json:"last_traffic_update,omitempty"`
	TrafficAvailable  bool   `json:"traffic_available"`
	ActiveNode        string `json:"active_node,omitempty"`
	ActiveCountry     string `json:"active_country,omitempty"`
	ActiveProtocol    string `json:"active_protocol,omitempty"`
	ActiveLatency     int    `json:"active_latency_ms,omitempty"`
	ActiveNodeStatus  string `json:"active_node_status,omitempty"`
	FailoverMessage   string `json:"failover_message,omitempty"`
}

func NewRuntimeController(store *storage.Store, adapter engine.Adapter, dns ...DNSManager) *RuntimeController {
	controller := &RuntimeController{Store: store, Adapter: adapter}
	if store != nil && strings.TrimSpace(store.Root) != "" {
		controller.RuleSets = ruleset.NewResolver(filepath.Join(store.Root, "rule-sets"), nil)
	}
	if len(dns) > 0 {
		controller.DNS = dns[0]
	}
	return controller
}

func (c *RuntimeController) Start(profileID string) (snapshot RuntimeSnapshot, resultErr error) {
	stage := "request"
	startedAt := time.Now()
	if c.Journal != nil {
		c.Journal.Info("runtime", "start_requested", "Запрошен запуск VPN", map[string]any{"profile_id": profileID})
		defer func() {
			fields := map[string]any{
				"profile_id":  profileID,
				"stage":       stage,
				"duration_ms": time.Since(startedAt).Milliseconds(),
				"status":      snapshot.Status,
			}
			if resultErr != nil {
				c.Journal.Error("runtime", "start_failed", safeRuntimeError(resultErr), fields)
				return
			}
			c.Journal.Info("runtime", "start_completed", "VPN runtime запущен", fields)
		}()
	}
	if c.Store == nil || c.Adapter == nil {
		return c.snapshot(), errors.New("runtime is not configured")
	}
	if profileID == "" {
		return c.snapshot(), errors.New("profile ID is required")
	}

	connectionPolicy, err := c.Store.LoadConnectionPolicy()
	if err != nil {
		return c.snapshot(), err
	}
	routing, err := c.Store.LoadRoutingPolicy()
	if err != nil {
		return c.snapshot(), err
	}

	c.startInProgress.Store(true)
	defer c.startInProgress.Store(false)
	c.mu.Lock()
	owned := false
	if c.startOwner {
		if c.runtime == nil && !c.startingOverlay {
			c.mu.Unlock()
			return c.snapshot(), errors.New("start already in progress")
		}
	} else {
		c.startOwner = true
		c.startAborted = false
		owned = true
	}
	snapshot, resultErr = c.startHoldingLock(&stage, profileID, connectionPolicy, routing)
	if owned {
		c.startOwner = false
	}
	c.mu.Unlock()
	return snapshot, resultErr
}

func (c *RuntimeController) startHoldingLock(
	stage *string,
	profileID string,
	connectionPolicy policy.ConnectionPolicy,
	routing policy.RoutingPolicy,
) (RuntimeSnapshot, error) {
	c.stopIdleProbeLocked()
	if c.olcProc != nil {
		if c.profileID == profileID {
			return c.snapshotLocked(), nil
		}
		return c.snapshotLocked(), errors.New("another profile is already running")
	}
	if c.runtime != nil {
		status := c.runtime.Status()
		if c.startingOverlay || status == engine.Connected || status == engine.Starting || status == engine.Stopping {
			if c.sessionContainsLocked(profileID) {
				return c.snapshotLocked(), nil
			}
			return c.snapshotLocked(), errors.New("another profile is already running")
		}
		_ = c.restoreDNSLocked()
		_ = c.stopSidecarsLocked()
		c.runtime = nil
		c.runtimeCancel = nil
		c.profileID = ""
		c.runtimeAliases = nil
		c.sessionIDs = nil
		c.resetProbeStateLocked()
	}

	*stage = "load_profile"
	value, err := c.Store.Load(profileID)
	if err != nil {
		c.lastError = "profile could not be loaded"
		return c.snapshotLocked(), err
	}
	if value.Subscription.Expired(nowUTC()) {
		c.lastError = "profile subscription has expired"
		return c.snapshotLocked(), errors.New(c.lastError)
	}
	if olcShouldStart(value) {
		return c.startOlcRTCLocked(stage, value)
	}
	activeValues := c.activeStoredProfilesLocked()
	if len(activeValues) == 0 {
		activeValues = []profile.Profile{value}
	}
	singBox := profile.SingBoxProfiles(activeValues)
	awg := profile.AmneziaWGProfiles(activeValues)
	if len(singBox) > 0 {
		return c.startUnifiedLocked(stage, profileID, singBox, awg, connectionPolicy, routing)
	}
	if profile.IsAmneziaWG(value) || len(awg) > 0 {
		return c.startAmneziaOnlyLocked(stage, profileID, awg, connectionPolicy, routing)
	}
	c.lastError = "profile has no supported VPN engine"
	return c.snapshotLocked(), errors.New(c.lastError)
}

type candidateProbeResult struct {
	candidate policy.Candidate
	latency   int
	err       error
}

func candidateProbeGroups(candidates []policy.Candidate, connectionPolicy policy.ConnectionPolicy) [][]policy.Candidate {
	preferred := make([]policy.Candidate, 0)
	remaining := make([]policy.Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		if connectionPolicy.Mode != policy.ConnectionAuto && candidateMatchesPreference(candidate, connectionPolicy) {
			preferred = append(preferred, candidate)
		} else {
			remaining = append(remaining, candidate)
		}
	}
	if connectionPolicy.Mode == policy.ConnectionManualStrict && len(preferred) > 0 {
		return [][]policy.Candidate{preferred}
	}

	groups := make([][]policy.Candidate, 0, 3)
	if connectionPolicy.Mode == policy.ConnectionManualFallback && len(preferred) > 0 {
		groups = append(groups, preferred)
	} else if len(preferred) > 0 {
		remaining = append(preferred, remaining...)
	}
	nonTUIC := make([]policy.Candidate, 0, len(remaining))
	tuic := make([]policy.Candidate, 0, len(remaining))
	for _, candidate := range remaining {
		if candidateIsTUIC(candidate) {
			tuic = append(tuic, candidate)
		} else {
			nonTUIC = append(nonTUIC, candidate)
		}
	}
	if connectionPolicy.Mode == policy.ConnectionAuto && len(nonTUIC) > 0 {
		nonTUIC = policy.RankCandidates(connectionPolicy.NetworkClass, nonTUIC)
		if len(nonTUIC) > policy.AutoCandidateLimit {
			nonTUIC = append([]policy.Candidate(nil), nonTUIC[:policy.AutoCandidateLimit]...)
		}
	}
	if len(nonTUIC) > 0 {
		groups = append(groups, nonTUIC)
	}
	if connectionPolicy.TUICFallbackEnabled && len(tuic) > 0 {
		groups = append(groups, tuic)
	}
	return groups
}

func candidateMatchesPreference(candidate policy.Candidate, connectionPolicy policy.ConnectionPolicy) bool {
	country := strings.TrimSpace(connectionPolicy.PreferredCountry)
	protocol := strings.TrimSpace(connectionPolicy.PreferredProtocol)
	if country == "" && protocol == "" {
		return false
	}
	return (country == "" || strings.EqualFold(country, candidate.Country)) &&
		(protocol == "" || strings.EqualFold(protocol, candidate.Protocol))
}

func candidateIsTUIC(candidate policy.Candidate) bool {
	return strings.EqualFold(strings.TrimSpace(candidate.Protocol), "TUIC") ||
		strings.EqualFold(strings.TrimSpace(candidate.Type), "tuic")
}

// ReplaceProfile atomically installs a validated profile. If this profile is
// part of the running runtime, it performs a controlled reconnect and starts
// the new merged configuration. A failed start restores the previous file and
// brings the previous runtime back.
func (c *RuntimeController) ReplaceProfile(value profile.Profile) (RuntimeSnapshot, error) {
	if c.Store == nil || c.Adapter == nil {
		return c.snapshot(), errors.New("runtime is not configured")
	}
	if value.ID == "" || len(value.Config) == 0 {
		return c.snapshot(), errors.New("profile is incomplete")
	}

	c.mu.Lock()
	if c.olcProc != nil && (c.profileID == value.ID || c.profileID == "") {
		snapshot, err := c.refreshOlcRTCLocked(value)
		c.mu.Unlock()
		return snapshot, err
	}
	runtimeProfileID := ""
	wasRunning := c.runtime != nil && c.IsRuntimeActiveLocked()
	if wasRunning {
		runtimeProfileID = c.profileID
		if c.recoveryCancel != nil {
			c.recoveryCancel()
			c.recoveryCancel = nil
		}
		if err := c.restoreDNSLocked(); err != nil {
			c.mu.Unlock()
			return c.snapshot(), err
		}
		stopErr := c.runtime.Stop()
		if c.runtimeCancel != nil {
			c.runtimeCancel()
		}
		sidecarErr := c.stopSidecarsLocked()
		c.runtime = nil
		c.runtimeCancel = nil
		c.profileID = ""
		c.runtimeAliases = nil
		c.sessionIDs = nil
		c.resetProbeStateLocked()
		if stopErr != nil {
			c.mu.Unlock()
			return c.snapshot(), fmt.Errorf("stop runtime for profile replacement: %w", stopErr)
		}
		if sidecarErr != nil {
			c.mu.Unlock()
			return c.snapshot(), fmt.Errorf("stop amneziawg sidecar for profile replacement: %w", sidecarErr)
		}
	}
	c.mu.Unlock()

	if err := c.Store.SaveVersioned(value); err != nil {
		if wasRunning {
			_, _ = c.Start(runtimeProfileID)
		}
		return c.snapshot(), err
	}
	if !wasRunning {
		return c.Status(), nil
	}

	snapshot, err := c.Start(runtimeProfileID)
	if err == nil {
		return snapshot, nil
	}
	rollbackErr := error(nil)
	if _, rollbackErr = c.Store.Rollback(value.ID); rollbackErr == nil {
		_, _ = c.Start(runtimeProfileID)
	}
	if rollbackErr != nil {
		return c.snapshot(), errors.Join(err, fmt.Errorf("rollback failed: %w", rollbackErr))
	}
	return c.snapshot(), fmt.Errorf("new profile runtime failed; previous version restored: %w", err)
}

// Reconnect rebuilds the current runtime using the latest stored policy and
// profiles. It is used for settings that cannot be applied through selector
// hot-switch, such as route rules and process routing.
func (c *RuntimeController) Reconnect() (RuntimeSnapshot, error) {
	c.mu.Lock()
	profileID := c.profileID
	running := c.runtime != nil && c.IsRuntimeActiveLocked()
	c.mu.Unlock()
	if !running || profileID == "" {
		return c.Status(), nil
	}
	if _, err := c.Stop(); err != nil {
		return c.Status(), err
	}
	return c.Start(profileID)
}

func (c *RuntimeController) startRecoveryMonitorLocked(profileID string, runtime engine.Runtime) {
	if c.recoveryCancel != nil {
		c.recoveryCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.recoveryCancel = cancel
	go c.monitorRuntime(ctx, profileID, runtime)
	go c.monitorActiveHealth(ctx)
}

func (c *RuntimeController) monitorRuntime(ctx context.Context, profileID string, runtime engine.Runtime) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if runtime.Status() != engine.Failed {
			continue
		}
		if c.Journal != nil {
			message := "sing-box завершился неожиданно"
			if reporter, ok := runtime.(engine.FailureReporter); ok {
				if err := reporter.Failure(); err != nil {
					message = safeRuntimeError(err)
				}
			}
			c.Journal.Error("runtime", "unexpected_failure", message, map[string]any{"profile_id": profileID})
		}
		for attempt := 0; attempt < recoveryAttempts; attempt++ {
			if attempt > 0 {
				timer := time.NewTimer(recoveryBackoff * time.Duration(1<<(attempt-1)))
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
			}
			if c.Journal != nil {
				c.Journal.Warn("runtime", "recovery_attempt", "Попытка восстановить VPN runtime", map[string]any{
					"profile_id": profileID,
					"attempt":    attempt + 1,
				})
			}
			if _, err := c.restartAfterFailure(ctx, profileID, runtime); err == nil {
				if c.Journal != nil {
					c.Journal.Info("runtime", "recovery_completed", "VPN runtime восстановлен", map[string]any{
						"profile_id": profileID,
						"attempt":    attempt + 1,
					})
				}
				return
			}
			if ctx.Err() != nil {
				return
			}
		}
		c.mu.Lock()
		if c.runtime == runtime {
			c.lastError = "sing-box не удалось автоматически восстановить после сбоя"
			if c.Journal != nil {
				c.Journal.Error("runtime", "recovery_exhausted", c.lastError, map[string]any{
					"profile_id": profileID,
					"attempts":   recoveryAttempts,
				})
			}
			if c.recoveryCancel != nil {
				c.recoveryCancel()
				c.recoveryCancel = nil
			}
		}
		c.mu.Unlock()
		return
	}
}

func (c *RuntimeController) restartAfterFailure(ctx context.Context, profileID string, failed engine.Runtime) (RuntimeSnapshot, error) {
	c.mu.Lock()
	if c.runtime != failed || c.profileID != profileID || failed.Status() != engine.Failed {
		snapshot := c.snapshotLocked()
		c.mu.Unlock()
		return snapshot, errors.New("runtime changed during recovery")
	}
	_ = c.restoreDNSLocked()
	_ = c.stopSidecarsLocked()
	c.runtime = nil
	c.runtimeCancel = nil
	c.profileID = ""
	c.runtimeAliases = nil
	c.sessionIDs = nil
	c.resetProbeStateLocked()
	c.mu.Unlock()

	select {
	case <-ctx.Done():
		return c.Status(), ctx.Err()
	default:
	}
	return c.Start(profileID)
}

func (c *RuntimeController) networkClass() policy.NetworkClass {
	if c.NetworkClass != nil {
		value := c.NetworkClass()
		switch value {
		case policy.NetworkWiFi, policy.NetworkCellular, policy.NetworkEthernet:
			return value
		}
	}
	return policy.NetworkUnknown
}

func (c *RuntimeController) effectiveNetworkClassLocked(stored policy.NetworkClass) policy.NetworkClass {
	if c.lastObservedClass != policy.NetworkUnknown {
		return c.lastObservedClass
	}
	if live := c.networkClass(); live != policy.NetworkUnknown {
		return live
	}
	return stored
}

func (c *RuntimeController) Stop() (snapshot RuntimeSnapshot, resultErr error) {
	startedAt := time.Now()
	if c.Journal != nil {
		c.Journal.Info("runtime", "stop_requested", "Запрошена остановка VPN", nil)
		defer func() {
			fields := map[string]any{
				"duration_ms": time.Since(startedAt).Milliseconds(),
				"status":      snapshot.Status,
			}
			if resultErr != nil {
				c.Journal.Error("runtime", "stop_failed", safeRuntimeError(resultErr), fields)
				return
			}
			c.Journal.Info("runtime", "stop_completed", "VPN runtime остановлен", fields)
		}()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.startAborted = true
	olcEngine := c.activeEngine == profile.EngineOlcRTC
	if err := c.stopOlcRTCLocked(); err != nil {
		c.lastError = safeRuntimeError(err)
		return c.snapshotLocked(), err
	}
	c.stopIdleProbeLocked()
	if c.recoveryCancel != nil {
		c.recoveryCancel()
		c.recoveryCancel = nil
	}
	if c.runtime == nil {
		if !olcEngine {
			if err := c.restoreDNSLocked(); err != nil {
				c.lastError = safeRuntimeError(err)
				return c.snapshotLocked(), err
			}
		}
		if err := c.stopSidecarsLocked(); err != nil {
			c.lastError = safeRuntimeError(err)
			return c.snapshotLocked(), err
		}
		c.profileID = ""
		c.runtimeAliases = nil
		c.sessionIDs = nil
		c.resetProbeStateLocked()
		c.lastError = ""
		return c.snapshotLocked(), nil
	}
	dnsErr := c.restoreDNSLocked()
	err := c.runtime.Stop()
	if c.runtimeCancel != nil {
		c.runtimeCancel()
		c.runtimeCancel = nil
	}
	c.runtime = nil
	c.profileID = ""
	c.runtimeAliases = nil
	c.sessionIDs = nil
	c.resetProbeStateLocked()
	sidecarErr := c.stopSidecarsLocked()
	if err == nil {
		err = dnsErr
	} else if dnsErr != nil {
		err = errors.Join(err, dnsErr)
	}
	if err == nil {
		err = sidecarErr
	} else if sidecarErr != nil {
		err = errors.Join(err, sidecarErr)
	}
	if err != nil {
		c.lastError = safeRuntimeError(err)
		return c.snapshotLocked(), err
	}
	c.lastError = ""
	return c.snapshotLocked(), nil
}

// Select changes the active leaf outbound without stopping the runtime.
// profileID is optional for callers operating on a merged runtime; when it is
// provided, the target is validated against that saved profile first.
func (c *RuntimeController) Select(profileID, target string) (snapshot RuntimeSnapshot, resultErr error) {
	if c.Journal != nil {
		c.Journal.Info("runtime", "select_requested", "Запрошена смена VPN-узла", map[string]any{
			"profile_id": profileID,
			"node":       target,
		})
		defer func() {
			fields := map[string]any{"profile_id": profileID, "node": target, "status": snapshot.Status}
			if resultErr != nil {
				c.Journal.Error("runtime", "select_failed", safeRuntimeError(resultErr), fields)
				return
			}
			c.Journal.Info("runtime", "select_completed", "VPN-узел переключён", fields)
		}()
	}
	if strings.TrimSpace(target) == "" {
		return c.snapshot(), errors.New("outbound is required")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.runtime == nil || !c.IsRuntimeActiveLocked() {
		return c.snapshotLocked(), errors.New("runtime is not connected")
	}
	if profileID != "" {
		if !c.sessionContainsLocked(profileID) {
			return c.snapshotLocked(), errors.New("another profile is active")
		}
		value, err := c.Store.Load(profileID)
		if err != nil {
			return c.snapshotLocked(), err
		}
		runtimeConfig := profile.RuntimeSingBoxConfig(value.Config)
		if profile.IsAmneziaWG(value) {
			if target != awgcfg.LeafTag && target != value.ID+"::"+awgcfg.LeafTag && target != awgcfg.SelectorTag && !c.knownRuntimeTargetLocked(target) {
				return c.snapshotLocked(), errors.New("selected node is not available in profile")
			}
		} else {
			available, err := profileContainsSelectTarget(runtimeConfig, target)
			if err != nil || !available {
				if !c.knownRuntimeTargetLocked(target) && !awgcfg.IsLeafTag(target) {
					return c.snapshotLocked(), errors.New("selected node is not available in profile")
				}
			}
		}
	}
	runtimeTarget := c.resolveSelectTargetLocked(profileID, target)
	if err := c.amneziaSelectErrorLocked(profileID, target, runtimeTarget); err != nil {
		c.lastError = awgcfg.SafeError(err)
		return c.snapshotLocked(), err
	}
	switcher, ok := c.runtime.(engine.SelectorSwitcher)
	if !ok {
		return c.snapshotLocked(), errors.New("runtime does not support hot node selection")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := switcher.Select(ctx, runtimeTarget); err != nil {
		c.lastError = safeRuntimeError(err)
		return c.snapshotLocked(), err
	}
	c.activeRuntimeTag = runtimeTarget
	c.healthFailCount = 0
	c.failoverMessage = ""
	c.lastError = ""
	sourceTag := c.sourceTagLocked(runtimeTarget, target)
	c.persistSelectedModeLocked(sourceTag)
	country, protocol := "", ""
	if candidate, ok := c.candidateByRuntimeTagLocked(runtimeTarget); ok {
		country = candidate.Country
		protocol = candidate.Protocol
	}
	c.pinManualSelectionLocked(sourceTag, country, protocol)
	return c.snapshotLocked(), nil
}

func concreteProfileSelection(mode string) bool {
	mode = strings.TrimSpace(mode)
	return mode != "" && !strings.EqualFold(mode, "auto") && mode != profile.UnifiedSelectorTag
}

// candidatesForSelection keeps a manually chosen leaf in place. Automatic
// mode and the urltest tag still receive the full ranked list. An unknown
// concrete tag yields no candidates so a later health pass cannot wander.
func candidatesForSelection(candidates []policy.Candidate, selected string) []policy.Candidate {
	if !concreteProfileSelection(selected) {
		return candidates
	}
	matched := make([]policy.Candidate, 0, 1)
	for _, candidate := range candidates {
		if candidate.SourceTag == selected || candidate.RuntimeTag == selected || candidate.ID == selected {
			matched = append(matched, candidate)
		}
	}
	return matched
}

func (c *RuntimeController) PinManualSelection(sourceTag, country, protocol string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pinManualSelectionLocked(sourceTag, country, protocol)
}

func selectionLabels(raw []byte, mode string) (string, string) {
	if meta, err := profile.InspectOutboundMetadata(profile.RuntimeSingBoxConfig(raw)); err == nil {
		if node, ok := meta[mode]; ok {
			return node.Country, node.Protocol
		}
	}
	return profile.NodeMetadata(mode, "").Country, ""
}

func (c *RuntimeController) pinManualSelectionLocked(sourceTag, country, protocol string) {
	if c.Store == nil || !concreteProfileSelection(sourceTag) {
		return
	}
	if err := c.Store.PatchConnectionPolicy(func(current *policy.ConnectionPolicy) error {
		current.Mode = policy.ConnectionManualStrict
		if strings.TrimSpace(country) != "" {
			current.PreferredCountry = country
		}
		if strings.TrimSpace(protocol) != "" {
			current.PreferredProtocol = protocol
		}
		return nil
	}); err != nil {
		return
	}
	c.connectionMode = policy.ConnectionManualStrict
}

func (c *RuntimeController) resolveSelectTargetLocked(profileID, target string) string {
	target = strings.TrimSpace(target)
	if target == "" {
		return target
	}
	if resolved, ok := c.resolveAmneziaSelectLocked(profileID, target); ok {
		return resolved
	}
	keys := []string{target}
	if profileID != "" {
		keys = append(keys, profileID+"::"+target)
	}
	if c.profileID != "" && c.profileID != profileID {
		keys = append(keys, c.profileID+"::"+target)
	}
	for id := range c.sessionIDs {
		if id != "" && id != profileID && id != c.profileID {
			keys = append(keys, id+"::"+target)
		}
	}
	for _, key := range keys {
		if alias, ok := c.runtimeAliases[key]; ok {
			return alias
		}
	}
	for _, alias := range c.runtimeAliases {
		if alias == target {
			return target
		}
	}
	source := sourceTagFromRuntime(target)
	if source != target {
		if awgcfg.IsLeafTag(source) {
			return target
		}
		return c.resolveSelectTargetLocked(profileID, source)
	}
	return target
}

func (c *RuntimeController) sourceTagLocked(runtimeTarget, requested string) string {
	if candidate, ok := c.candidateByRuntimeTagLocked(runtimeTarget); ok {
		return candidate.SourceTag
	}
	if source := sourceTagFromRuntime(runtimeTarget); source != "" {
		return source
	}
	return sourceTagFromRuntime(requested)
}

func (c *RuntimeController) persistSelectedModeLocked(sourceTag string) {
	sourceTag = strings.TrimSpace(sourceTag)
	if sourceTag == "" || c.Store == nil {
		return
	}
	if strings.EqualFold(sourceTag, "auto") || sourceTag == profile.UnifiedSelectorTag {
		return
	}
	ownerID := c.profileID
	if candidate, ok := c.candidateByRuntimeTagLocked(c.activeRuntimeTag); ok && candidate.ProfileID != "" {
		ownerID = candidate.ProfileID
	}
	if ownerID == "" {
		return
	}
	value, err := c.Store.Load(ownerID)
	if err != nil {
		return
	}
	if profile.IsAmneziaWG(value) {
		if sourceTag != awgcfg.LeafTag && !awgcfg.IsLeafTag(sourceTag) {
			return
		}
		sourceTag = awgcfg.LeafTag
	} else {
		available, err := profile.ContainsNode(profile.RuntimeSingBoxConfig(value.Config), sourceTag)
		if err != nil || !available {
			return
		}
	}
	if value.SelectedMode == sourceTag {
		return
	}
	value.SelectedMode = sourceTag
	_ = c.Store.Save(value)
}

func (c *RuntimeController) IsRuntimeActiveLocked() bool {
	if c.runtime == nil {
		return false
	}
	status := c.runtime.Status()
	return status == engine.Starting || status == engine.Connected
}

func (c *RuntimeController) restoreDNSLocked() error {
	var result error
	if c.dnsRestore != nil {
		restore := c.dnsRestore
		c.dnsRestore = nil
		result = restore()
	}
	if c.proxyRestore != nil {
		restore := c.proxyRestore
		c.proxyRestore = nil
		if err := restore(); err != nil {
			result = errors.Join(result, err)
		}
	}
	return result
}

func (c *RuntimeController) IsActive(profileID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.olcProc != nil && c.profileID == profileID {
		return true
	}
	if c.runtime == nil || !c.sessionContainsLocked(profileID) {
		return false
	}
	status := c.runtime.Status()
	return status == engine.Starting || status == engine.Connected || status == engine.Stopping
}

func (c *RuntimeController) IsRunning() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.olcProc != nil {
		return true
	}
	if c.runtime == nil {
		return false
	}
	status := c.runtime.Status()
	return status == engine.Starting || status == engine.Connected || status == engine.Stopping
}

func (c *RuntimeController) Status() RuntimeSnapshot {
	if !c.mu.TryLock() {
		c.cacheMu.Lock()
		snapshot := c.cachedSnapshot
		c.cacheMu.Unlock()
		if c.startInProgress.Load() &&
			snapshot.Status != string(engine.Connected) &&
			snapshot.Status != string(engine.Starting) {
			snapshot.Status = string(engine.Starting)
		}
		return snapshot
	}
	defer c.mu.Unlock()
	return c.snapshotLocked()
}

func (c *RuntimeController) snapshot() RuntimeSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.snapshotLocked()
}

func (c *RuntimeController) snapshotLocked() RuntimeSnapshot {
	snapshot := RuntimeSnapshot{
		Status:          string(engine.Stopped),
		ProfileID:       c.profileID,
		Error:           c.lastError,
		FailoverMessage: c.failoverMessage,
	}
	if c.activeEngine != "" {
		snapshot.Engine = string(c.activeEngine)
	} else if c.Adapter != nil {
		snapshot.Engine = string(c.Adapter.Type())
	}
	if c.olcProc != nil {
		snapshot.Status = string(engine.Connected)
		snapshot.Engine = string(profile.EngineOlcRTC)
		snapshot.ActiveNode = c.activeRuntimeTag
		if snapshot.ActiveNode == "" {
			snapshot.ActiveNode = olcprofile.NodeTag
		}
		snapshot.ActiveCountry = profile.NodeMetadata(snapshot.ActiveNode, "olcrtc").Country
		snapshot.ActiveProtocol = "olcRTC"
		snapshot.UploadBytes = c.olcUpload
		snapshot.DownloadBytes = c.olcDownload
		snapshot.UploadRateBytes = c.olcUploadRate
		snapshot.DownloadRateBytes = c.olcDownloadRate
		snapshot.TrafficAvailable = c.olcStatsOK
		if c.olcLatency > 0 {
			snapshot.ActiveLatency = c.olcLatency
		}
		if c.olcStatsOK && !c.olcStatsAt.IsZero() {
			snapshot.LastTrafficUpdate = c.olcStatsAt.UTC().Format(time.RFC3339)
		}
	} else if c.runtime != nil {
		if c.startingOverlay {
			snapshot.Status = string(engine.Starting)
		} else {
			snapshot.Status = string(c.runtime.Status())
		}
		stats := c.runtime.Stats()
		snapshot.UploadBytes = stats.Upload
		snapshot.DownloadBytes = stats.Download
		snapshot.UploadRateBytes = stats.UploadRate
		snapshot.DownloadRateBytes = stats.DownloadRate
		snapshot.TrafficAvailable = stats.Available
		if !stats.SessionStarted.IsZero() {
			snapshot.SessionStartedAt = stats.SessionStarted.UTC().Format(time.RFC3339)
		}
		if !stats.LastUpdated.IsZero() {
			snapshot.LastTrafficUpdate = stats.LastUpdated.UTC().Format(time.RFC3339)
		}
		if reporter, ok := c.runtime.(engine.DetailReporter); ok {
			details := reporter.Details()
			snapshot.ActiveNode = details.Node
			snapshot.ActiveCountry = details.Country
			snapshot.ActiveProtocol = details.Protocol
			snapshot.ActiveLatency = details.Latency
			snapshot.ActiveNodeStatus = details.Status
			if clashLeafIsConcrete(details) {
				if candidate, ok := c.candidateByDisplayedNodeLocked(details.Node); ok {
					c.activeRuntimeTag = candidate.RuntimeTag
					snapshot.ActiveNode = candidate.SourceTag
					if candidate.Country != "" {
						snapshot.ActiveCountry = candidate.Country
					}
					if candidate.Protocol != "" {
						snapshot.ActiveProtocol = candidate.Protocol
					}
				}
			} else if c.activeRuntimeTag != "" {
				c.applyProbeNodeLocked(&snapshot)
			}
		} else if c.activeRuntimeTag != "" {
			c.applyProbeNodeLocked(&snapshot)
		}
		if entry, ok := c.activeProbeEntryLocked(); ok {
			switch entry.Status {
			case ProbeHealthy:
				if entry.LatencyMS > 0 {
					snapshot.ActiveLatency = entry.LatencyMS
					if snapshot.ActiveNodeStatus == "" || snapshot.ActiveNodeStatus == "checking" {
						snapshot.ActiveNodeStatus = string(ProbeHealthy)
					}
				}
			case ProbeFailed:
				snapshot.ActiveLatency = 0
			}
		}
		if snapshot.Status == string(engine.Failed) {
			_ = c.restoreDNSLocked()
			if reporter, ok := c.runtime.(engine.FailureReporter); ok {
				if err := reporter.Failure(); err != nil {
					c.lastError = safeRuntimeError(err)
					snapshot.Error = c.lastError
				}
			}
		}
	}
	c.cacheMu.Lock()
	c.cachedSnapshot = snapshot
	c.cacheMu.Unlock()
	return snapshot
}

func nowUTC() (value time.Time) {
	return time.Now().UTC()
}

const (
	tunCapabilityError    = "sing-box не получил CAP_NET_ADMIN для создания TUN. Выдай capability бинарнику sing-box и повтори подключение."
	tunWindowsStuckError  = "Не удалось создать сетевой адаптер VPN: старый TUN ещё занят. Закрой другие VPN и повтори подключение, при необходимости перезагрузи Windows."
	tunWindowsCreateError = "Не удалось создать TUN-адаптер Windows. Подтверди запрос UAC и повтори подключение."
)

func tunCreateErrorMessage(message string) string {
	lower := strings.ToLower(message)
	stuck := strings.Contains(lower, "already exists") ||
		strings.Contains(lower, "element not found") ||
		strings.Contains(lower, "cannot create a file")
	if stuck {
		return tunWindowsStuckError
	}
	if runtime.GOOS == "windows" {
		return tunWindowsCreateError
	}
	return tunCapabilityError
}

func safeRuntimeError(err error) string {
	if err == nil {
		return ""
	}
	message := ansiEscapePattern.ReplaceAllString(awgcfg.SafeError(err), "")
	if strings.Contains(message, "TUNSETIFF") || strings.Contains(message, "configure tun interface") {
		return tunCreateErrorMessage(message)
	}
	lines := strings.Split(strings.TrimSpace(message), "\n")
	for index := len(lines) - 1; index >= 0; index-- {
		line := strings.TrimSpace(lines[index])
		upper := strings.ToUpper(line)
		if strings.Contains(upper, "FATAL") ||
			strings.Contains(upper, "ERROR") ||
			strings.Contains(upper, "OPERATION NOT PERMITTED") {
			return truncateRuntimeError(line)
		}
	}
	return truncateRuntimeError(message)
}

func truncateRuntimeError(message string) string {
	message = strings.TrimSpace(message)
	if len(message) > 600 {
		return "…" + message[len(message)-599:]
	}
	return message
}

// identityUnifiedAliases maps GET /v1/nodes runtime tags (profileID::source)
// onto the original outbound tags used when the session is not merged.
func identityUnifiedAliases(profileID string, config []byte) map[string]string {
	aliases := make(map[string]string)
	options, err := profile.InspectMode(config)
	if err != nil {
		return aliases
	}
	var walk func(profile.Node)
	walk = func(node profile.Node) {
		tag := strings.TrimSpace(node.Tag)
		if tag != "" {
			aliases[tag] = tag
			if profileID != "" {
				aliases[profileID+"::"+tag] = tag
			}
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	for _, node := range options.Nodes {
		walk(node)
	}
	return aliases
}

func sourceTagFromRuntime(tag string) string {
	tag = strings.TrimSpace(tag)
	if separator := strings.LastIndex(tag, "::"); separator >= 0 {
		return tag[separator+2:]
	}
	return tag
}

func profileContainsSelectTarget(config []byte, target string) (bool, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return false, nil
	}
	ok, err := profile.ContainsNode(config, target)
	if err != nil || ok {
		return ok, err
	}
	source := sourceTagFromRuntime(target)
	if source == target {
		return false, nil
	}
	return profile.ContainsNode(config, source)
}
