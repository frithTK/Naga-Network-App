package singbox

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"

	"naga.network/core/profile"
)

// NormalizeLinuxConfig adapts profiles produced for older sing-box versions
// to the Linux runtime format used by the current sing-box release. The
// stored/source profile is never changed; normalization only affects the
// temporary config written for a running process.
func NormalizeLinuxConfig(config []byte) ([]byte, error) {
	var document map[string]any
	decoder := json.NewDecoder(bytes.NewReader(config))
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("decode sing-box config: %w", err)
	}
	if document == nil {
		return nil, fmt.Errorf("sing-box config must be a JSON object")
	}
	collapseIdenticalOutbounds(document)
	profile.OmitDanglingDetours(document)

	var rules []any
	var haveRules bool
	if rawRoute, ok := document["route"]; ok {
		if route, ok := rawRoute.(map[string]any); ok {
			if rawRules, ok := route["rules"]; ok {
				if decodedRules, ok := rawRules.([]any); ok {
					rules = decodedRules
					haveRules = true
				}
			}
		}
	}

	routeChanged := false
	route, hasRoute := document["route"].(map[string]any)
	for _, rawInbound := range inbounds(document) {
		inbound, ok := rawInbound.(map[string]any)
		if !ok {
			continue
		}
		normalizeLinuxTUNInbound(inbound)
		tag, _ := inbound["tag"].(string)
		if tag == "" {
			continue
		}

		if _, ok := inbound["sniff"]; ok {
			if !hasRoute {
				route = map[string]any{}
				document["route"] = route
				hasRoute = true
			}
			if !haveRules {
				rules = []any{}
				haveRules = true
			}
			if !hasRuleAction(rules, tag, "sniff") {
				rules = append(rules, map[string]any{
					"inbound": []any{tag},
					"action":  "sniff",
				})
				routeChanged = true
			}
		}

		if strategy, ok := inbound["domain_strategy"].(string); ok && strategy != "" {
			if !hasRoute {
				route = map[string]any{}
				document["route"] = route
				hasRoute = true
			}
			if !haveRules {
				rules = []any{}
				haveRules = true
			}
			if !hasRuleAction(rules, tag, "resolve") {
				rules = append(rules, map[string]any{
					"inbound":  []any{tag},
					"action":   "resolve",
					"strategy": strategy,
				})
				routeChanged = true
			}
		}

		delete(inbound, "sniff")
		delete(inbound, "sniff_timeout")
		delete(inbound, "sniff_override_destination")
		delete(inbound, "domain_strategy")
		delete(inbound, "udp_disable_domain_unmapping")
	}

	if hasRoute {
		if routeChanged {
			route["rules"] = rules
		}
		// This is an Android-only compatibility option and is rejected by the
		// current Linux sing-box schema.
		delete(route, "override_android_vpn")
	}
	normalizeDNS(document, route)
	normalizeRemoteRuleSets(document)

	return json.MarshalIndent(document, "", "  ")
}

// collapseIdenticalOutbounds drops a later outbound when its tag and body
// already exist. The stored subscription stays untouched; sing-box rejects
// the duplicate tag even when both objects are the same node.
func collapseIdenticalOutbounds(document map[string]any) {
	raw, ok := document["outbounds"].([]any)
	if !ok {
		return
	}
	kept := make([]any, 0, len(raw))
	index := make(map[string]int, len(raw))
	for _, item := range raw {
		outbound, ok := item.(map[string]any)
		if !ok {
			kept = append(kept, item)
			continue
		}
		tag, _ := outbound["tag"].(string)
		if tag == "" {
			kept = append(kept, item)
			continue
		}
		if previous, exists := index[tag]; exists && reflect.DeepEqual(kept[previous], outbound) {
			continue
		}
		if _, exists := index[tag]; !exists {
			index[tag] = len(kept)
		}
		kept = append(kept, item)
	}
	for _, item := range kept {
		outbound, ok := item.(map[string]any)
		if !ok {
			continue
		}
		refs, ok := outbound["outbounds"].([]any)
		if !ok {
			continue
		}
		seen := make(map[string]struct{}, len(refs))
		next := make([]any, 0, len(refs))
		for _, ref := range refs {
			name, ok := ref.(string)
			if !ok {
				next = append(next, ref)
				continue
			}
			if _, dup := seen[name]; dup {
				continue
			}
			seen[name] = struct{}{}
			next = append(next, name)
		}
		outbound["outbounds"] = next
	}
	document["outbounds"] = kept
}

