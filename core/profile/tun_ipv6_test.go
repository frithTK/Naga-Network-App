package profile

import (
	"encoding/json"
	"reflect"
	"runtime"
	"testing"

	"naga.network/core/policy"
)

func TestLinuxTrafficModeCoversBothAddressFamilies(t *testing.T) {
	config := []byte(`{"outbounds":[{"type":"direct","tag":"direct"}]}`)
	raw, err := ApplyTrafficMode(config, policy.TrafficTUN)
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Inbounds []struct {
			Type    string
			Address []string
		}
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	expected := []string{"172.19.0.1/30"}
	if runtime.GOOS == "linux" && linuxIPv6Enabled() {
		expected = append(expected, runtimeTUNIPv6)
	}
	for _, inbound := range document.Inbounds {
		if inbound.Type == "tun" {
			if !reflect.DeepEqual(inbound.Address, expected) {
				t.Fatalf("TUN addresses = %v, want %v", inbound.Address, expected)
			}
			return
		}
	}
	t.Fatal("TUN inbound is missing")
}

func TestEnsureTUNIPv6PreservesExplicitConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name string
		auto bool
		in   any
		want any
	}{
		{"single address", true, "172.19.0.1/30", []any{"172.19.0.1/30", runtimeTUNIPv6}},
		{"custom dual stack", true, []any{"172.19.0.1/30", "fd00:1234::1/126"}, []any{"172.19.0.1/30", "fd00:1234::1/126"}},
		{"custom v6", true, "fd00:1234::1/126", "fd00:1234::1/126"},
		{"manual routes", false, "172.19.0.1/30", "172.19.0.1/30"},
		{"malformed", true, "invalid", "invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inbound := map[string]any{"auto_route": tc.auto, "address": tc.in, "route_exclude_address": []any{"fd00:6789::/64"}}
			ensureTUNIPv6(inbound)
			ensureTUNIPv6(inbound)
			if !reflect.DeepEqual(inbound["address"], tc.want) ||
				!reflect.DeepEqual(inbound["route_exclude_address"], []any{"fd00:6789::/64"}) {
				t.Fatalf("unexpected TUN options: %#v", inbound)
			}
		})
	}
}
