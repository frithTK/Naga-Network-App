package control

import (
	"bytes"
	"context"
	"encoding/json"
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
	c.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 9*time.Second)
	out, stats, matErr := c.RuleSets.Materialize(ctx, config)
	cancel()
	c.mu.Lock()
	c.journalRuleSetStats(stats, routing, matErr)
	if c.startAborted {
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

func (c *RuntimeController) startEngineWithRuleSetRetry(ctx context.Context, config []byte) (engine.Runtime, error) {
	runtime, err := c.Adapter.Start(ctx, config)
	if err == nil {
		return runtime, nil
	}
	if !ruleSetStartFailed(err) {
		return nil, err
	}
	stripped := dropLocalRuleSets(config)
	if bytes.Equal(stripped, config) {
		return nil, err
	}
	retried, retryErr := c.Adapter.Start(ctx, stripped)
	if retryErr != nil {
		return nil, err
	}
	return retried, nil
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

func dropLocalRuleSets(config []byte) []byte {
	var document map[string]any
	if err := json.Unmarshal(config, &document); err != nil {
		return config
	}
	route, _ := document["route"].(map[string]any)
	if route == nil {
		return config
	}
	rawSets, _ := route["rule_set"].([]any)
	changed := false
	for i, raw := range rawSets {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if kind, _ := item["type"].(string); kind != "local" {
			continue
		}
		tag, _ := item["tag"].(string)
		rawSets[i] = map[string]any{
			"tag":  tag,
			"type": "remote",
			"url":  "https://example.invalid/naga-ruleset-retry",
		}
		changed = true
	}
	if !changed {
		return config
	}
	route["rule_set"] = rawSets
	out, err := json.Marshal(document)
	if err != nil {
		return config
	}
	return out
}
