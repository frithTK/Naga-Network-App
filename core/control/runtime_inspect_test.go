package control

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestInspectRuntimeConfigOmitsSecrets(t *testing.T) {
	config := []byte(`{
		"experimental": {
			"clash_api": {
				"external_controller": "127.0.0.1:9090",
				"secret": "bearer-secret"
			}
		},
		"dns": {
			"servers": [
				{"tag": "remote", "address": "https://dns.google/dns-query", "detour": "proxy"}
			]
		},
		"outbounds": [
			{"type": "vless", "tag": "ee-vless", "uuid": "11111111-1111-1111-1111-111111111111", "password": "secret-pass", "server": "vpn.example.com"},
			{"type": "http", "tag": "http-out", "server": "127.0.0.1"},
			{"type": "urltest", "tag": "Auto"}
		],
		"route": {
			"final": "Naga-Policy",
			"rules": [
				{
					"rule_set": [{"tag": "geoip-ru", "type": "remote", "url": "https://example.com/ru.srs"}],
					"outbound": "direct"
				}
			]
		}
	}`)
	inspect, err := inspectRuntimeConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	if inspect.Final != "Naga-Policy" {
		t.Fatalf("final = %q", inspect.Final)
	}
	encoded, err := json.Marshal(inspect)
	if err != nil {
		t.Fatal(err)
	}
	body := string(encoded)
	for _, forbidden := range []string{"://", "uuid", "password", "clash_api", "bearer"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("inspect leaked %q: %s", forbidden, body)
		}
	}
	hasHTTP := false
	for _, outbound := range inspect.Outbounds {
		if outbound.Type == "http" && outbound.Tag == "http-out" {
			hasHTTP = true
		}
	}
	if !hasHTTP {
		t.Fatalf("missing type=http outbound: %s", body)
	}
	if len(inspect.Rules) != 1 || inspect.Rules[0].Outbound != "direct" {
		t.Fatalf("rules = %#v", inspect.Rules)
	}
	if len(inspect.Rules[0].RuleSet) != 1 || inspect.Rules[0].RuleSet[0] != "geoip-ru" {
		t.Fatalf("rule_set = %#v", inspect.Rules[0].RuleSet)
	}
	if len(inspect.DNSDetours) != 1 || inspect.DNSDetours[0].Tag != "remote" || inspect.DNSDetours[0].Detour != "proxy" {
		t.Fatalf("dns_detours = %#v", inspect.DNSDetours)
	}
}
