package singbox

import (
	"net"
	"net/url"
	"strconv"
	"strings"
)

func normalizeDNS(document map[string]any, route map[string]any) {
	rawDNS, ok := document["dns"].(map[string]any)
	if !ok {
		return
	}
	delete(rawDNS, "independent_cache")
	fakeIP, _ := rawDNS["fakeip"].(map[string]any)
	rawServers, _ := rawDNS["servers"].([]any)
	migrated := make([]any, 0, len(rawServers)+1)
	rcodeByTag := map[string]string{}
	for _, rawServer := range rawServers {
		server, ok := rawServer.(map[string]any)
		if !ok {
			migrated = append(migrated, rawServer)
			continue
		}
		keep, rcode := migrateLegacyDNSServer(server, fakeIP)
		if rcode != "" {
			if tag, _ := server["tag"].(string); tag != "" {
				rcodeByTag[tag] = rcode
			}
			continue
		}
		if keep {
			migrated = append(migrated, server)
		}
	}
	if len(fakeIP) > 0 {
		applyFakeIPRanges(migrated, fakeIP)
	}
	delete(rawDNS, "fakeip")
	rawDNS["servers"] = migrated
	applyStashedServerOptions(rawDNS)
	if len(rcodeByTag) > 0 {
		rewriteRcodeDNSRules(rawDNS, rcodeByTag)
		if final, _ := rawDNS["final"].(string); final != "" {
			if rcode, ok := rcodeByTag[final]; ok {
				rules, _ := rawDNS["rules"].([]any)
				rawDNS["rules"] = append(rules, map[string]any{
					"action": "predefined",
					"rcode":  rcode,
				})
				if fallback := firstDNSServerTag(migrated); fallback != "" {
					rawDNS["final"] = fallback
				} else {
					delete(rawDNS, "final")
				}
			}
		}
	}
	normalizeDNSDetours(document, route)
	migrateOutboundDNSRules(document, route)
	ensureDefaultDomainResolver(document, route)
}

func migrateLegacyDNSServer(server map[string]any, fakeIP map[string]any) (keep bool, rcode string) {
	serverType, _ := server["type"].(string)
	serverType = strings.TrimSpace(serverType)
	if serverType != "" && !strings.EqualFold(serverType, "legacy") {
		migrateResolverFields(server)
		delete(server, "address")
		return true, ""
	}
	address, _ := server["address"].(string)
	address = strings.TrimSpace(address)
	if address == "" {
		migrateResolverFields(server)
		return true, ""
	}

	kind, host, port, path, iface, rcodeValue := parseLegacyDNSAddress(address)
	if kind == "rcode" {
		return false, rcodeValue
	}

	strategy, _ := server["strategy"].(string)
	clientSubnet := server["client_subnet"]
	migrateResolverFields(server)
	delete(server, "address")
	delete(server, "strategy")
	delete(server, "client_subnet")

	server["type"] = kind
	switch kind {
	case "local":
	case "dhcp":
		if iface != "" {
			server["interface"] = iface
		}
	case "fakeip":
		applyFakeIPFields(server, fakeIP)
	default:
		if host != "" {
			server["server"] = host
		}
		if port > 0 {
			server["server_port"] = port
		}
		if path != "" && path != "/dns-query" {
			server["path"] = path
		}
	}

	tag, _ := server["tag"].(string)
	attachServerOptionsToRules(server, tag, strings.TrimSpace(strategy), clientSubnet)
	return true, ""
}

func migrateResolverFields(server map[string]any) {
	if resolver, ok := server["address_resolver"]; ok {
		if _, exists := server["domain_resolver"]; !exists {
			server["domain_resolver"] = resolver
		}
		delete(server, "address_resolver")
	}
	if strategy, ok := server["address_strategy"]; ok {
		if _, exists := server["domain_strategy"]; !exists {
			server["domain_strategy"] = strategy
		}
		delete(server, "address_strategy")
	}
}

