package profile

import "encoding/json"

// PrepareLatencyProbeConfig keeps outbound/DNS dependencies but removes
// traffic interception and routing lists. Clash URL tests select an outbound
// explicitly, so provider rule downloads are unnecessary for these checks.
func PrepareLatencyProbeConfig(config []byte) ([]byte, error) {
	var root map[string]any
	if err := json.Unmarshal(config, &root); err != nil {
		return nil, err
	}
	delete(root, "inbounds")
	if outbounds, ok := root["outbounds"].([]any); ok {
		for _, raw := range outbounds {
			outbound, ok := raw.(map[string]any)
			if !ok || outbound["type"] != "urltest" {
				continue
			}
			outbound["type"] = "selector"
			delete(outbound, "url")
			delete(outbound, "interval")
			delete(outbound, "tolerance")
			delete(outbound, "idle_timeout")
		}
	}
	needed := map[string]bool{}
	if dns, ok := root["dns"].(map[string]any); ok {
		probeRuleSetReferences(dns["rules"], needed)
	}
	if route, ok := root["route"].(map[string]any); ok {
		delete(route, "rules")
		sets, _ := route["rule_set"].([]any)
		kept := make([]any, 0, len(sets))
		for _, raw := range sets {
			if set, ok := raw.(map[string]any); ok {
				tag, _ := set["tag"].(string)
				if needed[tag] {
					kept = append(kept, set)
				}
			}
		}
		delete(route, "rule_set")
		if len(kept) > 0 {
			route["rule_set"] = kept
		}
	}
	return json.Marshal(root)
}

// DNS decisions may be needed to resolve the proxy endpoint itself. Keep their
// transitive rule-set references even though traffic routing is disabled.
func probeRuleSetReferences(value any, needed map[string]bool) {
	switch item := value.(type) {
	case map[string]any:
		for key, child := range item {
			if key == "rule_set" {
				switch tags := child.(type) {
				case string:
					needed[tags] = true
				case []any:
					for _, tag := range tags {
						if name, ok := tag.(string); ok {
							needed[name] = true
						}
					}
				}
			} else {
				probeRuleSetReferences(child, needed)
			}
		}
	case []any:
		for _, child := range item {
			probeRuleSetReferences(child, needed)
		}
	}
}
