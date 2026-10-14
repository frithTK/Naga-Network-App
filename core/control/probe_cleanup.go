package control

import (
	"context"
	"errors"
	"time"
)

var ErrProbeCleanupTimeout = errors.New("не удалось завершить очистку проверки за отведённое время")

const probeCleanupBudget = 5 * time.Second

// A timeout does not relinquish ownership. The job stays registered until the
// stop call returns; progress/cancel and VPN Stop remain available meanwhile.
func (c *RuntimeController) stopOwnedProbeResource(ctx context.Context, stop func() error) error {
	job := probeJobFromContext(ctx)
	if job == nil {
		return stop()
	}
	c.probeMu.Lock()
	if job.cleanupActive == 0 {
		budget := probeCleanupBudget
		if c.probeCleanupBudget > 0 {
			budget = c.probeCleanupBudget
		}
		job.cleanupDeadline = time.Now().Add(budget)
	}
	job.cleanupActive++
	deadline := job.cleanupDeadline
	c.probeMu.Unlock()
	done := make(chan error, 1)
	go func() { done <- stop() }()
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	select {
	case err := <-done:
		c.probeMu.Lock()
		job.cleanupActive--
		job.cleanupErr = errors.Join(job.cleanupErr, err)
		c.probeMu.Unlock()
		return err
	case <-timer.C:
		c.probeMu.Lock()
		job.cleanupErr = errors.Join(job.cleanupErr, ErrProbeCleanupTimeout)
		job.cleanupPending = append(job.cleanupPending, done)
		job.cancel()
		c.probeMu.Unlock()
		return ErrProbeCleanupTimeout
	}
}

func (c *RuntimeController) finishDelayedProbeCleanup(job *nodeProbeJob) {
	for _, done := range job.cleanupPending {
		<-done
	}
	c.probeMu.Lock()
	defer c.probeMu.Unlock()
	job.cleanupActive = 0
	job.cleanupPending = nil
	job.snapshot.CleanupComplete = true
	job.snapshot.Revision++
	job.completedAt = time.Now()
	finished := job.completedAt.UTC()
	job.snapshot.FinishedAt = &finished
	if c.probeJob == job {
		c.probeJob = nil
		c.lastProbeJob = job
	}
}
