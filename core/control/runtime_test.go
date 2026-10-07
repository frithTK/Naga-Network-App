package control

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"naga.network/core/engine"
	awgengine "naga.network/core/engine/amneziawg"
	"naga.network/core/engine/singbox"
	"naga.network/core/policy"
	"naga.network/core/profile"
	awgcfg "naga.network/core/profile/amneziawg"
	"naga.network/core/storage"
)

type fakeDNSManager struct {
	configured bool
	restored   bool
	restoreErr error
}

type fakeTUNPreflight struct {
	err   error
	calls int
}

func (p *fakeTUNPreflight) CheckTUN() error {
	p.calls++
	return p.err
}

type selectableRuntime struct {
	mu          sync.Mutex
	status      engine.Status
	selected    string
	stopCount   int
	selectCount int
}

func (r *selectableRuntime) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stopCount++
	r.status = engine.Stopped
	return nil
}

func (r *selectableRuntime) Status() engine.Status {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.status
}

func (r *selectableRuntime) Stats() engine.TrafficStats { return engine.TrafficStats{} }

func (r *selectableRuntime) Select(_ context.Context, outbound string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.selectCount++
	r.selected = outbound
	return nil
}

func (r *selectableRuntime) selectedTag() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.selected
}

func (r *selectableRuntime) selects() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.selectCount
}

func (r *selectableRuntime) stops() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stopCount
}

func (r *selectableRuntime) setStatus(status engine.Status) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status = status
}

type selectableAdapter struct {
	runtime *selectableRuntime
}

type probeOutcome struct {
	latency int
	err     error
}

type probingRuntime struct {
	selectableRuntime
	mu       sync.Mutex
	outcomes map[string]probeOutcome
	attempts []string
	timeouts map[string]time.Duration
	delay    time.Duration
}

func (r *probingRuntime) Probe(_ context.Context, outbound string, timeout time.Duration) (int, error) {
	if r.delay > 0 {
		time.Sleep(r.delay)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.attempts = append(r.attempts, outbound)
	if r.timeouts == nil {
		r.timeouts = make(map[string]time.Duration)
	}
	r.timeouts[outbound] = timeout
	result, ok := r.outcomes[outbound]
	if !ok {
		return 0, errors.New("unexpected probe")
	}
	return result.latency, result.err
}

type reportingRuntime struct {
	*probingRuntime
	details engine.RuntimeDetails
}

func (r *reportingRuntime) Details() engine.RuntimeDetails {
	return r.details
}

type reportingAdapter struct {
	runtime *reportingRuntime
}

func (reportingAdapter) Type() engine.Type                      { return engine.SingBox }
func (reportingAdapter) Validate(context.Context, []byte) error { return nil }
func (a reportingAdapter) Start(context.Context, []byte) (engine.Runtime, error) {
	a.runtime.setStatus(engine.Connected)
	return a.runtime, nil
}

func (r *probingRuntime) setOutcome(outbound string, outcome probeOutcome) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.outcomes[outbound] = outcome
}

func (r *probingRuntime) wasProbed(outbound string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, attempt := range r.attempts {
		if attempt == outbound {
			return true
		}
	}
	return false
}

func (r *probingRuntime) probeCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.attempts)
}

type probingAdapter struct {
	runtime *probingRuntime
}

func (probingAdapter) Type() engine.Type                      { return engine.SingBox }
func (probingAdapter) Validate(context.Context, []byte) error { return nil }
func (a probingAdapter) Start(context.Context, []byte) (engine.Runtime, error) {
	a.runtime.setStatus(engine.Connected)
	return a.runtime, nil
}

type replacementAdapter struct {
	mu      sync.Mutex
	starts  int
	runtime *selectableRuntime
}

func (a *replacementAdapter) Type() engine.Type                      { return engine.SingBox }
func (a *replacementAdapter) Validate(context.Context, []byte) error { return nil }
func (a *replacementAdapter) Start(context.Context, []byte) (engine.Runtime, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.starts++
	if a.starts == 2 {
		return nil, errors.New("new runtime failed to start")
	}
	a.runtime.setStatus(engine.Connected)
	return a.runtime, nil
}

type recoverableRuntime struct {
	mu     sync.RWMutex
	status engine.Status
}

func (r *recoverableRuntime) Stop() error {
	r.mu.Lock()
	r.status = engine.Stopped
	r.mu.Unlock()
	return nil
}

func (r *recoverableRuntime) Status() engine.Status {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.status
}

func (r *recoverableRuntime) Stats() engine.TrafficStats { return engine.TrafficStats{} }

func (r *recoverableRuntime) fail() {
	r.mu.Lock()
	r.status = engine.Failed
	r.mu.Unlock()
}

type recoveringAdapter struct {
	mu      sync.Mutex
	starts  int
	current *recoverableRuntime
	first   *recoverableRuntime
}

func (a *recoveringAdapter) Type() engine.Type                      { return engine.SingBox }
func (a *recoveringAdapter) Validate(context.Context, []byte) error { return nil }
func (a *recoveringAdapter) Start(context.Context, []byte) (engine.Runtime, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.starts++
	runtime := &recoverableRuntime{status: engine.Connected}
	if a.starts == 1 {
		a.first = runtime
	}
	a.current = runtime
	return runtime, nil
}

func (selectableAdapter) Type() engine.Type { return engine.SingBox }
func (selectableAdapter) Validate(context.Context, []byte) error {
	return nil
}
func (a selectableAdapter) Start(context.Context, []byte) (engine.Runtime, error) {
	a.runtime.setStatus(engine.Connected)
	return a.runtime, nil
}

func (m *fakeDNSManager) Configure() (func() error, error) {
	m.configured = true
	return func() error {
		m.restored = true
		return m.restoreErr
	}, nil
}

func TestSafeRuntimeErrorKeepsFatalLine(t *testing.T) {
	err := errors.New("exit status 1: \x1b[33mWARN\x1b[0m legacy DNS\n\x1b[31mFATAL\x1b[0m start service: TUNSETIFF: operation not permitted")

	if got := safeRuntimeError(err); got != tunCapabilityError && got != tunWindowsCreateError {
		t.Fatalf("safeRuntimeError() = %q", got)
	}
	if runtime.GOOS != "windows" {
		if got := safeRuntimeError(err); got != tunCapabilityError {
			t.Fatalf("linux safeRuntimeError() = %q", got)
		}
	}

	err = errors.New("exit status 1: warning output\nFATAL start service: connection refused")
	if got := safeRuntimeError(err); !strings.Contains(got, "FATAL start service: connection refused") {
		t.Fatalf("safeRuntimeError() dropped fatal line: %q", got)
	}
}

func TestSafeRuntimeErrorMapsWindowsStuckTUN(t *testing.T) {
	err := errors.New("exit status 1: FATAL start inbound/tun[tun-in]: configure tun interface: (create adapter: Cannot create a file when that file already exists. | open existing adapter: Element not found.)")
	if got := safeRuntimeError(err); got != tunWindowsStuckError {
		t.Fatalf("stuck tun = %q", got)
	}
}

func TestStatusDoesNotBlockWhileStartHoldsLock(t *testing.T) {
	controller := &RuntimeController{}
	controller.startInProgress.Store(true)
	controller.mu.Lock()
	defer controller.mu.Unlock()

	done := make(chan RuntimeSnapshot, 1)
	go func() {
		done <- controller.Status()
	}()
	select {
	case snapshot := <-done:
		if snapshot.Status != string(engine.Starting) {
			t.Fatalf("status while starting = %#v", snapshot)
		}
	case <-time.After(time.Second):
		t.Fatal("Status blocked while Start holds the runtime mutex")
	}
}

