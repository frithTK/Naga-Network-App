package profile

import (
	"encoding/json"
	"strings"
	"testing"

	"naga.network/core/policy"
)

func TestMergeProfilesNamespacesOutboundsAndCreatesUnifiedSelector(t *testing.T) {
	values := []Profile{
		{
			ID:     "profile-a",
			Engine: EngineSingBox,
			Config: []byte(`{"route":{"final":"Mode","rules":[{"domain_suffix":["ru"],"outbound":"direct"}]},"outbounds":[{"type":"selector","tag":"Mode","outbounds":["a"],"default":"a"},{"type":"vless","tag":"a","server":"a.example","server_port":443},{"type":"direct","tag":"direct"}]}`),
		},
		{
			ID:     "profile-b",
			Engine: EngineSingBox,
			Config: []byte(`{"outbounds":[{"type":"selector","tag":"Mode","outbounds":["b"],"default":"b"},{"type":"tuic","tag":"b","server":"b.example","server_port":443},{"type":"direct","tag":"direct"}]}`),
		},
	}

	merged, err := MergeProfilesForNetwork(values, "unknown")
	if err != nil {
		t.Fatal(err)
	}
	if len(merged.Candidates) != 2 {
		t.Fatalf("candidates = %#v", merged.Candidates)
	}
	if !strings.Contains(string(merged.Config), UnifiedSelectorTag) {
		t.Fatalf("unified selector missing: %s", merged.Config)
	}
	if !strings.Contains(string(merged.Config), "profile-a::Mode") {
		t.Fatalf("source selector was not namespaced: %s", merged.Config)
	}
	var document map[string]any
	if err := json.Unmarshal(merged.Config, &document); err != nil {
		t.Fatal(err)
	}
	route := document["route"].(map[string]any)
	if route["final"] != UnifiedSelectorTag {
		t.Fatalf("route.final = %#v", route["final"])
	}
}

func TestMergeProfilesRewritesOnlyTypedTagReferences(t *testing.T) {
	merged, err := MergeProfilesForNetwork([]Profile{
		{
			ID:     "profile-safe",
			Engine: EngineSingBox,
			Config: []byte(`{
				"dns":{"servers":[{"tag":"dns-direct","address":"local","detour":"direct"}],"rules":[{"domain_suffix":["rules"],"rule_set":["direct"]}]},
				"route":{
					"final":"Mode",
					"rule_set":[{"type":"remote","tag":"direct","format":"binary","url":"https://direct/rules.srs","download_detour":"direct"}],
					"rules":[{"domain_suffix":["direct"],"rule_set":["direct"],"outbound":"direct"}]
				},
				"outbounds":[
					{"type":"selector","tag":"Mode","outbounds":["node","direct"],"default":"direct"},
					{"type":"vless","tag":"node","server":"direct","detour":"direct"},
					{"type":"direct","tag":"direct"}
				]
			}`),
		},
	}, "unknown")
	if err != nil {
		t.Fatal(err)
	}

	var document map[string]any
	if err := json.Unmarshal(merged.Config, &document); err != nil {
		t.Fatal(err)
	}
	outbounds := document["outbounds"].([]any)
	byTag := make(map[string]map[string]any, len(outbounds))
	for _, raw := range outbounds {
		outbound := raw.(map[string]any)
		byTag[outbound["tag"].(string)] = outbound
		if outboundType, _ := outbound["type"].(string); strings.Contains(outboundType, "::") {
			t.Fatalf("protocol type was rewritten as a tag: %#v", outbound)
		}
	}

	direct := byTag["profile-safe::direct"]
	if direct["type"] != "direct" {
		t.Fatalf("direct.type = %#v", direct["type"])
	}
	node := byTag["profile-safe::node"]
	if node["server"] != "direct" {
		t.Fatalf("node.server = %#v", node["server"])
	}
	if node["detour"] != "profile-safe::direct" {
		t.Fatalf("node.detour = %#v", node["detour"])
	}
	selector := byTag["profile-safe::Mode"]
	if selector["default"] != "profile-safe::direct" {
		t.Fatalf("selector.default = %#v", selector["default"])
	}
	selectorTags := selector["outbounds"].([]any)
	if selectorTags[0] != "profile-safe::node" || selectorTags[1] != "profile-safe::direct" {
		t.Fatalf("selector.outbounds = %#v", selectorTags)
	}

	route := document["route"].(map[string]any)
	rule := route["rules"].([]any)[0].(map[string]any)
	if rule["outbound"] != "profile-safe::direct" {
		t.Fatalf("route rule outbound = %#v", rule["outbound"])
	}
	if rule["domain_suffix"].([]any)[0] != "direct" {
		t.Fatalf("ordinary rule value was rewritten: %#v", rule["domain_suffix"])
	}
	if rule["rule_set"].([]any)[0] != "profile-safe::ruleset::direct" {
		t.Fatalf("route rule_set = %#v", rule["rule_set"])
	}
	ruleSet := route["rule_set"].([]any)[0].(map[string]any)
	if ruleSet["download_detour"] != "profile-safe::direct" {
		t.Fatalf("rule-set download_detour = %#v", ruleSet["download_detour"])
	}
	if ruleSet["url"] != "https://direct/rules.srs" {
		t.Fatalf("rule-set URL was rewritten: %#v", ruleSet["url"])
	}

	dns := document["dns"].(map[string]any)
	dnsServer := dns["servers"].([]any)[0].(map[string]any)
	if dnsServer["detour"] != "profile-safe::direct" {
		t.Fatalf("dns detour = %#v", dnsServer["detour"])
	}
	dnsRule := dns["rules"].([]any)[0].(map[string]any)
	if dnsRule["rule_set"].([]any)[0] != "profile-safe::ruleset::direct" {
		t.Fatalf("dns rule_set = %#v", dnsRule["rule_set"])
	}
}

