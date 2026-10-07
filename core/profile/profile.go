package profile

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"naga.network/core/policy"
)

type Engine string

const (
	EngineUnknown   Engine = "unknown"
	EngineSingBox   Engine = "sing-box"
	EngineMihomo    Engine = "mihomo"
	EngineXray      Engine = "xray"
	EngineAmneziaWG Engine = "amneziawg"
	EngineOlcRTC    Engine = "olcrtc"
)

type SubscriptionInfo struct {
	Upload   int64
	Download int64
	Total    int64
	Expire   time.Time
}

func (s SubscriptionInfo) Unlimited() bool {
	return s.Total == 0
}

func (s SubscriptionInfo) Expired(now time.Time) bool {
	return !s.Expire.IsZero() && !now.Before(s.Expire)
}

type Profile struct {
	SchemaVersion  int
	Revision       int64
	UpdatedAt      time.Time
	ID             string
	Name           string
	ProviderName   string
	SourceURL      string
	Engine         Engine
	Config         []byte
	SelectedMode   string
	Subscription   SubscriptionInfo
	UpdateInterval time.Duration
	ImportedAt     time.Time
}

// SingBoxProfiles returns profiles that can enter MergeProfilesForNetwork.
// Naga envelopes contribute their inner sing-box JSON without rewriting disk.
func SingBoxProfiles(values []Profile) []Profile {
	out := make([]Profile, 0, len(values))
	for _, value := range values {
		if env, ok, err := ParseEnvelope(value.Config); ok && err == nil {
			copy := value
			copy.Engine = EngineSingBox
			copy.Config = append([]byte(nil), env.SingBox...)
			out = append(out, copy)
			continue
		}
		if IsAmneziaWG(value) {
			continue
		}
		if value.Engine != "" && value.Engine != EngineSingBox {
			continue
		}
		out = append(out, value)
	}
	return out
}

// AmneziaWGProfiles returns stored AmneziaWG configs and envelope AWG conf.
func AmneziaWGProfiles(values []Profile) []Profile {
	out := make([]Profile, 0)
	for _, value := range values {
		if env, ok, err := ParseEnvelope(value.Config); ok && err == nil {
			if len(env.AmneziaConf) == 0 {
				continue
			}
			copy := value
			copy.Engine = EngineAmneziaWG
			copy.Config = append([]byte(nil), env.AmneziaConf...)
			out = append(out, copy)
			continue
		}
		if IsAmneziaWG(value) {
			out = append(out, value)
		}
	}
	return out
}

func IsAmneziaWG(value Profile) bool {
	return value.Engine == EngineAmneziaWG
}

// NodeCandidate is kept as a profile-facing alias for the shared policy
// candidate model.
type NodeCandidate = policy.Candidate

// Node describes a selectable outbound without exposing its configuration.
type Node struct {
	Tag      string
	Type     string
	Country  string
	Protocol string
	Children []Node
}

// NodeMetadata extracts only display-safe metadata from an outbound tag and
// type. Country is the country declared by the subscription's node name; it
// is not a geo-IP assertion.
func NodeMetadata(tag, outboundType string) Node {
	displayTag := tag
	if separator := strings.LastIndex(tag, "::"); separator >= 0 {
		displayTag = tag[separator+2:]
	}
	protocol := protocolFromType(outboundType)
	if strings.EqualFold(displayTag, "AmneziaWG") || strings.EqualFold(tag, "AmneziaWG") {
		protocol = "AmneziaWG"
	}
	return Node{
		Tag:      displayTag,
		Type:     outboundType,
		Country:  countryFromTag(displayTag),
		Protocol: protocol,
		Children: nil,
	}
}

// ModeOptions contains the safe subset of a sing-box selector needed by the UI.
type ModeOptions struct {
	Tag     string
	Default string
	Nodes   []Node
}

type SelectorChoice struct {
	Selector string
	Outbound string
}

