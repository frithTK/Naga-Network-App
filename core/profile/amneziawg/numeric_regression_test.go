package amneziawg

import (
	"os"
	"strings"
	"testing"
)

func TestInvalidNumericFieldRejected(t *testing.T) {
	raw, err := os.ReadFile("testdata/awg31.valid.conf")
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"Jc = 4", "MTU = 1280", "PersistentKeepalive = 25"} {
		t.Run(field, func(t *testing.T) {
			broken := strings.Replace(string(raw), field, strings.Split(field, "=")[0]+"= broken", 1)
			cfg, err := Parse([]byte(broken))
			if err == nil {
				t.Errorf("malformed integer accepted: %s; JC=%d MTU=%d keepalive=%d", field, cfg.Jc, cfg.MTU, cfg.Peers[0].PersistentKeepalive)
			}
		})
	}
}

func TestJunkMinimumCannotExceedZeroMaximum(t *testing.T) {
	raw := testdata(t, "awg31.valid.conf")
	raw = []byte(strings.Replace(string(raw), "Jmax = 70", "Jmax = 0", 1))
	if cfg, err := Parse(raw); err == nil {
		t.Fatalf("accepted an unsigned junk-size underflow: jmin=%d jmax=%d", cfg.Jmin, cfg.Jmax)
	}
}

func TestNumericFieldBoundsAndRedaction(t *testing.T) {
	for _, test := range []struct{ line, replacement string }{
		{"Jc = 4", "Jc = "},
		{"Jc = 4", "Jc = -1"},
		{"MTU = 1280", "MTU = 99999999999999999999999"},
		{"Jmax = 70", "Jmax = 4294967295"},
		{"PersistentKeepalive = 25", "PersistentKeepalive = 65536"},
		{"S1 = 20", "S1 = 65536"},
		{"Jc = 4", "Jc = sensitive-invalid-value"},
	} {
		raw := strings.Replace(string(testdata(t, "awg31.valid.conf")), test.line, test.replacement, 1)
		_, err := Parse([]byte(raw))
		if err == nil {
			t.Errorf("accepted %s", test.replacement)
			continue
		}
		if strings.Contains(err.Error(), "sensitive-invalid-value") || strings.Contains(err.Error(), "AAAAAAAAAAA") {
			t.Errorf("leaked input: %v", err)
		}
	}
}

func TestTransportPaddingMustFitNativePacketBuffer(t *testing.T) {
	raw, err := os.ReadFile("testdata/awg31.valid.conf")
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, []byte("\n")...)
	// S4 belongs in Interface, before the first Peer section.
	raw = []byte(strings.Replace(string(raw), "[Peer]", "S4 = 65535\n[Peer]", 1))
	if _, err := Parse(raw); err == nil {
		t.Fatal("accepted S4 that places packet start outside native buffer")
	}
}