func TestRuntimeControllerStartsAndStopsSavedProfile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh as a stand-in sing-box process")
	}
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	value := profile.Profile{
		ID:             "profile-runtime",
		Name:           "Runtime test",
		Engine:         profile.EngineSingBox,
		Config:         []byte(`{"outbounds":[{"type":"direct"}]}`),
		ImportedAt:     time.Now().UTC(),
		UpdateInterval: 24 * time.Hour,
	}
	if err := store.Save(value); err != nil {
		t.Fatal(err)
	}

	controller := NewRuntimeController(
		&store,
		singbox.Adapter{BinaryPath: "/bin/sh", Args: []string{"-c", "sleep 10"}},
	)
	dns := &fakeDNSManager{}
	controller.DNS = dns
	started, err := controller.Start(value.ID)
	if err != nil {
		t.Fatal(err)
	}
	if started.Status != "connected" || started.ProfileID != value.ID {
		t.Fatalf("start snapshot = %#v", started)
	}
	if _, err := controller.Stop(); err != nil {
		t.Fatal(err)
	}
	if !dns.configured || !dns.restored {
		t.Fatalf("DNS lifecycle = configured:%v restored:%v", dns.configured, dns.restored)
	}
	if stopped := controller.Status(); stopped.Status != "stopped" || stopped.ProfileID != "" {
		t.Fatalf("stop snapshot = %#v", stopped)
	}
}

func TestRuntimeControllerRejectsTUNConflictBeforeAdapterStart(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	value := profile.Profile{
		ID:     "profile-tun-conflict",
		Engine: profile.EngineSingBox,
		Config: []byte(`{"outbounds":[{"type":"direct","tag":"direct"}]}`),
	}
	if err := store.Save(value); err != nil {
		t.Fatal(err)
	}
	preflight := &fakeTUNPreflight{err: errors.New("другой TUN-интерфейс уже активен: Meta")}
	adapter := &flakyAdapter{runtime: &testRuntime{status: engine.Stopped}}
	controller := NewRuntimeController(&store, adapter)
	controller.TUN = preflight

	if _, err := controller.Start(value.ID); err == nil || !strings.Contains(err.Error(), "Meta") {
		t.Fatalf("Start() error = %v, want TUN conflict", err)
	}
	if preflight.calls != 1 {
		t.Fatalf("preflight calls = %d, want 1", preflight.calls)
	}
	if adapter.startCount() != 0 {
		t.Fatalf("adapter starts = %d, want 0", adapter.startCount())
	}
}

func TestRuntimeControllerSelectsNodeWithoutStoppingRuntime(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	value := profile.Profile{
		ID:     "profile-select",
		Engine: profile.EngineSingBox,
		Config: []byte(`{"route":{"final":"Mode"},"outbounds":[{"type":"selector","tag":"Mode","outbounds":["fast"],"default":"fast"},{"type":"vless","tag":"fast"}]}`),
	}
	if err := store.Save(value); err != nil {
		t.Fatal(err)
	}
	runtime := &selectableRuntime{}
	controller := NewRuntimeController(&store, selectableAdapter{runtime: runtime})
	if _, err := controller.Start(value.ID); err != nil {
		t.Fatal(err)
	}
	snapshot, err := controller.Select(value.ID, "fast")
	if err != nil {
		t.Fatal(err)
	}
	if runtime.selectedTag() != "fast" || snapshot.Status != string(engine.Connected) {
		t.Fatalf("selected=%q snapshot=%#v", runtime.selectedTag(), snapshot)
	}
}

func TestRuntimeControllerSelectAcceptsUnifiedRuntimeTagWithoutMerge(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	value := profile.Profile{
		ID:     "profile-select",
		Engine: profile.EngineSingBox,
		Config: []byte(`{"route":{"final":"Mode"},"outbounds":[{"type":"selector","tag":"Mode","outbounds":["fast","slow"],"default":"fast"},{"type":"vless","tag":"fast"},{"type":"vless","tag":"slow"}]}`),
	}
	if err := store.Save(value); err != nil {
		t.Fatal(err)
	}
	runtime := &selectableRuntime{}
	controller := NewRuntimeController(&store, selectableAdapter{runtime: runtime})
	if _, err := controller.Start(value.ID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = controller.Stop() })

	snapshot, err := controller.Select("", value.ID+"::slow")
	if err != nil {
		t.Fatal(err)
	}
	if runtime.selectedTag() != "slow" {
		t.Fatalf("selected=%q, want source tag slow", runtime.selectedTag())
	}
	if snapshot.ActiveNode != "slow" {
		t.Fatalf("active node = %q, want slow", snapshot.ActiveNode)
	}
	saved, err := store.Load(value.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.SelectedMode != "slow" {
		t.Fatalf("persisted SelectedMode = %q, want slow", saved.SelectedMode)
	}
}

func TestRuntimeControllerSelectAcceptsNamespacedTagWithProfileID(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	value := profile.Profile{
		ID:     "profile-select",
		Engine: profile.EngineSingBox,
		Config: []byte(`{"route":{"final":"Mode"},"outbounds":[{"type":"selector","tag":"Mode","outbounds":["fast","slow"],"default":"fast"},{"type":"vless","tag":"fast"},{"type":"vless","tag":"slow"}]}`),
	}
	if err := store.Save(value); err != nil {
		t.Fatal(err)
	}
	runtime := &selectableRuntime{}
	controller := NewRuntimeController(&store, selectableAdapter{runtime: runtime})
	if _, err := controller.Start(value.ID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = controller.Stop() })

	if _, err := controller.Select(value.ID, value.ID+"::slow"); err != nil {
		t.Fatalf("Select with namespaced tag: %v", err)
	}
	if runtime.selectedTag() != "slow" {
		t.Fatalf("selected=%q, want slow", runtime.selectedTag())
	}
}

func TestRuntimeControllerSelectKeepsMergedRuntimeTag(t *testing.T) {
	store, value := saveAutoProbeProfile(t)
	vlessTag := value.ID + "::vless"
	tuicTag := value.ID + "::tuic"
	runtime := &probingRuntime{outcomes: map[string]probeOutcome{
		vlessTag: {latency: 42},
		tuicTag:  {latency: 80},
	}}
	controller := NewRuntimeController(&store, probingAdapter{runtime: runtime})
	if _, err := controller.Start(value.ID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = controller.Stop() })
	snapshot, err := controller.Select("", tuicTag)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.selectedTag() != tuicTag {
		t.Fatalf("selected=%q, want merged tag %q", runtime.selectedTag(), tuicTag)
	}
	if snapshot.ActiveNode != "tuic" {
		t.Fatalf("active node = %q, want tuic", snapshot.ActiveNode)
	}
}

func TestCandidateProbeGroupsAutoKeepsThreeNonTUIC(t *testing.T) {
	candidates := []policy.Candidate{
		{ID: "slow-hy2", Protocol: "Hysteria2", LatencyMS: 80},
		{ID: "fast-vless", Protocol: "VLESS", LatencyMS: 20},
		{ID: "mid-trojan", Protocol: "Trojan", LatencyMS: 40},
		{ID: "mid-vless", Protocol: "VLESS", LatencyMS: 30},
		{ID: "tuic", Protocol: "TUIC", LatencyMS: 5},
		{ID: "slowest-hy2", Protocol: "Hysteria2", LatencyMS: 90},
	}
	groups := candidateProbeGroups(candidates, policy.ConnectionPolicy{
		Mode:                policy.ConnectionAuto,
		NetworkClass:        policy.NetworkWiFi,
		TUICFallbackEnabled: true,
	})
	if len(groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(groups))
	}
	if len(groups[0]) != policy.AutoCandidateLimit {
		t.Fatalf("auto group size = %d, want %d", len(groups[0]), policy.AutoCandidateLimit)
	}
	got := []string{groups[0][0].ID, groups[0][1].ID, groups[0][2].ID}
	want := []string{"fast-vless", "mid-vless", "mid-trojan"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("auto group = %v, want %v", got, want)
	}
	if len(groups[1]) != 1 || groups[1][0].ID != "tuic" {
		t.Fatalf("TUIC fallback group = %#v", groups[1])
	}
}

