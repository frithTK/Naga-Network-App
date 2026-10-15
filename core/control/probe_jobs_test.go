package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"naga.network/core/engine"
	awgengine "naga.network/core/engine/amneziawg"
	"naga.network/core/policy"
	"naga.network/core/profile"
)

type jobBarrierRuntime struct {
	slow        string
	entered     chan struct{}
	gate        chan struct{}
	stopEntered chan struct{}
	stopGate    chan struct{}
	stops       atomic.Int32
	onceCalls   atomic.Int32
	warmCalls   atomic.Int32
}

func (r *jobBarrierRuntime) Status() engine.Status      { return engine.Connected }
func (r *jobBarrierRuntime) Stats() engine.TrafficStats { return engine.TrafficStats{} }
func (r *jobBarrierRuntime) Stop() error {
	stops := r.stops.Add(1)
	if stops == 1 && r.stopEntered != nil {
		close(r.stopEntered)
		<-r.stopGate
	}
	return nil
}

type deadlineProbeAdapter struct{ entered chan struct{} }

func (deadlineProbeAdapter) Type() engine.Type                      { return engine.SingBox }
func (deadlineProbeAdapter) Validate(context.Context, []byte) error { return nil }
func (a deadlineProbeAdapter) Start(ctx context.Context, _ []byte) (engine.Runtime, error) {
	close(a.entered)
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestProbeJobDeadlineIncludesPreparation(t *testing.T) {
	store, _ := saveAutoProbeProfile(t)
	c := NewRuntimeController(&store, deadlineProbeAdapter{make(chan struct{})})
	c.probeBudget = 30 * time.Millisecond
	job, err := c.StartProbeJob()
	if err != nil {
		t.Fatal(err)
	}
	final := awaitJob(t, c, job.JobID, func(s ProbeJobSnapshot) bool { return s.FinishedAt != nil })
	if final.State != "timed_out" || final.Cancelled != 2 || final.Failed != 0 {
		t.Fatalf("deadline ignores preparation: %+v", final)
	}
}

type blockedReadySidecar struct {
	*awgengine.LoopbackSidecar
	entered chan struct{}
	stops   atomic.Int32
}

func (s *blockedReadySidecar) Ready(ctx context.Context) error {
	close(s.entered)
	<-ctx.Done()
	return ctx.Err()
}
func (s *blockedReadySidecar) Stop() error { s.stops.Add(1); return s.LoopbackSidecar.Stop() }

func TestProbeJobSlowAWGDoesNotDelayStandardResultsAndReadyIsCancelled(t *testing.T) {
	store, _ := saveAutoProbeProfile(t)
	if err := store.Save(profile.Profile{ID: "awg", Engine: profile.EngineAmneziaWG, Config: awgRuntimeTestdata(t, "awg31.valid.conf")}); err != nil {
		t.Fatal(err)
	}
	sidecar := &blockedReadySidecar{LoopbackSidecar: awgengine.NewLoopbackSidecar(), entered: make(chan struct{})}
	c := NewRuntimeController(&store, jobBarrierAdapter{&jobBarrierRuntime{}})
	c.AWGFactory = func() awgengine.Sidecar { return sidecar }
	job, _ := c.StartProbeJob()
	<-sidecar.entered
	partial := awaitJob(t, c, job.JobID, func(s ProbeJobSnapshot) bool { return s.Healthy == 2 })
	if partial.State != "running" {
		t.Fatal(partial.State)
	}
	_, _ = c.CancelProbeJob(job.JobID)
	final := awaitJob(t, c, job.JobID, func(s ProbeJobSnapshot) bool { return s.FinishedAt != nil })
	if final.Cancelled != 1 || sidecar.stops.Load() != 1 {
		t.Fatalf("AWG cancellation lost: %+v stops=%d", final, sidecar.stops.Load())
	}
}

func TestProbeJobStartWaitsForIdleCleanup(t *testing.T) {
	store, value := saveAutoProbeProfile(t)
	runtime := &jobBarrierRuntime{slow: value.ID + "::tuic", entered: make(chan struct{}), gate: make(chan struct{}), stopEntered: make(chan struct{}), stopGate: make(chan struct{})}
	c := NewRuntimeController(&store, jobBarrierAdapter{runtime})
	c.HealthInterval = time.Hour
	job, _ := c.StartProbeJob()
	<-runtime.entered
	started := make(chan error, 1)
	go func() { _, err := c.Start(value.ID); started <- err }()
	<-runtime.stopEntered
	if _, err := c.StartProbeJob(); !errors.Is(err, ErrProbeInProgress) {
		t.Fatal("job accepted during Start cleanup")
	}
	select {
	case err := <-started:
		t.Fatal("Start did not wait for probe cleanup", err)
	default:
	}
	close(runtime.stopGate)
	if err := <-started; err != nil {
		t.Fatal(err)
	}
	snapshot, _ := c.ProbeJobSnapshot(job.JobID, 0)
	if snapshot.State != "cancelled" || !snapshot.Invalidated {
		t.Fatalf("start did not invalidate job: %+v", snapshot)
	}
	_, _ = c.Stop()
}

type timedPoolRuntime struct {
	jobBarrierRuntime
	active atomic.Int32
	peak   atomic.Int32
	delay  time.Duration
}

func (r *timedPoolRuntime) ProbeOnce(ctx context.Context, _ string, _ time.Duration) (int, error) {
	active := r.active.Add(1)
	defer r.active.Add(-1)
	for old := r.peak.Load(); active > old; old = r.peak.Load() {
		if r.peak.CompareAndSwap(old, active) {
			break
		}
	}
	select {
	case <-time.After(r.delay):
		return 10, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

func TestProbePoolControlledScalingAndFairness(t *testing.T) {
	queue := fairProbeQueue([]policy.Candidate{{ID: "a1", ProfileID: "a"}, {ID: "a2", ProfileID: "a"}, {ID: "a3", ProfileID: "a"}, {ID: "b1", ProfileID: "b"}, {ID: "b2", ProfileID: "b"}})
	if queue[1].ID != "b1" || queue[3].ID != "b2" {
		t.Fatalf("unfair queue: %+v", queue)
	}
	for _, size := range []int{100, 1000} {
		for _, workers := range []int{8, 16, 20} {
			t.Run(fmt.Sprintf("nodes=%d/workers=%d", size, workers), func(t *testing.T) {
				runtime := &timedPoolRuntime{delay: time.Millisecond}
				c := &RuntimeController{ManualProbeConcurrency: workers}
				candidates := make([]policy.Candidate, size)
				for i := range candidates {
					candidates[i] = policy.Candidate{ID: fmt.Sprint(i), ProfileID: fmt.Sprint(i % 10), RuntimeTag: fmt.Sprint(i)}
				}
				start := time.Now()
				if err := c.probeWithRuntime(context.Background(), runtime, candidates, nil); err != nil {
					t.Fatal(err)
				}
				if int(runtime.peak.Load()) > c.manualProbeConcurrency() || len(c.ProbeSnapshot()) != size {
					t.Fatal("pool exceeds capacity or loses nodes")
				}
				t.Logf("synthetic nodes=%d workers=%d peak=%d total_ms=%d", size, workers, runtime.peak.Load(), time.Since(start).Milliseconds())
			})
		}
	}
}
func (r *jobBarrierRuntime) Probe(context.Context, string, time.Duration) (int, error) {
	r.warmCalls.Add(1)
	return 99, nil
}
func (r *jobBarrierRuntime) ProbeOnce(ctx context.Context, tag string, _ time.Duration) (int, error) {
	r.onceCalls.Add(1)
	if tag == r.slow {
		close(r.entered)
		select {
		case <-r.gate:
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
	return 25, nil
}

type jobBarrierAdapter struct{ runtime *jobBarrierRuntime }

func (jobBarrierAdapter) Type() engine.Type                      { return engine.SingBox }
func (jobBarrierAdapter) Validate(context.Context, []byte) error { return nil }
func (a jobBarrierAdapter) Start(context.Context, []byte) (engine.Runtime, error) {
	return a.runtime, nil
}

func awaitJob(t *testing.T, c *RuntimeController, id string, predicate func(ProbeJobSnapshot) bool) ProbeJobSnapshot {
	t.Helper()
	return awaitJobWithin(t, c, id, 2*time.Second, predicate)
}

func awaitJobWithin(t *testing.T, c *RuntimeController, id string, timeout time.Duration, predicate func(ProbeJobSnapshot) bool) ProbeJobSnapshot {
	t.Helper()
	deadline := time.After(timeout)
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		snapshot, ok := c.ProbeJobSnapshot(id, 0)
		if ok && predicate(snapshot) {
			return snapshot
		}
		select {
		case <-ticker.C:
		case <-deadline:
			t.Fatalf("job did not reach expected state: state=%s healthy=%d failed=%d skipped=%d nodes=%d cleanup=%t",
				snapshot.State, snapshot.Healthy, snapshot.Failed, snapshot.Skipped, len(snapshot.Nodes), snapshot.CleanupComplete)
		}
	}
}

func TestProbeJobPublishesFastNodeBeforeSlowNode(t *testing.T) {
	store, value := saveAutoProbeProfile(t)
	runtime := &jobBarrierRuntime{slow: value.ID + "::tuic", entered: make(chan struct{}), gate: make(chan struct{})}
	c := NewRuntimeController(&store, jobBarrierAdapter{runtime})
	started, err := c.StartProbeJob()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = c.CancelProbeJob(started.JobID); _ = c.waitProbeCleanup(context.Background()) }()
	<-runtime.entered
	partial := awaitJob(t, c, started.JobID, func(s ProbeJobSnapshot) bool { return s.Healthy == 1 })
	if partial.State != "running" || partial.FinishedAt != nil {
		t.Fatalf("partial state: %+v", partial)
	}
	if c.ProbeSnapshot()[value.ID+"::vless"].LatencyMS != 25 {
		t.Fatal("fast result not cached while slow node is blocked")
	}
	delta, ok := c.ProbeJobSnapshot(started.JobID, partial.Revision)
	if !ok || len(delta.Nodes) != 0 {
		t.Fatalf("unchanged delta: %+v", delta)
	}
	close(runtime.gate)
	final := awaitJob(t, c, started.JobID, func(s ProbeJobSnapshot) bool { return s.FinishedAt != nil })
	if final.State != "completed" || final.Healthy != 2 || runtime.stops.Load() != 1 {
		t.Fatalf("final: %+v stops=%d", final, runtime.stops.Load())
	}
}

func TestProbeJobCancelWaitsForOwnedCleanup(t *testing.T) {
	store, value := saveAutoProbeProfile(t)
	runtime := &jobBarrierRuntime{slow: value.ID + "::tuic", entered: make(chan struct{}), gate: make(chan struct{}), stopEntered: make(chan struct{}), stopGate: make(chan struct{})}
	c := NewRuntimeController(&store, jobBarrierAdapter{runtime})
	job, err := c.StartProbeJob()
	if err != nil {
		t.Fatal(err)
	}
	<-runtime.entered
	awaitJob(t, c, job.JobID, func(s ProbeJobSnapshot) bool { return s.Healthy == 1 })
	_, _ = c.CancelProbeJob(job.JobID)
	<-runtime.stopEntered
	snapshot, _ := c.ProbeJobSnapshot(job.JobID, 0)
	if snapshot.State != "cancelling" || snapshot.FinishedAt != nil {
		t.Fatalf("cleanup published as finished: %+v", snapshot)
	}
	if _, err := c.StartProbeJob(); !errors.Is(err, ErrProbeInProgress) {
		t.Fatalf("replacement accepted during cleanup: %v", err)
	}
	close(runtime.stopGate)
	final := awaitJob(t, c, job.JobID, func(s ProbeJobSnapshot) bool { return s.FinishedAt != nil })
	if final.Cancelled != 1 || final.Failed != 0 || final.Healthy != 1 {
		t.Fatalf("cancelled nodes marked failed: %+v", final)
	}
	if _, err := c.CancelProbeJob(job.JobID); err != nil {
		t.Fatal("cancel is not idempotent", err)
	}
}

func TestProbeJobCancelDoesNotStopBorrowedRuntime(t *testing.T) {
	store, value := saveAutoProbeProfile(t)
	runtime := &jobBarrierRuntime{slow: value.ID + "::tuic", entered: make(chan struct{}), gate: make(chan struct{})}
	c := NewRuntimeController(&store, jobBarrierAdapter{runtime})
	c.runtime = runtime
	c.runtimeAliases = map[string]string{value.ID + "::vless": value.ID + "::vless", value.ID + "::tuic": value.ID + "::tuic"}
	job, _ := c.StartProbeJob()
	<-runtime.entered
	_, _ = c.CancelProbeJob(job.JobID)
	awaitJob(t, c, job.JobID, func(s ProbeJobSnapshot) bool { return s.FinishedAt != nil })
	if runtime.stops.Load() != 0 {
		t.Fatal("borrowed runtime stopped by job")
	}
}

func TestProbeJobProgressAndCancelDoNotWaitForRuntimeLock(t *testing.T) {
	store, _ := saveAutoProbeProfile(t)
	c := NewRuntimeController(&store, jobBarrierAdapter{&jobBarrierRuntime{}})
	c.mu.Lock()
	job, err := c.StartProbeJob()
	if err != nil {
		c.mu.Unlock()
		t.Fatal(err)
	}
	response := make(chan struct{})
	go func() { _, _ = c.ProbeJobSnapshot(job.JobID, 0); _, _ = c.CancelProbeJob(job.JobID); close(response) }()
	select {
	case <-response:
	case <-time.After(time.Second):
		c.mu.Unlock()
		t.Fatal("progress/cancel blocked by runtime lock")
	}
	c.mu.Unlock()
	_ = c.waitProbeCleanup(context.Background())
}

func TestProbeJobRejectsLateResultAfterTransition(t *testing.T) {
	c := &RuntimeController{manualProbeCache: map[string]ProbeCacheEntry{"old": {Status: ProbeHealthy}}}
	job := &nodeProbeJob{ctx: context.Background(), cancel: func() {}, generation: 0, nodes: map[string]ProbeNodeProgress{"node": {UnifiedNodeSummary: UnifiedNodeSummary{ID: "node", RuntimeTag: "old"}}}}
	c.probeJob = job
	ctx := context.WithValue(context.Background(), probeJobContextKey{}, job)
	finish := c.beginProbeTransition()
	c.publishProbeResult(ctx, candidateProbeResult{candidate: policy.Candidate{ID: "node"}, latency: 10})
	finish()
	if len(c.ProbeSnapshot()) != 0 {
		t.Fatal("stale callback restored invalidated cache")
	}
}

func TestProbeJobHTTPRegistrationSurvivesRequestCancellationAndAuthPrecedesCancel(t *testing.T) {
	store, value := saveAutoProbeProfile(t)
	runtime := &jobBarrierRuntime{slow: value.ID + "::tuic", entered: make(chan struct{}), gate: make(chan struct{})}
	c := NewRuntimeController(&store, jobBarrierAdapter{runtime})
	api := Server{Storage: &store, Runtime: c, Token: "test-only"}
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/probe/start", nil).WithContext(ctx)
	req.Header.Set("Authorization", "Bearer test-only")
	rec := httptest.NewRecorder()
	api.ServeHTTP(rec, req)
	cancel()
	if rec.Code != http.StatusAccepted {
		t.Fatal(rec.Code, rec.Body.String())
	}
	var job ProbeJobSnapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &job); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = c.CancelProbeJob(job.JobID); _ = c.waitProbeCleanup(context.Background()) }()
	<-runtime.entered
	unauthorized := httptest.NewRecorder()
	api.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodPost, "/v1/nodes/probe/jobs/"+job.JobID+"/cancel", nil))
	snapshot, _ := c.ProbeJobSnapshot(job.JobID, 0)
	if unauthorized.Code != http.StatusUnauthorized || snapshot.State == "cancelling" {
		t.Fatal("unauthorized request cancelled job")
	}
	if snapshot.FinishedAt != nil {
		t.Fatal("registration request owns job context")
	}
}

