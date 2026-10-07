package profile

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"naga.network/core/policy"
)

const (
	UnifiedSelectorTag = "Naga-Policy"
	UnifiedDirectTag   = "Naga-Direct"
)

type MergedProfile struct {
	Config              []byte
	Candidates          []NodeCandidate
	PreferredRuntimeTag string
}

// MergeProfilesForNetwork builds a single runtime configuration from sing-box
// profiles. It keeps the first profile's global settings, combines route rules
// and tagged outbounds from all profiles, and never mutates source JSON.
// network selects the initial default outbound; the runtime can later switch
// the selector without rebuilding the tunnel.
func MergeProfilesForNetwork(values []Profile, network string) (MergedProfile, error) {
	if len(values) == 0 {
		return MergedProfile{}, errors.New("no profiles to merge")
	}

	var base map[string]any
	mergedOutbounds := make([]any, 0)
	mergedRules := make([]any, 0)
	mergedRuleSets := make([]any, 0)
	candidates := make([]NodeCandidate, 0)
	seenCandidates := make(map[string]struct{})

	for index, value := range values {
		if value.Engine != "" && value.Engine != EngineSingBox {
			return MergedProfile{}, fmt.Errorf("profile %q is not a sing-box profile", value.ID)
		}
		if err := ValidateSingBoxConfig(value.Config); err != nil {
			return MergedProfile{}, fmt.Errorf("profile %q is invalid: %w", value.ID, err)
		}
		document, err := decodeConfig(value.Config)
		if err != nil {
			return MergedProfile{}, err
		}
		// A profile can omit the ShadowTLS transport and still leave a leaf
		// whose detour points at that missing tag. sing-box then refuses to
		// start the whole runtime, including every other node.
		OmitDanglingDetours(document)
		if index == 0 {
			base = cloneObject(document)
		}
		cleaned, err := json.Marshal(document)
		if err != nil {
			return MergedProfile{}, err
		}
		options, err := InspectMode(cleaned)
		if err != nil {
			return MergedProfile{}, fmt.Errorf("profile %q has no selectable nodes: %w", value.ID, err)
		}
		outbounds, err := outboundMap(document)
		if err != nil {
			return MergedProfile{}, fmt.Errorf("profile %q: %w", value.ID, err)
		}
		tagMap := make(map[string]string, len(outbounds))
		for tag := range outbounds {
			tagMap[tag] = runtimeTag(value.ID, tag)
		}
		if index == 0 {
			rewriteOutboundTagReferences(base, tagMap)
		}

		for tag, outbound := range outbounds {
			copy := cloneObject(outbound)
			copy["tag"] = tagMap[tag]
			rewriteOutboundTagReferences(copy, tagMap)
			mergedOutbounds = append(mergedOutbounds, copy)
		}

		for _, node := range options.Nodes {
			collectCandidates(value.ID, node, tagMap, outbounds, &candidates, seenCandidates)
		}

		if route, ok := document["route"].(map[string]any); ok {
			ruleSetMap := make(map[string]string)
			if ruleSets, ok := route["rule_set"].([]any); ok {
				for _, ruleSet := range ruleSets {
					copy := cloneValue(ruleSet)
					if item, ok := copy.(map[string]any); ok {
						if tag, ok := item["tag"].(string); ok && tag != "" {
							ruleSetMap[tag] = runtimeTag(value.ID, "ruleset::"+tag)
						}
					}
					rewriteOutboundTagReferences(copy, tagMap)
					rewriteRuleSetTag(copy, value.ID)
					mergedRuleSets = append(mergedRuleSets, copy)
				}
			}
			if index == 0 {
				rewriteRuleSetTagReferences(base, ruleSetMap)
			}
			if rules, ok := route["rules"].([]any); ok {
				for _, rule := range rules {
					copy := cloneValue(rule)
					rewriteOutboundTagReferences(copy, tagMap)
					rewriteRuleSetTagReferences(copy, ruleSetMap)
					mergedRules = append(mergedRules, copy)
				}
			}
		}
	}

	if len(candidates) == 0 {
		return MergedProfile{}, errors.New("profiles contain no proxy leaf nodes")
	}

	mergedOutbounds = append(mergedOutbounds, map[string]any{
		"type": "direct",
		"tag":  UnifiedDirectTag,
	})
	ordered := policy.RankCandidates(policy.NetworkClass(network), candidates)
	preferredRuntimeTag := preferredRuntimeTag(values, candidates)
	candidateTags := make([]any, 0, len(ordered))
	for _, candidate := range ordered {
		candidateTags = append(candidateTags, candidate.RuntimeTag)
	}
	mergedOutbounds = append(mergedOutbounds, map[string]any{
		"type":      "selector",
		"tag":       UnifiedSelectorTag,
		"outbounds": candidateTags,
		"default":   ordered[0].RuntimeTag,
	})
	if preferredRuntimeTag != "" {
		selector := mergedOutbounds[len(mergedOutbounds)-1].(map[string]any)
		selector["default"] = preferredRuntimeTag
	}

	base["outbounds"] = mergedOutbounds
	route, _ := base["route"].(map[string]any)
	if route == nil {
		route = map[string]any{}
		base["route"] = route
	}
	route["final"] = UnifiedSelectorTag
	if len(mergedRules) > 0 {
		route["rules"] = mergedRules
	}
	if len(mergedRuleSets) > 0 {
		route["rule_set"] = mergedRuleSets
	}

	config, err := json.MarshalIndent(base, "", "  ")
	if err != nil {
		return MergedProfile{}, fmt.Errorf("encode merged profile: %w", err)
	}
	if err := ValidateSingBoxConfig(config); err != nil {
		return MergedProfile{}, fmt.Errorf("merged profile is invalid: %w", err)
	}
	return MergedProfile{
		Config:              config,
		Candidates:          candidates,
		PreferredRuntimeTag: preferredRuntimeTag,
	}, nil
}

