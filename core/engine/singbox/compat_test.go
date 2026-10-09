package singbox

import (
	"encoding/json"
	"runtime"
	"strings"
	"testing"
)

func TestNormalizeLinuxConfigDropsHiddenShadowTLSDetour(t *testing.T) {
	input := []byte(`{
    "route": {"final":"Mode"},
    "outbounds": [
      {"type":"selector","tag":"Mode","outbounds":["good","bad"],"default":"bad"},
      {"type":"vless","tag":"good","server":"a.example"},
      {"type":"shadowsocks","tag":"bad","detour":"bad_shadowtls-out §hide§"}
    ]
  }`)
	got, err := NormalizeLinuxConfig(input)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(input), "good") && strings.Contains(string(got), "§hide§") {
		t.Fatalf("hidden detour survived normalization: %s", got)
	}
	if strings.Contains(string(got), `"bad"`) {
		t.Fatalf("dangling outbound survived normalization: %s", got)
	}
	var document map[string]any
	if err := json.Unmarshal(got, &document); err != nil {
		t.Fatal(err)
	}
	mode := document["outbounds"].([]any)[0].(map[string]any)
	refs := mode["outbounds"].([]any)
	if len(refs) != 1 || refs[0] != "good" || mode["default"] != "good" {
		t.Fatalf("selector = %#v", mode)
	}
}

func TestNormalizeLinuxConfigCollapsesIdenticalOutbound(t *testing.T) {
	input := []byte(`{
    "outbounds": [
      {"type":"selector","tag":"Mode","outbounds":["leaf","leaf"]},
      {"type":"shadowsocks","tag":"leaf","password":"same"},
      {"type":"shadowsocks","tag":"leaf","password":"same"}
    ]
  }`)
	got, err := NormalizeLinuxConfig(input)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(got, &document); err != nil {
		t.Fatal(err)
	}
	outbounds := document["outbounds"].([]any)
	if len(outbounds) != 2 {
		t.Fatalf("outbounds = %d, want 2", len(outbounds))
	}
	mode := outbounds[0].(map[string]any)
	refs := mode["outbounds"].([]any)
	if len(refs) != 1 || refs[0] != "leaf" {
		t.Fatalf("selector refs = %#v", refs)
	}
}

func TestNormalizeLinuxConfigMigratesLegacyInboundFields(t *testing.T) {
	input := []byte(`{
    "inbounds": [
      {"type":"mixed", "tag":"mixed-in", "sniff":true, "sniff_timeout":"300ms", "sniff_override_destination":true, "domain_strategy":"prefer_ipv4"},
      {"type":"direct", "tag":"dns-in", "override_address":"8.8.8.8", "override_port":53}
    ],
    "dns": {"servers":[{"tag":"dns-remote", "address":"1.1.1.1", "detour":"Select"}]},
    "route": {"override_android_vpn":true, "final":"Mode", "rules":[]},
    "outbounds": [{"type":"direct", "tag":"Mode"}]
  }`)

	got, err := NormalizeLinuxConfig(input)
	if err != nil {
		t.Fatal(err)
	}

	var document map[string]any
	if err := json.Unmarshal(got, &document); err != nil {
		t.Fatal(err)
	}
	inbounds := document["inbounds"].([]any)
	mixed := inbounds[0].(map[string]any)
	for _, key := range []string{"sniff", "sniff_timeout", "sniff_override_destination", "domain_strategy"} {
		if _, ok := mixed[key]; ok {
			t.Fatalf("legacy field %q was not removed", key)
		}
	}

	dns := inbounds[1].(map[string]any)
	if dns["override_address"] != "8.8.8.8" || dns["override_port"] != float64(53) {
		t.Fatalf("direct DNS override was changed: %#v", dns)
	}

	route := document["route"].(map[string]any)
	if _, ok := route["override_android_vpn"]; ok {
		t.Fatal("Android-only route option was not removed")
	}
	rules := route["rules"].([]any)
	if len(rules) != 2 {
		t.Fatalf("expected sniff and resolve migration rules, got %d", len(rules))
	}
	dnsConfig := document["dns"].(map[string]any)
	servers := dnsConfig["servers"].([]any)
	server := servers[0].(map[string]any)
	if server["type"] != "udp" || server["server"] != "1.1.1.1" {
		t.Fatalf("legacy DNS server was not migrated: %#v", server)
	}
	if _, ok := server["address"]; ok {
		t.Fatalf("legacy DNS address field was kept: %#v", server)
	}
	if _, ok := server["detour"]; ok {
		t.Fatalf("DNS detour to empty direct outbound was kept: %#v", server)
	}
}