func TestBridgeRuntimePreservesProbeOnceAndWarmProbe(t *testing.T) {
	runtime := &jobBarrierRuntime{}
	bridge := &bridgeRuntime{Runtime: runtime}
	if _, err := probeOutboundOnce(bridge, context.Background(), "node", time.Second); err != nil {
		t.Fatal(err)
	}
	if runtime.onceCalls.Load() != 1 || runtime.warmCalls.Load() != 0 {
		t.Fatal("manual probe lost single-shot method")
	}
	_, _ = bridge.Probe(context.Background(), "node", time.Second)
	if runtime.warmCalls.Load() != 1 {
		t.Fatal("startup/health contract changed")
	}
	legacy := &bridgeRuntime{Runtime: &probingRuntime{outcomes: map[string]probeOutcome{"node": {latency: 33}}}}
	if latency, err := legacy.ProbeOnce(context.Background(), "node", time.Second); err != nil || latency != 33 {
		t.Fatal("fallback missing", latency, err)
	}
}

func TestProbeJobHTTPConflictDeltaAndInvalidRevision(t *testing.T) {
	store, value := saveAutoProbeProfile(t)
	runtime := &jobBarrierRuntime{slow: value.ID + "::tuic", entered: make(chan struct{}), gate: make(chan struct{})}
	c := NewRuntimeController(&store, jobBarrierAdapter{runtime})
	api := Server{Storage: &store, Runtime: c}
	request := func(method, path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		api.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
		return rec
	}
	start := request(http.MethodPost, "/v1/nodes/probe/start")
	var job ProbeJobSnapshot
	if start.Code != 202 || json.Unmarshal(start.Body.Bytes(), &job) != nil {
		t.Fatal(start.Code, start.Body.String())
	}
	defer func() { _, _ = c.CancelProbeJob(job.JobID); _ = c.waitProbeCleanup(context.Background()) }()
	<-runtime.entered
	partial := awaitJob(t, c, job.JobID, func(s ProbeJobSnapshot) bool { return s.Healthy == 1 })
	conflict := request(http.MethodPost, "/v1/nodes/probe/start")
	if conflict.Code != 409 || !strings.Contains(conflict.Body.String(), job.JobID) {
		t.Fatal("duplicate created another job", conflict.Body.String())
	}
	for _, revision := range []string{"-1", "bad", "999999"} {
		if rec := request(http.MethodGet, "/v1/nodes/probe/jobs/"+job.JobID+"?after_revision="+revision); rec.Code != 400 {
			t.Fatal("invalid revision accepted", revision, rec.Code)
		}
	}
	delta := request(http.MethodGet, fmt.Sprintf("/v1/nodes/probe/jobs/%s?after_revision=%d", job.JobID, partial.Revision))
	var progress ProbeJobSnapshot
	if delta.Code != 200 || json.Unmarshal(delta.Body.Bytes(), &progress) != nil || len(progress.Nodes) != 0 || progress.Healthy != 1 {
		t.Fatal("delta lost counts", delta.Body.String())
	}
	if rec := request(http.MethodGet, "/v1/nodes/probe/jobs/unknown"); rec.Code != 404 {
		t.Fatal("unknown ID accepted")
	}
}

