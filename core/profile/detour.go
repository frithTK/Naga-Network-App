package profile

import "strings"

// OmitDanglingDetours removes proxy outbounds whose detour target is absent
// and drops those tags from selector and urltest lists. Callers pass a
// decoded runtime copy; the stored subscription JSON is left unchanged.
func OmitDanglingDetours(document map[string]any) {
	raw, ok := document["outbounds"].([]any)
	if !ok || len(raw) == 0 {
		return
	}
	for range raw {
		tags := outboundTags(raw)
		drop := map[string]struct{}{}
		for _, item := range raw {
			outbound, ok := item.(map[string]any)
			if !ok {
				continue
			}
			tag, _ := outbound["tag"].(string)
			detour := strings.TrimSpace(stringField(outbound, "detour"))
			if tag == "" || detour == "" {
				continue
			}
			if _, exists := tags[detour]; !exists {
				drop[tag] = struct{}{}
			}
		}
		next := make([]any, 0, len(raw))
		changed := len(drop) > 0
		for _, item := range raw {
			outbound, ok := item.(map[string]any)
			if !ok {
				next = append(next, item)
				continue
			}
			tag, _ := outbound["tag"].(string)
			if _, gone := drop[tag]; gone {
				continue
			}
			if refs, ok := outbound["outbounds"].([]any); ok {
				filtered := make([]any, 0, len(refs))
				for _, ref := range refs {
					name, ok := ref.(string)
					if !ok {
						filtered = append(filtered, ref)
						continue
					}
					if _, gone := drop[name]; gone {
						changed = true
						continue
					}
					if _, exists := tags[name]; !exists {
						changed = true
						continue
					}
					filtered = append(filtered, name)
				}
				outbound["outbounds"] = filtered
				if def := stringField(outbound, "default"); def != "" && !containsString(filtered, def) {
					if len(filtered) > 0 {
						outbound["default"] = filtered[0]
					} else {
						delete(outbound, "default")
					}
					changed = true
				}
				kind := strings.ToLower(stringField(outbound, "type"))
				if len(filtered) == 0 && (kind == "selector" || kind == "urltest") {
					changed = true
					continue
				}
			}
			next = append(next, outbound)
		}
		document["outbounds"] = next
		raw = next
		if !changed {
			return
		}
	}
}

func outboundTags(raw []any) map[string]struct{} {
	tags := make(map[string]struct{}, len(raw))
	for _, item := range raw {
		outbound, ok := item.(map[string]any)
		if !ok {
			continue
		}
		tag, _ := outbound["tag"].(string)
		if tag != "" {
			tags[tag] = struct{}{}
		}
	}
	return tags
}

func stringField(outbound map[string]any, key string) string {
	value, _ := outbound[key].(string)
	return value
}

func containsString(values []any, target string) bool {
	for _, value := range values {
		if name, ok := value.(string); ok && name == target {
			return true
		}
	}
	return false
}
