package singbox

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestFormatTunHostExitPrefersStderr(t *testing.T) {
	got := formatTunHostExit(errors.New("exit status 1"), " FATAL[xyz] create tun: Access is denied.\n")
	if !strings.Contains(got, "Access is denied") {
		t.Fatalf("message = %q", got)
	}
	if got := formatTunHostExit(nil, ""); got != "sing-box exited unexpectedly" {
		t.Fatalf("empty = %q", got)
	}
}

func TestTunHostReplyUsesSameDecoder(t *testing.T) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(tunHostReply{OK: true}); err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(&buf).Encode(tunHostReply{Error: "create tun failed"}); err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(&buf)
	var ready tunHostReply
	if err := decoder.Decode(&ready); err != nil {
		t.Fatal(err)
	}
	if !ready.OK {
		t.Fatalf("first reply = %#v", ready)
	}
	var failed tunHostReply
	if err := decoder.Decode(&failed); err != nil {
		t.Fatal(err)
	}
	err := tunHostReplyError(failed, nil)
	if err == nil || err.Error() != "create tun failed" {
		t.Fatalf("second reply error = %v", err)
	}
}

func TestStaleWintunCleanupScriptRejectsInjection(t *testing.T) {
	script, ok := staleWintunCleanupScript("naga-tun0")
	if !ok || !strings.Contains(script, "naga-tun0") {
		t.Fatalf("script = %q ok=%v", script, ok)
	}
	for _, name := range []string{"", "naga;calc", "naga tun", "naga'foo", "../tun"} {
		if _, allowed := staleWintunCleanupScript(name); allowed {
			t.Fatalf("accepted unsafe adapter name %q", name)
		}
	}
}