func TestCandidateProbeGroupsManualFallbackDoesNotTrim(t *testing.T) {
	candidates := []policy.Candidate{
		{ID: "a", Protocol: "VLESS", LatencyMS: 10},
		{ID: "b", Protocol: "VLESS", LatencyMS: 20},
		{ID: "c", Protocol: "VLESS", LatencyMS: 30},
		{ID: "d", Protocol: "VLESS", LatencyMS: 40},
	}
	groups := candidateProbeGroups(candidates, policy.ConnectionPolicy{
		Mode:         policy.ConnectionManualFallback,
		NetworkClass: policy.NetworkWiFi,
	})
	if len(groups) != 1 || len(groups[0]) != 4 {
		t.Fatalf("manual groups = %#v", groups)
	}
}

func TestRuntimeControllerKeepsTUICAsUnprobedFallbackWhenPrimaryIsHealthy(t *testing.T) {
	store, value := saveAutoProbeProfile(t)
	vlessTag := value.ID + "::vless"
	tuicTag := value.ID + "::tuic"
	runtime := &probingRuntime{outcomes: map[string]probeOutcome{
		vlessTag: {latency: 42},
		tuicTag:  {latency: 5},
	}}
	controller := NewRuntimeController(&store, probingAdapter{runtime: runtime})
	if _, err := controller.Start(value.ID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = controller.Stop() })
	if runtime.selectedTag() != vlessTag {
		t.Fatalf("selected = %q, want %q", runtime.selectedTag(), vlessTag)
	}
	if runtime.wasProbed(tuicTag) {
		t.Fatal("TUIC was probed although a non-TUIC candidate was healthy")
	}
}

func TestRuntimeControllerUsesTUICOnlyAfterPrimaryProbeFails(t *testing.T) {
	store, value := saveAutoProbeProfile(t)
	vlessTag := value.ID + "::vless"
	tuicTag := value.ID + "::tuic"
	runtime := &probingRuntime{outcomes: map[string]probeOutcome{
		vlessTag: {err: errors.New("primary unavailable")},
		tuicTag:  {latency: 73},
	}}
	controller := NewRuntimeController(&store, probingAdapter{runtime: runtime})
	if _, err := controller.Start(value.ID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = controller.Stop() })
	if runtime.selectedTag() != tuicTag {
		t.Fatalf("selected = %q, want TUIC fallback %q", runtime.selectedTag(), tuicTag)
	}
	if !runtime.wasProbed(vlessTag) || !runtime.wasProbed(tuicTag) {
		t.Fatalf("probe attempts = %#v", runtime.attempts)
	}
}

func TestRuntimeControllerRejectsConnectedStateWhenEveryProbeFails(t *testing.T) {
	store, value := saveAutoProbeProfile(t)
	runtime := &probingRuntime{outcomes: map[string]probeOutcome{
		value.ID + "::vless": {err: errors.New("primary unavailable")},
		value.ID + "::tuic":  {err: errors.New("fallback unavailable")},
	}}
	controller := NewRuntimeController(&store, probingAdapter{runtime: runtime})
	if _, err := controller.Start(value.ID); err == nil || !strings.Contains(err.Error(), "ни один VPN-узел") {
		t.Fatalf("Start() error = %v", err)
	}
	if runtime.Status() != engine.Stopped {
		t.Fatalf("runtime status = %q", runtime.Status())
	}
}

func saveAutoProbeProfile(t *testing.T) (storage.Store, profile.Profile) {
	t.Helper()
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	value := profile.Profile{
		ID:           "profile-auto-probe",
		Engine:       profile.EngineSingBox,
		SelectedMode: "Auto",
		Config: []byte(`{
			"route":{"final":"Mode"},
			"outbounds":[
				{"type":"selector","tag":"Mode","outbounds":["Auto"]},
				{"type":"urltest","tag":"Auto","outbounds":["vless","tuic"]},
				{"type":"vless","tag":"vless"},
				{"type":"tuic","tag":"tuic"}
			]
		}`),
	}
	if err := store.Save(value); err != nil {
		t.Fatal(err)
	}
	return store, value
}