func TestProbeJobSkipsChangedConfigInLiveRuntime(t *testing.T) {
	store, value := saveAutoProbeProfile(t)
	runtime := &jobBarrierRuntime{}
	c := NewRuntimeController(&store, jobBarrierAdapter{runtime})
	c.runtime = runtime
	c.runtimeAliases = map[string]string{value.ID + "::vless": value.ID + "::vless", value.ID + "::tuic": value.ID + "::tuic"}
	c.runtimeConfigHashes = map[string]string{value.ID: probeConfigHash(value.Config)}
	value.Config = append(value.Config, ' ')
	if err := store.SaveVersioned(value); err != nil {
		t.Fatal(err)
	}
	job, _ := c.StartProbeJob()
	final := awaitJob(t, c, job.JobID, func(s ProbeJobSnapshot) bool { return s.FinishedAt != nil })
	if final.State != "completed" || final.Skipped != 2 || final.Failed != 0 || runtime.onceCalls.Load() != 0 {
		t.Fatalf("changed profile measured through stale runtime: %+v", final)
	}
	for _, node := range final.Nodes {
		if node.Reason != "runtime_requires_reload" {
			t.Fatal(node.Reason)
		}
	}
}

func TestProbeJobRetentionExpires(t *testing.T) {
	finished := time.Now().Add(-probeJobRetention - time.Second)
	c := &RuntimeController{lastProbeJob: &nodeProbeJob{snapshot: ProbeJobSnapshot{JobID: "expired", FinishedAt: &finished}}}
	if _, ok := c.ProbeJobSnapshot("expired", 0); ok || c.lastProbeJob != nil {
		t.Fatal("expired history retained")
	}
}