func TestNormalizeLinuxConfigMigratesLegacyDNSServers(t *testing.T) {
	input := []byte(`{
    "dns": {
      "independent_cache": true,
      "fakeip": {"inet4_range":"198.18.0.0/15","inet6_range":"fc00::/18"},
      "servers": [
        {"tag":"local","address":"local"},
        {"tag":"remote","address":"tls://1.1.1.1","detour":"Select","strategy":"ipv4_only"},
        {"tag":"doh","address":"https://dns.google/dns-query","address_resolver":"local"},
        {"tag":"fakeip","address":"fakeip"},
        {"tag":"block","address":"rcode://refused"}
      ],
      "rules": [
        {"outbound":"any","server":"local"},
        {"query_type":["A","AAAA"],"server":"fakeip"},
        {"domain_suffix":["ads.example"],"server":"block"}
      ],
      "final":"remote"
    },
    "outbounds": [{"type":"direct","tag":"Mode"}],
    "route": {"final":"Mode"}
  }`)

	got, err := NormalizeLinuxConfig(input)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(input), `"type": "udp"`) {
		t.Fatal("source config was unexpectedly changed")
	}

	var document map[string]any
	if err := json.Unmarshal(got, &document); err != nil {
		t.Fatal(err)
	}
	dns := document["dns"].(map[string]any)
	if _, ok := dns["independent_cache"]; ok {
		t.Fatal("independent_cache was kept")
	}
	if _, ok := dns["fakeip"]; ok {
		t.Fatal("legacy dns.fakeip object was kept")
	}
	servers := map[string]map[string]any{}
	for _, raw := range dns["servers"].([]any) {
		server := raw.(map[string]any)
		servers[server["tag"].(string)] = server
	}
	if servers["local"]["type"] != "local" {
		t.Fatalf("local server = %#v", servers["local"])
	}
	if servers["remote"]["type"] != "tls" || servers["remote"]["server"] != "1.1.1.1" {
		t.Fatalf("remote server = %#v", servers["remote"])
	}
	if _, ok := servers["remote"]["detour"]; ok {
		t.Fatalf("remote detour to empty direct was kept: %#v", servers["remote"])
	}
	if servers["doh"]["type"] != "https" || servers["doh"]["server"] != "dns.google" || servers["doh"]["domain_resolver"] != "local" {
		t.Fatalf("doh server = %#v", servers["doh"])
	}
	if servers["fakeip"]["type"] != "fakeip" || servers["fakeip"]["inet4_range"] != "198.18.0.0/15" {
		t.Fatalf("fakeip server = %#v", servers["fakeip"])
	}
	if _, ok := servers["block"]; ok {
		t.Fatalf("rcode server was kept: %#v", servers["block"])
	}

	route := document["route"].(map[string]any)
	if route["default_domain_resolver"] != "local" {
		t.Fatalf("outbound:any was not migrated: %#v", route["default_domain_resolver"])
	}
	var sawAds, sawFakeIP, sawAny bool
	for _, raw := range dns["rules"].([]any) {
		rule := raw.(map[string]any)
		if _, ok := rule["outbound"]; ok {
			t.Fatalf("legacy outbound DNS rule was kept: %#v", rule)
		}
		if _, ok := rule["domain_suffix"]; ok {
			sawAds = true
			if rule["action"] != "predefined" || rule["rcode"] != "REFUSED" {
				t.Fatalf("rcode rule = %#v", rule)
			}
		}
		if _, ok := rule["query_type"]; ok {
			sawFakeIP = true
			if rule["server"] != "fakeip" {
				t.Fatalf("fakeip rule = %#v", rule)
			}
		}
		if rule["server"] == "local" && len(rule) <= 2 {
			sawAny = true
		}
	}
	if sawAny {
		t.Fatal("bare outbound:any DNS rule was kept")
	}
	if !sawAds || !sawFakeIP {
		t.Fatalf("expected rewritten DNS rules, got %#v", dns["rules"])
	}
}