func parseLegacyDNSAddress(address string) (kind, host string, port int, path, iface, rcode string) {
	lower := strings.ToLower(address)
	switch {
	case lower == "local":
		return "local", "", 0, "", "", ""
	case lower == "fakeip":
		return "fakeip", "", 0, "", "", ""
	case strings.HasPrefix(lower, "rcode://"):
		return "rcode", "", 0, "", "", normalizeRcode(address[len("rcode://"):])
	case strings.HasPrefix(lower, "dhcp://"):
		name := strings.TrimSpace(address[len("dhcp://"):])
		if name == "" || strings.EqualFold(name, "auto") {
			return "dhcp", "", 0, "", "", ""
		}
		return "dhcp", "", 0, "", name, ""
	}

	parsed, err := url.Parse(address)
	if err == nil && parsed.Scheme != "" && parsed.Host != "" {
		kind = strings.ToLower(parsed.Scheme)
		switch kind {
		case "tcp", "udp", "tls", "quic", "https", "h3":
			host, port = splitDNSHostPort(parsed.Host)
			path = parsed.Path
			return kind, host, port, path, "", ""
		}
	}

	host, port = splitDNSHostPort(address)
	return "udp", host, port, "", "", ""
}

func splitDNSHostPort(raw string) (string, int) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", 0
	}
	if ip := net.ParseIP(strings.Trim(raw, "[]")); ip != nil {
		return ip.String(), 0
	}
	host, portText, err := net.SplitHostPort(raw)
	if err != nil {
		return raw, 0
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port <= 0 || port > 65535 {
		return raw, 0
	}
	return host, port
}

func normalizeRcode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "success", "noerror":
		return "NOERROR"
	case "formerr":
		return "FORMERR"
	case "servfail":
		return "SERVFAIL"
	case "nxdomain":
		return "NXDOMAIN"
	case "notimp":
		return "NOTIMP"
	case "refused":
		return "REFUSED"
	default:
		return strings.ToUpper(strings.TrimSpace(raw))
	}
}

func applyFakeIPRanges(servers []any, fakeIP map[string]any) {
	for _, rawServer := range servers {
		server, ok := rawServer.(map[string]any)
		if !ok {
			continue
		}
		if kind, _ := server["type"].(string); kind != "fakeip" {
			continue
		}
		applyFakeIPFields(server, fakeIP)
	}
}

func applyFakeIPFields(server, fakeIP map[string]any) {
	if fakeIP == nil {
		return
	}
	for _, key := range []string{"inet4_range", "inet6_range"} {
		if _, exists := server[key]; exists {
			continue
		}
		if value, ok := fakeIP[key]; ok {
			server[key] = value
		}
	}
}

func attachServerOptionsToRules(server map[string]any, tag, strategy string, clientSubnet any) {
	if strategy == "" && clientSubnet == nil {
		return
	}
	if tag == "" {
		return
	}
	// Per-server strategy/client_subnet moved to DNS rules in sing-box 1.12+.
	// Rules are rewritten by the caller that has the dns object. Stash on the
	// server under a private key that rewriteDNSRuleOptions will consume.
	if strategy != "" {
		server["_naga_strategy"] = strategy
	}
	if clientSubnet != nil {
		server["_naga_client_subnet"] = clientSubnet
	}
}

func rewriteRcodeDNSRules(dns map[string]any, rcodeByTag map[string]string) {
	rules, _ := dns["rules"].([]any)
	for _, rawRule := range rules {
		rule, ok := rawRule.(map[string]any)
		if !ok {
			continue
		}
		server, _ := rule["server"].(string)
		rcode, ok := rcodeByTag[server]
		if !ok {
			continue
		}
		rule["action"] = "predefined"
		rule["rcode"] = rcode
		delete(rule, "server")
	}
}