func TestProbeJobCleanupTimeoutKeepsOwnershipAndDoesNotBlockVPNStop(t *testing.T) {
	store, value := saveAutoProbeProfile(t)
	runtime := &jobBarrierRuntime{slow: value.ID + "::tuic", entered: make(chan struct{}), gate: make(chan struct{}), stopEntered: make(chan struct{}), stopGate: make(chan struct{})}
	c := NewRuntimeController(&store, jobBarrierAdapter{runtime})
	c.probeCleanupBudget = 20 * time.Millisecond
	job, _ := c.StartProbeJob()
	<-runtime.entered
	_, _ = c.CancelProbeJob(job.JobID)
	<-runtime.stopEntered
	failed := awaitJob(t, c, job.JobID, func(s ProbeJobSnapshot) bool { return s.State == "failed" })
	if failed.CleanupComplete || failed.FinishedAt != nil {
		t.Fatal("hung cleanup falsely released ownership")
	}
	if _, err := c.StartProbeJob(); !errors.Is(err, ErrProbeInProgress) {
		t.Fatal("new probe allowed while resources remain", err)
	}
	if _, err := c.Start(value.ID); !errors.Is(err, ErrProbeCleanupTimeout) {
		t.Fatal("Start ignored failed cleanup", err)
	}
	if snapshot, err := c.Stop(); snapshot.Status != string(engine.Stopped) || !errors.Is(err, ErrProbeCleanupTimeout) {
		t.Fatal("VPN Stop blocked or hid failed cleanup", snapshot, err)
	}
	close(runtime.stopGate)
	final := awaitJob(t, c, job.JobID, func(s ProbeJobSnapshot) bool { return s.CleanupComplete })
	if final.FinishedAt == nil || final.State != "failed" {
		t.Fatal("late cleanup lost failure", final)
	}
	c.Adapter = jobBarrierAdapter{&jobBarrierRuntime{}}
	replacement, err := c.StartProbeJob()
	if err != nil {
		t.Fatal("resources closed but replacement still blocked", err)
	}
	awaitJob(t, c, replacement.JobID, func(s ProbeJobSnapshot) bool { return s.FinishedAt != nil })
}