// InspectMode returns the selector used by route.final, falling back to the
// selector tagged Mode. Only outbound tags and types are returned to callers.
func InspectMode(config []byte) (ModeOptions, error) {
	document, err := decodeConfig(config)
	if err != nil {
		return ModeOptions{}, err
	}

	rawOutbounds, ok := document["outbounds"].([]any)
	if !ok {
		return ModeOptions{}, errors.New("sing-box config must contain an outbounds array")
	}
	outbounds := make(map[string]map[string]any, len(rawOutbounds))
	selectorTags := make([]string, 0)
	for _, raw := range rawOutbounds {
		outbound, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		tag, _ := outbound["tag"].(string)
		if tag == "" {
			continue
		}
		outbounds[tag] = outbound
		if outboundType, _ := outbound["type"].(string); outboundType == "selector" {
			selectorTags = append(selectorTags, tag)
		}
	}

	selectorTag := ""
	if route, ok := document["route"].(map[string]any); ok {
		if final, _ := route["final"].(string); final != "" {
			if outbound, exists := outbounds[final]; exists {
				if outboundType, _ := outbound["type"].(string); outboundType == "selector" {
					selectorTag = final
				}
			}
		}
	}
	if selectorTag == "" {
		if outbound, exists := outbounds["Mode"]; exists {
			if outboundType, _ := outbound["type"].(string); outboundType == "selector" {
				selectorTag = "Mode"
			}
		}
	}
	if selectorTag == "" && len(selectorTags) > 0 {
		selectorTag = selectorTags[0]
	}
	if selectorTag == "" {
		return ModeOptions{}, errors.New("profile has no selectable mode")
	}

	selector := outbounds[selectorTag]
	defaultTag, _ := selector["default"].(string)
	options, _ := selector["outbounds"].([]any)
	nodes := make([]Node, 0, len(options))
	for _, raw := range options {
		tag, ok := raw.(string)
		if !ok || tag == "" {
			continue
		}
		outboundType := "unknown"
		if outbound, exists := outbounds[tag]; exists {
			outboundType, _ = outbound["type"].(string)
			if outboundType == "" {
				outboundType = "unknown"
			}
		}
		node := NodeMetadata(tag, outboundType)
		node.Children = nodeChildren(tag, outbounds, make(map[string]bool))
		nodes = append(nodes, node)
	}
	return ModeOptions{Tag: selectorTag, Default: defaultTag, Nodes: nodes}, nil
}

func nodeChildren(tag string, outbounds map[string]map[string]any, visiting map[string]bool) []Node {
	if visiting[tag] {
		return nil
	}
	outbound, ok := outbounds[tag]
	if !ok {
		return nil
	}
	rawChildren, ok := outbound["outbounds"].([]any)
	if !ok || len(rawChildren) == 0 {
		return nil
	}
	visiting[tag] = true
	defer delete(visiting, tag)
	children := make([]Node, 0, len(rawChildren))
	for _, rawChild := range rawChildren {
		childTag, ok := rawChild.(string)
		if !ok || childTag == "" {
			continue
		}
		child, exists := outbounds[childTag]
		if !exists {
			continue
		}
		childType, _ := child["type"].(string)
		if childType == "" {
			childType = "unknown"
		}
		node := NodeMetadata(childTag, childType)
		node.Children = nodeChildren(childTag, outbounds, visiting)
		children = append(children, node)
	}
	return children
}

// ContainsNode reports whether a tag is reachable from the profile's main
// selector. This keeps the API from accepting arbitrary outbound tags.
func ContainsNode(config []byte, target string) (bool, error) {
	options, err := InspectMode(config)
	if err != nil {
		return false, err
	}
	for _, node := range options.Nodes {
		if nodeContains(node, target) {
			return true, nil
		}
	}
	return false, nil
}

// SelectionPath returns the selector choices required to reach target from
// the profile's main selector.
func SelectionPath(config []byte, target string) ([]SelectorChoice, error) {
	options, err := InspectMode(config)
	if err != nil {
		return nil, err
	}
	document, err := decodeConfig(config)
	if err != nil {
		return nil, err
	}
	rawOutbounds, ok := document["outbounds"].([]any)
	if !ok {
		return nil, errors.New("sing-box config must contain an outbounds array")
	}
	outbounds := make(map[string]map[string]any, len(rawOutbounds))
	for _, raw := range rawOutbounds {
		outbound, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		tag, _ := outbound["tag"].(string)
		if tag != "" {
			outbounds[tag] = outbound
		}
	}
	path := findNodePath(options.Tag, target, outbounds, make(map[string]bool))
	if len(path) == 0 {
		return nil, errors.New("selected mode is not available in profile")
	}
	choices := make([]SelectorChoice, 0, len(path))
	for index := 0; index+1 < len(path); index++ {
		parent := outbounds[path[index]]
		parentType, _ := parent["type"].(string)
		if parentType == "selector" || parentType == "urltest" {
			choices = append(choices, SelectorChoice{
				Selector: path[index],
				Outbound: path[index+1],
			})
		}
	}
	return choices, nil
}

