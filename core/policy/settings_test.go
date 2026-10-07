package policy

import (
	"encoding/json"
	"testing"
)

func TestRoutingPolicyRejectsEmptySelectedModes(t *testing.T) {
	for _, mode := range []RoutingMode{RoutingSelectedVPN, RoutingSelectedDirect} {
		err := RoutingPolicy{Mode: mode, Apps: []AppRoute{}}.Validate()
		if err == nil {
			t.Fatalf("mode %s accepted an empty application list", mode)
		}
	}
}

func TestRoutingPolicyAcceptsSelectedModeWithEnabledApp(t *testing.T) {
	err := RoutingPolicy{
		Mode: RoutingSelectedVPN,
		Apps: []AppRoute{{
			ID:                 "firefox",
			PackageOrProcessID: "firefox",
			Route:              "direct",
			Enabled:            true,
		}},
	}.Validate()
	if err != nil {
		t.Fatal(err)
	}
}

func TestRoutingPolicyIgnoresDisabledAppsForSelectedMode(t *testing.T) {
	err := RoutingPolicy{
		Mode: RoutingSelectedDirect,
		Apps: []AppRoute{{
			ID:                 "firefox",
			PackageOrProcessID: "firefox",
			Route:              "vpn",
			Enabled:            false,
		}},
	}.Validate()
	if err == nil {
		t.Fatal("disabled-only app list must not enable selected routing")
	}
}

func TestRoutingPolicyMigratesLegacyRUDirect(t *testing.T) {
	var parsed RoutingPolicy
	if err := json.Unmarshal([]byte(`{"mode":"all_vpn","ru_direct":false,"apps":[]}`), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.ProviderRules || parsed.BuiltinRU || !parsed.BuiltinPrivate {
		t.Fatalf("migrated policy = %#v", parsed)
	}
	raw, err := json.Marshal(parsed)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(raw) {
		t.Fatal(raw)
	}
	var round routingPolicyJSON
	if err := json.Unmarshal(raw, &round); err != nil {
		t.Fatal(err)
	}
	if round.ProviderRules == nil || *round.ProviderRules || round.BuiltinRU == nil || *round.BuiltinRU {
		t.Fatalf("wire = %#v", round)
	}
}

func TestRoutingPolicyDefaultsNewToggles(t *testing.T) {
	var parsed RoutingPolicy
	if err := json.Unmarshal([]byte(`{"mode":"all_vpn"}`), &parsed); err != nil {
		t.Fatal(err)
	}
	if !parsed.ProviderRules || !parsed.BuiltinRU || !parsed.BuiltinPrivate {
		t.Fatalf("defaults = %#v", parsed)
	}
}

func TestFallbackIfNoEnabledAppsReturnsToAllVPN(t *testing.T) {
	routing := RoutingPolicy{
		Mode: RoutingSelectedVPN,
		Apps: []AppRoute{{
			ID:                 "firefox",
			PackageOrProcessID: "firefox",
			Route:              "vpn",
			Enabled:            false,
		}},
	}
	routing.FallbackIfNoEnabledApps()
	if routing.Mode != RoutingAllVPN {
		t.Fatalf("mode = %s, want all_vpn", routing.Mode)
	}
}
