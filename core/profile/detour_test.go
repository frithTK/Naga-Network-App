package profile

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestOmitDanglingDetoursDropsHiddenShadowTLS(t *testing.T) {
	document := map[string]any{}
	if err := json.Unmarshal([]byte(`{
		"route":{"final":"Mode"},
		"outbounds":[
			{"type":"selector","tag":"Mode","outbounds":["Auto"],"default":"Auto"},
			{"type":"urltest","tag":"Auto","outbounds":["good","bad"],"default":"bad"},
			{"type":"shadowsocks","tag":"good","server":"1.1.1.1"},
			{"type":"shadowsocks","tag":"bad","detour":"bad_shadowtls-out §hide§"},
			{"type":"shadowsocks","tag":"wrap","detour":"bad"}
		]
	}`), &document); err != nil {
		t.Fatal(err)
	}
	OmitDanglingDetours(document)
	raw, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if strings.Contains(text, "§hide§") || strings.Contains(text, `"bad"`) || strings.Contains(text, `"wrap"`) {
		t.Fatalf("dangling detour chain was kept: %s", text)
	}
	if !strings.Contains(text, `"good"`) {
		t.Fatalf("healthy outbound was removed: %s", text)
	}
	outbounds := document["outbounds"].([]any)
	var auto map[string]any
	for _, item := range outbounds {
		outbound := item.(map[string]any)
		if outbound["tag"] == "Auto" {
			auto = outbound
		}
	}
	refs := auto["outbounds"].([]any)
	if len(refs) != 1 || refs[0] != "good" {
		t.Fatalf("urltest refs = %#v", refs)
	}
	if auto["default"] != "good" {
		t.Fatalf("default = %#v", auto["default"])
	}
}

func TestMergeDropsHiddenShadowTLSFromCandidates(t *testing.T) {
	merged, err := MergeProfilesForNetwork([]Profile{{
		ID:     "profile-a",
		Engine: EngineSingBox,
		Config: []byte(`{
			"route":{"final":"Mode"},
			"outbounds":[
				{"type":"selector","tag":"Mode","outbounds":["good","bad"],"default":"good"},
				{"type":"vless","tag":"good","server":"a.example"},
				{"type":"shadowsocks","tag":"bad","detour":"bad_shadowtls-out §hide§"},
				{"type":"direct","tag":"direct"}
			]
		}`),
	}}, "unknown")
	if err != nil {
		t.Fatal(err)
	}
	if len(merged.Candidates) != 1 || merged.Candidates[0].SourceTag != "good" {
		t.Fatalf("candidates = %#v", merged.Candidates)
	}
	if strings.Contains(string(merged.Config), "§hide§") || strings.Contains(string(merged.Config), "bad") {
		t.Fatalf("runtime config still contains the hidden detour: %s", merged.Config)
	}
}