func nodeContains(node Node, target string) bool {
	if node.Tag == target {
		return true
	}
	for _, child := range node.Children {
		if nodeContains(child, target) {
			return true
		}
	}
	return false
}

// InspectOutboundMetadata returns safe metadata for every tagged outbound.
// It is used internally by the runtime to map the active Clash API tag back
// to a country and protocol without exposing the raw profile.
func InspectOutboundMetadata(config []byte) (map[string]Node, error) {
	document, err := decodeConfig(config)
	if err != nil {
		return nil, err
	}
	rawOutbounds, ok := document["outbounds"].([]any)
	if !ok {
		return nil, errors.New("sing-box config must contain an outbounds array")
	}
	result := make(map[string]Node)
	for _, raw := range rawOutbounds {
		outbound, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		tag, _ := outbound["tag"].(string)
		typeName, _ := outbound["type"].(string)
		if tag == "" || typeName == "" {
			continue
		}
		result[tag] = NodeMetadata(tag, typeName)
	}
	return result, nil
}

func countryFromTag(tag string) string {
	open := strings.LastIndex(tag, "(")
	close := strings.LastIndex(tag, ")")
	if open <= 0 || close <= open+1 {
		return ""
	}
	code := strings.TrimSpace(tag[open+1 : close])
	if len(code) != 2 || strings.ToUpper(code) != code {
		return ""
	}
	prefix := strings.TrimSpace(tag[:open])
	prefix = strings.TrimLeftFunc(prefix, func(r rune) bool {
		return r >= 0x1F1E6 && r <= 0x1F1FF
	})
	return strings.TrimSpace(prefix)
}

// ProtocolFromType returns the UI protocol label for an outbound type.
func ProtocolFromType(outboundType string) string {
	return protocolFromType(outboundType)
}

func protocolFromType(outboundType string) string {
	switch strings.ToLower(strings.TrimSpace(outboundType)) {
	case "vless":
		return "VLESS"
	case "hysteria2":
		return "Hysteria2"
	case "tuic":
		return "TUIC"
	case "shadowsocks":
		return "Shadowsocks"
	case "trojan":
		return "Trojan"
	case "vmess":
		return "VMess"
	case "amneziawg":
		return "AmneziaWG"
	default:
		return outboundType
	}
}

// ApplyModeSelection returns a runtime-only config with a validated selector
// default. The persisted source config remains unchanged.
func ApplyModeSelection(config []byte, mode string) ([]byte, error) {
	options, err := InspectMode(config)
	if err != nil {
		return nil, err
	}
	document, err := decodeConfig(config)
	if err != nil {
		return nil, err
	}
	rawOutbounds, ok := document["outbounds"].([]any)
	if !ok {
		return nil, errors.New("sing-box config must contain an outbounds array")
	}
	outbounds := make(map[string]map[string]any, len(rawOutbounds))
	for _, raw := range rawOutbounds {
		outbound, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		tag, _ := outbound["tag"].(string)
		if tag != "" {
			outbounds[tag] = outbound
		}
	}
	path := findNodePath(options.Tag, mode, outbounds, make(map[string]bool))
	if len(path) == 0 {
		return nil, errors.New("selected mode is not available in profile")
	}
	for index := 0; index+1 < len(path); index++ {
		parent := outbounds[path[index]]
		if parent["type"] != "selector" {
			continue
		}
		parent["default"] = path[index+1]
	}
	return json.MarshalIndent(document, "", "  ")
}

