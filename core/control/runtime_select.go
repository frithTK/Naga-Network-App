package control

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"naga.network/core/diagnostics"
	"naga.network/core/engine"
	awgengine "naga.network/core/engine/amneziawg"
	"naga.network/core/policy"
)

type groupProbeOutcome struct {
	skip     bool
	recorded []candidateProbeResult
	selected *candidateProbeResult
}

const awgSelectionWindow = 350 * time.Millisecond

// probeCandidateGroups keeps "connected" honest: the local process and Clash
// API must be ready, and a leaf outbound must also complete an HTTP
// generate_204 probe. In automatic mode TUIC is a separate final fallback group.
func probeCandidateGroups(
	parent context.Context,
	journal *diagnostics.Logger,
	runtime engine.Runtime,
	candidates []policy.Candidate,
	connectionPolicy policy.ConnectionPolicy,
	timeout time.Duration,
	awgProbers map[string]awgengine.HTTPSProber,
) (groupProbeOutcome, error) {
	if timeout <= 0 {
		timeout = outboundProbeTimeout
	}
	prober, canProbe := runtime.(engine.OutboundProber)
	_, canSelect := runtime.(engine.SelectorSwitcher)
	if !canSelect || (!canProbe && len(awgProbers) == 0) {
		if journal != nil {
			journal.Warn("runtime", "node_probe_unavailable", "Runtime не поддерживает проверку доступности VPN-узлов", nil)
		}
		return groupProbeOutcome{skip: true}, nil
	}

	groups := candidateProbeGroups(candidates, connectionPolicy)
	if len(groups) == 0 {
		return groupProbeOutcome{}, errors.New("нет разрешённых VPN-узлов для проверки доступности")
	}
	var recorded []candidateProbeResult
	for groupIndex, group := range groups {
		if len(group) == 0 {
			continue
		}
		if journal != nil {
			journal.Info("runtime", "node_probe_group_started", "Начата проверка группы VPN-узлов", map[string]any{
				"group":      groupIndex + 1,
				"candidates": len(group),
				"tuic":       candidateIsTUIC(group[0]),
			})
		}
		groupContext, cancelGroup := context.WithCancel(parent)
		results := make(chan candidateProbeResult, len(group))
		earlySelection := false
		awgSlots := make(chan struct{}, 4)
		for _, candidate := range group {
			candidate := candidate
			if _, ok := awgProbers[candidate.RuntimeTag].(*deferredAWG); ok {
				earlySelection = true
			}
			go func() {
				startedAt := time.Now()
				nodeTimeout := probeTimeoutFor(candidate, timeout)
				result := candidateProbeResult{candidate: candidate}
				if awg := awgProbers[candidate.RuntimeTag]; awg != nil {
					select {
					case awgSlots <- struct{}{}:
						defer func() { <-awgSlots }()
					case <-groupContext.Done():
						return
					}
					result = probeAWGHTTPS(groupContext, candidate, awg, nodeTimeout, true)
				} else if canProbe {
					result.latency, result.err = prober.Probe(groupContext, candidate.RuntimeTag, nodeTimeout)
				} else {
					result.err = errors.New("runtime does not support probing this outbound")
				}
				if groupContext.Err() != nil {
					return
				}
				if journal != nil {
					fields := map[string]any{
						"node":        candidate.SourceTag,
						"country":     candidate.Country,
						"protocol":    candidate.Protocol,
						"duration_ms": time.Since(startedAt).Milliseconds(),
					}
					if result.source != "" {
						fields["probe_source"] = result.source
						fields["probe_target"] = result.target
					}
					if result.err != nil {
						fields["error"] = result.err
						journal.Warn("runtime", "node_probe_failed", "VPN-узел не прошёл проверку доступности", fields)
					} else {
						fields["latency_ms"] = result.latency
						journal.Info("runtime", "node_probe_succeeded", "VPN-узел доступен", fields)
					}
				}
				results <- result
			}()
		}

		var selected *candidateProbeResult
		var selectionTimer *time.Timer
		var selectionDeadline <-chan time.Time
	collect:
		for range group {
			var result candidateProbeResult
			select {
			case result = <-results:
			case <-selectionDeadline:
				break collect
			case <-groupContext.Done():
				break collect
			}
			recorded = append(recorded, result)
			if result.err != nil || result.latency <= 0 {
				continue
			}
			usingReserve := result.target == "cloudflare"
			if selected == nil || (selected.target == "cloudflare" && !usingReserve) ||
				((selected.target == "cloudflare") == usingReserve && result.latency < selected.latency) {
				copy := result
				selected = &copy
			}
			if earlySelection && selectionTimer == nil {
				selectionTimer = time.NewTimer(awgSelectionWindow)
				selectionDeadline = selectionTimer.C
			}
		}
		if selectionTimer != nil {
			selectionTimer.Stop()
		}
		cancelGroup()
		if err := parent.Err(); err != nil {
			return groupProbeOutcome{recorded: recorded}, err
		}
		if selected == nil {
			continue
		}
		return groupProbeOutcome{recorded: recorded, selected: selected}, nil
	}
	return groupProbeOutcome{recorded: recorded}, errors.New("ни один VPN-узел не прошёл проверку доступности")
}

