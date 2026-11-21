//go:build linux && !android

package olcrtc

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
)

// MarkedRoute is an experimental IPv4 route reservation. It is not yet
// connected to the runtime: crash recovery and network-change handling must
// be implemented before it can protect a system tunnel.
type MarkedRoute struct {
	Mark      uint32
	Table     uint32
	Priority  uint32
	Gateway   string
	Device    string
	routeOn   bool
	ruleOn    bool
	uncertain bool
}

const markedRouteProtocol = "240"

// InstallMarkedRoute refuses a non-baseline policy, an occupied table and an
// invalid gateway before changing anything. A command failure leaves ownership
// uncertain: callers must retain the state and reconcile it before cleanup.
func InstallMarkedRoute(execer Execer, mark, table, priority uint32, route DefaultRoute) (*MarkedRoute, error) {
	state := &MarkedRoute{Mark: mark, Table: table, Priority: priority, Gateway: route.Gateway, Device: route.Device}
	if mark == 0 || table < 256 || priority < 1 || priority >= 32766 || net.ParseIP(route.Gateway).To4() == nil || route.Device == "" {
		return state, errors.New("некорректные параметры маршрута olcRTC")
	}
	if err := CheckPolicyRoutingBaseline(execer); err != nil {
		return state, err
	}
	raw, err := execer.Output("ip", "-j", "-4", "route", "show", "table", "all")
	if err != nil {
		return state, fmt.Errorf("не удалось проверить таблицы маршрутов: %w", err)
	}
	var routes []struct {
		Table json.RawMessage `json:"table"`
	}
	if err := json.Unmarshal(raw, &routes); err != nil {
		return state, errors.New("не удалось разобрать таблицы маршрутов")
	}
	for _, item := range routes {
		var value string
		if len(item.Table) == 0 {
			continue
		}
		if err := json.Unmarshal(item.Table, &value); err != nil {
			return state, errors.New("не удалось проверить таблицы маршрутов")
		}
		if value == strconv.FormatUint(uint64(table), 10) {
			return state, ErrPolicyRoutingConflict
		}
	}
	if err := execer.Run("ip", state.routeArgs("add")...); err != nil {
		// A timed-out command can have succeeded in the kernel. Neither
		// claiming nor deleting the route is safe without reconciliation.
		state.uncertain = true
		return state, fmt.Errorf("не удалось создать маршрут olcRTC: %w", err)
	}
	state.routeOn = true
	if err := execer.Run("ip", state.ruleArgs("add")...); err != nil {
		state.uncertain = true
		return state, fmt.Errorf("не удалось создать правило olcRTC: %w", err)
	}
	state.ruleOn = true
	return state, nil
}

func (s *MarkedRoute) routeArgs(action string) []string {
	return []string{"-4", "route", action, "default", "via", s.Gateway, "dev", s.Device, "proto", markedRouteProtocol, "table", strconv.FormatUint(uint64(s.Table), 10)}
}

func (s *MarkedRoute) ruleArgs(action string) []string {
	return []string{"-4", "rule", action, "priority", strconv.FormatUint(uint64(s.Priority), 10), "fwmark", fmt.Sprintf("%#x/0xffffffff", s.Mark), "lookup", strconv.FormatUint(uint64(s.Table), 10), "protocol", markedRouteProtocol}
}

// Remove deletes only the exact rule and route installed by this object. A
// failed delete leaves the state uncertain until it can be reconciled.
// External concurrent changes still require a stronger ownership protocol;
// this experimental helper must not be used in a live VPN yet.
func (s *MarkedRoute) Remove(execer Execer) error {
	if s == nil {
		return nil
	}
	if s.uncertain {
		return errors.New("состояние маршрута olcRTC не подтверждено; требуется проверка")
	}
	var result error
	if s.ruleOn {
		if err := execer.Run("ip", s.ruleArgs("del")...); err != nil {
			s.uncertain = true
			result = errors.Join(result, fmt.Errorf("не удалось удалить правило olcRTC: %w", err))
		} else {
			s.ruleOn = false
		}
	}
	if s.routeOn && !s.ruleOn {
		if err := execer.Run("ip", s.routeArgs("del")...); err != nil {
			s.uncertain = true
			result = errors.Join(result, fmt.Errorf("не удалось удалить маршрут olcRTC: %w", err))
		} else {
			s.routeOn = false
		}
	}
	return result
}
