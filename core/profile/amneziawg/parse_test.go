package amneziawg

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"naga.network/core/profile"
)

func testdata(t *testing.T, name string) []byte {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	payload, err := os.ReadFile(filepath.Join(filepath.Dir(file), "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func TestClassifyJSON(t *testing.T) {
	format, err := Classify([]byte(`{"outbounds":[{"type":"direct","tag":"direct"}]}`))
	if err != nil || format != FormatJSON {
		t.Fatalf("Classify(json) = %v %v", format, err)
	}
}

func TestClassifyAmneziaWG(t *testing.T) {
	format, err := Classify(testdata(t, "awg31.valid.conf"))
	if err != nil || format != FormatAmneziaWG {
		t.Fatalf("Classify(valid) = %v %v", format, err)
	}
}

func TestClassifyPlainWireGuard(t *testing.T) {
	_, err := Classify(testdata(t, "awg31.plain-wg.conf"))
	if err != ErrPlainWireGuard {
		t.Fatalf("Classify(plain) = %v, want ErrPlainWireGuard", err)
	}
}

func TestClassifyUnknown(t *testing.T) {
	_, err := Classify([]byte("hello world"))
	if err != ErrUnknownFormat {
		t.Fatalf("Classify(garbage) = %v, want ErrUnknownFormat", err)
	}
}

func TestParseValidConfig(t *testing.T) {
	cfg, err := Parse(testdata(t, "awg31.valid.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Jc != 4 || cfg.Jmin != 40 || cfg.Jmax != 70 {
		t.Fatalf("junk params = %+v", cfg)
	}
	if len(cfg.Peers) != 1 || cfg.Peers[0].Endpoint != "203.0.113.10:51820" {
		t.Fatalf("peers = %+v", cfg.Peers)
	}
	if cfg.I[0] == "" {
		t.Fatal("expected I1 payload")
	}
}

func TestParseIncomplete(t *testing.T) {
	_, err := Parse(testdata(t, "awg31.incomplete.conf"))
	if err != ErrPrivateKeyMissing {
		t.Fatalf("Parse(incomplete) = %v, want ErrPrivateKeyMissing", err)
	}
	if strings.Contains(err.Error(), "AAAAAAAAAAAAAAAA") {
		t.Fatalf("error leaked key material: %v", err)
	}
}

func TestParseErrorDoesNotContainPrivateKey(t *testing.T) {
	raw := testdata(t, "awg31.valid.conf")
	err := Validate(Config{})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if strings.Contains(err.Error(), string(raw)) || strings.Contains(SafeError(err), "AAAA") {
		t.Fatalf("error leaked config: %v", err)
	}
}

func TestBuildRuntimeJSONHasTUNAndSocks(t *testing.T) {
	raw, err := BuildRuntimeJSON("127.0.0.1", 12345, []string{"1.1.1.1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := profile.ValidateSingBoxConfig(raw); err != nil {
		t.Fatal(err)
	}
	options, err := profile.InspectMode(raw)
	if err != nil {
		t.Fatal(err)
	}
	if options.Tag != SelectorTag || options.Default != LeafTag {
		t.Fatalf("mode = %+v", options)
	}
	if !strings.Contains(string(raw), `"type": "tun"`) || !strings.Contains(string(raw), `"type": "socks"`) {
		t.Fatalf("wrapper missing tun/socks: %s", raw)
	}
}

func TestBuildRuntimeJSONHasDNS(t *testing.T) {
	raw, err := BuildRuntimeJSON("127.0.0.1", 12345, []string{"8.8.8.8"})
	if err != nil {
		t.Fatal(err)
	}
	if err := profile.ValidateSingBoxConfig(raw); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"dns"`) || !strings.Contains(string(raw), `"8.8.8.8"`) {
		t.Fatalf("wrapper missing dns: %s", raw)
	}
	if !strings.Contains(string(raw), `"detour": "AmneziaWG"`) {
		t.Fatalf("dns detour should use AmneziaWG leaf: %s", raw)
	}
	if strings.Count(string(raw), `"type": "tun"`) != 1 {
		t.Fatalf("expected one tun inbound: %s", raw)
	}
}

func TestParseJ1AndITime(t *testing.T) {
	cfg, err := Parse(testdata(t, "awg31.j1.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.J[0] == "" || cfg.J[1] == "" || cfg.J[2] == "" {
		t.Fatalf("J packets = %+v", cfg.J)
	}
	if cfg.ITime != 10 {
		t.Fatalf("ITime = %d", cfg.ITime)
	}
}

func TestParseAcceptsS3AndHeaderProtection(t *testing.T) {
	cfg, err := Parse(testdata(t, "awg31.s3.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.S[2] != 10 || cfg.S[3] != 16 {
		t.Fatalf("S3/S4 = %v", cfg.S)
	}
	if cfg.HeaderProtectionKey == "" || cfg.ContentPaddingAddition != "10-64" {
		t.Fatalf("hp/padding = %+v", cfg)
	}
	if cfg.RandomTrailers != "on" || cfg.DisableCookies != "on" {
		t.Fatalf("toggles = %+v", cfg)
	}
}

func TestJunkPacketCountAlias(t *testing.T) {
	raw := []byte(`[Interface]
PrivateKey = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=
Address = 10.8.0.2/32
JunkPacketCount = 8
Jmin = 10
Jmax = 20
[Peer]
PublicKey = AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE=
Endpoint = 203.0.113.10:51820
AllowedIPs = 0.0.0.0/0
`)
	cfg, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Jc != 8 {
		t.Fatalf("Jc alias = %d", cfg.Jc)
	}
}

func FuzzParseAmneziaWG(f *testing.F) {
	valid, err := os.ReadFile(filepath.Join("testdata", "awg31.valid.conf"))
	if err != nil {
		f.Fatal(err)
	}
	f.Add(valid)
	incomplete, err := os.ReadFile(filepath.Join("testdata", "awg31.incomplete.conf"))
	if err != nil {
		f.Fatal(err)
	}
	f.Add(incomplete)
	f.Add([]byte(""))
	f.Add([]byte("[Interface]\nPrivateKey = x\n"))
	f.Fuzz(func(t *testing.T, raw []byte) {
		cfg, err := Parse(raw)
		if err != nil {
			if strings.Contains(err.Error(), cfg.PrivateKey) && cfg.PrivateKey != "" {
				t.Fatalf("parse error leaked private key")
			}
			return
		}
		_ = cfg
	})
}