// ApplyConnectionPolicy applies an explicit manual preference to a runtime
// config. Automatic mode keeps the network-aware selector order produced by
// MergeProfilesForNetwork. Manual fallback keeps that order when the
// preference is unavailable; strict manual mode reports the missing target.
func ApplyConnectionPolicy(config []byte, value policy.ConnectionPolicy) ([]byte, error) {
	if err := value.Validate(); err != nil {
		return nil, err
	}
	if value.Mode == policy.ConnectionAuto {
		return config, nil
	}
	preferredCountry := strings.TrimSpace(value.PreferredCountry)
	preferredProtocol := strings.TrimSpace(value.PreferredProtocol)
	if preferredCountry == "" && preferredProtocol == "" {
		return config, nil
	}
	options, err := InspectMode(config)
	if err != nil {
		return nil, err
	}
	metadata, err := InspectOutboundMetadata(config)
	if err != nil {
		return nil, err
	}
	var target string
	var find func(Node)
	find = func(node Node) {
		if target != "" {
			return
		}
		countryMatches := preferredCountry == "" || strings.EqualFold(node.Country, preferredCountry)
		protocolMatches := preferredProtocol == "" || strings.EqualFold(node.Protocol, preferredProtocol)
		if len(node.Children) == 0 && countryMatches && protocolMatches {
			for tag, candidate := range metadata {
				if candidate.Tag == node.Tag && candidate.Country == node.Country && candidate.Protocol == node.Protocol {
					target = tag
					break
				}
			}
			return
		}
		for _, child := range node.Children {
			find(child)
		}
	}
	for _, node := range options.Nodes {
		find(node)
	}
	if target == "" {
		if value.Mode == policy.ConnectionManualStrict {
			return nil, errors.New("strict manual connection preference is unavailable")
		}
		return config, nil
	}
	return ApplyModeSelection(config, target)
}

const (
	SystemProxyPort = 2080
	RuntimeTUNName  = "naga-tun0"
)

// ApplyTrafficMode changes only the temporary runtime config. The imported
// subscription JSON remains untouched. TUN is the default mode; system proxy
// exposes a loopback mixed inbound for desktop applications.
func ApplyTrafficMode(config []byte, mode policy.TrafficMode) ([]byte, error) {
	if mode != policy.TrafficTUN && mode != policy.TrafficSystemProxy {
		return nil, errors.New("traffic mode is invalid")
	}
	if mode == policy.TrafficTUN {
		document, err := decodeConfig(config)
		if err != nil {
			return nil, err
		}
		rawInbounds, _ := document["inbounds"].([]any)
		for _, raw := range rawInbounds {
			inbound, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if tag, _ := inbound["tag"].(string); tag == "tun-in" {
				if inboundType, _ := inbound["type"].(string); inboundType == "tun" {
					inbound["interface_name"] = RuntimeTUNName
				}
			}
		}
		removeUnconditionalQUICReject(document)
		ensureLoopbackProxyInbound(document)
		return json.MarshalIndent(document, "", "  ")
	}
	document, err := decodeConfig(config)
	if err != nil {
		return nil, err
	}
	rawInbounds, _ := document["inbounds"].([]any)
	inbounds := make([]any, 0, len(rawInbounds)+1)
	haveProxy := false
	for _, raw := range rawInbounds {
		inbound, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if inboundType, _ := inbound["type"].(string); inboundType == "tun" {
			continue
		}
		if tag, _ := inbound["tag"].(string); tag == "naga-system-proxy" {
			haveProxy = true
			continue
		}
		inbounds = append(inbounds, inbound)
	}
	if !haveProxy {
		inbounds = append(inbounds, map[string]any{
			"type":        "mixed",
			"tag":         "naga-system-proxy",
			"listen":      "127.0.0.1",
			"listen_port": float64(SystemProxyPort),
		})
	}
	document["inbounds"] = inbounds
	if route, ok := document["route"].(map[string]any); ok {
		if rules, ok := route["rules"].([]any); ok {
			for _, rawRule := range rules {
				rule, ok := rawRule.(map[string]any)
				if !ok {
					continue
				}
				inboundTags, ok := rule["inbound"].([]any)
				if !ok {
					continue
				}
				for index, rawTag := range inboundTags {
					if tag, ok := rawTag.(string); ok && tag == "tun-in" {
						inboundTags[index] = "naga-system-proxy"
					}
				}
			}
		}
	}
	return json.MarshalIndent(document, "", "  ")
}