func TestMergeProfilesRejectsNonSingBox(t *testing.T) {
	_, err := MergeProfilesForNetwork([]Profile{{ID: "mihomo", Engine: EngineMihomo, Config: []byte("{}")}}, "unknown")
	if err == nil {
		t.Fatal("expected non-sing-box profile to be rejected")
	}
}

func TestMergeProfilesPlacesTUICAfterNonTUICByDefault(t *testing.T) {
	merged, err := MergeProfilesForNetwork([]Profile{{
		ID:     "profile-a",
		Engine: EngineSingBox,
		Config: []byte(`{"route":{"final":"Mode"},"outbounds":[{"type":"selector","tag":"Mode","outbounds":["tuic","vless"],"default":"tuic"},{"type":"tuic","tag":"tuic"},{"type":"vless","tag":"vless"}]}`),
	}}, "unknown")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(merged.Config), "profile-a::vless") {
		t.Fatalf("non-TUIC outbound was not preferred: %s", merged.Config)
	}
}

func TestMergeProfilesUsesCellularVLESSPriority(t *testing.T) {
	merged, err := MergeProfilesForNetwork([]Profile{{
		ID:     "profile-cellular",
		Engine: EngineSingBox,
		Config: []byte(`{"route":{"final":"Mode"},"outbounds":[{"type":"selector","tag":"Mode","outbounds":["tuic","vless"]},{"type":"tuic","tag":"tuic"},{"type":"vless","tag":"vless"}]}`),
	}}, string(policy.NetworkCellular))
	if err != nil {
		t.Fatal(err)
	}
	if merged.PreferredRuntimeTag != "" {
		t.Fatalf("unexpected persisted preference: %q", merged.PreferredRuntimeTag)
	}
	var document map[string]any
	if err := json.Unmarshal(merged.Config, &document); err != nil {
		t.Fatal(err)
	}
	outbounds := document["outbounds"].([]any)
	selector := outbounds[len(outbounds)-1].(map[string]any)
	if selector["default"] != "profile-cellular::vless" {
		t.Fatalf("cellular default = %#v", selector["default"])
	}
}

func TestMergeProfilesUsesPersistedLeafPreference(t *testing.T) {
	merged, err := MergeProfilesForNetwork([]Profile{{
		ID:           "profile-preference",
		Engine:       EngineSingBox,
		SelectedMode: "slow",
		Config:       []byte(`{"route":{"final":"Mode"},"outbounds":[{"type":"selector","tag":"Mode","outbounds":["fast","slow"]},{"type":"vless","tag":"fast"},{"type":"vless","tag":"slow"}]}`),
	}}, string(policy.NetworkWiFi))
	if err != nil {
		t.Fatal(err)
	}
	if merged.PreferredRuntimeTag != "profile-preference::slow" {
		t.Fatalf("preferred runtime tag = %q", merged.PreferredRuntimeTag)
	}
	var document map[string]any
	if err := json.Unmarshal(merged.Config, &document); err != nil {
		t.Fatal(err)
	}
	outbounds := document["outbounds"].([]any)
	selector := outbounds[len(outbounds)-1].(map[string]any)
	if selector["default"] != "profile-preference::slow" {
		t.Fatalf("selected default = %#v", selector["default"])
	}
}

func TestMergeProfilesIgnoresAutoSelectionForPolicyDefault(t *testing.T) {
	merged, err := MergeProfilesForNetwork([]Profile{{
		ID:           "profile-auto",
		Engine:       EngineSingBox,
		SelectedMode: "Auto",
		Config:       []byte(`{"route":{"final":"Mode"},"outbounds":[{"type":"selector","tag":"Mode","outbounds":["Auto"]},{"type":"urltest","tag":"Auto","outbounds":["tuic","vless"]},{"type":"tuic","tag":"tuic"},{"type":"vless","tag":"vless"}]}`),
	}}, string(policy.NetworkWiFi))
	if err != nil {
		t.Fatal(err)
	}
	if merged.PreferredRuntimeTag != "" {
		t.Fatalf("automatic mode became a persisted preference: %q", merged.PreferredRuntimeTag)
	}
	var document map[string]any
	if err := json.Unmarshal(merged.Config, &document); err != nil {
		t.Fatal(err)
	}
	outbounds := document["outbounds"].([]any)
	selector := outbounds[len(outbounds)-1].(map[string]any)
	if selector["default"] != "profile-auto::vless" {
		t.Fatalf("automatic default = %#v, want non-TUIC vless", selector["default"])
	}
}