func TestNormalizeLinuxConfigStripsEmptyDirectDNSDetour(t *testing.T) {
	input := []byte(`{
    "log": {"level":"error"},
    "dns": {
      "servers": [
        {"tag":"dns-local","address":"8.8.8.8","detour":"direct"},
        {"tag":"dns-remote","address":"tcp://1.1.1.1","detour":"Select"}
      ],
      "rules": [{"outbound":"any","server":"dns-local"}],
      "final":"dns-remote"
    },
    "inbounds": [{"type":"tun","tag":"tun-in","address":["172.19.0.1/30"],"auto_route":true,"stack":"gvisor"}],
    "outbounds": [
      {"type":"direct","tag":"direct"},
      {"type":"selector","tag":"Mode","outbounds":["direct"],"default":"direct"}
    ],
    "route": {"final":"Mode"}
  }`)
	got, err := NormalizeLinuxConfig(input)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(input), `"type": "udp"`) {
		t.Fatal("source config was unexpectedly changed")
	}
	var document map[string]any
	if err := json.Unmarshal(got, &document); err != nil {
		t.Fatal(err)
	}
	servers := map[string]map[string]any{}
	for _, raw := range document["dns"].(map[string]any)["servers"].([]any) {
		server := raw.(map[string]any)
		servers[server["tag"].(string)] = server
	}
	local := servers["dns-local"]
	if local["type"] != "udp" || local["server"] != "8.8.8.8" {
		t.Fatalf("dns-local = %#v", local)
	}
	if _, ok := local["detour"]; ok {
		t.Fatalf("empty direct DNS detour was kept: %#v", local)
	}
	remote := servers["dns-remote"]
	if remote["type"] != "tcp" || remote["detour"] != "Mode" {
		t.Fatalf("dns-remote = %#v", remote)
	}

	binary, err := DiscoverBinary("")
	if err != nil {
		return
	}
	adapter := Adapter{BinaryPath: binary}
	if err := adapter.checkConfig(t.Context(), got); err != nil {
		t.Fatalf("sing-box rejected stripped direct DNS detour: %v\n%s", err, got)
	}
}

func TestNormalizeLinuxConfigLegacyDNSPassesSingBoxCheck(t *testing.T) {
	binary, err := DiscoverBinary("")
	if err != nil {
		t.Skip("sing-box is not installed")
	}
	input := []byte(`{
    "log": {"level":"error"},
    "dns": {
      "servers": [
        {"tag":"local","address":"local"},
        {"tag":"remote","address":"1.1.1.1","detour":"proxy"}
      ],
      "rules": [{"outbound":"any","server":"local"}],
      "final":"remote"
    },
    "inbounds": [{"type":"tun","tag":"tun-in","address":["172.19.0.1/30"],"auto_route":true,"stack":"gvisor"}],
    "outbounds": [
      {"type":"selector","tag":"proxy","outbounds":["direct"],"default":"direct"},
      {"type":"direct","tag":"direct"}
    ],
    "route": {"final":"proxy"}
  }`)
	got, err := NormalizeLinuxConfig(input)
	if err != nil {
		t.Fatal(err)
	}
	adapter := Adapter{BinaryPath: binary}
	if err := adapter.checkConfig(t.Context(), got); err != nil {
		t.Fatalf("sing-box rejected migrated DNS: %v\n%s", err, got)
	}
}

