package control

import (
	"encoding/json"
	"fmt"

	"naga.network/core/engine"
)

// RuntimeInspect is a secret-free summary of the runtime config that was
// handed to Adapter.Start. It does not reflect sing-box NormalizeLinuxConfig
// (DNS detour 1.14) or clash_api injected inside the engine.
type RuntimeInspect struct {
	Status     string             `json:"status"`
	Final      string             `json:"final,omitempty"`
	Outbounds  []inspectOutbound  `json:"outbounds,omitempty"`
	Rules      []inspectRule      `json:"rules,omitempty"`
	DNSDetours []inspectDNSDetour `json:"dns_detours,omitempty"`
}

type inspectOutbound struct {
	Type string `json:"type,omitempty"`
	Tag  string `json:"tag,omitempty"`
}

type inspectRule struct {
	Outbound string   `json:"outbound,omitempty"`
	RuleSet  []string `json:"rule_set,omitempty"`
}

type inspectDNSDetour struct {
	Tag    string `json:"tag,omitempty"`
	Detour string `json:"detour,omitempty"`
}

func inspectRuntimeConfig(config []byte) (RuntimeInspect, error) {
	var document map[string]any
	if err := json.Unmarshal(config, &document); err != nil {
		return RuntimeInspect{}, fmt.Errorf("decode runtime inspect: %w", err)
	}
	inspect := RuntimeInspect{}
	if route, ok := document["route"].(map[string]any); ok {
		inspect.Final, _ = route["final"].(string)
		if rawRules, ok := route["rules"].([]any); ok {
			for _, raw := range rawRules {
				rule, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				item := inspectRule{}
				item.Outbound, _ = rule["outbound"].(string)
				item.RuleSet = ruleSetTags(rule["rule_set"])
				if item.Outbound == "" && len(item.RuleSet) == 0 {
					continue
				}
				inspect.Rules = append(inspect.Rules, item)
			}
		}
	}
	if rawOutbounds, ok := document["outbounds"].([]any); ok {
		for _, raw := range rawOutbounds {
			outbound, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			item := inspectOutbound{}
			item.Type, _ = outbound["type"].(string)
			item.Tag, _ = outbound["tag"].(string)
			if item.Type == "" && item.Tag == "" {
				continue
			}
			inspect.Outbounds = append(inspect.Outbounds, item)
		}
	}
	if rawDNS, ok := document["dns"].(map[string]any); ok {
		if rawServers, ok := rawDNS["servers"].([]any); ok {
			for _, raw := range rawServers {
				server, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				item := inspectDNSDetour{}
				item.Tag, _ = server["tag"].(string)
				item.Detour, _ = server["detour"].(string)
				if item.Tag == "" && item.Detour == "" {
					continue
				}
				inspect.DNSDetours = append(inspect.DNSDetours, item)
			}
		}
	}
	return inspect, nil
}

func ruleSetTags(raw any) []string {
	switch value := raw.(type) {
	case string:
		if value == "" {
			return nil
		}
		return []string{value}
	case []any:
		tags := make([]string, 0, len(value))
		for _, item := range value {
			switch entry := item.(type) {
			case string:
				if entry != "" {
					tags = append(tags, entry)
				}
			case map[string]any:
				if tag, _ := entry["tag"].(string); tag != "" {
					tags = append(tags, tag)
				}
			}
		}
		if len(tags) == 0 {
			return nil
		}
		return tags
	default:
		return nil
	}
}

func (c *RuntimeController) Inspect() RuntimeInspect {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.inspectLocked()
}

func (c *RuntimeController) inspectLocked() RuntimeInspect {
	status := inspectStatusLocked(c)
	if c.inspect == nil {
		return RuntimeInspect{Status: status}
	}
	out := *c.inspect
	out.Status = status
	return out
}

func inspectStatusLocked(c *RuntimeController) string {
	if c.runtime == nil {
		return string(engine.Stopped)
	}
	if c.startingOverlay {
		return string(engine.Starting)
	}
	return string(c.runtime.Status())
}