func (c *RuntimeController) applyProbeSelectionLocked(parent context.Context, runtime engine.Runtime, outcome groupProbeOutcome) error {
	for _, result := range outcome.recorded {
		c.recordProbeLocked(result)
	}
	if outcome.selected == nil {
		return errors.New("ни один VPN-узел не прошёл проверку доступности")
	}
	selected := outcome.selected
	previous := c.activeRuntimeTag
	if err := c.selectProbedCandidateLocked(parent, runtime, selected.candidate); err != nil {
		return err
	}
	if previous != "" && previous != selected.candidate.RuntimeTag {
		if entry, ok := c.probeCache[previous]; ok && entry.Status == ProbeFailed {
			c.failoverMessage = fmt.Sprintf("Переключили на %s — предыдущий не ответил", selected.candidate.SourceTag)
		} else {
			c.failoverMessage = ""
		}
	} else if selected.candidate.SourceTag != "" && !strings.Contains(c.failoverMessage, selected.candidate.SourceTag) {
		c.failoverMessage = ""
	}
	if previous != selected.candidate.RuntimeTag && c.Journal != nil {
		c.Journal.Info("runtime", "healthy_node_selected", "Выбран проверенный VPN-узел", map[string]any{
			"node":       selected.candidate.SourceTag,
			"country":    selected.candidate.Country,
			"protocol":   selected.candidate.Protocol,
			"latency_ms": selected.latency,
			"tuic":       candidateIsTUIC(selected.candidate),
		})
	}
	return nil
}

func (c *RuntimeController) selectProbedCandidateLocked(parent context.Context, runtime engine.Runtime, candidate policy.Candidate) error {
	switcher, canSelect := runtime.(engine.SelectorSwitcher)
	if !canSelect {
		return nil
	}
	if candidate.RuntimeTag != c.activeRuntimeTag {
		ctx, cancel := context.WithTimeout(parent, 5*time.Second)
		defer cancel()
		if err := switcher.Select(ctx, candidate.RuntimeTag); err != nil {
			return fmt.Errorf("выбрать VPN-узел: %w", err)
		}
	}
	c.activeRuntimeTag = candidate.RuntimeTag
	c.healthFailCount = 0
	return nil
}

// CacheConnectionMode stores the last applied connection mode so health ticks
// do not read settings.json while holding c.mu.
func (c *RuntimeController) CacheConnectionMode(mode policy.ConnectionMode) {
	c.mu.Lock()
	c.connectionMode = mode
	c.mu.Unlock()
}