// ensureLoopbackProxyInbound keeps a loopback mixed inbound next to TUN so
// naga-control can refresh a subscription through the running tunnel. The
// provider can block the subscription host on the physical network; traffic
// that enters sing-box on 127.0.0.1 leaves via the selected outbound.
func ensureLoopbackProxyInbound(document map[string]any) {
	rawInbounds, _ := document["inbounds"].([]any)
	for _, raw := range rawInbounds {
		inbound, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if tag, _ := inbound["tag"].(string); tag == "naga-system-proxy" {
			return
		}
	}
	document["inbounds"] = append(rawInbounds, map[string]any{
		"type":        "mixed",
		"tag":         "naga-system-proxy",
		"listen":      "127.0.0.1",
		"listen_port": float64(SystemProxyPort),
	})
}

// removeUnconditionalQUICReject removes a legacy subscription rule which
// drops every UDP/QUIC connection to port 443. In system-proxy mode browsers
// use TCP automatically, but in TUN mode this rule can keep them retrying
// QUIC without ever reaching the selected outbound. Conditional reject rules
// are preserved, and the source profile is never modified.
func removeUnconditionalQUICReject(document map[string]any) {
	route, _ := document["route"].(map[string]any)
	rules, ok := route["rules"].([]any)
	if !ok {
		return
	}
	filtered := make([]any, 0, len(rules))
	for _, rawRule := range rules {
		rule, ok := rawRule.(map[string]any)
		if ok && isUnconditionalQUICReject(rule) {
			continue
		}
		filtered = append(filtered, rawRule)
	}
	route["rules"] = filtered
}

func isUnconditionalQUICReject(rule map[string]any) bool {
	action, _ := rule["action"].(string)
	if !strings.EqualFold(action, "reject") || !onlyStringValue(rule["protocol"], "quic") || !onlyNumberValue(rule["port"], 443) {
		return false
	}
	for key := range rule {
		switch key {
		case "action", "protocol", "port", "method":
		default:
			return false
		}
	}
	return true
}

func onlyStringValue(value any, expected string) bool {
	switch current := value.(type) {
	case string:
		return strings.EqualFold(current, expected)
	case []any:
		return len(current) == 1 && strings.EqualFold(fmt.Sprint(current[0]), expected)
	default:
		return false
	}
}

func onlyNumberValue(value any, expected float64) bool {
	switch current := value.(type) {
	case float64:
		return current == expected
	case []any:
		return len(current) == 1 && current[0] == expected
	default:
		return false
	}
}

// ApplyRoutingPolicy applies the platform-neutral subset of routing policy to
// a runtime-only sing-box config. Desktop process rules use process_name or
// process_path (Linux and Windows); mobile package rules are generated by
// their native runtime.
func ApplyRoutingPolicy(config []byte, routing policy.RoutingPolicy) ([]byte, error) {
	if (routing.Mode == policy.RoutingSelectedVPN || routing.Mode == policy.RoutingSelectedDirect) &&
		!routing.HasEnabledApp() {
		routing.Mode = policy.RoutingAllVPN
	}
	if err := routing.Validate(); err != nil {
		return nil, err
	}
	document, err := decodeConfig(config)
	if err != nil {
		return nil, err
	}
	route, _ := document["route"].(map[string]any)
	if route == nil {
		route = map[string]any{}
		document["route"] = route
	}
	final, _ := route["final"].(string)
	direct := directOutboundTag(document)
	needsDirect := routing.Mode == policy.RoutingAllDirect ||
		routing.Mode == policy.RoutingSelectedVPN ||
		routing.Mode == policy.RoutingSelectedDirect
	for _, app := range routing.Apps {
		needsDirect = needsDirect || app.Enabled && app.Route == "direct"
	}
	if direct == "" && needsDirect {
		return nil, errors.New("routing policy requires a direct outbound")
	}

	vpnFinal := final
	switch routing.Mode {
	case policy.RoutingAllDirect:
		final = direct
	case policy.RoutingSelectedVPN:
		final = direct
	case policy.RoutingSelectedDirect:
		if final == "" || final == direct {
			return nil, errors.New("selected direct routing requires a VPN final outbound")
		}
	}
	if final != "" {
		route["final"] = final
	}
	if !routing.ProviderRules {
		stripProviderRuleSets(document)
		route, _ = document["route"].(map[string]any)
		if route == nil {
			route = map[string]any{}
			document["route"] = route
		}
	}

	appRules := make([]any, 0, len(routing.Apps))
	for _, app := range routing.Apps {
		if !app.Enabled || strings.TrimSpace(app.PackageOrProcessID) == "" {
			continue
		}
		key := processMatchKey(app.PackageOrProcessID)
		target := vpnFinal
		if app.Route == "direct" {
			target = direct
		}
		if target == "" {
			continue
		}
		appRules = append(appRules, map[string]any{
			key:        []any{app.PackageOrProcessID},
			"outbound": target,
		})
	}
	builtin := builtinDirectRules(routing, direct)
	oldRules, _ := route["rules"].([]any)
	if len(appRules) > 0 || len(builtin) > 0 {
		route["rules"] = append(append(appRules, builtin...), oldRules...)
	}

	result, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode routed config: %w", err)
	}
	return result, nil
}

