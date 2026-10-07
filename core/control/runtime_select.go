package control

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"naga.network/core/diagnostics"
	"naga.network/core/engine"
	"naga.network/core/policy"
)

type groupProbeOutcome struct {
	skip     bool
	recorded []candidateProbeResult
	selected *candidateProbeResult
}

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
) (groupProbeOutcome, error) {
	if timeout <= 0 {
		timeout = outboundProbeTimeout
	}
	prober, canProbe := runtime.(engine.OutboundProber)
	_, canSelect := runtime.(engine.SelectorSwitcher)
	if !canProbe || !canSelect {
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
		results := make(chan candidateProbeResult, len(group))
		for _, candidate := range group {
			candidate := candidate
			go func() {
				startedAt := time.Now()
				latency, err := prober.Probe(parent, candidate.RuntimeTag, probeTimeoutFor(candidate, timeout))
				if journal != nil {
					fields := map[string]any{
						"node":        candidate.SourceTag,
						"country":     candidate.Country,
						"protocol":    candidate.Protocol,
						"duration_ms": time.Since(startedAt).Milliseconds(),
					}
					if err != nil {
						fields["error"] = err
						journal.Warn("runtime", "node_probe_failed", "VPN-узел не прошёл проверку доступности", fields)
					} else {
						fields["latency_ms"] = latency
						journal.Info("runtime", "node_probe_succeeded", "VPN-узел доступен", fields)
					}
				}
				results <- candidateProbeResult{candidate: candidate, latency: latency, err: err}
			}()
		}

		var selected *candidateProbeResult
		for range group {
			result := <-results
			recorded = append(recorded, result)
			if result.err != nil || result.latency <= 0 {
				continue
			}
			if selected == nil || result.latency < selected.latency {
				copy := result
				selected = &copy
			}
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
	switcher, canSelect := runtime.(engine.SelectorSwitcher)
	if !canSelect {
		return nil
	}
	selected := outcome.selected
	if selected.candidate.RuntimeTag != c.activeRuntimeTag {
		selectContext, cancel := context.WithTimeout(parent, 5*time.Second)
		err := switcher.Select(selectContext, selected.candidate.RuntimeTag)
		cancel()
		if err != nil {
			return fmt.Errorf("выбрать проверенный VPN-узел: %w", err)
		}
	}
	previous := c.activeRuntimeTag
	c.activeRuntimeTag = selected.candidate.RuntimeTag
	c.healthFailCount = 0
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

// CacheConnectionMode stores the last applied connection mode so health ticks
// do not read settings.json while holding c.mu.
func (c *RuntimeController) CacheConnectionMode(mode policy.ConnectionMode) {
	c.mu.Lock()
	c.connectionMode = mode
	c.mu.Unlock()
}
