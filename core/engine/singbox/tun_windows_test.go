//go:build windows

package singbox

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAdaptTUNForPlatformEnablesStrictRoute(t *testing.T) {
	input := []byte(`{
  "inbounds": [
    {
      "type": "tun",
      "tag": "tun-in",
      "auto_route": true,
      "strict_route": false,
      "stack": "gvisor"
    }
  ]
}`)
	got, err := adaptTUNForPlatform(input)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(got, &document); err != nil {
		t.Fatal(err)
	}
	inbound := document["inbounds"].([]any)[0].(map[string]any)
	if inbound["strict_route"] != true {
		t.Fatalf("strict_route = %#v, want true", inbound["strict_route"])
	}
	if inbound["stack"] != "system" {
		t.Fatalf("stack = %#v, want system", inbound["stack"])
	}
	if inbound["mtu"] != float64(1500) {
		t.Fatalf("mtu = %#v, want 1500", inbound["mtu"])
	}
	if strings.Contains(string(input), `"strict_route": true`) {
		t.Fatal("source config must stay untouched")
	}
}

func TestPrepareRuntimeConfigSkipsLinuxDNSInbound(t *testing.T) {
	input := []byte(`{
  "inbounds": [{"type":"tun","tag":"tun-in","auto_route":true}],
  "outbounds": [{"type":"direct","tag":"Mode"}],
  "route": {"final":"Mode"}
}`)
	got, _, err := prepareRuntimeConfig(input)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(got, &document); err != nil {
		t.Fatal(err)
	}
	for _, raw := range document["inbounds"].([]any) {
		inbound := raw.(map[string]any)
		tag, _ := inbound["tag"].(string)
		port, _ := inbound["listen_port"].(float64)
		if tag == "dns-in" && port == 53 {
			t.Fatalf("Windows runtime must not bind dns-in on port 53: %#v", inbound)
		}
	}
}

func TestAdaptTUNForPlatformLeavesNonAutoRoute(t *testing.T) {
	input := []byte(`{"inbounds":[{"type":"tun","tag":"tun-in","auto_route":false,"strict_route":false}]}`)
	got, err := adaptTUNForPlatform(input)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(got, &document); err != nil {
		t.Fatal(err)
	}
	inbound := document["inbounds"].([]any)[0].(map[string]any)
	if inbound["strict_route"] != false {
		t.Fatalf("strict_route = %#v, want false", inbound["strict_route"])
	}
}