var builtinRUSuffixes = []any{".ru", ".su", ".xn--p1ai"}

func processMatchKey(id string) string {
	if strings.ContainsAny(id, `/\`) {
		return "process_path"
	}
	return "process_name"
}

func isBuiltinRuleSetTag(tag string) bool {
	return strings.HasPrefix(tag, "naga-builtin-")
}

func builtinDirectRules(routing policy.RoutingPolicy, direct string) []any {
	if direct == "" {
		return nil
	}
	rules := make([]any, 0, 3)
	if routing.BuiltinPrivate {
		rules = append(rules, map[string]any{
			"ip_is_private": true,
			"outbound":      direct,
		}, map[string]any{
			"domain_suffix": []any{".local", ".lan", "localhost"},
			"outbound":      direct,
		})
	}
	if routing.BuiltinRU {
		rules = append(rules, map[string]any{
			"domain_suffix": builtinRUSuffixes,
			"outbound":      direct,
		})
	}
	return rules
}

func stripProviderRuleSets(document map[string]any) {
	providerTags := providerRuleSetTags(document)
	if route, ok := document["route"].(map[string]any); ok {
		if sets, ok := route["rule_set"].([]any); ok {
			kept := make([]any, 0, len(sets))
			for _, raw := range sets {
				item, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				tag, _ := item["tag"].(string)
				if isBuiltinRuleSetTag(tag) {
					kept = append(kept, raw)
				}
			}
			if len(kept) == 0 {
				delete(route, "rule_set")
			} else {
				route["rule_set"] = kept
			}
		}
		if rules, ok := route["rules"].([]any); ok {
			route["rules"] = filterRulesWithoutProviderRuleSets(rules, providerTags)
		}
	}
	dns, ok := document["dns"].(map[string]any)
	if !ok {
		return
	}
	if rules, ok := dns["rules"].([]any); ok {
		dns["rules"] = filterRulesWithoutProviderRuleSets(rules, providerTags)
	}
}

func providerRuleSetTags(document map[string]any) map[string]bool {
	tags := make(map[string]bool)
	route, ok := document["route"].(map[string]any)
	if !ok {
		return tags
	}
	sets, ok := route["rule_set"].([]any)
	if !ok {
		return tags
	}
	for _, raw := range sets {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		tag, _ := item["tag"].(string)
		if tag == "" || isBuiltinRuleSetTag(tag) {
			continue
		}
		tags[tag] = true
	}
	return tags
}

func filterRulesWithoutProviderRuleSets(rules []any, providerTags map[string]bool) []any {
	if len(providerTags) == 0 {
		return rules
	}
	kept := make([]any, 0, len(rules))
	for _, rule := range rules {
		if ruleUsesProviderRuleSet(rule, providerTags) {
			continue
		}
		kept = append(kept, rule)
	}
	return kept
}

func ruleUsesProviderRuleSet(value any, providerTags map[string]bool) bool {
	switch current := value.(type) {
	case map[string]any:
		for key, item := range current {
			if key == "rule_set" {
				for _, tag := range ruleSetTagList(item) {
					if providerTags[tag] {
						return true
					}
				}
			}
			if ruleUsesProviderRuleSet(item, providerTags) {
				return true
			}
		}
	case []any:
		for _, item := range current {
			if ruleUsesProviderRuleSet(item, providerTags) {
				return true
			}
		}
	}
	return false
}

func ruleSetTagList(value any) []string {
	switch current := value.(type) {
	case string:
		if current != "" {
			return []string{current}
		}
	case []any:
		tags := make([]string, 0, len(current))
		for _, item := range current {
			if tag, ok := item.(string); ok && tag != "" {
				tags = append(tags, tag)
			}
		}
		return tags
	}
	return nil
}

func directOutboundTag(document map[string]any) string {
	for tag := range directOutboundTags(document) {
		if tag == UnifiedDirectTag {
			return tag
		}
	}
	for tag := range directOutboundTags(document) {
		return tag
	}
	return ""
}

func directOutboundTags(document map[string]any) map[string]bool {
	result := make(map[string]bool)
	if outbounds, ok := document["outbounds"].([]any); ok {
		for _, raw := range outbounds {
			outbound, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if outboundType, _ := outbound["type"].(string); outboundType != "direct" {
				continue
			}
			if tag, _ := outbound["tag"].(string); tag != "" {
				result[tag] = true
			}
		}
	}
	return result
}

func ruleTargetsDirect(value any, directTags map[string]bool) bool {
	switch current := value.(type) {
	case map[string]any:
		for key, item := range current {
			if key == "outbound" {
				if tag, ok := item.(string); ok && directTags[tag] {
					return true
				}
			}
			if ruleTargetsDirect(item, directTags) {
				return true
			}
		}
	case []any:
		for _, item := range current {
			if ruleTargetsDirect(item, directTags) {
				return true
			}
		}
	}
	return false
}

func findNodePath(current, target string, outbounds map[string]map[string]any, visiting map[string]bool) []string {
	if path := findNodePathAllowing(current, target, outbounds, map[string]bool{}, false); len(path) > 0 {
		return path
	}
	return findNodePathAllowing(current, target, outbounds, map[string]bool{}, true)
}

func findNodePathAllowing(current, target string, outbounds map[string]map[string]any, visiting map[string]bool, allowURLTest bool) []string {
	if current == target {
		return []string{current}
	}
	if visiting[current] {
		return nil
	}
	outbound, ok := outbounds[current]
	if !ok {
		return nil
	}
	rawChildren, ok := outbound["outbounds"].([]any)
	if !ok {
		return nil
	}
	visiting[current] = true
	defer delete(visiting, current)
	for _, rawChild := range rawChildren {
		child, ok := rawChild.(string)
		if !ok {
			continue
		}
		if !allowURLTest {
			if nested, exists := outbounds[child]; exists {
				if nestedType, _ := nested["type"].(string); nestedType == "urltest" {
					continue
				}
			}
		}
		path := findNodePathAllowing(child, target, outbounds, visiting, allowURLTest)
		if len(path) > 0 {
			return append([]string{current}, path...)
		}
	}
	return nil
}

func decodeConfig(config []byte) (map[string]any, error) {
	var document map[string]any
	if err := json.Unmarshal(config, &document); err != nil {
		return nil, fmt.Errorf("parse sing-box JSON: %w", err)
	}
	if document == nil {
		return nil, errors.New("sing-box config must be a non-empty object")
	}
	return document, nil
}

// IDFromSourceURL gives a stable local identifier without storing the URL in
// the identifier itself or exposing it in logs.
func IDFromSourceURL(sourceURL string) string {
	digest := sha256.Sum256([]byte(sourceURL))
	return fmt.Sprintf("profile-%x", digest[:12])
}

func IDFromConfig(raw []byte) string {
	digest := sha256.Sum256(raw)
	return fmt.Sprintf("profile-%x", digest[:12])
}

func ParseSubscriptionUserinfo(value string) (SubscriptionInfo, error) {
	if strings.TrimSpace(value) == "" {
		return SubscriptionInfo{}, errors.New("subscription-userinfo is empty")
	}

	fields := make(map[string]string)
	for _, item := range strings.Split(value, ";") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		key, raw, ok := strings.Cut(item, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return SubscriptionInfo{}, fmt.Errorf("invalid subscription-userinfo field %q", item)
		}
		fields[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(raw)
	}

	upload, err := parseNonNegativeInt(fields, "upload")
	if err != nil {
		return SubscriptionInfo{}, err
	}
	download, err := parseNonNegativeInt(fields, "download")
	if err != nil {
		return SubscriptionInfo{}, err
	}
	total, err := parseNonNegativeInt(fields, "total")
	if err != nil {
		return SubscriptionInfo{}, err
	}
	expire, err := parseNonNegativeInt(fields, "expire")
	if err != nil {
		return SubscriptionInfo{}, err
	}

	var expireAt time.Time
	if expire > 0 {
		expireAt = time.Unix(expire, 0).UTC()
	}
	return SubscriptionInfo{Upload: upload, Download: download, Total: total, Expire: expireAt}, nil
}

func parseNonNegativeInt(fields map[string]string, key string) (int64, error) {
	raw, ok := fields[key]
	if !ok || raw == "" {
		return 0, fmt.Errorf("subscription-userinfo field %q is missing", key)
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("subscription-userinfo field %q is invalid", key)
	}
	return value, nil
}

func DecodeProfileTitle(value string) (string, error) {
	const prefix = "base64:"
	if !strings.HasPrefix(strings.ToLower(value), prefix) {
		return strings.TrimSpace(value), nil
	}
	raw := strings.TrimSpace(value[len(prefix):])
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(raw)
	}
	if err != nil {
		return "", fmt.Errorf("decode profile-title: %w", err)
	}
	return string(decoded), nil
}

func ValidateSingBoxConfig(config []byte) error {
	if len(strings.TrimSpace(string(config))) == 0 {
		return errors.New("sing-box config is empty")
	}

	var document map[string]json.RawMessage
	if err := json.Unmarshal(config, &document); err != nil {
		return fmt.Errorf("parse sing-box JSON: %w", err)
	}
	if len(document) == 0 {
		return errors.New("sing-box config must be a non-empty object")
	}

	raw, ok := document["outbounds"]
	if !ok {
		return errors.New("sing-box config must contain an outbounds array")
	}
	var outbounds []json.RawMessage
	if err := json.Unmarshal(raw, &outbounds); err != nil || len(outbounds) == 0 {
		return errors.New("sing-box config must contain a non-empty outbounds array")
	}

	tags := make(map[string]json.RawMessage, len(outbounds))
	for index, rawOutbound := range outbounds {
		var outbound map[string]json.RawMessage
		if err := json.Unmarshal(rawOutbound, &outbound); err != nil || outbound == nil {
			return fmt.Errorf("sing-box outbound %d must be an object", index)
		}
		var outboundType string
		if rawType, ok := outbound["type"]; !ok || json.Unmarshal(rawType, &outboundType) != nil || strings.TrimSpace(outboundType) == "" {
			return fmt.Errorf("sing-box outbound %d has no valid type", index)
		}
		var tag string
		if rawTag, ok := outbound["tag"]; ok {
			if json.Unmarshal(rawTag, &tag) != nil {
				return fmt.Errorf("sing-box outbound %d has an invalid tag", index)
			}
			if tag != "" {
				if previous, exists := tags[tag]; exists {
					if !sameJSON(previous, rawOutbound) {
						return fmt.Errorf("sing-box outbound tag %q is duplicated", tag)
					}
					continue
				}
				tags[tag] = append(json.RawMessage(nil), rawOutbound...)
			}
		}
	}

	if rawRoute, ok := document["route"]; ok {
		var route map[string]json.RawMessage
		if json.Unmarshal(rawRoute, &route) != nil || route == nil {
			return errors.New("sing-box route must be an object")
		}
		if rawFinal, ok := route["final"]; ok {
			var final string
			if json.Unmarshal(rawFinal, &final) != nil || final == "" {
				return errors.New("sing-box route.final is invalid")
			}
			if _, exists := tags[final]; !exists {
				return fmt.Errorf("sing-box route.final references unknown outbound %q", final)
			}
		}
	}
	return nil
}

// sameJSON reports whether two JSON values decode to the same structure.
// A subscription can emit the same outbound twice; identical copies are one
// node, while differing copies stay an error.
func sameJSON(left, right json.RawMessage) bool {
	var a, b any
	if json.Unmarshal(left, &a) != nil || json.Unmarshal(right, &b) != nil {
		return false
	}
	return reflect.DeepEqual(a, b)
}
