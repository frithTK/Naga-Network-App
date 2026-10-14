package profile

import (
	"encoding/json"
	"testing"
)

func TestLatencyProbeKeepsDetoursAndDNSWithoutTrafficOrRuleDownloads(t *testing.T) {
	config := []byte(`{"inbounds":[{"type":"tun","tag":"tun"}],"outbounds":[{"type":"urltest","tag":"auto","outbounds":["leaf"],"url":"https://www.gstatic.com/generate_204","interval":"3m","tolerance":50},{"type":"vless","tag":"leaf","detour":"hop","domain_resolver":"remote"},{"type":"socks","tag":"hop"}],"route":{"final":"auto","default_domain_resolver":"remote","rules":[{"rule_set":"blocked","outbound":"leaf"}],"rule_set":[{"tag":"blocked","type":"remote","url":"https://example.invalid/rules"},{"tag":"unused","type":"remote","url":"https://example.invalid/unused"}]},"dns":{"servers":[{"type":"tls","tag":"remote","detour":"hop"}],"rules":[{"rule_set":"blocked","server":"remote"}],"final":"remote"}}`)
	minimal, err := PrepareLatencyProbeConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(minimal, &root); err != nil {
		t.Fatal(err)
	}
	if _, exists := root["inbounds"]; exists {
		t.Fatal("probe intercepts system traffic")
	}
	route := root["route"].(map[string]any)
	if route["rules"] != nil || len(route["rule_set"].([]any)) != 1 {
		t.Fatal("probe lost DNS rule dependencies or kept unused traffic rules")
	}
	if route["default_domain_resolver"] != "remote" {
		t.Fatal("default resolver lost")
	}
	outbounds := root["outbounds"].([]any)
	if len(outbounds) != 3 || outbounds[1].(map[string]any)["detour"] != "hop" {
		t.Fatal("transitive outbound dependency lost")
	}
	if outbounds[0].(map[string]any)["type"] != "selector" {
		t.Fatal("automatic urltest remains active during manual probes")
	}
	dns := root["dns"].(map[string]any)
	if len(dns["rules"].([]any)) != 1 || len(dns["servers"].([]any)) != 1 {
		t.Fatal("DNS dependency incorrectly stripped")
	}
}
