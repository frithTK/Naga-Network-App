//go:build linux && !android

package olcrtc

import (
	"errors"
	"strings"
	"testing"
)

type markedRouteTestExec struct {
	commands []string
	fail     map[string]bool
}

func (e *markedRouteTestExec) Output(_ string, args ...string) ([]byte, error) {
	if strings.Contains(strings.Join(args, " "), "route show") {
		return []byte(`[]`), nil
	}
	if args[1] == "-6" {
		return []byte(`[{"priority":0,"src":"all","table":"local"},{"priority":32766,"src":"all","table":"main"}]`), nil
	}
	return []byte(`[{"priority":0,"src":"all","table":"local"},{"priority":32766,"src":"all","table":"main"},{"priority":32767,"src":"all","table":"default"}]`), nil
}

func (e *markedRouteTestExec) Run(_ string, args ...string) error {
	command := strings.Join(args, " ")
	e.commands = append(e.commands, command)
	for substr := range e.fail {
		if strings.Contains(command, substr) && e.fail[substr] {
			return errors.New("injected failure")
		}
	}
	return nil
}

func TestMarkedRouteFailedRuleAddDoesNotGuessKernelState(t *testing.T) {
	execer := &markedRouteTestExec{fail: map[string]bool{"rule add": true}}
	state, err := InstallMarkedRoute(execer, 0x5a, 26001, 12001, DefaultRoute{Gateway: "192.0.2.1", Device: "eth0"})
	if err == nil || !state.routeOn || !state.uncertain {
		t.Fatal("failed add must preserve uncertainty and the route state")
	}
	if err := state.Remove(execer); err == nil {
		t.Fatal("cleanup must not guess whether the rule was created")
	}
	for _, command := range execer.commands {
		if strings.Contains(command, "route del") || strings.Contains(command, "rule del") {
			t.Fatal("uncertain state was modified")
		}
	}
}

func TestMarkedRouteFailedRouteAddDoesNotAssumeOwnership(t *testing.T) {
	execer := &markedRouteTestExec{fail: map[string]bool{"route add": true}}
	state, err := InstallMarkedRoute(execer, 0x5a, 26001, 12001, DefaultRoute{Gateway: "192.0.2.1", Device: "eth0"})
	if err == nil || !state.uncertain || state.Remove(execer) == nil {
		t.Fatal("failed route creation must remain uncertain")
	}
}

func TestMarkedRouteDoesNotRemoveRouteBeforeRule(t *testing.T) {
	execer := &markedRouteTestExec{fail: map[string]bool{"rule del": true}}
	state, err := InstallMarkedRoute(execer, 0x5a, 26001, 12001, DefaultRoute{Gateway: "192.0.2.1", Device: "eth0"})
	if err != nil {
		t.Fatal(err)
	}
	if err := state.Remove(execer); err == nil || !state.ruleOn || !state.routeOn || !state.uncertain {
		t.Fatal("failure must retain both rule and its route for recovery")
	}
	for _, command := range execer.commands {
		if strings.Contains(command, "route del") {
			t.Fatal("route deleted while its rule remains")
		}
	}
	execer.fail["rule del"] = false
	if err := state.Remove(execer); err == nil {
		t.Fatal("uncertain cleanup must require reconciliation")
	}
}