// preferredRuntimeTag maps a persisted source-level selection to the
// namespaced leaf outbound used by the merged runtime. A group selection is
// resolved to its first reachable leaf, while an empty selection keeps the
// policy-ranked default.
func preferredRuntimeTag(values []Profile, candidates []NodeCandidate) string {
	for _, value := range values {
		selectedMode := strings.TrimSpace(value.SelectedMode)
		// Auto is a policy mode, not a persisted leaf/group preference. The
		// source profile often implements it as urltest, which may select TUIC
		// solely because its measured latency is lower.
		if selectedMode == "" || strings.EqualFold(selectedMode, "auto") {
			continue
		}
		options, err := InspectMode(value.Config)
		if err != nil {
			continue
		}
		tags := make(map[string]struct{})
		for _, node := range options.Nodes {
			collectSelectedLeafTags(node, selectedMode, false, tags)
		}
		for _, candidate := range candidates {
			if candidate.ProfileID == value.ID {
				if _, ok := tags[candidate.SourceTag]; ok {
					return candidate.RuntimeTag
				}
			}
		}
	}
	return ""
}

func collectSelectedLeafTags(node Node, target string, selected bool, result map[string]struct{}) {
	selected = selected || node.Tag == target
	if selected && len(node.Children) == 0 && isProxyLeaf(node.Type) {
		result[node.Tag] = struct{}{}
		return
	}
	for _, child := range node.Children {
		collectSelectedLeafTags(child, target, selected, result)
	}
}

func outboundMap(document map[string]any) (map[string]map[string]any, error) {
	raw, ok := document["outbounds"].([]any)
	if !ok || len(raw) == 0 {
		return nil, errors.New("sing-box config must contain outbounds")
	}
	result := make(map[string]map[string]any, len(raw))
	for _, value := range raw {
		outbound, ok := value.(map[string]any)
		if !ok {
			continue
		}
		tag, _ := outbound["tag"].(string)
		if tag == "" {
			continue
		}
		result[tag] = outbound
	}
	return result, nil
}

func collectCandidates(profileID string, node Node, tagMap map[string]string, outbounds map[string]map[string]any, result *[]NodeCandidate, seen map[string]struct{}) {
	if isProxyLeaf(node.Type) {
		outbound := outbounds[node.Tag]
		fingerprintValue := cloneObject(outbound)
		delete(fingerprintValue, "tag")
		encoded, _ := json.Marshal(fingerprintValue)
		// The source tag is part of the selectable identity. Two different
		// nodes can legitimately have identical transport settings, and
		// collapsing them would make a persisted selection unreachable.
		identity := []byte(profileID + "\x00" + node.Tag + "\x00")
		digest := sha256.Sum256(append(identity, encoded...))
		fingerprint := hex.EncodeToString(digest[:])
		if _, exists := seen[fingerprint]; !exists {
			seen[fingerprint] = struct{}{}
			*result = append(*result, NodeCandidate{
				ID:         "node-" + fingerprint[:16],
				ProfileID:  profileID,
				SourceTag:  node.Tag,
				RuntimeTag: tagMap[node.Tag],
				Country:    node.Country,
				Protocol:   node.Protocol,
				Type:       node.Type,
			})
		}
	}
	for _, child := range node.Children {
		collectCandidates(profileID, child, tagMap, outbounds, result, seen)
	}
}

