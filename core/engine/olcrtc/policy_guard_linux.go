//go:build linux && !android

package olcrtc

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ErrPolicyRoutingConflict is deliberately generic: route rules can contain
// addresses belonging to other VPNs and must not appear in diagnostics.
var ErrPolicyRoutingConflict = errors.New("обнаружены нестандартные правила маршрутизации; отключите другой VPN перед запуском olcRTC")

// CheckPolicyRoutingBaseline is a conservative precondition for experimental
// marked routing. It does not reserve a mark, a routing table, or a priority;
// callers must still implement atomic ownership and recovery before use.
func CheckPolicyRoutingBaseline(execer Execer) error {
	for _, family := range []string{"-4", "-6"} {
		data, err := execer.Output("ip", "-j", family, "rule", "show")
		if err != nil {
			return fmt.Errorf("не удалось проверить правила маршрутизации: %w", err)
		}
		if err := validateBuiltinRules(data, family); err != nil {
			return err
		}
	}
	return nil
}

func validateBuiltinRules(raw []byte, family string) error {
	var rules []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rules); err != nil || len(rules) == 0 {
		return errors.New("не удалось разобрать правила маршрутизации")
	}
	seen := make(map[int]bool)
	for _, rule := range rules {
		for key := range rule {
			switch key {
			case "priority", "src", "table", "protocol":
			default:
				return ErrPolicyRoutingConflict
			}
		}
		var priority int
		var source, table, protocol string
		if json.Unmarshal(rule["priority"], &priority) != nil ||
			json.Unmarshal(rule["src"], &source) != nil || source != "all" ||
			json.Unmarshal(rule["table"], &table) != nil {
			return ErrPolicyRoutingConflict
		}
		if value, ok := rule["protocol"]; ok {
			if json.Unmarshal(value, &protocol) != nil || (protocol != "kernel" && protocol != "unspec") {
				return ErrPolicyRoutingConflict
			}
		}
		if seen[priority] {
			return ErrPolicyRoutingConflict
		}
		seen[priority] = true
		if !((priority == 0 && table == "local") ||
			(priority == 32766 && table == "main") ||
			(priority == 32767 && table == "default")) {
			return ErrPolicyRoutingConflict
		}
	}
	if !seen[0] || !seen[32766] || (family == "-4" && !seen[32767]) {
		return ErrPolicyRoutingConflict
	}
	return nil
}
