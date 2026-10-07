package profile

import (
	"encoding/json"
	"strings"
	"testing"

	"naga.network/core/policy"
	"time"
)

func TestParseSubscriptionUserinfo(t *testing.T) {
	info, err := ParseSubscriptionUserinfo("upload=10; download=20; total=0; expire=1796169599")
	if err != nil {
		t.Fatal(err)
	}
	if info.Upload != 10 || info.Download != 20 || !info.Unlimited() {
		t.Fatalf("unexpected info: %+v", info)
	}
	if info.Expired(time.Unix(1796169598, 0)) {
		t.Fatal("profile should not be expired before expire timestamp")
	}
	if !info.Expired(time.Unix(1796169600, 0)) {
		t.Fatal("profile should be expired after expire timestamp")
	}
}

func TestDecodeProfileTitle(t *testing.T) {
	name, err := DecodeProfileTitle("base64:TmFnYSBOZXR3b3Jr")
	if err != nil {
		t.Fatal(err)
	}
	if name != "Naga Network" {
		t.Fatalf("unexpected title %q", name)
	}
}

func TestValidateSingBoxConfig(t *testing.T) {
	valid := []string{
		`{"outbounds":[{"type":"direct","tag":"direct"}]}`,
		`{"route":{"final":"x"},"outbounds":[{"type":"direct","tag":"x"},{"tag":"x","type":"direct"}]}`,
	}
	for _, config := range valid {
		if err := ValidateSingBoxConfig([]byte(config)); err != nil {
			t.Fatal(err)
		}
	}
	invalid := []struct {
		name   string
		config string
	}{
		{name: "empty outbounds", config: `{"outbounds":[]}`},
		{name: "primitive outbound", config: `{"outbounds":["direct"]}`},
		{name: "missing type", config: `{"outbounds":[{"tag":"direct"}]}`},
		{name: "duplicate tag", config: `{"outbounds":[{"type":"direct","tag":"x"},{"type":"block","tag":"x"}]}`},
		{name: "unknown final", config: `{"route":{"final":"missing"},"outbounds":[{"type":"direct","tag":"direct"}]}`},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidateSingBoxConfig([]byte(test.config)); err == nil {
				t.Fatalf("ValidateSingBoxConfig(%s) unexpectedly succeeded", test.config)
			}
		})
	}
}

func TestInspectAndApplyMode(t *testing.T) {
	config := []byte(`{"route":{"final":"Mode"},"outbounds":[{"type":"selector","tag":"Mode","outbounds":["fast","direct"],"default":"direct"},{"type":"shadowsocks","tag":"fast"},{"type":"direct","tag":"direct"}]}`)
	options, err := InspectMode(config)
	if err != nil {
		t.Fatal(err)
	}
	if options.Tag != "Mode" || options.Default != "direct" || len(options.Nodes) != 2 {
		t.Fatalf("unexpected mode options: %+v", options)
	}
	updated, err := ApplyModeSelection(config, "fast")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updated), `"default": "fast"`) {
		t.Fatalf("updated config does not select fast: %s", updated)
	}
	if strings.Contains(string(config), `"default": "fast"`) {
		t.Fatal("source config was mutated")
	}
	if _, err := ApplyModeSelection(config, "missing"); err == nil {
		t.Fatal("expected unavailable mode to fail")
	}
}

func TestNodeMetadataExtractsDisplaySafeCountryAndProtocol(t *testing.T) {
	node := NodeMetadata("🇪🇪 Estonia (EE) TUIC", "tuic")
	if node.Country != "Estonia" || node.Protocol != "TUIC" {
		t.Fatalf("unexpected node metadata: %+v", node)
	}
	unknown := NodeMetadata("Auto", "urltest")
	if unknown.Country != "" || unknown.Protocol != "urltest" {
		t.Fatalf("unexpected group metadata: %+v", unknown)
	}
}

