package amneziawg

import (
	"encoding/json"
	"fmt"
	"strings"
)

// InjectSOCKSLeaf adds a runtime-only SOCKS outbound to a sing-box JSON
// document and appends it to the unified selector (Naga-Policy), falling
// back to Mode or the first selector. It never adds a TUN inbound.
func InjectSOCKSLeaf(config []byte, socksHost string, socksPort int, runtimeTag string) ([]byte, error) {
	runtimeTag = strings.TrimSpace(runtimeTag)
	if runtimeTag == "" {
		return nil, fmt.Errorf("amneziawg runtime tag is required")
	}
	if socksPort <= 0 || socksPort > 65535 {
		return nil, fmt.Errorf("amneziawg socks port is invalid")
	}
	var document map[string]any
	if err := json.Unmarshal(config, &document); err != nil || document == nil {
		return nil, fmt.Errorf("amneziawg inject requires sing-box JSON")
	}
	rawOutbounds, ok := document["outbounds"].([]any)
	if !ok {
		return nil, fmt.Errorf("amneziawg inject requires outbounds")
	}
	for _, raw := range rawOutbounds {
		outbound, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if tag, _ := outbound["tag"].(string); tag == runtimeTag {
			return nil, fmt.Errorf("amneziawg runtime tag is duplicated")
		}
	}
	selector, err := selectorForInject(rawOutbounds)
	if err != nil {
		return nil, err
	}
	children, _ := selector["outbounds"].([]any)
	selector["outbounds"] = append(children, runtimeTag)
	if defaultTag, _ := selector["default"].(string); strings.TrimSpace(defaultTag) == "" {
		selector["default"] = runtimeTag
	}
	document["outbounds"] = append(rawOutbounds, SocksOutbound(socksHost, socksPort, runtimeTag))
	return json.MarshalIndent(document, "", "  ")
}

func selectorForInject(outbounds []any) (map[string]any, error) {
	var mode, first map[string]any
	for _, raw := range outbounds {
		outbound, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if typ, _ := outbound["type"].(string); typ != "selector" {
			continue
		}
		tag, _ := outbound["tag"].(string)
		if tag == "Naga-Policy" {
			return outbound, nil
		}
		if tag == SelectorTag && mode == nil {
			mode = outbound
		}
		if first == nil {
			first = outbound
		}
	}
	if mode != nil {
		return mode, nil
	}
	if first != nil {
		return first, nil
	}
	return nil, fmt.Errorf("amneziawg inject found no selector")
}
