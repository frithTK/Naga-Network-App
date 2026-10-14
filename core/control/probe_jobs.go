package control

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"time"

	"naga.network/core/policy"
	"naga.network/core/profile"
)

const probeJobRetention = 5 * time.Minute
const maxProbeJobNodes = 10000
const maxProbeJobMetadataBytes = 8 << 20

var ErrProbeJobTooLarge = errors.New("список проверки слишком большой; уменьшите количество или длину названий узлов")

var ErrProbeJobNotFound = errors.New("probe job not found")
var ErrInvalidProbeConcurrency = errors.New("invalid probe concurrency")

type ProbeConcurrency struct {
	SingBox int `json:"sing_box"`
	AWG     int `json:"awg"`
	OlcRTC  int `json:"olcrtc"`
}

type ProbeNodeProgress struct {
	UnifiedNodeSummary
	Phase    string `json:"phase"`
	Reason   string `json:"reason,omitempty"`
	Revision uint64 `json:"revision"`
}

func probeConfigHash(config []byte) string {
	hash := sha256.Sum256(config)
	return hex.EncodeToString(hash[:])
}
func probeProfileHashes(values []profile.Profile) map[string]string {
	hashes := make(map[string]string, len(values))
	for _, value := range values {
		hashes[value.ID] = probeConfigHash(value.Config)
	}
	return hashes
}

type ProbeJobSnapshot struct {
	ElapsedMS       int64               `json:"elapsed_ms"`
	Concurrency     ProbeConcurrency    `json:"concurrency"`
	CleanupComplete bool                `json:"cleanup_complete"`
	Invalidated     bool                `json:"invalidated,omitempty"`
	JobID           string              `json:"job_id"`
	State           string              `json:"state"`
	Revision        uint64              `json:"revision"`
	StartedAt       time.Time           `json:"started_at"`
	Deadline        time.Time           `json:"deadline"`
	FinishedAt      *time.Time          `json:"finished_at,omitempty"`
	Total           *int                `json:"total"`
	Completed       int                 `json:"completed"`
	Healthy         int                 `json:"healthy"`
	Failed          int                 `json:"failed"`
	Cancelled       int                 `json:"cancelled"`
	Skipped         int                 `json:"skipped"`
	Nodes           []ProbeNodeProgress `json:"nodes"`
	Error           string              `json:"error,omitempty"`
}

type nodeProbeJob struct {
	ctx             context.Context
	cancel          context.CancelFunc
	done            chan struct{}
	generation      uint64
	snapshot        ProbeJobSnapshot
	nodes           map[string]ProbeNodeProgress
	profileHashes   map[string]string
	cleanupActive   int
	cleanupDeadline time.Time
	cleanupPending  []<-chan error
	cleanupErr      error
	startedAt       time.Time
	completedAt     time.Time
	err             error
}

type probeJobContextKey struct{}

func probeJobFromContext(ctx context.Context) *nodeProbeJob {
	job, _ := ctx.Value(probeJobContextKey{}).(*nodeProbeJob)
	return job
}

func probeNodeID(candidate policy.Candidate) string {
	if candidate.ID != "" {
		return candidate.ID
	}
	return candidate.ProfileID + "::" + candidate.SourceTag
}

func (c *RuntimeController) startProbeJob() (*nodeProbeJob, ProbeJobSnapshot, error) {
	return c.startProbeJobWithConcurrency(0)
}