func TestRuntimeControllerRecoversAfterUnexpectedFailure(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	value := profile.Profile{
		ID:     "profile-recovery",
		Engine: profile.EngineSingBox,
		Config: []byte(`{"outbounds":[{"type":"direct","tag":"direct"}]}`),
	}
	if err := store.Save(value); err != nil {
		t.Fatal(err)
	}
	adapter := &recoveringAdapter{}
	controller := NewRuntimeController(&store, adapter)
	if _, err := controller.Start(value.ID); err != nil {
		t.Fatal(err)
	}
	adapter.mu.Lock()
	first := adapter.first
	adapter.mu.Unlock()
	first.fail()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		adapter.mu.Lock()
		starts := adapter.starts
		adapter.mu.Unlock()
		if starts >= 2 && controller.Status().Status == string(engine.Connected) {
			if _, err := controller.Stop(); err != nil {
				t.Fatal(err)
			}
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("runtime was not recovered: starts=%d snapshot=%#v", adapter.starts, controller.Status())
}

func TestRuntimeControllerRollsBackWhenReplacementFails(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	old := profile.Profile{
		ID:     "profile-replace-failure",
		Engine: profile.EngineSingBox,
		Config: []byte(`{"outbounds":[{"type":"direct","tag":"old"}]}`),
	}
	if err := store.Save(old); err != nil {
		t.Fatal(err)
	}
	runtime := &selectableRuntime{}
	adapter := &replacementAdapter{runtime: runtime}
	controller := NewRuntimeController(&store, adapter)
	if _, err := controller.Start(old.ID); err != nil {
		t.Fatal(err)
	}
	updated := old
	updated.Config = []byte(`{"outbounds":[{"type":"direct","tag":"new"}]}`)
	if _, err := controller.ReplaceProfile(updated); err == nil {
		t.Fatal("expected replacement start failure")
	}
	saved, err := store.Load(old.ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(saved.Config) != string(old.Config) {
		t.Fatalf("profile was not rolled back: %s", saved.Config)
	}
	if controller.Status().Status != string(engine.Connected) {
		t.Fatalf("runtime was not restored: %#v", controller.Status())
	}
	if _, err := controller.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeControllerRecordsProbeCacheAndClearsOnStop(t *testing.T) {
	store, value := saveAutoProbeProfile(t)
	vlessTag := value.ID + "::vless"
	tuicTag := value.ID + "::tuic"
	runtime := &probingRuntime{outcomes: map[string]probeOutcome{
		vlessTag: {latency: 42},
		tuicTag:  {latency: 5},
	}}
	controller := NewRuntimeController(&store, probingAdapter{runtime: runtime})
	if _, err := controller.Start(value.ID); err != nil {
		t.Fatal(err)
	}
	cache := controller.ProbeSnapshot()
	if cache[vlessTag].Status != ProbeHealthy || cache[vlessTag].LatencyMS != 42 {
		t.Fatalf("probe cache = %#v", cache)
	}
	if _, err := controller.Stop(); err != nil {
		t.Fatal(err)
	}
	if got := controller.ProbeSnapshot(); len(got) != 0 {
		t.Fatalf("probe cache after stop = %#v", got)
	}
}

func TestRuntimeSnapshotPrefersProbeCacheOverClashHistory(t *testing.T) {
	store, value := saveAutoProbeProfile(t)
	vlessTag := value.ID + "::vless"
	tuicTag := value.ID + "::tuic"
	runtime := &reportingRuntime{
		probingRuntime: &probingRuntime{outcomes: map[string]probeOutcome{
			vlessTag: {latency: 42},
			tuicTag:  {latency: 5},
		}},
		details: engine.RuntimeDetails{
			Node:     "Auto",
			Latency:  180,
			Protocol: "URLTest",
			Status:   "active",
		},
	}
	controller := NewRuntimeController(&store, reportingAdapter{runtime: runtime})
	if _, err := controller.Start(value.ID); err != nil {
		t.Fatal(err)
	}
	snap := controller.Status()
	if snap.ActiveLatency == 180 {
		t.Fatalf("snapshot used Clash group history: %#v", snap)
	}
	if snap.ActiveLatency != 42 && snap.ActiveLatency != 5 {
		t.Fatalf("snapshot latency = %d, want probe cache", snap.ActiveLatency)
	}
	if _, err := controller.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeSnapshotShowsClashLeafWhenItDiffersFromProbePick(t *testing.T) {
	store, value := saveAutoProbeProfile(t)
	vlessTag := value.ID + "::vless"
	tuicTag := value.ID + "::tuic"
	runtime := &reportingRuntime{
		probingRuntime: &probingRuntime{outcomes: map[string]probeOutcome{
			vlessTag: {latency: 465},
			tuicTag:  {latency: 900},
		}},
		details: engine.RuntimeDetails{
			Node:     "🇨🇿 Czechia (CZ) Hysteria2",
			Country:  "Czechia",
			Latency:  354,
			Protocol: "Hysteria2",
			Status:   "active",
		},
	}
	controller := NewRuntimeController(&store, reportingAdapter{runtime: runtime})
	started, err := controller.Start(value.ID)
	if err != nil {
		t.Fatal(err)
	}
	if started.ActiveNode != "🇨🇿 Czechia (CZ) Hysteria2" {
		t.Fatalf("start active node = %q, want clash leaf", started.ActiveNode)
	}
	if started.ActiveCountry != "Czechia" || started.ActiveProtocol != "Hysteria2" {
		t.Fatalf("start snapshot = %#v", started)
	}
	if _, err := controller.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestProbeCacheLookupFallsBackToSourceTag(t *testing.T) {
	cache := map[string]ProbeCacheEntry{
		"AmneziaWG": {LatencyMS: 36, Status: ProbeHealthy},
	}
	entry, ok := probeCacheForCandidate(cache, policy.Candidate{
		RuntimeTag: "profile-awg::AmneziaWG",
		ProfileID:  "profile-awg",
		SourceTag:  "AmneziaWG",
	})
	if !ok || entry.LatencyMS != 36 {
		t.Fatalf("alias lookup = %#v ok=%v", entry, ok)
	}
}

func TestRuntimeControllerIdleProbeDoesNotConnectVPN(t *testing.T) {
	store, value := saveAutoProbeProfile(t)
	vlessTag := value.ID + "::vless"
	tuicTag := value.ID + "::tuic"
	runtime := &probingRuntime{outcomes: map[string]probeOutcome{
		vlessTag: {latency: 42},
		tuicTag:  {latency: 5},
	}}
	controller := NewRuntimeController(&store, probingAdapter{runtime: runtime})
	if err := controller.ProbeNodes(context.Background()); err != nil {
		t.Fatal(err)
	}
	if controller.runtime != nil {
		t.Fatal("idle probe left VPN runtime running")
	}
	if runtime.stops() != 1 {
		t.Fatalf("idle runtime stops = %d", runtime.stops())
	}
	if !runtime.wasProbed(vlessTag) || !runtime.wasProbed(tuicTag) {
		t.Fatalf("idle probe attempts = %#v", runtime.attempts)
	}
	cache := controller.ProbeSnapshot()
	if cache[vlessTag].LatencyMS != 42 || cache[tuicTag].LatencyMS != 5 {
		t.Fatalf("idle probe cache = %#v", cache)
	}
}

func TestRuntimeControllerIdleProbeMeasuresAmneziaWG(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	jsonProfile := profile.Profile{
		ID:     "profile-json",
		Engine: profile.EngineSingBox,
		Config: []byte(`{"route":{"final":"Mode"},"outbounds":[{"type":"selector","tag":"Mode","outbounds":["fast"],"default":"fast"},{"type":"vless","tag":"fast"},{"type":"direct","tag":"direct"}]}`),
	}
	awgProfile := profile.Profile{
		ID:     "profile-awg-idle",
		Engine: profile.EngineAmneziaWG,
		Config: awgRuntimeTestdata(t, "awg31.valid.conf"),
	}
	if err := store.Save(jsonProfile); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(awgProfile); err != nil {
		t.Fatal(err)
	}
	vlessTag := jsonProfile.ID + "::fast"
	awgTag := awgProfile.ID + "::" + awgcfg.LeafTag
	runtime := &probingRuntime{outcomes: map[string]probeOutcome{
		vlessTag: {latency: 18},
		awgTag:   {latency: 41},
	}}
	sidecar := &countingSidecar{LoopbackSidecar: awgengine.NewLoopbackSidecar()}
	controller := NewRuntimeController(&store, probingAdapter{runtime: runtime})
	controller.AWGFactory = func() awgengine.Sidecar { return sidecar }
	if err := controller.ProbeNodes(context.Background()); err != nil {
		t.Fatal(err)
	}
	if controller.runtime != nil {
		t.Fatal("idle probe left VPN runtime running")
	}
	if sidecar.stops != 1 {
		t.Fatalf("idle AWG sidecar stops = %d", sidecar.stops)
	}
	if !runtime.wasProbed(awgTag) {
		t.Fatalf("AmneziaWG was not probed: %#v", runtime.attempts)
	}
	cache := controller.ProbeSnapshot()
	if cache[awgTag].LatencyMS != 41 || cache[vlessTag].LatencyMS != 18 {
		t.Fatalf("idle mixed cache = %#v", cache)
	}
	if runtime.timeouts[vlessTag] != nodeListProbeTimeout {
		t.Fatalf("VLESS probe timeout = %s", runtime.timeouts[vlessTag])
	}
	if runtime.timeouts[awgTag] != awgFirstProbeTimeout {
		t.Fatalf("AWG probe timeout = %s", runtime.timeouts[awgTag])
	}
}

func TestRuntimeControllerIdleProbeAmneziaWGOnly(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	value := profile.Profile{
		ID:     "profile-awg-only",
		Engine: profile.EngineAmneziaWG,
		Config: awgRuntimeTestdata(t, "awg31.valid.conf"),
	}
	if err := store.Save(value); err != nil {
		t.Fatal(err)
	}
	runtime := &probingRuntime{outcomes: map[string]probeOutcome{
		awgcfg.LeafTag: {latency: 36},
	}}
	sidecar := &countingSidecar{LoopbackSidecar: awgengine.NewLoopbackSidecar()}
	controller := NewRuntimeController(&store, probingAdapter{runtime: runtime})
	controller.AWGFactory = func() awgengine.Sidecar { return sidecar }
	if err := controller.ProbeNodes(context.Background()); err != nil {
		t.Fatal(err)
	}
	if controller.runtime != nil {
		t.Fatal("idle AWG probe left VPN runtime running")
	}
	if sidecar.stops != 1 {
		t.Fatalf("idle AWG sidecar stops = %d", sidecar.stops)
	}
	if !runtime.wasProbed(awgcfg.LeafTag) {
		t.Fatalf("bare AmneziaWG tag was not probed: %#v", runtime.attempts)
	}
	cache := controller.ProbeSnapshot()
	if cache[awgcfg.LeafTag].LatencyMS != 36 {
		t.Fatalf("idle AWG-only cache = %#v", cache)
	}
}

func TestRuntimeControllerIdleProbeRecordsFailedAmneziaWG(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	jsonProfile := profile.Profile{
		ID:     "profile-json",
		Engine: profile.EngineSingBox,
		Config: []byte(`{"route":{"final":"Mode"},"outbounds":[{"type":"selector","tag":"Mode","outbounds":["fast"],"default":"fast"},{"type":"vless","tag":"fast"},{"type":"direct","tag":"direct"}]}`),
	}
	awgProfile := profile.Profile{
		ID:     "profile-awg-fail",
		Engine: profile.EngineAmneziaWG,
		Config: awgRuntimeTestdata(t, "awg31.valid.conf"),
	}
	if err := store.Save(jsonProfile); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(awgProfile); err != nil {
		t.Fatal(err)
	}
	vlessTag := jsonProfile.ID + "::fast"
	awgTag := awgProfile.ID + "::" + awgcfg.LeafTag
	runtime := &probingRuntime{outcomes: map[string]probeOutcome{
		vlessTag: {latency: 18},
	}}
	controller := NewRuntimeController(&store, probingAdapter{runtime: runtime})
	controller.AWGFactory = func() awgengine.Sidecar { return awgengine.FailingSidecar{} }
	if err := controller.ProbeNodes(context.Background()); err != nil {
		t.Fatal(err)
	}
	if runtime.wasProbed(awgTag) {
		t.Fatal("failed AWG sidecar should not be Clash-probed")
	}
	cache := controller.ProbeSnapshot()
	if cache[awgTag].Status != ProbeFailed {
		t.Fatalf("failed AWG status = %#v", cache[awgTag])
	}
	if cache[vlessTag].LatencyMS != 18 {
		t.Fatalf("sing-box probe cache = %#v", cache)
	}
}

func TestRuntimeControllerProbeUsesSourceTagsWhenRuntimeIsNotMerged(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	value := profile.Profile{
		ID:     "profile-select",
		Engine: profile.EngineSingBox,
		Config: []byte(`{"route":{"final":"Mode"},"outbounds":[{"type":"selector","tag":"Mode","outbounds":["fast","slow"],"default":"fast"},{"type":"vless","tag":"fast"},{"type":"vless","tag":"slow"}]}`),
	}
	if err := store.Save(value); err != nil {
		t.Fatal(err)
	}
	runtime := &probingRuntime{outcomes: map[string]probeOutcome{
		"fast": {latency: 12},
		"slow": {latency: 88},
	}}
	controller := NewRuntimeController(&store, probingAdapter{runtime: runtime})
	if _, err := controller.Start(value.ID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = controller.Stop() })
	if err := controller.ProbeNodes(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !runtime.wasProbed("fast") || !runtime.wasProbed("slow") {
		t.Fatalf("probe attempts = %#v, want source tags", runtime.attempts)
	}
	if runtime.wasProbed(value.ID + "::fast") {
		t.Fatal("probed namespaced tag on a non-merged runtime")
	}
	cache := controller.ProbeSnapshot()
	fast := cache[value.ID+"::fast"]
	slow := cache[value.ID+"::slow"]
	if fast.Status != ProbeHealthy || fast.LatencyMS != 12 {
		t.Fatalf("fast cache = %#v", fast)
	}
	if slow.Status != ProbeHealthy || slow.LatencyMS != 88 {
		t.Fatalf("slow cache = %#v", slow)
	}
}

func TestRuntimeControllerSelectByPolicyDoesNotStopRuntime(t *testing.T) {
	store, value := saveAutoProbeProfile(t)
	vlessTag := value.ID + "::vless"
	tuicTag := value.ID + "::tuic"
	runtime := &probingRuntime{outcomes: map[string]probeOutcome{
		vlessTag: {latency: 42},
		tuicTag:  {latency: 5},
	}}
	controller := NewRuntimeController(&store, probingAdapter{runtime: runtime})
	if _, err := controller.Start(value.ID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = controller.Stop() })
	if _, err := controller.SelectByPolicy(); err != nil {
		t.Fatal(err)
	}
	if runtime.selectedTag() != vlessTag {
		t.Fatalf("selected = %q", runtime.selectedTag())
	}
	if runtime.stops() != 0 {
		t.Fatalf("stopCount = %d, want 0", runtime.stops())
	}
	selects := runtime.selects()
	if _, err := controller.SelectByPolicy(); err != nil {
		t.Fatal(err)
	}
	if runtime.selects() != selects {
		t.Fatalf("SelectByPolicy re-selected the already active node: %d -> %d", selects, runtime.selects())
	}
}

func TestRuntimeControllerNetworkClassChangeSelectsWithoutReconnect(t *testing.T) {
	store, value := saveAutoProbeProfile(t)
	vlessTag := value.ID + "::vless"
	tuicTag := value.ID + "::tuic"
	runtime := &probingRuntime{outcomes: map[string]probeOutcome{
		vlessTag: {latency: 42},
		tuicTag:  {latency: 5},
	}}
	controller := NewRuntimeController(&store, probingAdapter{runtime: runtime})
	controller.NetworkClass = func() policy.NetworkClass { return policy.NetworkWiFi }
	if _, err := controller.Start(value.ID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = controller.Stop() })
	stops := runtime.stops()
	probes := runtime.probeCount()
	selects := runtime.selects()
	if _, err := controller.OnNetworkClass(policy.NetworkEthernet); err != nil {
		t.Fatal(err)
	}
	if runtime.stops() != stops {
		t.Fatalf("network class change stopped the runtime: %d -> %d", stops, runtime.stops())
	}
	if runtime.probeCount() <= probes {
		t.Fatalf("network class change did not re-probe: before=%d after=%d", probes, runtime.probeCount())
	}
	if runtime.selects() != selects {
		t.Fatalf("network class change re-selected the active node: %d -> %d", selects, runtime.selects())
	}
	saved, err := store.LoadConnectionPolicy()
	if err != nil {
		t.Fatal(err)
	}
	if saved.NetworkClass != policy.NetworkEthernet {
		t.Fatalf("persisted network class = %q, want ethernet", saved.NetworkClass)
	}
}

func TestRuntimeControllerHealthFailoverUsesCooldown(t *testing.T) {
	store, value := saveAutoProbeProfile(t)
	connectionPolicy := policy.DefaultConnectionPolicy()
	connectionPolicy.Mode = policy.ConnectionManualFallback
	connectionPolicy.PreferredProtocol = "VLESS"
	if err := store.SaveConnectionPolicy(connectionPolicy); err != nil {
		t.Fatal(err)
	}
	vlessTag := value.ID + "::vless"
	tuicTag := value.ID + "::tuic"
	runtime := &probingRuntime{outcomes: map[string]probeOutcome{
		vlessTag: {latency: 42},
		tuicTag:  {latency: 5},
	}}
	controller := NewRuntimeController(&store, probingAdapter{runtime: runtime})
	controller.HealthInterval = 20 * time.Millisecond
	controller.HealthFailThreshold = 2
	if _, err := controller.Start(value.ID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = controller.Stop() })
	runtime.setOutcome(vlessTag, probeOutcome{err: errors.New("timeout")})
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if runtime.selectedTag() == tuicTag && controller.Status().FailoverMessage != "" {
			if runtime.stops() != 0 {
				t.Fatalf("health failover stopped TUN: %d", runtime.stops())
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("health failover did not switch: selected=%q snapshot=%#v", runtime.selectedTag(), controller.Status())
}

func TestRuntimeControllerAutoHealthReselectsFasterNode(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	value := profile.Profile{
		ID:           "profile-auto-health",
		Engine:       profile.EngineSingBox,
		SelectedMode: "Auto",
		Config: []byte(`{
			"route":{"final":"Mode"},
			"outbounds":[
				{"type":"selector","tag":"Mode","outbounds":["Auto"]},
				{"type":"urltest","tag":"Auto","outbounds":["fast","slow"]},
				{"type":"vless","tag":"fast"},
				{"type":"vless","tag":"slow"}
			]
		}`),
	}
	if err := store.Save(value); err != nil {
		t.Fatal(err)
	}
	fastTag := value.ID + "::fast"
	slowTag := value.ID + "::slow"
	runtime := &probingRuntime{outcomes: map[string]probeOutcome{
		fastTag: {latency: 10},
		slowTag: {latency: 80},
	}}
	controller := NewRuntimeController(&store, probingAdapter{runtime: runtime})
	controller.HealthInterval = 20 * time.Millisecond
	if _, err := controller.Start(value.ID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = controller.Stop() })
	if runtime.selectedTag() != fastTag {
		t.Fatalf("start selected = %q, want %q", runtime.selectedTag(), fastTag)
	}
	runtime.setOutcome(fastTag, probeOutcome{latency: 90})
	runtime.setOutcome(slowTag, probeOutcome{latency: 15})
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if runtime.selectedTag() == slowTag {
			if runtime.stops() != 0 {
				t.Fatalf("auto health reselect stopped TUN: %d", runtime.stops())
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("auto health did not switch to faster node: selected=%q", runtime.selectedTag())
}

func TestRuntimeControllerAutoHealthFailoversWithoutFailThreshold(t *testing.T) {
	store, value := saveAutoProbeProfile(t)
	vlessTag := value.ID + "::vless"
	tuicTag := value.ID + "::tuic"
	runtime := &probingRuntime{outcomes: map[string]probeOutcome{
		vlessTag: {latency: 42},
		tuicTag:  {latency: 5},
	}}
	controller := NewRuntimeController(&store, probingAdapter{runtime: runtime})
	controller.HealthInterval = 20 * time.Millisecond
	controller.HealthFailThreshold = 8
	if _, err := controller.Start(value.ID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = controller.Stop() })
	runtime.setOutcome(vlessTag, probeOutcome{err: errors.New("timeout")})
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if runtime.selectedTag() == tuicTag && controller.Status().FailoverMessage != "" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("auto health did not fail over: selected=%q snapshot=%#v", runtime.selectedTag(), controller.Status())
}

type countingSidecar struct {
	*awgengine.LoopbackSidecar
	stops int
}

func (s *countingSidecar) Stop() error {
	s.stops++
	return s.LoopbackSidecar.Stop()
}

type failStartAdapter struct{}

func (failStartAdapter) Type() engine.Type                      { return engine.SingBox }
func (failStartAdapter) Validate(context.Context, []byte) error { return nil }
func (failStartAdapter) Start(context.Context, []byte) (engine.Runtime, error) {
	return nil, errors.New("adapter start failed")
}

func awgRuntimeTestdata(t *testing.T, name string) []byte {
	t.Helper()
	payload, err := os.ReadFile(filepath.Join("..", "profile", "amneziawg", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func TestRuntimeControllerStartsAndStopsAmneziaWG(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	value := profile.Profile{
		ID:     "profile-awg",
		Name:   "AWG",
		Engine: profile.EngineAmneziaWG,
		Config: awgRuntimeTestdata(t, "awg31.valid.conf"),
	}
	if err := store.Save(value); err != nil {
		t.Fatal(err)
	}
	sidecar := &countingSidecar{LoopbackSidecar: awgengine.NewLoopbackSidecar()}
	controller := NewRuntimeController(&store, selectableAdapter{runtime: &selectableRuntime{}})
	controller.AWGFactory = func() awgengine.Sidecar { return sidecar }
	dns := &fakeDNSManager{}
	controller.DNS = dns

	started, err := controller.Start(value.ID)
	if err != nil {
		t.Fatal(err)
	}
	if started.Status != "connected" || started.Engine != string(profile.EngineAmneziaWG) {
		t.Fatalf("start snapshot = %#v", started)
	}
	if started.ActiveNode != awgcfg.LeafTag {
		t.Fatalf("active node = %q", started.ActiveNode)
	}
	if _, err := controller.Stop(); err != nil {
		t.Fatal(err)
	}
	if sidecar.stops == 0 {
		t.Fatal("expected sidecar stop")
	}
	if !dns.configured || !dns.restored {
		t.Fatalf("DNS lifecycle = configured:%v restored:%v", dns.configured, dns.restored)
	}
}

func TestRuntimeControllerAmneziaWGSidecarFailsBeforeAdapter(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	value := profile.Profile{
		ID:     "profile-awg-fail",
		Engine: profile.EngineAmneziaWG,
		Config: awgRuntimeTestdata(t, "awg31.valid.conf"),
	}
	if err := store.Save(value); err != nil {
		t.Fatal(err)
	}
	started := false
	controller := NewRuntimeController(&store, startFlagAdapter{started: &started})
	controller.AWGFactory = func() awgengine.Sidecar { return awgengine.FailingSidecar{} }
	if _, err := controller.Start(value.ID); err == nil {
		t.Fatal("expected sidecar failure")
	}
	if started {
		t.Fatal("adapter started after sidecar failure")
	}
	if status := controller.Status(); status.Status != "stopped" {
		t.Fatalf("status = %#v", status)
	}
}

func TestRuntimeControllerAmneziaWGRollsBackSidecarWhenAdapterFails(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	value := profile.Profile{
		ID:     "profile-awg-adapter",
		Engine: profile.EngineAmneziaWG,
		Config: awgRuntimeTestdata(t, "awg31.valid.conf"),
	}
	if err := store.Save(value); err != nil {
		t.Fatal(err)
	}
	sidecar := &countingSidecar{LoopbackSidecar: awgengine.NewLoopbackSidecar()}
	controller := NewRuntimeController(&store, failStartAdapter{})
	controller.AWGFactory = func() awgengine.Sidecar { return sidecar }
	if _, err := controller.Start(value.ID); err == nil {
		t.Fatal("expected adapter failure")
	}
	if sidecar.stops == 0 {
		t.Fatal("sidecar was not stopped after adapter failure")
	}
	if sidecar.ListenAddr() != "" {
		t.Fatalf("sidecar still listening on %s", sidecar.ListenAddr())
	}
}

func TestRuntimeControllerStartsSingBoxWhenAmneziaProfileAlsoStored(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	jsonProfile := profile.Profile{
		ID:     "profile-json",
		Engine: profile.EngineSingBox,
		Config: []byte(`{"route":{"final":"Mode"},"outbounds":[{"type":"selector","tag":"Mode","outbounds":["fast"],"default":"fast"},{"type":"vless","tag":"fast"}]}`),
	}
	awgProfile := profile.Profile{
		ID:     "profile-awg-extra",
		Engine: profile.EngineAmneziaWG,
		Config: awgRuntimeTestdata(t, "awg31.valid.conf"),
	}
	if err := store.Save(jsonProfile); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(awgProfile); err != nil {
		t.Fatal(err)
	}
	controller := NewRuntimeController(&store, selectableAdapter{runtime: &selectableRuntime{}})
	controller.AWGFactory = func() awgengine.Sidecar { return awgengine.NewLoopbackSidecar() }
	started, err := controller.Start(jsonProfile.ID)
	if err != nil {
		t.Fatal(err)
	}
	if started.Status != "connected" || started.Engine != string(profile.EngineSingBox) {
		t.Fatalf("start snapshot = %#v", started)
	}
	if !controller.IsActive(jsonProfile.ID) || !controller.IsActive(awgProfile.ID) {
		t.Fatal("unified session should include both profile IDs")
	}
	again, err := controller.Start(awgProfile.ID)
	if err != nil {
		t.Fatal(err)
	}
	if again.Status != "connected" {
		t.Fatalf("start(awg) while unified = %#v", again)
	}
	if _, err := controller.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeControllerStartsNagaEnvelope(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	conf := awgRuntimeTestdata(t, "awg31.valid.conf")
	body, err := json.Marshal(map[string]any{
		"v":       1,
		"app":     "nagavpn",
		"singbox": json.RawMessage(`{"route":{"final":"Mode"},"outbounds":[{"type":"selector","tag":"Mode","outbounds":["fast"],"default":"fast"},{"type":"vless","tag":"fast"}]}`),
		"amnezia": map[string]any{"conf": string(conf)},
	})
	if err != nil {
		t.Fatal(err)
	}
	value := profile.Profile{
		ID:     "profile-envelope",
		Engine: profile.EngineSingBox,
		Config: body,
	}
	if err := store.Save(value); err != nil {
		t.Fatal(err)
	}
	adapter := &recordingAdapter{runtime: &selectableRuntime{}}
	controller := NewRuntimeController(&store, adapter)
	controller.AWGFactory = func() awgengine.Sidecar { return awgengine.NewLoopbackSidecar() }
	started, err := controller.Start(value.ID)
	if err != nil {
		t.Fatal(err)
	}
	if started.Status != "connected" || started.Engine != string(profile.EngineSingBox) {
		t.Fatalf("start snapshot = %#v", started)
	}
	if strings.Contains(string(adapter.last), `"app"`) {
		t.Fatal("runtime started with envelope instead of inner sing-box")
	}
	if !strings.Contains(string(adapter.last), `"fast"`) {
		t.Fatalf("runtime config missing node: %s", adapter.last)
	}
	if _, err := controller.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeControllerAmneziaWGHandshakeTimeoutDoesNotProbe(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	value := profile.Profile{
		ID:     "profile-awg-handshake",
		Engine: profile.EngineAmneziaWG,
		Config: awgRuntimeTestdata(t, "awg31.valid.conf"),
	}
	if err := store.Save(value); err != nil {
		t.Fatal(err)
	}
	started := false
	controller := NewRuntimeController(&store, startFlagAdapter{started: &started})
	controller.AWGFactory = func() awgengine.Sidecar { return &awgengine.NoHandshakeSidecar{} }
	_, err = controller.Start(value.ID)
	if err == nil {
		t.Fatal("expected handshake failure")
	}
	if started {
		t.Fatal("adapter started after handshake timeout")
	}
	status := controller.Status()
	if !strings.Contains(status.Error, "туннель") && !strings.Contains(err.Error(), "туннель") {
		t.Fatalf("error = %v snapshot=%#v", err, status)
	}
}

func TestRuntimeControllerUnifiedInjectsAmneziaWGWithoutSecondTUN(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	jsonProfile := profile.Profile{
		ID:     "profile-json",
		Engine: profile.EngineSingBox,
		Config: []byte(`{"route":{"final":"Mode"},"outbounds":[{"type":"selector","tag":"Mode","outbounds":["fast"],"default":"fast"},{"type":"vless","tag":"fast"},{"type":"direct","tag":"direct"}]}`),
	}
	awgProfile := profile.Profile{
		ID:     "profile-awg-extra",
		Engine: profile.EngineAmneziaWG,
		Config: awgRuntimeTestdata(t, "awg31.valid.conf"),
	}
	if err := store.Save(jsonProfile); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(awgProfile); err != nil {
		t.Fatal(err)
	}
	runtime := &selectableRuntime{}
	adapter := &recordingAdapter{runtime: runtime}
	controller := NewRuntimeController(&store, adapter)
	controller.AWGFactory = func() awgengine.Sidecar { return awgengine.NewLoopbackSidecar() }
	started, err := controller.Start(jsonProfile.ID)
	if err != nil {
		t.Fatal(err)
	}
	if started.Status != "connected" {
		t.Fatalf("start = %#v", started)
	}
	if strings.Count(string(adapter.last), `"type": "tun"`) > 1 {
		t.Fatalf("second TUN in unified config: %s", adapter.last)
	}
	if !strings.Contains(string(adapter.last), `"type": "socks"`) {
		t.Fatalf("missing socks inject: %s", adapter.last)
	}
	if !strings.Contains(string(adapter.last), awgProfile.ID+"::"+awgcfg.LeafTag) {
		t.Fatalf("missing namespaced AWG tag: %s", adapter.last)
	}
	if _, err := controller.Select(awgProfile.ID, awgcfg.LeafTag); err != nil {
		t.Fatal(err)
	}
	if runtime.stops() != 0 {
		t.Fatalf("select stopped TUN: %d", runtime.stops())
	}
	if runtime.selectedTag() != awgProfile.ID+"::"+awgcfg.LeafTag {
		t.Fatalf("selected = %q", runtime.selectedTag())
	}
	if _, err := controller.Select("", awgcfg.LeafTag); err != nil {
		t.Fatalf("Select bare AmneziaWG: %v", err)
	}
	if runtime.selectedTag() != awgProfile.ID+"::"+awgcfg.LeafTag {
		t.Fatalf("bare select = %q", runtime.selectedTag())
	}
	saved, err := store.Load(awgProfile.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.SelectedMode != awgcfg.LeafTag {
		t.Fatalf("persist SelectedMode = %q", saved.SelectedMode)
	}
	if _, err := controller.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeControllerSkipsFailedAmneziaWGSidecar(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	jsonProfile := profile.Profile{
		ID:     "profile-json",
		Engine: profile.EngineSingBox,
		Config: []byte(`{"route":{"final":"Mode"},"outbounds":[{"type":"selector","tag":"Mode","outbounds":["fast"],"default":"fast"},{"type":"vless","tag":"fast"},{"type":"direct","tag":"direct"}]}`),
	}
	bad := profile.Profile{
		ID:     "profile-awg-bad",
		Engine: profile.EngineAmneziaWG,
		Config: awgRuntimeTestdata(t, "awg31.valid.conf"),
	}
	good := profile.Profile{
		ID:     "profile-awg-good",
		Engine: profile.EngineAmneziaWG,
		Config: awgRuntimeTestdata(t, "awg31.valid.conf"),
	}
	for _, value := range []profile.Profile{jsonProfile, bad, good} {
		if err := store.Save(value); err != nil {
			t.Fatal(err)
		}
	}
	calls := 0
	controller := NewRuntimeController(&store, selectableAdapter{runtime: &selectableRuntime{}})
	controller.AWGFactory = func() awgengine.Sidecar {
		calls++
		if calls == 1 {
			return awgengine.FailingSidecar{}
		}
		return awgengine.NewLoopbackSidecar()
	}
	started, err := controller.Start(jsonProfile.ID)
	if err != nil {
		t.Fatal(err)
	}
	if started.Status != "connected" {
		t.Fatalf("start = %#v", started)
	}
	if !controller.IsActive(good.ID) {
		t.Fatal("expected successful AWG in session")
	}
	if _, err := controller.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeControllerSelectReportsSkippedAmneziaWG(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	jsonProfile := profile.Profile{
		ID:     "profile-json",
		Engine: profile.EngineSingBox,
		Config: []byte(`{"route":{"final":"Mode"},"outbounds":[{"type":"selector","tag":"Mode","outbounds":["fast"],"default":"fast"},{"type":"vless","tag":"fast"},{"type":"direct","tag":"direct"}]}`),
	}
	awgProfile := profile.Profile{
		ID:     "profile-awg-skip",
		Engine: profile.EngineAmneziaWG,
		Config: awgRuntimeTestdata(t, "awg31.valid.conf"),
	}
	if err := store.Save(jsonProfile); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(awgProfile); err != nil {
		t.Fatal(err)
	}
	runtime := &selectableRuntime{}
	controller := NewRuntimeController(&store, selectableAdapter{runtime: runtime})
	controller.AWGFactory = func() awgengine.Sidecar { return awgengine.FailingSidecar{} }
	started, err := controller.Start(jsonProfile.ID)
	if err != nil {
		t.Fatal(err)
	}
	if started.Status != "connected" {
		t.Fatalf("start = %#v", started)
	}
	_, err = controller.Select("", awgProfile.ID+"::"+awgcfg.LeafTag)
	if err == nil {
		t.Fatal("expected skipped AWG select to fail")
	}
	if !strings.Contains(err.Error(), "AmneziaWG недоступен") {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(err.Error(), "not selectable") {
		t.Fatalf("misleading selectable error: %v", err)
	}
	if _, err := controller.Stop(); err != nil {
		t.Fatal(err)
	}
}

type startFlagAdapter struct {
	started *bool
}

func (startFlagAdapter) Type() engine.Type                      { return engine.SingBox }
func (startFlagAdapter) Validate(context.Context, []byte) error { return nil }
func (a startFlagAdapter) Start(context.Context, []byte) (engine.Runtime, error) {
	*a.started = true
	runtime := &selectableRuntime{}
	runtime.setStatus(engine.Connected)
	return runtime, nil
}

type recordingAdapter struct {
	runtime *selectableRuntime
	last    []byte
}

func (recordingAdapter) Type() engine.Type                      { return engine.SingBox }
func (recordingAdapter) Validate(context.Context, []byte) error { return nil }
func (a *recordingAdapter) Start(_ context.Context, config []byte) (engine.Runtime, error) {
	a.last = append([]byte(nil), config...)
	a.runtime.setStatus(engine.Connected)
	return a.runtime, nil
}

func TestRuntimeControllerRejectsExpiredProfile(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	value := profile.Profile{
		ID:     "profile-expired",
		Engine: profile.EngineSingBox,
		Config: []byte(`{"outbounds":[{"type":"direct","tag":"direct"}]}`),
		Subscription: profile.SubscriptionInfo{
			Expire: time.Unix(1, 0).UTC(),
		},
	}
	if err := store.Save(value); err != nil {
		t.Fatal(err)
	}
	controller := NewRuntimeController(&store, testAdapter{runtime: &testRuntime{status: engine.Stopped}})
	if _, err := controller.Start(value.ID); err == nil {
		t.Fatal("expired profile started")
	}
	if status := controller.Status(); status.Status == string(engine.Connected) {
		t.Fatalf("expired profile left runtime connected: %#v", status)
	}
}

func TestRuntimeControllerStopReturnsDNSRestoreError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh as a stand-in sing-box process")
	}
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	value := profile.Profile{
		ID:     "profile-dns-restore",
		Engine: profile.EngineSingBox,
		Config: []byte(`{"outbounds":[{"type":"direct"}]}`),
	}
	if err := store.Save(value); err != nil {
		t.Fatal(err)
	}
	controller := NewRuntimeController(
		&store,
		singbox.Adapter{BinaryPath: "/bin/sh", Args: []string{"-c", "sleep 10"}},
	)
	dns := &fakeDNSManager{restoreErr: errors.New("resolvectl revert failed")}
	controller.DNS = dns
	if _, err := controller.Start(value.ID); err != nil {
		t.Fatal(err)
	}
	snapshot, err := controller.Stop()
	if err == nil {
		t.Fatal("Stop() ignored DNS restore error")
	}
	if snapshot.Status == string(engine.Connected) {
		t.Fatalf("Stop() left runtime connected: %#v", snapshot)
	}
	if !dns.configured || !dns.restored {
		t.Fatalf("DNS lifecycle = configured:%v restored:%v", dns.configured, dns.restored)
	}
}

func TestRuntimeControllerNetworkClassPatchFailureKeepsConnected(t *testing.T) {
	store, value := saveAutoProbeProfile(t)
	if err := store.SaveConnectionPolicy(policy.DefaultConnectionPolicy()); err != nil {
		t.Fatal(err)
	}
	vlessTag := value.ID + "::vless"
	tuicTag := value.ID + "::tuic"
	runtime := &probingRuntime{outcomes: map[string]probeOutcome{
		vlessTag: {latency: 42},
		tuicTag:  {latency: 5},
	}}
	controller := NewRuntimeController(&store, probingAdapter{runtime: runtime})
	controller.NetworkClass = func() policy.NetworkClass { return policy.NetworkWiFi }
	if _, err := controller.Start(value.ID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = controller.Stop() })

	settingsDir := filepath.Join(store.Root, ".settings")
	t.Cleanup(func() { _ = os.Chmod(settingsDir, 0o700) })
	if err := os.Chmod(settingsDir, 0o500); err != nil {
		t.Fatal(err)
	}

	snapshot, err := controller.OnNetworkClass(policy.NetworkEthernet)
	if snapshot.Status != string(engine.Connected) {
		t.Fatalf("patch failure dropped runtime: %#v err=%v", snapshot, err)
	}
	if strings.Contains(strings.ToLower(snapshot.Error), "vpn упал") {
		t.Fatalf("patch failure set lastError: %q", snapshot.Error)
	}
}

func TestTickActiveHealthDoesNotHoldMutexDuringProbe(t *testing.T) {
	store, value := saveAutoProbeProfile(t)
	vlessTag := value.ID + "::vless"
	tuicTag := value.ID + "::tuic"
	runtime := &probingRuntime{
		delay: 250 * time.Millisecond,
		outcomes: map[string]probeOutcome{
			vlessTag: {latency: 42},
			tuicTag:  {latency: 5},
		},
	}
	controller := NewRuntimeController(&store, probingAdapter{runtime: runtime})
	controller.HealthInterval = 20 * time.Millisecond
	if _, err := controller.Start(value.ID); err != nil {
		t.Fatal(err)
	}
	probesAfterStart := runtime.probeCount()
	deadline := time.Now().Add(400 * time.Millisecond)
	for time.Now().Before(deadline) && runtime.probeCount() == probesAfterStart {
		time.Sleep(5 * time.Millisecond)
	}
	if runtime.probeCount() == probesAfterStart {
		t.Fatal("health tick did not start a probe")
	}

	started := time.Now()
	_ = controller.Status()
	if took := time.Since(started); took > 150*time.Millisecond {
		t.Fatalf("Status() blocked during probe: %s", took)
	}
	started = time.Now()
	if _, err := controller.Stop(); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(started); took > 200*time.Millisecond {
		t.Fatalf("Stop() blocked during probe: %s", took)
	}
}

type countingProbeAdapter struct {
	runtime *probingRuntime
	mu      sync.Mutex
	starts  int
}

func (a *countingProbeAdapter) Type() engine.Type                      { return engine.SingBox }
func (a *countingProbeAdapter) Validate(context.Context, []byte) error { return nil }
func (a *countingProbeAdapter) Start(context.Context, []byte) (engine.Runtime, error) {
	a.mu.Lock()
	a.starts++
	a.mu.Unlock()
	a.runtime.setStatus(engine.Connected)
	return a.runtime, nil
}

func (a *countingProbeAdapter) startCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.starts
}

func TestStartDoesNotHoldMutexDuringFirstProbe(t *testing.T) {
	store, value := saveAutoProbeProfile(t)
	vlessTag := value.ID + "::vless"
	tuicTag := value.ID + "::tuic"
	runtime := &probingRuntime{
		delay: 250 * time.Millisecond,
		outcomes: map[string]probeOutcome{
			vlessTag: {latency: 42},
			tuicTag:  {latency: 5},
		},
	}
	adapter := &countingProbeAdapter{runtime: runtime}
	controller := NewRuntimeController(&store, adapter)

	done := make(chan error, 1)
	go func() {
		_, err := controller.Start(value.ID)
		done <- err
	}()

	deadline := time.Now().Add(time.Second)
	for {
		if controller.Status().Status == string(engine.Starting) {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("Start finished before overlay: %v status=%#v", err, controller.Status())
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("Start never entered starting overlay: %#v", controller.Status())
		}
		time.Sleep(5 * time.Millisecond)
	}

	started := time.Now()
	snap := controller.Status()
	if took := time.Since(started); took > 150*time.Millisecond {
		t.Fatalf("Status() blocked during first Start probe: %s", took)
	}
	if snap.Status != string(engine.Starting) {
		t.Fatalf("status during probe = %q", snap.Status)
	}

	again, err := controller.Start(value.ID)
	if err != nil {
		t.Fatal(err)
	}
	if again.Status != string(engine.Starting) {
		t.Fatalf("second Start status = %q", again.Status)
	}
	if adapter.startCount() != 1 {
		t.Fatalf("Adapter.Start calls = %d, want 1", adapter.startCount())
	}

	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got := controller.Status().Status; got != string(engine.Connected) {
		t.Fatalf("status after Start = %q", got)
	}
	if _, err := controller.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestCandidatesForSelectionKeepsTheChosenNode(t *testing.T) {
	candidates := []policy.Candidate{
		{SourceTag: "🇨🇿 Czechia (CZ) Hysteria2", RuntimeTag: "p::cz", Country: "Czechia", Protocol: "Hysteria2"},
		{SourceTag: "🇪🇪 Estonia (EE) Hysteria2", RuntimeTag: "p::ee", Country: "Estonia", Protocol: "Hysteria2"},
	}
	kept := candidatesForSelection(candidates, "🇨🇿 Czechia (CZ) Hysteria2")
	if len(kept) != 1 || kept[0].SourceTag != "🇨🇿 Czechia (CZ) Hysteria2" {
		t.Fatalf("pinned = %+v", kept)
	}
	if got := candidatesForSelection(candidates, "Auto"); len(got) != 2 {
		t.Fatalf("auto list = %d", len(got))
	}
	if got := candidatesForSelection(candidates, "missing"); len(got) != 0 {
		t.Fatalf("unknown concrete selection = %+v", got)
	}
}
