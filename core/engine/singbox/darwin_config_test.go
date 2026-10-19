package singbox

import (
	"encoding/json"
	"testing"
)

func TestDarwinTUNConfigPreservesRoutingAndRemovesUserFiles(t *testing.T) {
	source := []byte(`{
		"inbounds":[{"type":"tun","tag":"tun-in","interface_name":"naga-tun0",
			"auto_route":true,"strict_route":true,"auto_redirect":true,
			"iproute2_table_index":100,"inet4_address":["10.0.0.1/30"]}],
		"outbounds":[{"type":"direct","tag":"direct"}],
		"route":{"rules":[{"port":53,"action":"hijack-dns"}],"final":"direct"},
		"experimental":{"cache_file":{"enabled":true,"path":"/user/cache.db"}},
		"log":{"output":"/user/log.txt"}
	}`)
	prepared, err := prepareDarwinConfig(source)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(prepared, &doc); err != nil {
		t.Fatal(err)
	}
	inbound := inbounds(doc)[0].(map[string]any)
	if inbound["auto_route"] != false || inbound["interface_name"] != "utun123" || inbound["stack"] != "gvisor" {
		t.Fatalf("incorrect Darwin TUN: %v", inbound)
	}
	for _, key := range []string{"strict_route", "auto_redirect", "iproute2_table_index", "inet4_address"} {
		if _, exists := inbound[key]; exists {
			t.Fatalf("Linux setting survived adaptation: %s", key)
		}
	}
	if doc["log"] != nil || doc["experimental"] != nil {
		t.Fatal("user-controlled log/cache paths crossed into root config")
	}
	route := doc["route"].(map[string]any)
	if route["final"] != "direct" || route["rules"].([]any)[0].(map[string]any)["action"] != "hijack-dns" {
		t.Fatal("Darwin adaptation lost routing/DNS policy")
	}
	// API settings must be generated after dropping the user's experimental block.
	final, api, err := prepareRuntimeConfig(prepared)
	if err != nil || api == nil {
		t.Fatalf("prepare runtime API: %v", err)
	}
	if err := json.Unmarshal(final, &doc); err != nil {
		t.Fatal(err)
	}
	apiConfig := doc["experimental"].(map[string]any)["clash_api"].(map[string]any)
	if len(apiConfig["secret"].(string)) != 32 {
		t.Fatal("runtime API has no session secret")
	}
}

func TestDarwinAdaptationDoesNotChangeUserModeProbes(t *testing.T) {
	source := []byte(`{"outbounds":[{"type":"direct"}],"experimental":{"cache_file":{"enabled":true}},"log":{"level":"debug"}}`)
	prepared, err := prepareDarwinConfig(source)
	if err != nil || string(prepared) != string(source) {
		t.Fatalf("user-mode config changed: %s, %v", prepared, err)
	}
	if _, err := prepareDarwinConfig([]byte("null")); err == nil {
		t.Fatal("null config accepted")
	}
}