// Remote rule-sets that were not materialized to local files stay as type
// remote. The stock Go HTTPS stack in sing-box can block Clash API startup, so
// leftover remotes become an inline placeholder that never matches real
// traffic. The stored profile is unchanged; the tag still resolves. An empty
// inline set is rejected by sing-box, so the placeholder rule is required.
func normalizeRemoteRuleSets(document map[string]any) {
	route, ok := document["route"].(map[string]any)
	if !ok {
		return
	}
	rawSets, ok := route["rule_set"].([]any)
	if !ok {
		return
	}
	for i, raw := range rawSets {
		ruleSet, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if kind, _ := ruleSet["type"].(string); kind != "remote" {
			continue
		}
		tag, _ := ruleSet["tag"].(string)
		route["rule_set"].([]any)[i] = map[string]any{
			"tag":  tag,
			"type": "inline",
			"rules": []any{
				map[string]any{
					"domain_suffix": []any{".naga-placeholder.invalid"},
				},
			},
		}
	}
}

// normalizeLinuxTUNInbound keeps the imported profile portable while making
// the temporary Linux runtime deterministic. The system stack depends on the
// host kernel's TCP and policy-routing behavior; on distributions with strict
// reverse-path filtering it can accept UDP while silently losing TCP replies.
// gVisor provides the same userspace TCP/IP path used by the working desktop
// clients on these systems.
//
// strict_route is intentionally disabled while auto_redirect is unavailable:
// the stricter ip-rule set is not needed for Naga's explicit resolver setup
// and is more prone to conflicts with other TUN clients and virtual networks.
func normalizeLinuxTUNInbound(inbound map[string]any) {
	if inboundType, _ := inbound["type"].(string); inboundType != "tun" {
		return
	}
	inbound["stack"] = "gvisor"
	if autoRoute, _ := inbound["auto_route"].(bool); autoRoute {
		inbound["strict_route"] = false
	}
}

// Some Naga profiles were generated with the Clash-style selector name
// "Select", while the sing-box outbound is named by route.final (currently
// "Mode"). Keep the source profile intact and repair only an unresolved DNS
// detour for the temporary runtime config.
func normalizeDNSDetours(document map[string]any, route map[string]any) {
	rawDNS, ok := document["dns"].(map[string]any)
	if !ok {
		return
	}
	rawOutbounds, _ := document["outbounds"].([]any)
	outboundTags := make(map[string]struct{}, len(rawOutbounds))
	emptyDirect := make(map[string]struct{})
	for _, rawOutbound := range rawOutbounds {
		outbound, ok := rawOutbound.(map[string]any)
		if !ok {
			continue
		}
		tag, _ := outbound["tag"].(string)
		if tag == "" {
			continue
		}
		outboundTags[tag] = struct{}{}
		if typ, _ := outbound["type"].(string); typ == "direct" && directOutboundIsEmpty(outbound) {
			emptyDirect[tag] = struct{}{}
		}
	}
	final := ""
	if route != nil {
		final, _ = route["final"].(string)
	}
	rawServers, _ := rawDNS["servers"].([]any)
	for _, rawServer := range rawServers {
		server, ok := rawServer.(map[string]any)
		if !ok {
			continue
		}
		detour, _ := server["detour"].(string)
		if detour == "" {
			continue
		}
		if _, empty := emptyDirect[detour]; empty {
			// sing-box 1.13+: DNS already dials like an empty direct outbound.
			delete(server, "detour")
			continue
		}
		if _, exists := outboundTags[detour]; exists {
			continue
		}
		if final == "" {
			continue
		}
		if _, ok := outboundTags[final]; !ok {
			continue
		}
		if _, empty := emptyDirect[final]; empty {
			delete(server, "detour")
			continue
		}
		server["detour"] = final
	}
}

func directOutboundIsEmpty(outbound map[string]any) bool {
	for key := range outbound {
		switch key {
		case "type", "tag":
			continue
		default:
			return false
		}
	}
	return true
}

func inbounds(document map[string]any) []any {
	raw, _ := document["inbounds"].([]any)
	return raw
}

func hasRuleAction(rules []any, tag, action string) bool {
	for _, rawRule := range rules {
		rule, ok := rawRule.(map[string]any)
		if !ok {
			continue
		}
		if ruleAction, _ := rule["action"].(string); ruleAction != action {
			continue
		}
		inboundTags, ok := rule["inbound"].([]any)
		if !ok {
			continue
		}
		for _, rawTag := range inboundTags {
			if ruleTag, _ := rawTag.(string); ruleTag == tag {
				return true
			}
		}
	}
	return false
}