func TestInspectAndApplyNestedCountryProtocolSelection(t *testing.T) {
	config := []byte(`{"route":{"final":"Mode"},"outbounds":[
		{"type":"selector","tag":"Mode","outbounds":["Auto","Manual"],"default":"Auto"},
		{"type":"urltest","tag":"Auto","outbounds":["🇪🇪 Estonia (EE) TUIC"]},
		{"type":"selector","tag":"Manual","outbounds":["🇪🇪 Estonia (EE)","🇨🇦 Canada (CA)"],"default":"🇪🇪 Estonia (EE)"},
		{"type":"selector","tag":"🇪🇪 Estonia (EE)","outbounds":["🇪🇪 Estonia (EE) TUIC"],"default":"🇪🇪 Estonia (EE) TUIC"},
		{"type":"tuic","tag":"🇪🇪 Estonia (EE) TUIC"},
		{"type":"selector","tag":"🇨🇦 Canada (CA)","outbounds":["🇨🇦 Canada (CA) VLESS"],"default":"🇨🇦 Canada (CA) VLESS"},
		{"type":"vless","tag":"🇨🇦 Canada (CA) VLESS"}
	]}`)

	options, err := InspectMode(config)
	if err != nil {
		t.Fatal(err)
	}
	if len(options.Nodes) != 2 || len(options.Nodes[1].Children) != 2 {
		t.Fatalf("expected country groups under Manual: %+v", options.Nodes)
	}
	if options.Nodes[1].Children[0].Country != "Estonia" ||
		options.Nodes[1].Children[0].Children[0].Protocol != "TUIC" {
		t.Fatalf("unexpected nested metadata: %+v", options.Nodes[1])
	}

	updated, err := ApplyModeSelection(config, "🇨🇦 Canada (CA) VLESS")
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(updated, &document); err != nil {
		t.Fatal(err)
	}
	defaults := make(map[string]string)
	for _, raw := range document["outbounds"].([]any) {
		outbound := raw.(map[string]any)
		if value, ok := outbound["default"].(string); ok {
			defaults[outbound["tag"].(string)] = value
		}
	}
	if defaults["Mode"] != "Manual" ||
		defaults["Manual"] != "🇨🇦 Canada (CA)" ||
		defaults["🇨🇦 Canada (CA)"] != "🇨🇦 Canada (CA) VLESS" {
		t.Fatalf("nested defaults were not applied: %+v", defaults)
	}
}

func TestSelectionPathReturnsNestedSelectorChoices(t *testing.T) {
	config := []byte(`{"route":{"final":"Mode"},"outbounds":[
		{"type":"selector","tag":"Mode","outbounds":["Manual"]},
		{"type":"selector","tag":"Manual","outbounds":["node"]},
		{"type":"vless","tag":"node"}]}`)

	path, err := SelectionPath(config, "node")
	if err != nil {
		t.Fatal(err)
	}
	if len(path) != 2 ||
		path[0].Selector != "Mode" || path[0].Outbound != "Manual" ||
		path[1].Selector != "Manual" || path[1].Outbound != "node" {
		t.Fatalf("selection path = %#v", path)
	}
}

func TestSelectionPathPrefersCountrySelectorOverURLTest(t *testing.T) {
	config := []byte(`{"route":{"final":"Mode"},"outbounds":[
		{"type":"selector","tag":"Mode","outbounds":["Auto","🇪🇪 Estonia (EE)","🇨🇿 Czechia (CZ)"]},
		{"type":"urltest","tag":"Auto","outbounds":["🇪🇪 Estonia (EE) VLESS","🇨🇿 Czechia (CZ) Hysteria2"]},
		{"type":"selector","tag":"🇪🇪 Estonia (EE)","outbounds":["🇪🇪 Estonia (EE) VLESS"]},
		{"type":"selector","tag":"🇨🇿 Czechia (CZ)","outbounds":["🇨🇿 Czechia (CZ) Hysteria2"]},
		{"type":"vless","tag":"🇪🇪 Estonia (EE) VLESS"},
		{"type":"hysteria2","tag":"🇨🇿 Czechia (CZ) Hysteria2"}]}`)

	path, err := SelectionPath(config, "🇪🇪 Estonia (EE) VLESS")
	if err != nil {
		t.Fatal(err)
	}
	if len(path) != 2 ||
		path[0].Selector != "Mode" || path[0].Outbound != "🇪🇪 Estonia (EE)" ||
		path[1].Selector != "🇪🇪 Estonia (EE)" || path[1].Outbound != "🇪🇪 Estonia (EE) VLESS" {
		t.Fatalf("preferred path = %#v", path)
	}
}