func TestProbeJobKeepsEqualSingBoxAndOlcNamesDistinct(t *testing.T) {
	store, value := saveAutoProbeProfile(t)
	var singbox map[string]any
	if err := json.Unmarshal(value.Config, &singbox); err != nil {
		t.Fatal(err)
	}
	value.Config, _ = json.Marshal(map[string]any{"v": 1, "app": "naga-network", "singbox": singbox, "olcrtc": map[string]any{"profiles": []any{map[string]any{"name": "vless", "provider": "jitsi", "transport": "datachannel", "room": "https://meet.example/room", "key": strings.Repeat("01", 32)}}}})
	if err := store.SaveVersioned(value); err != nil {
		t.Fatal(err)
	}
	c := NewRuntimeController(&store, jobBarrierAdapter{&jobBarrierRuntime{}})
	c.OlcRTCBinary = t.TempDir() + "/missing"
	job, _ := c.StartProbeJob()
	final := awaitJob(t, c, job.JobID, func(s ProbeJobSnapshot) bool { return s.FinishedAt != nil })
	if len(final.Nodes) != 3 || final.Healthy != 2 || final.Skipped != 1 {
		t.Fatalf("equal names overwrite rows: %+v", final)
	}
	cache := c.ProbeSnapshot()
	if entry, ok := probeCacheForCandidate(cache, policy.Candidate{ProfileID: value.ID, SourceTag: "vless", RuntimeTag: value.ID + "::vless"}); !ok || entry.LatencyMS != 25 {
		t.Fatal("sing-box result lost", entry)
	}
	if _, ok := probeCacheForCandidate(cache, policy.Candidate{ProfileID: value.ID, SourceTag: "vless", Type: "olcrtc"}); ok {
		t.Fatal("olcRTC borrowed sing-box latency")
	}
}

