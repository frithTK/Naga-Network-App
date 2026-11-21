//go:build linux && !android

package olcrtc

import (
	"errors"
	"testing"
)

func TestPolicyRoutingBaseline(t *testing.T) {
	base := `[{"priority":0,"src":"all","table":"local"},{"priority":32766,"src":"all","table":"main"},{"priority":32767,"src":"all","table":"default"}]`
	if err := validateBuiltinRules([]byte(base), "-4"); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		`[{"priority":0,"src":"all","table":"local"},{"priority":9000,"src":"all","table":"2022"},{"priority":32766,"src":"all","table":"main"},{"priority":32767,"src":"all","table":"default"}]`,
		`[{"priority":0,"src":"all","table":"local"},{"priority":32766,"src":"all","table":"main","fwmark":"0x01"},{"priority":32767,"src":"all","table":"default"}]`,
		`[{"priority":0,"src":"all","table":"local"},{"priority":32766,"src":"all","table":"main"},{"priority":32766,"src":"all","table":"main"},{"priority":32767,"src":"all","table":"default"}]`,
		`[{"priority":0,"src":"all","table":"local"},{"priority":32766,"src":"all","table":"main"}]`,
	} {
		if !errors.Is(validateBuiltinRules([]byte(raw), "-4"), ErrPolicyRoutingConflict) {
			t.Fatal("conflicting or incomplete rules were accepted")
		}
	}
	if err := validateBuiltinRules([]byte(`[{"priority":0,"src":"all","table":"local"},{"priority":32766,"src":"all","table":"main"}]`), "-6"); err != nil {
		t.Fatal(err)
	}
	if err := validateBuiltinRules([]byte(`[]`), "-6"); err == nil {
		t.Fatal("empty rules must not be accepted")
	}
}
