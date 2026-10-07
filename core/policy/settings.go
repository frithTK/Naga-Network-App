package policy

import (
	"encoding/json"
	"errors"
	"strings"
)

type ConnectionMode string

const (
	ConnectionAuto           ConnectionMode = "auto"
	ConnectionManualFallback ConnectionMode = "manual_fallback"
	ConnectionManualStrict   ConnectionMode = "manual_strict"
)

type TrafficMode string

const (
	TrafficTUN         TrafficMode = "tun"
	TrafficSystemProxy TrafficMode = "system_proxy"
)

type RoutingMode string

const (
	RoutingAllVPN         RoutingMode = "all_vpn"
	RoutingSelectedVPN    RoutingMode = "selected_vpn"
	RoutingSelectedDirect RoutingMode = "selected_direct"
	RoutingAllDirect      RoutingMode = "all_direct"
)

type ConnectionPolicy struct {
	SchemaVersion       int            `json:"schema_version"`
	Revision            int64          `json:"revision"`
	Mode                ConnectionMode `json:"mode"`
	TrafficMode         TrafficMode    `json:"traffic_mode"`
	PreferredCountry    string         `json:"preferred_country,omitempty"`
	PreferredProtocol   string         `json:"preferred_protocol,omitempty"`
	NetworkClass        NetworkClass   `json:"network_class"`
	TUICFallbackEnabled bool           `json:"tuic_fallback_enabled"`
}

type AppRoute struct {
	ID                 string `json:"id"`
	DisplayName        string `json:"display_name"`
	Platform           string `json:"platform"`
	PackageOrProcessID string `json:"package_name_or_process_id"`
	Route              string `json:"route"`
	Enabled            bool   `json:"enabled"`
}

type RoutingPolicy struct {
	SchemaVersion  int         `json:"schema_version"`
	Revision       int64       `json:"revision"`
	Mode           RoutingMode `json:"mode"`
	RUDirect       bool        `json:"ru_direct"`
	ProviderRules  bool        `json:"provider_rules"`
	BuiltinPrivate bool        `json:"builtin_private"`
	BuiltinRU      bool        `json:"builtin_ru"`
	Apps           []AppRoute  `json:"apps"`
}

func DefaultConnectionPolicy() ConnectionPolicy {
	return ConnectionPolicy{
		SchemaVersion:       1,
		Mode:                ConnectionAuto,
		TrafficMode:         TrafficTUN,
		NetworkClass:        NetworkUnknown,
		TUICFallbackEnabled: true,
	}
}

func DefaultRoutingPolicy() RoutingPolicy {
	return RoutingPolicy{
		SchemaVersion:  2,
		Mode:           RoutingAllVPN,
		RUDirect:       true,
		ProviderRules:  true,
		BuiltinPrivate: true,
		BuiltinRU:      true,
		Apps:           []AppRoute{},
	}
}

func (p ConnectionPolicy) Validate() error {
	if p.Mode != ConnectionAuto && p.Mode != ConnectionManualFallback && p.Mode != ConnectionManualStrict {
		return errors.New("connection policy mode is invalid")
	}
	if p.TrafficMode != TrafficTUN && p.TrafficMode != TrafficSystemProxy {
		return errors.New("traffic mode is invalid")
	}
	return nil
}

func (p RoutingPolicy) Validate() error {
	if p.Mode != RoutingAllVPN && p.Mode != RoutingSelectedVPN && p.Mode != RoutingSelectedDirect && p.Mode != RoutingAllDirect {
		return errors.New("routing policy mode is invalid")
	}
	for _, app := range p.Apps {
		if app.ID == "" || app.Route != "vpn" && app.Route != "direct" {
			return errors.New("application route is invalid")
		}
	}
	if p.Mode == RoutingSelectedVPN || p.Mode == RoutingSelectedDirect {
		if !p.HasEnabledApp() {
			return errors.New("selected routing requires at least one enabled application")
		}
	}
	return nil
}

type routingPolicyJSON struct {
	SchemaVersion  int         `json:"schema_version"`
	Revision       int64       `json:"revision"`
	Mode           RoutingMode `json:"mode"`
	RUDirect       *bool       `json:"ru_direct"`
	ProviderRules  *bool       `json:"provider_rules"`
	BuiltinPrivate *bool       `json:"builtin_private"`
	BuiltinRU      *bool       `json:"builtin_ru"`
	Apps           []AppRoute  `json:"apps"`
}

func (p *RoutingPolicy) UnmarshalJSON(data []byte) error {
	var parsed routingPolicyJSON
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	p.SchemaVersion = parsed.SchemaVersion
	p.Revision = parsed.Revision
	p.Mode = parsed.Mode
	p.Apps = parsed.Apps
	if p.Apps == nil {
		p.Apps = []AppRoute{}
	}
	ru := true
	if parsed.RUDirect != nil {
		ru = *parsed.RUDirect
	}
	if parsed.ProviderRules != nil {
		p.ProviderRules = *parsed.ProviderRules
	} else {
		p.ProviderRules = ru
	}
	if parsed.BuiltinRU != nil {
		p.BuiltinRU = *parsed.BuiltinRU
	} else {
		p.BuiltinRU = ru
	}
	if parsed.BuiltinPrivate != nil {
		p.BuiltinPrivate = *parsed.BuiltinPrivate
	} else {
		p.BuiltinPrivate = true
	}
	p.RUDirect = p.BuiltinRU
	if p.SchemaVersion == 0 {
		p.SchemaVersion = 2
	}
	return nil
}

func (p RoutingPolicy) MarshalJSON() ([]byte, error) {
	if p.Apps == nil {
		p.Apps = []AppRoute{}
	}
	p.RUDirect = p.BuiltinRU
	type wire struct {
		SchemaVersion  int         `json:"schema_version"`
		Revision       int64       `json:"revision"`
		Mode           RoutingMode `json:"mode"`
		RUDirect       bool        `json:"ru_direct"`
		ProviderRules  bool        `json:"provider_rules"`
		BuiltinPrivate bool        `json:"builtin_private"`
		BuiltinRU      bool        `json:"builtin_ru"`
		Apps           []AppRoute  `json:"apps"`
	}
	return json.Marshal(wire{
		SchemaVersion:  p.SchemaVersion,
		Revision:       p.Revision,
		Mode:           p.Mode,
		RUDirect:       p.BuiltinRU,
		ProviderRules:  p.ProviderRules,
		BuiltinPrivate: p.BuiltinPrivate,
		BuiltinRU:      p.BuiltinRU,
		Apps:           p.Apps,
	})
}

// HasEnabledApp reports whether at least one process rule would actually
// match. Empty selected_vpn / selected_direct must not silently send every
// flow to DIRECT.
func (p RoutingPolicy) HasEnabledApp() bool {
	for _, app := range p.Apps {
		if app.Enabled && strings.TrimSpace(app.PackageOrProcessID) != "" {
			return true
		}
	}
	return false
}

// FallbackIfNoEnabledApps keeps selected_vpn / selected_direct from becoming
// an invalid empty split after a per-app mutation. Whole-traffic VPN is the
// safe default.
func (p *RoutingPolicy) FallbackIfNoEnabledApps() {
	if p == nil {
		return
	}
	if (p.Mode == RoutingSelectedVPN || p.Mode == RoutingSelectedDirect) && !p.HasEnabledApp() {
		p.Mode = RoutingAllVPN
	}
}
