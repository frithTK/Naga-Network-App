package profile

import (
	"encoding/json"
	"naga.network/core/policy"
	"testing"
)

func TestSystemProxySurvivesExistingRuntimeInbound(t *testing.T) {
	input := []byte(`{"inbounds":[{"type":"mixed","tag":"naga-system-proxy","listen":"127.0.0.1","listen_port":2080}],"outbounds":[{"type":"direct","tag":"direct"}]}`)
	result, err := ApplyTrafficMode(input, policy.TrafficSystemProxy)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err = json.Unmarshal(result, &doc); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, raw := range doc["inbounds"].([]any) {
		value := raw.(map[string]any)
		if value["type"] == "mixed" && value["listen"] == "127.0.0.1" && value["listen_port"] == float64(2080) {
			found = true
		}
	}
	if !found {
		t.Fatalf("system settings will point at 2080, but runtime has no proxy listener: %s", result)
	}
}