func TestPrepareRuntimeConfigUsesSystemResolverPort(t *testing.T) {
	if !shouldPrepareSystemResolver() {
		t.Skip("system resolver inbound is Linux-only")
	}
	input := []byte(`{
    "inbounds": [{"type":"direct", "tag":"dns-in", "listen":"127.0.0.1", "listen_port":6450}],
    "outbounds": [{"type":"direct", "tag":"Mode"}],
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
	inbound := document["inbounds"].([]any)[0].(map[string]any)
	if inbound["listen"] != "127.0.0.1" || inbound["listen_port"] != float64(53) {
		t.Fatalf("runtime DNS inbound = %#v", inbound)
	}
}

func TestPrepareRuntimeConfigKeepsResolverPortBesideLoopbackProxy(t *testing.T) {
	if !shouldPrepareSystemResolver() {
		t.Skip("system resolver inbound is Linux-only")
	}
	input := []byte(`{
    "inbounds": [
      {"type":"tun","tag":"tun-in","interface_name":"naga-tun0","address":["172.19.0.1/30"],"auto_route":true},
      {"type":"direct","tag":"dns-in","listen":"127.0.0.1","listen_port":6450},
      {"type":"mixed","tag":"naga-system-proxy","listen":"127.0.0.1","listen_port":2080}
    ],
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
	var dnsPort float64
	for _, raw := range document["inbounds"].([]any) {
		inbound := raw.(map[string]any)
		if inbound["tag"] == "dns-in" {
			dnsPort, _ = inbound["listen_port"].(float64)
		}
	}
	if dnsPort != 53 {
		t.Fatalf("dns-in port = %v, want 53", dnsPort)
	}

	proxyOnly := []byte(`{
    "inbounds": [
      {"type":"direct","tag":"dns-in","listen":"127.0.0.1","listen_port":6450},
      {"type":"mixed","tag":"naga-system-proxy","listen":"127.0.0.1","listen_port":2080}
    ],
    "outbounds": [{"type":"direct","tag":"Mode"}],
    "route": {"final":"Mode"}
  }`)
	got, _, err = prepareRuntimeConfig(proxyOnly)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(got, &document); err != nil {
		t.Fatal(err)
	}
	for _, raw := range document["inbounds"].([]any) {
		inbound := raw.(map[string]any)
		if inbound["tag"] == "dns-in" && inbound["listen_port"] != float64(6450) {
			t.Fatalf("system-proxy mode moved dns-in to %#v", inbound["listen_port"])
		}
	}
}

func TestPrepareRuntimeConfigEnablesLinuxAutoRedirect(t *testing.T) {
	input := []byte(`{
    "inbounds": [{"type":"tun", "tag":"tun-in", "address":["172.19.0.1/30"], "auto_route":true}],
    "outbounds": [{"type":"direct", "tag":"Mode"}],
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
	inbound := document["inbounds"].([]any)[0].(map[string]any)
	if _, ok := inbound["auto_redirect"]; ok {
		t.Fatal("runtime config unexpectedly enables nftables auto_redirect")
	}
}

func TestNormalizeLinuxConfigUsesCompatibleTUNRuntime(t *testing.T) {
	input := []byte(`{
    "inbounds": [{
      "type":"tun",
      "tag":"tun-in",
      "interface_name":"naga-tun0",
      "address":["172.19.0.1/30"],
      "auto_route":true,
      "strict_route":true,
      "stack":"system",
      "mtu":9000
    }],
    "outbounds": [{"type":"direct", "tag":"Mode"}],
    "route": {"final":"Mode"}
  }`)

	got, err := NormalizeLinuxConfig(input)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(got, &document); err != nil {
		t.Fatal(err)
	}
	inbound := document["inbounds"].([]any)[0].(map[string]any)
	if inbound["stack"] != "gvisor" {
		t.Fatalf("Linux TUN stack = %#v, want gvisor", inbound["stack"])
	}
	if strict, _ := inbound["strict_route"].(bool); strict {
		t.Fatalf("Linux TUN strict_route = %#v, want false", inbound["strict_route"])
	}
	if inbound["mtu"] != float64(9000) {
		t.Fatalf("unrelated TUN MTU was changed: %#v", inbound["mtu"])
	}
	route := document["route"].(map[string]any)
	if runtime.GOOS != "android" && route["auto_detect_interface"] != true {
		t.Fatalf("auto_detect_interface = %#v, want true", route["auto_detect_interface"])
	}
	if strings.Contains(string(input), "gvisor") {
		t.Fatal("source config was unexpectedly changed")
	}
}

func TestNormalizeLinuxConfigAddsDefaultDomainResolver(t *testing.T) {
	input := []byte(`{
    "outbounds": [{"type":"selector","tag":"Mode","outbounds":["direct"],"default":"direct"},{"type":"direct","tag":"direct"}],
    "route": {"final":"Mode"},
    "dns": {
      "servers": [
        {"tag":"dns-remote","address":"tcp://1.1.1.1","detour":"Select"},
        {"tag":"dns-local","address":"8.8.8.8","detour":"direct"}
      ],
      "final": "dns-local"
    }
  }`)
	got, err := NormalizeLinuxConfig(input)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(got, &document); err != nil {
		t.Fatal(err)
	}
	route := document["route"].(map[string]any)
	if route["default_domain_resolver"] != "dns-local" {
		t.Fatalf("default_domain_resolver = %#v", route["default_domain_resolver"])
	}
	if strings.Contains(string(input), "default_domain_resolver") {
		t.Fatal("source config was unexpectedly changed")
	}
}

func TestNormalizeLinuxConfigReplacesRemoteRuleSets(t *testing.T) {
	input := []byte(`{
    "outbounds": [{"type":"direct","tag":"direct"},{"type":"direct","tag":"Mode"}],
    "route": {
      "final": "Mode",
      "rule_set": [{
        "tag": "subpanel-ru-custom",
        "type": "remote",
        "format": "source",
        "url": "https://example.invalid/rules/custom-ru-singbox.json",
        "download_detour": "direct"
      }],
      "rules": [{"rule_set":"subpanel-ru-custom","outbound":"direct"}]
    }
  }`)
	got, err := NormalizeLinuxConfig(input)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(got, &document); err != nil {
		t.Fatal(err)
	}
	ruleSet := document["route"].(map[string]any)["rule_set"].([]any)[0].(map[string]any)
	if ruleSet["type"] != "inline" {
		t.Fatalf("rule_set type = %#v, want inline", ruleSet["type"])
	}
	if ruleSet["tag"] != "subpanel-ru-custom" {
		t.Fatalf("rule_set tag = %#v", ruleSet["tag"])
	}
	if _, ok := ruleSet["url"]; ok {
		t.Fatal("runtime rule-set must not keep the remote URL")
	}
	// sing-box rejects an empty inline rule-set, so the runtime set must keep at
	// least one placeholder rule that never matches real traffic.
	rules, ok := ruleSet["rules"].([]any)
	if !ok || len(rules) == 0 {
		t.Fatalf("inline rule-set must not be empty, got %#v", ruleSet["rules"])
	}
	if strings.Contains(string(input), `"type": "inline"`) {
		t.Fatal("source config was unexpectedly changed")
	}
}
