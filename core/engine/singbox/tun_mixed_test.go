package singbox

import (
	"encoding/json"
	"testing"
)

func TestStripTUNInboundsEnsureMixed(t *testing.T) {
	input := []byte(`{
		"inbounds": [
			{"type":"tun","tag":"tun-in","address":["172.19.0.1/30"],"auto_route":true},
			{"type":"mixed","tag":"naga-system-proxy","listen":"127.0.0.1","listen_port":2080}
		],
		"outbounds": [{"type":"direct","tag":"direct"}]
	}`)
	got, err := stripTUNInboundsEnsureMixed(input)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(got, &document); err != nil {
		t.Fatal(err)
	}
	raw, _ := document["inbounds"].([]any)
	if len(raw) != 1 {
		t.Fatalf("inbounds = %#v", raw)
	}
	inbound := raw[0].(map[string]any)
	if inbound["type"] != "mixed" || inbound["tag"] != "naga-system-proxy" {
		t.Fatalf("mixed inbound = %#v", inbound)
	}
	if _, ok := inbound["auto_route"]; ok {
		t.Fatal("tun auto_route leaked into mixed inbound")
	}
}

func TestStripTUNInboundsAddsMixedWhenMissing(t *testing.T) {
	input := []byte(`{"inbounds":[{"type":"tun","tag":"tun-in"}],"outbounds":[{"type":"direct","tag":"direct"}]}`)
	got, err := stripTUNInboundsEnsureMixed(input)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(got, &document); err != nil {
		t.Fatal(err)
	}
	raw := document["inbounds"].([]any)
	if len(raw) != 1 {
		t.Fatalf("inbounds = %#v", raw)
	}
	if raw[0].(map[string]any)["type"] != "mixed" {
		t.Fatalf("expected mixed, got %#v", raw[0])
	}
}

func TestStripTUNInboundsInjectsAndroidDNSWhenMissing(t *testing.T) {
	input := []byte(`{
		"outbounds": [
			{"type":"selector","tag":"Mode","outbounds":["ee"],"default":"ee"},
			{"type":"vless","tag":"ee","server":"sub.example.test"}
		],
		"route": {"final":"Mode"}
	}`)
	got, err := stripTUNInboundsEnsureMixed(input)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(got, &document); err != nil {
		t.Fatal(err)
	}
	dns, _ := document["dns"].(map[string]any)
	if dns["final"] != "naga-dns" || dns["strategy"] != "prefer_ipv4" {
		t.Fatalf("dns = %#v", dns)
	}
	servers, _ := dns["servers"].([]any)
	if len(servers) != 2 {
		t.Fatalf("dns.servers = %#v", servers)
	}
	first := servers[0].(map[string]any)
	if first["type"] != "udp" || first["server"] != "1.1.1.1" {
		t.Fatalf("dns server = %#v", first)
	}
	if _, ok := first["detour"]; ok {
		t.Fatalf("empty direct detour leaked: %#v", first)
	}
	route, _ := document["route"].(map[string]any)
	if route["default_domain_resolver"] != "naga-dns" {
		t.Fatalf("route = %#v", route)
	}
	if route["auto_detect_interface"] != false {
		t.Fatalf("auto_detect_interface = %#v", route["auto_detect_interface"])
	}
	for _, raw := range document["outbounds"].([]any) {
		outbound := raw.(map[string]any)
		if outbound["type"] == "direct" {
			t.Fatalf("dummy direct outbound = %#v", outbound)
		}
	}
}

func TestStripTUNInboundsRemovesEmptyDirectDNSDetour(t *testing.T) {
	input := []byte(`{
		"dns": {"servers":[{"type":"udp","tag":"doh","server":"1.1.1.1","detour":"direct"}],"final":"doh"},
		"outbounds": [{"type":"direct","tag":"direct"}],
		"route": {"final":"direct","default_domain_resolver":"doh"}
	}`)
	got, err := stripTUNInboundsEnsureMixed(input)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(got, &document); err != nil {
		t.Fatal(err)
	}
	server := document["dns"].(map[string]any)["servers"].([]any)[0].(map[string]any)
	if _, ok := server["detour"]; ok {
		t.Fatalf("detour kept: %#v", server)
	}
}

func TestStripTUNInboundsDropsAndroidProcessRules(t *testing.T) {
	input := []byte(`{
		"outbounds": [{"type":"direct","tag":"direct"},{"type":"vless","tag":"ee"}],
		"route": {
			"final":"ee",
			"find_process": true,
			"rules": [
				{"process_name":["chrome"],"outbound":"direct"},
				{"domain_suffix":[".ru"],"outbound":"direct"},
				{"network_type":["wifi"],"process_name":["firefox"],"outbound":"direct"}
			]
		}
	}`)
	got, err := stripTUNInboundsEnsureMixed(input)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(got, &document); err != nil {
		t.Fatal(err)
	}
	route := document["route"].(map[string]any)
	if route["find_process"] != false {
		t.Fatalf("find_process = %#v", route["find_process"])
	}
	rules, _ := route["rules"].([]any)
	if len(rules) != 1 {
		t.Fatalf("rules = %#v", rules)
	}
	rule := rules[0].(map[string]any)
	if _, ok := rule["process_name"]; ok {
		t.Fatalf("process_name kept: %#v", rule)
	}
	suffix, _ := rule["domain_suffix"].([]any)
	if len(suffix) != 1 || suffix[0] != ".ru" {
		t.Fatalf("kept rule = %#v", rule)
	}
}

func TestStripTUNInboundsKeepsExistingRemoteDNS(t *testing.T) {
	input := []byte(`{
		"dns": {"servers":[{"type":"https","tag":"doh","server":"dns.google"}],"final":"doh"},
		"outbounds": [{"type":"direct","tag":"direct"}],
		"route": {"final":"direct","default_domain_resolver":"doh"}
	}`)
	got, err := stripTUNInboundsEnsureMixed(input)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(got, &document); err != nil {
		t.Fatal(err)
	}
	dns := document["dns"].(map[string]any)
	if dns["final"] != "doh" {
		t.Fatalf("dns = %#v", dns)
	}
	servers := dns["servers"].([]any)
	if len(servers) != 1 || servers[0].(map[string]any)["tag"] != "doh" {
		t.Fatalf("dns.servers = %#v", servers)
	}
}