func TestProbeJobBoundsMetadataAndNodeCount(t *testing.T) {
	if !errors.Is(validateProbeJobSize(make([]policy.Candidate, maxProbeJobNodes+1)), ErrProbeJobTooLarge) {
		t.Fatal("unbounded node count")
	}
	if !errors.Is(validateProbeJobSize([]policy.Candidate{{SourceTag: strings.Repeat("x", maxProbeJobMetadataBytes+1)}}), ErrProbeJobTooLarge) {
		t.Fatal("unbounded metadata")
	}
	if err := validateProbeJobSize(make([]policy.Candidate, 1000)); err != nil {
		t.Fatal(err)
	}
}

func TestManualProbeCacheDoesNotMaskNewHealthMeasurement(t *testing.T) {
	candidate := policy.Candidate{ID: "profile::node", ProfileID: "profile", SourceTag: "node", RuntimeTag: "node"}
	c := &RuntimeController{activeRuntimeTag: "node", runtimeCandidates: []policy.Candidate{candidate}, manualProbeCache: map[string]ProbeCacheEntry{"profile::node": {LatencyMS: 10, Status: ProbeHealthy, CheckedAt: time.Now().Add(-time.Second)}}}
	c.mu.Lock()
	c.recordProbeLocked(candidateProbeResult{candidate: candidate, latency: 40})
	entry, ok := c.activeProbeEntryLocked()
	c.mu.Unlock()
	if !ok || entry.LatencyMS != 40 {
		t.Fatal("old manual result masks current health", entry)
	}
}