func applyStashedServerOptions(dns map[string]any) {
	servers, _ := dns["servers"].([]any)
	options := map[string]map[string]any{}
	for _, rawServer := range servers {
		server, ok := rawServer.(map[string]any)
		if !ok {
			continue
		}
		tag, _ := server["tag"].(string)
		if tag == "" {
			continue
		}
		stash := map[string]any{}
		if strategy, ok := server["_naga_strategy"]; ok {
			stash["strategy"] = strategy
			delete(server, "_naga_strategy")
		}
		if subnet, ok := server["_naga_client_subnet"]; ok {
			stash["client_subnet"] = subnet
			delete(server, "_naga_client_subnet")
		}
		if len(stash) > 0 {
			options[tag] = stash
		}
	}
	if len(options) == 0 {
		return
	}
	rules, _ := dns["rules"].([]any)
	for _, rawRule := range rules {
		rule, ok := rawRule.(map[string]any)
		if !ok {
			continue
		}
		server, _ := rule["server"].(string)
		stash, ok := options[server]
		if !ok {
			continue
		}
		if _, exists := rule["strategy"]; !exists {
			if strategy, ok := stash["strategy"]; ok {
				rule["strategy"] = strategy
			}
		}
		if _, exists := rule["client_subnet"]; !exists {
			if subnet, ok := stash["client_subnet"]; ok {
				rule["client_subnet"] = subnet
			}
		}
	}
	if _, hasStrategy := dns["strategy"]; !hasStrategy {
		for _, stash := range options {
			if strategy, ok := stash["strategy"]; ok {
				dns["strategy"] = strategy
				break
			}
		}
	}
}

func firstDNSServerTag(servers []any) string {
	for _, rawServer := range servers {
		server, ok := rawServer.(map[string]any)
		if !ok {
			continue
		}
		if tag, _ := server["tag"].(string); tag != "" {
			return tag
		}
	}
	return ""
}

func migrateOutboundDNSRules(document map[string]any, route map[string]any) {
	rawDNS, ok := document["dns"].(map[string]any)
	if !ok {
		return
	}
	rawRules, ok := rawDNS["rules"].([]any)
	if !ok {
		return
	}
	kept := make([]any, 0, len(rawRules))
	for _, rawRule := range rawRules {
		rule, ok := rawRule.(map[string]any)
		if !ok {
			kept = append(kept, rawRule)
			continue
		}
		outbound, hasOutbound := rule["outbound"]
		if !hasOutbound {
			kept = append(kept, rule)
			continue
		}
		server, _ := rule["server"].(string)
		delete(rule, "outbound")
		if outboundStr, _ := outbound.(string); strings.EqualFold(strings.TrimSpace(outboundStr), "any") && server != "" {
			if route == nil {
				route = map[string]any{}
				document["route"] = route
			}
			if _, exists := route["default_domain_resolver"]; !exists {
				route["default_domain_resolver"] = server
			}
		}
		if dnsRuleIsEmptyAfterOutboundMigration(rule) {
			continue
		}
		kept = append(kept, rule)
	}
	rawDNS["rules"] = kept
}

func dnsRuleIsEmptyAfterOutboundMigration(rule map[string]any) bool {
	for key := range rule {
		switch key {
		case "server", "action", "disable_cache", "rewrite_ttl", "client_subnet", "strategy":
			continue
		default:
			return false
		}
	}
	return true
}

// Naga envelopes omit route.default_domain_resolver. sing-box 1.14 rejects
// that as a hard error unless a deprecation flag is set; fill it at runtime.
func ensureDefaultDomainResolver(document map[string]any, route map[string]any) {
	if route == nil {
		route = map[string]any{}
		document["route"] = route
	}
	if _, exists := route["default_domain_resolver"]; exists {
		return
	}
	rawDNS, _ := document["dns"].(map[string]any)
	if rawDNS == nil {
		return
	}
	tag, _ := rawDNS["final"].(string)
	tag = strings.TrimSpace(tag)
	if tag == "" {
		servers, _ := rawDNS["servers"].([]any)
		tag = firstDNSServerTag(servers)
	}
	if tag == "" {
		return
	}
	route["default_domain_resolver"] = tag
}