func (c *RuntimeController) startProbeJobWithConcurrency(concurrency int) (*nodeProbeJob, ProbeJobSnapshot, error) {
	if concurrency < 0 || concurrency > maxManualProbeConcurrency() {
		return nil, ProbeJobSnapshot{}, ErrInvalidProbeConcurrency
	}
	if concurrency == 0 {
		concurrency = c.manualProbeConcurrency()
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return nil, ProbeJobSnapshot{}, err
	}
	c.probeMu.Lock()
	defer c.probeMu.Unlock()
	if c.probeJob != nil {
		return nil, probeJobSnapshotLocked(c.probeJob, 0), ErrProbeInProgress
	}
	if c.probeTransitions != 0 {
		return nil, ProbeJobSnapshot{}, ErrProbeInProgress
	}
	now := time.Now()
	budget := nodeListProbeBudget
	if c.probeBudget > 0 {
		budget = c.probeBudget
	}
	ctx, cancel := context.WithDeadline(context.Background(), now.Add(budget))
	job := &nodeProbeJob{cancel: cancel, done: make(chan struct{}), generation: c.probeGeneration, startedAt: now,
		nodes: make(map[string]ProbeNodeProgress), snapshot: ProbeJobSnapshot{
			Concurrency: ProbeConcurrency{SingBox: concurrency, AWG: 1, OlcRTC: 1},
			JobID:       hex.EncodeToString(random[:]), State: "preparing", Revision: 1,
			StartedAt: now.UTC(), Deadline: now.Add(budget).UTC(),
		}}
	job.ctx = context.WithValue(ctx, probeJobContextKey{}, job)
	c.probeJob = job
	c.lastProbeJob = nil
	go c.runProbeJob(job)
	return job, probeJobSnapshotLocked(job, 0), nil
}

func (c *RuntimeController) StartProbeJob() (ProbeJobSnapshot, error) {
	_, snapshot, err := c.startProbeJob()
	return snapshot, err
}

func probeJobSnapshotLocked(job *nodeProbeJob, after uint64) ProbeJobSnapshot {
	snapshot := job.snapshot
	end := time.Now()
	if !job.completedAt.IsZero() {
		end = job.completedAt
	}
	if !job.startedAt.IsZero() {
		snapshot.ElapsedMS = end.Sub(job.startedAt).Milliseconds()
	}
	// Return values own their pointers/slices; callers never alias mutable state.
	if snapshot.Total != nil {
		total := *snapshot.Total
		snapshot.Total = &total
	}
	if snapshot.FinishedAt != nil {
		finished := *snapshot.FinishedAt
		snapshot.FinishedAt = &finished
	}
	snapshot.Nodes = make([]ProbeNodeProgress, 0, len(job.nodes))
	for _, node := range job.nodes {
		if node.Revision > after {
			snapshot.Nodes = append(snapshot.Nodes, node)
		}
	}
	sort.Slice(snapshot.Nodes, func(i, j int) bool { return snapshot.Nodes[i].ID < snapshot.Nodes[j].ID })
	return snapshot
}

func (c *RuntimeController) retainedProbeJobLocked(id string) *nodeProbeJob {
	if c.lastProbeJob != nil && time.Since(*c.lastProbeJob.snapshot.FinishedAt) > probeJobRetention {
		c.lastProbeJob = nil
	}
	if c.probeJob != nil && (id == "active" || c.probeJob.snapshot.JobID == id) {
		return c.probeJob
	}
	if id != "active" && c.lastProbeJob != nil && c.lastProbeJob.snapshot.JobID == id &&
		time.Since(*c.lastProbeJob.snapshot.FinishedAt) <= probeJobRetention {
		return c.lastProbeJob
	}
	return nil
}

func (c *RuntimeController) ProbeJobSnapshot(id string, after uint64) (ProbeJobSnapshot, bool) {
	c.probeMu.Lock()
	defer c.probeMu.Unlock()
	job := c.retainedProbeJobLocked(id)
	if job == nil {
		return ProbeJobSnapshot{}, false
	}
	return probeJobSnapshotLocked(job, after), true
}

func (c *RuntimeController) CancelProbeJob(id string) (ProbeJobSnapshot, error) {
	c.probeMu.Lock()
	defer c.probeMu.Unlock()
	job := c.retainedProbeJobLocked(id)
	if job == nil {
		return ProbeJobSnapshot{}, ErrProbeJobNotFound
	}
	if job.snapshot.FinishedAt == nil && job.snapshot.State != "cancelling" && job.cleanupErr == nil {
		job.snapshot.State = "cancelling"
		job.snapshot.Revision++
		job.cancel()
	}
	return probeJobSnapshotLocked(job, 0), nil
}