func TestSelectionPathFallsBackToURLTestWhenNoSelectorPathExists(t *testing.T) {
	config := []byte(`{"route":{"final":"Mode"},"outbounds":[
		{"type":"selector","tag":"Mode","outbounds":["Auto"]},
		{"type":"urltest","tag":"Auto","outbounds":["leaf"]},
		{"type":"vless","tag":"leaf"}]}`)

	path, err := SelectionPath(config, "leaf")
	if err != nil {
		t.Fatal(err)
	}
	if len(path) != 2 ||
		path[0].Selector != "Mode" || path[0].Outbound != "Auto" ||
		path[1].Selector != "Auto" || path[1].Outbound != "leaf" {
		t.Fatalf("urltest fallback path = %#v", path)
	}
}

func TestApplyRoutingPolicyAddsLinuxProcessRules(t *testing.T) {
	config := []byte(`{"route":{"final":"Mode","rules":[{"domain_suffix":["ru"],"outbound":"direct"}]},"outbounds":[{"type":"selector","tag":"Mode","outbounds":["node"]},{"type":"vless","tag":"node"},{"type":"direct","tag":"direct"}]}`)
	routed, err := ApplyRoutingPolicy(config, policy.RoutingPolicy{
		Mode:     policy.RoutingSelectedVPN,
		RUDirect: true,
		Apps: []policy.AppRoute{{
			ID:                 "firefox",
			DisplayName:        "Firefox",
			Platform:           "linux",
			PackageOrProcessID: "firefox",
			Route:              "vpn",
			Enabled:            true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(routed, &document); err != nil {
		t.Fatal(err)
	}
	route := document["route"].(map[string]any)
	if route["final"] != "direct" {
		t.Fatalf("selected VPN final = %#v", route["final"])
	}
	rules := route["rules"].([]any)
	first := rules[0].(map[string]any)
	if first["process_name"].([]any)[0] != "firefox" || first["outbound"] != "Mode" {
		t.Fatalf("application route = %#v", first)
	}
}

func TestApplyRoutingPolicySkipsProcessRulesOnAndroid(t *testing.T) {
	prev := writeAppProcessRules
	writeAppProcessRules = false
	t.Cleanup(func() { writeAppProcessRules = prev })
	config := []byte(`{"route":{"final":"Mode"},"outbounds":[{"type":"selector","tag":"Mode","outbounds":["node"]},{"type":"vless","tag":"node"},{"type":"direct","tag":"direct"}]}`)
	routed, err := ApplyRoutingPolicy(config, policy.RoutingPolicy{
		Mode: policy.RoutingSelectedVPN,
		Apps: []policy.AppRoute{{
			ID:                 "org.mozilla.firefox",
			DisplayName:        "Firefox",
			Platform:           "android",
			PackageOrProcessID: "org.mozilla.firefox",
			Route:              "vpn",
			Enabled:            true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(routed), "process_name") || strings.Contains(string(routed), "package_name") {
		t.Fatalf("android routing leaked process rules: %s", routed)
	}
}

func TestApplyRoutingPolicyUsesWindowsProcessPath(t *testing.T) {
	config := []byte(`{"route":{"final":"Mode"},"outbounds":[{"type":"selector","tag":"Mode","outbounds":["node"]},{"type":"vless","tag":"node"},{"type":"direct","tag":"direct"}]}`)
	routed, err := ApplyRoutingPolicy(config, policy.RoutingPolicy{
		Mode:     policy.RoutingSelectedVPN,
		RUDirect: true,
		Apps: []policy.AppRoute{{
			ID:                 "Telegram.exe",
			DisplayName:        "Telegram",
			Platform:           "windows",
			PackageOrProcessID: `C:\Program Files\Telegram Desktop\Telegram.exe`,
			Route:              "vpn",
			Enabled:            true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(routed, &document); err != nil {
		t.Fatal(err)
	}
	first := document["route"].(map[string]any)["rules"].([]any)[0].(map[string]any)
	if first["process_path"].([]any)[0] != `C:\Program Files\Telegram Desktop\Telegram.exe` || first["outbound"] != "Mode" {
		t.Fatalf("windows application route = %#v", first)
	}
}

func TestApplyRoutingPolicyEmptySelectedVPNKeepsVPNFinal(t *testing.T) {
	config := []byte(`{"route":{"final":"Mode"},"outbounds":[{"type":"selector","tag":"Mode","outbounds":["node"]},{"type":"vless","tag":"node"},{"type":"direct","tag":"direct"}]}`)
	routed, err := ApplyRoutingPolicy(config, policy.RoutingPolicy{
		Mode:     policy.RoutingSelectedVPN,
		RUDirect: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(routed, &document); err != nil {
		t.Fatal(err)
	}
	route := document["route"].(map[string]any)
	if route["final"] != "Mode" {
		t.Fatalf("empty selected_vpn must not send all traffic DIRECT, final=%#v", route["final"])
	}
}

func TestApplyConnectionPolicySelectsPreferredNode(t *testing.T) {
	config := []byte(`{"route":{"final":"Mode"},"outbounds":[{"type":"selector","tag":"Mode","outbounds":["Germany (DE) VLESS","Netherlands (NL) TUIC"],"default":"Germany (DE) VLESS"},{"type":"vless","tag":"Germany (DE) VLESS"},{"type":"tuic","tag":"Netherlands (NL) TUIC"}]}`)
	updated, err := ApplyConnectionPolicy(config, policy.ConnectionPolicy{
		Mode:              policy.ConnectionManualFallback,
		TrafficMode:       policy.TrafficTUN,
		PreferredCountry:  "Netherlands",
		PreferredProtocol: "TUIC",
	})
	if err != nil {
		t.Fatalf("ApplyConnectionPolicy() error = %v", err)
	}
	options, err := InspectMode(updated)
	if err != nil {
		t.Fatalf("InspectMode() error = %v", err)
	}
	if options.Default != "Netherlands (NL) TUIC" {
		t.Fatalf("default = %q, want Netherlands (NL) TUIC", options.Default)
	}
}

func TestApplyConnectionPolicyStrictRejectsMissingPreference(t *testing.T) {
	config := []byte(`{"route":{"final":"Mode"},"outbounds":[{"type":"selector","tag":"Mode","outbounds":["vless (DE)"],"default":"vless (DE)"},{"type":"vless","tag":"vless (DE)"}]}`)
	_, err := ApplyConnectionPolicy(config, policy.ConnectionPolicy{
		Mode:             policy.ConnectionManualStrict,
		TrafficMode:      policy.TrafficTUN,
		PreferredCountry: "US",
	})
	if err == nil {
		t.Fatal("ApplyConnectionPolicy() unexpectedly accepted missing strict preference")
	}
}

func TestApplyTrafficModeAddsSystemProxyAndRemovesTUN(t *testing.T) {
	config := []byte(`{"inbounds":[{"type":"tun","tag":"tun-in"}],"outbounds":[{"type":"direct","tag":"direct"}]}`)
	updated, err := ApplyTrafficMode(config, policy.TrafficSystemProxy)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(updated, &document); err != nil {
		t.Fatal(err)
	}
	inbounds := document["inbounds"].([]any)
	if len(inbounds) != 1 {
		t.Fatalf("inbounds = %#v", inbounds)
	}
	proxy := inbounds[0].(map[string]any)
	if proxy["type"] != "mixed" || proxy["tag"] != "naga-system-proxy" || proxy["listen_port"] != float64(SystemProxyPort) {
		t.Fatalf("system proxy inbound = %#v", proxy)
	}
}

func TestApplyTrafficModeRenamesTUNInRuntimeOnly(t *testing.T) {
	config := []byte(`{"inbounds":[{"type":"tun","tag":"tun-in","interface_name":"tun0"}],"outbounds":[{"type":"direct","tag":"direct"}]}`)
	updated, err := ApplyTrafficMode(config, policy.TrafficTUN)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(updated, &document); err != nil {
		t.Fatal(err)
	}
	inbound := document["inbounds"].([]any)[0].(map[string]any)
	if inbound["interface_name"] != RuntimeTUNName {
		t.Fatalf("runtime TUN name = %#v, want %q", inbound["interface_name"], RuntimeTUNName)
	}
	foundProxy := false
	for _, raw := range document["inbounds"].([]any) {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if item["tag"] == "naga-system-proxy" && item["type"] == "mixed" {
			foundProxy = true
		}
	}
	if !foundProxy {
		t.Fatal("TUN runtime must keep a loopback mixed inbound for subscription refresh")
	}
	if strings.Contains(string(config), RuntimeTUNName) {
		t.Fatal("source config was unexpectedly changed")
	}
}

func TestApplyTrafficModeRemovesOnlyUnconditionalQUICReject(t *testing.T) {
	config := []byte(`{
		"inbounds":[{"type":"tun","tag":"tun-in"}],
		"route":{"rules":[
			{"protocol":"quic","port":[443],"action":"reject"},
			{"domain_suffix":["example.com"],"protocol":"quic","port":[443],"action":"reject"},
			{"protocol":"tcp","port":[443],"action":"reject"}
		]},
		"outbounds":[{"type":"direct","tag":"direct"}]
	}`)
	updated, err := ApplyTrafficMode(config, policy.TrafficTUN)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(updated, &document); err != nil {
		t.Fatal(err)
	}
	rules := document["route"].(map[string]any)["rules"].([]any)
	if len(rules) != 2 {
		t.Fatalf("runtime rules = %#v", rules)
	}
	if _, conditional := rules[0].(map[string]any)["domain_suffix"]; !conditional {
		t.Fatalf("conditional QUIC rule was removed: %#v", rules)
	}
	if !strings.Contains(string(config), `"protocol":"quic","port":[443],"action":"reject"`) {
		t.Fatal("source config was unexpectedly changed")
	}
}

func TestApplyRoutingPolicyStripsProviderRuleSets(t *testing.T) {
	config := []byte(`{"route":{"final":"Mode","rule_set":[{"tag":"sub-ru","type":"inline","rules":[{"domain_suffix":[".ru"]}]}],"rules":[{"rule_set":"sub-ru","outbound":"direct"},{"domain_suffix":["example"],"outbound":"Mode"}]},"outbounds":[{"type":"selector","tag":"Mode","outbounds":["node"]},{"type":"vless","tag":"node"},{"type":"direct","tag":"direct"}]}`)
	routed, err := ApplyRoutingPolicy(config, policy.RoutingPolicy{
		Mode:          policy.RoutingAllVPN,
		ProviderRules: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(routed, &document); err != nil {
		t.Fatal(err)
	}
	route := document["route"].(map[string]any)
	if _, ok := route["rule_set"]; ok {
		t.Fatalf("provider rule_set remained: %#v", route["rule_set"])
	}
	rules := route["rules"].([]any)
	if len(rules) != 1 {
		t.Fatalf("rules = %#v", rules)
	}
	if rules[0].(map[string]any)["domain_suffix"].([]any)[0] != "example" {
		t.Fatalf("kept rule = %#v", rules[0])
	}
}

func TestApplyRoutingPolicyAddsBuiltinRUWhenProviderDisabled(t *testing.T) {
	config := []byte(`{"route":{"final":"Mode","rule_set":[{"tag":"sub-ru","type":"remote","url":"https://example.invalid/ru.srs"}],"rules":[{"rule_set":"sub-ru","outbound":"direct"}]},"outbounds":[{"type":"selector","tag":"Mode","outbounds":["node"]},{"type":"vless","tag":"node"},{"type":"direct","tag":"direct"}]}`)
	routed, err := ApplyRoutingPolicy(config, policy.RoutingPolicy{
		Mode:           policy.RoutingAllVPN,
		ProviderRules:  false,
		BuiltinRU:      true,
		BuiltinPrivate: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(routed, &document); err != nil {
		t.Fatal(err)
	}
	route := document["route"].(map[string]any)
	if _, ok := route["rule_set"]; ok {
		t.Fatalf("provider rule_set remained: %#v", route["rule_set"])
	}
	foundRU := false
	for _, raw := range route["rules"].([]any) {
		rule := raw.(map[string]any)
		suffixes, ok := rule["domain_suffix"].([]any)
		if !ok {
			continue
		}
		for _, suffix := range suffixes {
			if suffix == ".ru" && rule["outbound"] == "direct" {
				foundRU = true
			}
		}
	}
	if !foundRU {
		t.Fatalf("builtin .ru missing: %#v", route["rules"])
	}
}

func TestApplyRoutingPolicyKeepsProviderRuleSetWhenEnabled(t *testing.T) {
	config := []byte(`{"route":{"final":"Mode","rule_set":[{"tag":"sub-ru","type":"local","path":"/tmp/ru.srs"}],"rules":[{"rule_set":"sub-ru","outbound":"direct"}]},"outbounds":[{"type":"selector","tag":"Mode","outbounds":["node"]},{"type":"vless","tag":"node"},{"type":"direct","tag":"direct"}]}`)
	routed, err := ApplyRoutingPolicy(config, policy.RoutingPolicy{
		Mode:          policy.RoutingAllVPN,
		ProviderRules: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(routed, &document); err != nil {
		t.Fatal(err)
	}
	sets := document["route"].(map[string]any)["rule_set"].([]any)
	if len(sets) != 1 || sets[0].(map[string]any)["tag"] != "sub-ru" {
		t.Fatalf("rule_set = %#v", sets)
	}
}

func TestApplyRoutingPolicyAddsBuiltinRUAndPrivate(t *testing.T) {
	config := []byte(`{"route":{"final":"Mode"},"outbounds":[{"type":"selector","tag":"Mode","outbounds":["node"]},{"type":"vless","tag":"node"},{"type":"direct","tag":"direct"}]}`)
	routed, err := ApplyRoutingPolicy(config, policy.RoutingPolicy{
		Mode:           policy.RoutingAllVPN,
		BuiltinPrivate: true,
		BuiltinRU:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(routed, &document); err != nil {
		t.Fatal(err)
	}
	rules := document["route"].(map[string]any)["rules"].([]any)
	if len(rules) < 2 {
		t.Fatalf("builtin rules = %#v", rules)
	}
	private := rules[0].(map[string]any)
	if private["ip_is_private"] != true || private["outbound"] != "direct" {
		t.Fatalf("private rule = %#v", private)
	}
	foundRU := false
	for _, raw := range rules {
		rule := raw.(map[string]any)
		suffixes, ok := rule["domain_suffix"].([]any)
		if !ok {
			continue
		}
		for _, suffix := range suffixes {
			if suffix == ".ru" {
				foundRU = true
			}
		}
	}
	if !foundRU {
		t.Fatalf("missing builtin .ru: %#v", rules)
	}
}

func TestSingBoxProfilesSkipsAmneziaWG(t *testing.T) {
	values := []Profile{
		{ID: "json", Engine: EngineSingBox, Config: []byte(`{}`)},
		{ID: "awg", Engine: EngineAmneziaWG, Config: []byte(`[Interface]`)},
		{ID: "legacy", Config: []byte(`{}`)},
	}
	filtered := SingBoxProfiles(values)
	if len(filtered) != 2 || filtered[0].ID != "json" || filtered[1].ID != "legacy" {
		t.Fatalf("SingBoxProfiles = %#v", filtered)
	}
	awg := AmneziaWGProfiles(values)
	if len(awg) != 1 || awg[0].ID != "awg" {
		t.Fatalf("AmneziaWGProfiles = %#v", awg)
	}
}

func FuzzParseSubscriptionUserinfo(f *testing.F) {
	f.Add("upload=0; download=0; total=0; expire=0")
	f.Add("upload=10; download=20; total=0; expire=1796169599")
	f.Add("upload=-1; download=0; total=0; expire=0")
	f.Add("not-a-userinfo")
	f.Add("")
	f.Add("upload=1; download=2; total=3; expire=4; extra=5")
	f.Fuzz(func(t *testing.T, value string) {
		_, _ = ParseSubscriptionUserinfo(value)
	})
}

func FuzzDecodeProfileTitle(f *testing.F) {
	f.Add("Naga Network")
	f.Add("base64:TmFnYSBOZXR3b3Jr")
	f.Add("base64:!!!")
	f.Add("")
	f.Fuzz(func(t *testing.T, value string) {
		_, _ = DecodeProfileTitle(value)
	})
}

func FuzzValidateSingBoxConfig(f *testing.F) {
	f.Add([]byte(`{"outbounds":[{"type":"direct","tag":"direct"}]}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`[`))
	f.Add([]byte(``))
	f.Add([]byte(`{"outbounds":[]}`))
	f.Fuzz(func(t *testing.T, config []byte) {
		_ = ValidateSingBoxConfig(config)
	})
}
