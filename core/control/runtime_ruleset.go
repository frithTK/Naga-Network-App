package control

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"time"

	"naga.network/core/engine"
	"naga.network/core/policy"
	"naga.network/core/ruleset"
)

func (c *RuntimeController) materializeProviderRuleSets(stage *string, config []byte, routing policy.RoutingPolicy) ([]byte, bool) {
	if !routing.ProviderRules || c.RuleSets == nil {
		return config, false
	}
	remotes, err := ruleset.ListRemote(config)
	if err != nil || len(remotes) == 0 {
		return config, false
	}
	*stage = "ruleset_fetch"
	parent := c.startupContextLocked()
	c.mu.Unlock()
	ctx, cancel := context.WithTimeout(parent, 9*time.Second)
	out, stats, matErr := c.RuleSets.Materialize(ctx, config)
	cancel()
	c.mu.Lock()
	c.journalRuleSetStats(stats, routing, matErr)
	if c.startAborted || parent.Err() != nil {
		return config, true
	}
	if len(out) == 0 {
		return config, false
	}
	return out, false
}

func (c *RuntimeController) materializeIdleRuleSets(ctx context.Context, config []byte) []byte {
	if c.RuleSets == nil || c.Store == nil {
		return config
	}
	routing, err := c.Store.LoadRoutingPolicy()
	if err != nil || !routing.ProviderRules {
		return config
	}
	remotes, err := ruleset.ListRemote(config)
	if err != nil || len(remotes) == 0 {
		return config
	}
	out, stats, matErr := c.RuleSets.Materialize(ctx, config)
	c.journalRuleSetStats(stats, routing, matErr)
	if len(out) == 0 {
		return config
	}
	return out
}

func (c *RuntimeController) startEngineWithRuleSetRetry(ctx context.Context, config, original []byte) (engine.Runtime, []byte, error) {
	runtime, err := c.Adapter.Start(ctx, config)
	if err == nil {
		return runtime, config, nil
	}
	if !ruleSetStartFailed(err) || ctx.Err() != nil || bytes.Equal(config, original) {
		return nil, config, err
	}
	// Restore the exact definitions used before cache materialization. Invented
	// placeholder URLs preserve syntax but can never restore provider rules.
	retried, retryErr := c.Adapter.Start(ctx, original)
	if retryErr != nil {
		return nil, config, errors.Join(err, retryErr)
	}
	if c.Journal != nil {
		c.Journal.Warn("runtime", "ruleset_cache_retry", "Движок запущен с исходными правилами провайдера после ошибки локального кеша", nil)
	}
	return retried, original, nil
}

func (c *RuntimeController) journalRuleSetStats(stats ruleset.Stats, routing policy.RoutingPolicy, err error) {
	if c.Journal == nil {
		return
	}
	fields := map[string]any{
		"fetched":         stats.Fetched,
		"cache":           stats.Cache,
		"skipped":         stats.Skipped,
		"provider_rules":  routing.ProviderRules,
		"builtin_private": routing.BuiltinPrivate,
		"builtin_ru":      routing.BuiltinRU,
	}
	if err != nil {
		c.Journal.Warn("runtime", "ruleset_materialize", "Наборы правил провайдера не полностью применены", fields)
		return
	}
	c.Journal.Info("runtime", "ruleset_materialize", "Наборы правил провайдера подготовлены", fields)
}

func ruleSetStartFailed(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "rule-set") ||
		strings.Contains(msg, "rule_set") ||
		strings.Contains(msg, "ruleset")
}