// A transition prevents registration until its owner finishes. It invalidates
// results immediately, but resource cleanup is waited for outside all locks.
func (c *RuntimeController) beginProbeTransition() func() {
	c.probeMu.Lock()
	c.probeTransitions++
	c.probeGeneration++
	c.manualProbeCache = nil
	if c.probeJob != nil {
		c.probeJob.snapshot.Invalidated = true
		if c.probeJob.cleanupErr == nil {
			c.probeJob.snapshot.State = "cancelling"
		}
		c.probeJob.snapshot.Revision++
		c.probeJob.cancel()
	}
	c.probeMu.Unlock()
	return func() { c.probeMu.Lock(); c.probeTransitions--; c.probeMu.Unlock() }
}

func (c *RuntimeController) waitProbeCleanup(ctx context.Context) error {
	c.probeMu.Lock()
	job := c.probeJob
	c.probeMu.Unlock()
	if job == nil {
		return nil
	}
	select {
	case <-job.done:
		c.probeMu.Lock()
		defer c.probeMu.Unlock()
		if !job.snapshot.CleanupComplete {
			return job.cleanupErr
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *RuntimeController) runProbeJob(job *nodeProbeJob) {
	var values []profile.Profile
	if c.Store != nil {
		values, _ = c.Store.List()
	}
	job.profileHashes = probeProfileHashes(values)
	revisions := map[string]int64{}
	for _, value := range values {
		revisions[value.ID] = value.Revision
	}
	candidates := c.allListCandidates()
	preparationErr := validateProbeJobSize(candidates)
	if preparationErr != nil {
		candidates = nil
	}
	c.probeMu.Lock()
	total := len(candidates)
	job.snapshot.Total = &total
	job.snapshot.Revision++
	for _, candidate := range candidates {
		job.nodes[probeNodeID(candidate)] = ProbeNodeProgress{UnifiedNodeSummary: UnifiedNodeSummary{
			ID: probeNodeID(candidate), ProfileID: candidate.ProfileID, Tag: candidate.SourceTag,
			RuntimeTag: candidate.RuntimeTag, Country: candidate.Country, Protocol: candidate.Protocol,
			ProbeStatus: string(ProbeUnknown), ProfileRevision: revisions[candidate.ProfileID]}, Phase: "queued", Revision: job.snapshot.Revision}
	}
	if job.ctx.Err() == nil {
		job.snapshot.State = "running"
	}
	c.probeMu.Unlock()
	var err error
	if preparationErr != nil {
		err = preparationErr
	} else if total == 0 {
		err = ErrNoProbeProfiles
	} else if job.ctx.Err() == nil {
		err = c.runProbeNodes(job.ctx)
	}
	// Owned stop calls have completed or exceeded the cleanup grace period.
	c.probeMu.Lock()
	defer c.probeMu.Unlock()
	job.err = err
	state := "completed"
	if job.ctx.Err() != nil {
		job.err = job.ctx.Err()
		state = "cancelled"
		if errors.Is(job.ctx.Err(), context.DeadlineExceeded) {
			state = "timed_out"
		}
	} else if err != nil {
		state = "failed"
		job.snapshot.Error = safeRuntimeError(err)
	}
	if job.cleanupErr != nil {
		state, job.err = "failed", job.cleanupErr
		job.snapshot.Error = "Не удалось завершить проверку серверов."
	}
	job.snapshot.State = state
	job.snapshot.Revision++
	for id, node := range job.nodes {
		if node.Phase == "queued" || node.Phase == "preparing" || node.Phase == "checking" {
			node.Phase, node.Reason = "skipped", "unavailable"
			if state == "cancelled" || state == "timed_out" {
				node.Phase, node.Reason = "cancelled", state
			}
			node.Revision = job.snapshot.Revision
			job.nodes[id] = node
		}
	}
	countProbeNodesLocked(job)
	job.snapshot.CleanupComplete = len(job.cleanupPending) == 0
	job.cancel()
	if job.snapshot.CleanupComplete {
		job.completedAt = time.Now()
		finished := job.completedAt.UTC()
		job.snapshot.FinishedAt = &finished
		c.probeJob = nil
		c.lastProbeJob = job
	} else {
		go c.finishDelayedProbeCleanup(job)
	}
	close(job.done)
}

func countProbeNodesLocked(job *nodeProbeJob) {
	job.snapshot.Completed, job.snapshot.Healthy, job.snapshot.Failed = 0, 0, 0
	job.snapshot.Cancelled, job.snapshot.Skipped = 0, 0
	for _, node := range job.nodes {
		switch node.Phase {
		case "healthy":
			job.snapshot.Healthy++
			job.snapshot.Completed++
		case "failed":
			job.snapshot.Failed++
			job.snapshot.Completed++
		case "cancelled":
			job.snapshot.Cancelled++
		case "skipped":
			job.snapshot.Skipped++
		}
	}
}

func (c *RuntimeController) publishProbePhase(ctx context.Context, candidate policy.Candidate, phase, reason string) {
	job := probeJobFromContext(ctx)
	if job == nil {
		return
	}
	c.probeMu.Lock()
	defer c.probeMu.Unlock()
	if c.probeJob != job || job.generation != c.probeGeneration || job.ctx.Err() != nil {
		return
	}
	node, ok := job.nodes[probeNodeID(candidate)]
	if !ok {
		return
	}
	job.snapshot.Revision++
	node.Phase, node.Reason, node.Revision = phase, reason, job.snapshot.Revision
	setProbeNodeLocked(job, node)
}

func (c *RuntimeController) publishProbeResult(ctx context.Context, result candidateProbeResult) {
	job := probeJobFromContext(ctx)
	if job == nil {
		if ctx.Err() == nil {
			c.mu.Lock()
			c.recordProbeLocked(result)
			c.mu.Unlock()
		}
		return
	}
	c.probeMu.Lock()
	defer c.probeMu.Unlock()
	if c.probeJob != job || job.generation != c.probeGeneration || job.ctx.Err() != nil {
		return
	}
	node, ok := job.nodes[probeNodeID(result.candidate)]
	if !ok {
		return
	}
	entry := ProbeCacheEntry{Status: ProbeFailed, CheckedAt: time.Now().UTC(), ConfigHash: job.profileHashes[result.candidate.ProfileID]}
	node.Phase, node.Reason = "failed", "probe_failed"
	if result.err == nil && result.latency > 0 {
		entry.Status, entry.LatencyMS = ProbeHealthy, result.latency
		node.Phase, node.Reason = "healthy", ""
	} else if errors.Is(result.err, context.DeadlineExceeded) {
		node.Reason = "timeout"
	}
	job.snapshot.Revision++
	node.LatencyMS, node.ProbeStatus, node.CheckedAt = entry.LatencyMS, string(entry.Status), entry.CheckedAt
	node.Revision = job.snapshot.Revision
	setProbeNodeLocked(job, node)
	if c.manualProbeCache == nil {
		c.manualProbeCache = make(map[string]ProbeCacheEntry)
	}
	if node.Protocol != "olcRTC" {
		c.manualProbeCache[node.RuntimeTag] = entry
	}
	key := node.ProfileID + "::" + node.Tag
	if node.Protocol == "olcRTC" {
		key = olcProbeKey(node.ProfileID, node.Tag)
	}
	c.manualProbeCache[key] = entry
}

func setProbeNodeLocked(job *nodeProbeJob, node ProbeNodeProgress) {
	previous, exists := job.nodes[node.ID]
	adjust := func(phase string, delta int) {
		switch phase {
		case "healthy":
			job.snapshot.Healthy += delta
			job.snapshot.Completed += delta
		case "failed":
			job.snapshot.Failed += delta
			job.snapshot.Completed += delta
		case "cancelled":
			job.snapshot.Cancelled += delta
		case "skipped":
			job.snapshot.Skipped += delta
		}
	}
	if exists {
		adjust(previous.Phase, -1)
	}
	adjust(node.Phase, 1)
	job.nodes[node.ID] = node
}

func validateProbeJobSize(candidates []policy.Candidate) error {
	if len(candidates) > maxProbeJobNodes {
		return ErrProbeJobTooLarge
	}
	metadata := 0
	for _, candidate := range candidates {
		metadata += len(candidate.ID) + len(candidate.ProfileID) + len(candidate.SourceTag) + len(candidate.RuntimeTag) + len(candidate.Country) + len(candidate.Protocol)
		if metadata > maxProbeJobMetadataBytes {
			return ErrProbeJobTooLarge
		}
	}
	return nil
}