func TestProbeJobHTTPOptionsValidatedAndOwnedByJob(t *testing.T) {
	store, value := saveAutoProbeProfile(t)
	runtime := &jobBarrierRuntime{slow: value.ID + "::tuic", entered: make(chan struct{}), gate: make(chan struct{})}
	c := NewRuntimeController(&store, jobBarrierAdapter{runtime})
	api := Server{Storage: &store, Runtime: c}
	post := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		api.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/nodes/probe/start", strings.NewReader(body)))
		return rec
	}
	for _, body := range []string{`{"concurrency":-1}`, `{"concurrency":21}`, `{"concurrency":1.5}`, `{"unknown":1}`, `{} {}`, strings.Repeat("x", 2048)} {
		if rec := post(body); rec.Code != 400 {
			t.Fatal("invalid options accepted", rec.Code)
		}
		if _, active := c.ProbeJobSnapshot("active", 0); active {
			t.Fatal("invalid options created job")
		}
	}
	limit := min(16, maxManualProbeConcurrency())
	rec := post(fmt.Sprintf(`{"concurrency":%d}`, limit))
	var job ProbeJobSnapshot
	if rec.Code != 202 || json.Unmarshal(rec.Body.Bytes(), &job) != nil || job.Concurrency.SingBox != limit {
		t.Fatal("job options lost", rec.Body.String())
	}
	<-runtime.entered
	_, _ = c.CancelProbeJob(job.JobID)
	final := awaitJob(t, c, job.JobID, func(s ProbeJobSnapshot) bool { return s.CleanupComplete })
	if final.Concurrency.SingBox != limit || c.ManualProbeConcurrency != 0 {
		t.Fatal("one-shot option changed controller default")
	}
}