func isProxyLeaf(outboundType string) bool {
	switch strings.ToLower(strings.TrimSpace(outboundType)) {
	case "", "selector", "urltest", "direct", "block", "dns":
		return false
	default:
		return true
	}
}

func runtimeTag(profileID, sourceTag string) string {
	id := strings.TrimSpace(profileID)
	if id == "" {
		id = "profile"
	}
	return id + "::" + sourceTag
}

// IdentityLeafCandidates returns proxy leaves of a single sing-box profile
// using source tags as runtime tags (no merge namespacing).
func IdentityLeafCandidates(value Profile) []NodeCandidate {
	options, err := InspectMode(value.Config)
	if err != nil {
		return nil
	}
	document, err := decodeConfig(value.Config)
	if err != nil {
		return nil
	}
	outbounds, err := outboundMap(document)
	if err != nil {
		return nil
	}
	tagMap := make(map[string]string)
	for tag := range outbounds {
		tagMap[tag] = tag
	}
	candidates := make([]NodeCandidate, 0)
	seen := make(map[string]struct{})
	for _, node := range options.Nodes {
		collectCandidates(value.ID, node, tagMap, outbounds, &candidates, seen)
	}
	return candidates
}

// rewriteOutboundTagReferences rewrites only fields which the sing-box schema
// defines as outbound-tag references. Rewriting every matching string is not
// safe: a tag such as "direct" can also be a protocol type, server name, or
// ordinary rule value.
func rewriteOutboundTagReferences(value any, tagMap map[string]string) {
	switch current := value.(type) {
	case map[string]any:
		for key, item := range current {
			if isOutboundTagReference(current, key) {
				current[key] = rewriteReferenceValue(item, tagMap)
				item = current[key]
			}
			rewriteOutboundTagReferences(item, tagMap)
		}
	case []any:
		for _, item := range current {
			rewriteOutboundTagReferences(item, tagMap)
		}
	}
}

func isOutboundTagReference(container map[string]any, key string) bool {
	switch key {
	case "outbound", "outbounds", "detour", "download_detour":
		return true
	case "default":
		outboundType, _ := container["type"].(string)
		return strings.EqualFold(outboundType, "selector")
	default:
		return false
	}
}

func rewriteRuleSetTagReferences(value any, tagMap map[string]string) {
	switch current := value.(type) {
	case map[string]any:
		for key, item := range current {
			if key == "rule_set" {
				current[key] = rewriteReferenceValue(item, tagMap)
				item = current[key]
			}
			rewriteRuleSetTagReferences(item, tagMap)
		}
	case []any:
		for _, item := range current {
			rewriteRuleSetTagReferences(item, tagMap)
		}
	}
}

func rewriteReferenceValue(value any, tagMap map[string]string) any {
	switch current := value.(type) {
	case string:
		if replacement, exists := tagMap[current]; exists {
			return replacement
		}
	case []any:
		for index, item := range current {
			tag, ok := item.(string)
			if !ok {
				continue
			}
			if replacement, exists := tagMap[tag]; exists {
				current[index] = replacement
			}
		}
	}
	return value
}

func rewriteRuleSetTag(value any, profileID string) {
	if item, ok := value.(map[string]any); ok {
		if tag, ok := item["tag"].(string); ok && tag != "" {
			item["tag"] = runtimeTag(profileID, "ruleset::"+tag)
		}
	}
}

func cloneObject(value map[string]any) map[string]any {
	result := make(map[string]any, len(value))
	for key, item := range value {
		result[key] = cloneValue(item)
	}
	return result
}

func cloneValue(value any) any {
	switch current := value.(type) {
	case map[string]any:
		return cloneObject(current)
	case []any:
		result := make([]any, len(current))
		for index, item := range current {
			result[index] = cloneValue(item)
		}
		return result
	default:
		return current
	}
}