type scenarioProbeRuntime struct {
	jobBarrierRuntime
	failures int
}

func (r *scenarioProbeRuntime) ProbeOnce(ctx context.Context, tag string, _ time.Duration) (int, error) {
	r.onceCalls.Add(1)
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if index := strings.LastIndex(tag, "::"); index >= 0 {
		tag = tag[index+2:]
	}
	index, err := strconv.Atoi(strings.TrimPrefix(tag, "node"))
	if err != nil {
		return 0, err
	}
	if index < r.failures {
		return 0, errors.New("synthetic unreachable node")
	}
	return 25, nil
}

type scenarioProbeAdapter struct{ runtime *scenarioProbeRuntime }

func (scenarioProbeAdapter) Type() engine.Type                      { return engine.SingBox }
func (scenarioProbeAdapter) Validate(context.Context, []byte) error { return nil }
func (a scenarioProbeAdapter) Start(context.Context, []byte) (engine.Runtime, error) {
	return a.runtime, nil
}

func TestProbeJobControlledOutcomeMatrix(t *testing.T) {
	for _, size := range []int{1, 10, 50, 100, 1000} {
		for _, percent := range []int{0, 10, 50, 100} {
			if size == 1000 && percent != 50 {
				continue
			}
			for _, live := range []bool{false, true} {
				t.Run(fmt.Sprintf("nodes=%d/fail=%d/live=%t", size, percent, live), func(t *testing.T) {
					store, value := saveAutoProbeProfile(t)
					tags := make([]string, size)
					outbounds := make([]any, 0, size+1)
					for i := range tags {
						tags[i] = fmt.Sprintf("node%04d", i)
						kind := "vless"
						if i%3 == 0 {
							kind = "tuic"
						}
						outbounds = append(outbounds, map[string]any{"type": kind, "tag": tags[i], "server": "example.invalid", "server_port": 443, "uuid": "00000000-0000-0000-0000-000000000000", "password": "synthetic"})
					}
					outbounds = append(outbounds, map[string]any{"type": "selector", "tag": "proxy", "outbounds": tags})
					value.Config, _ = json.Marshal(map[string]any{"route": map[string]any{"final": "proxy"}, "outbounds": outbounds})
					if err := store.SaveVersioned(value); err != nil {
						t.Fatal(err)
					}
					runtime := &scenarioProbeRuntime{failures: size * percent / 100}
					c := NewRuntimeController(&store, scenarioProbeAdapter{runtime})
					if live {
						c.runtime = runtime
						c.runtimeAliases = map[string]string{}
						for _, tag := range tags {
							c.runtimeAliases[value.ID+"::"+tag] = value.ID + "::" + tag
						}
					}
					job, err := c.StartProbeJob()
					if err != nil {
						t.Fatal(err)
					}
					// Race instrumentation and full snapshots of 1000 nodes can take
					// longer than the small barrier-based tests on shared CI runners.
					final := awaitJobWithin(t, c, job.JobID, 10*time.Second, func(s ProbeJobSnapshot) bool { return s.CleanupComplete })
					if final.State != "completed" || final.Total == nil || *final.Total != size || final.Healthy != size-runtime.failures || final.Failed != runtime.failures || len(final.Nodes) != size {
						t.Fatalf("matrix outcome mismatch: state=%s total=%v healthy=%d failed=%d nodes=%d", final.State, final.Total, final.Healthy, final.Failed, len(final.Nodes))
					}
					if int(runtime.onceCalls.Load()) != size || runtime.warmCalls.Load() != 0 {
						t.Fatal("missing probes or warm list fallback")
					}
					wantStops := int32(1)
					if live {
						wantStops = 0
					}
					if runtime.stops.Load() != wantStops {
						t.Fatal("wrong resource owner", runtime.stops.Load())
					}
				})
			}
		}
	}
}
