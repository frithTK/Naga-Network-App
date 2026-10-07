package profile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseEnvelopeExtractsSingBoxAndAmnezia(t *testing.T) {
	conf, err := os.ReadFile(filepath.Join("amneziawg", "testdata", "awg31.valid.conf"))
	if err != nil {
		t.Fatal(err)
	}
	singbox := []byte(`{"route":{"final":"Mode"},"outbounds":[{"type":"selector","tag":"Mode","outbounds":["ee"],"default":"ee"},{"type":"vless","tag":"ee"}]}`)
	raw, err := json.Marshal(map[string]any{
		"v":       1,
		"app":     "nagavpn",
		"singbox": json.RawMessage(singbox),
		"amnezia": map[string]any{"conf": string(conf)},
	})
	if err != nil {
		t.Fatal(err)
	}
	env, ok, err := ParseEnvelope(raw)
	if err != nil || !ok {
		t.Fatalf("ParseEnvelope() = ok=%v err=%v", ok, err)
	}
	if err := ValidateSingBoxConfig(env.SingBox); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(env.AmneziaConf), "[Interface]") {
		t.Fatalf("amnezia conf = %q", env.AmneziaConf)
	}
	if string(RuntimeSingBoxConfig(raw)) != string(env.SingBox) {
		t.Fatal("RuntimeSingBoxConfig did not unwrap envelope")
	}
}

func TestParseEnvelopeAcceptsNagaNetworkApp(t *testing.T) {
	raw := []byte(`{"v":1,"app":"naga-network","singbox":{"outbounds":[{"type":"direct","tag":"direct"}]},"amnezia":null}`)
	env, ok, err := ParseEnvelope(raw)
	if err != nil || !ok {
		t.Fatalf("ParseEnvelope() = ok=%v err=%v", ok, err)
	}
	if env.App != EnvelopeApp {
		t.Fatalf("app = %q", env.App)
	}
}

func TestParseEnvelopeAllowsNullAmnezia(t *testing.T) {
	raw := []byte(`{"v":1,"app":"nagavpn","singbox":{"outbounds":[{"type":"direct","tag":"direct"}]},"amnezia":null}`)
	env, ok, err := ParseEnvelope(raw)
	if err != nil || !ok {
		t.Fatalf("ParseEnvelope() = ok=%v err=%v", ok, err)
	}
	if len(env.AmneziaConf) != 0 {
		t.Fatalf("amnezia = %q", env.AmneziaConf)
	}
}

func TestParseEnvelopeRejectsUnknownVersion(t *testing.T) {
	raw := []byte(`{"v":2,"app":"nagavpn","singbox":{"outbounds":[{"type":"direct"}]}}`)
	_, ok, err := ParseEnvelope(raw)
	if !ok || err == nil {
		t.Fatal("expected version error")
	}
}

func TestParseEnvelopeIgnoresPlainSingBox(t *testing.T) {
	raw := []byte(`{"outbounds":[{"type":"direct","tag":"direct"}]}`)
	_, ok, err := ParseEnvelope(raw)
	if ok || err != nil {
		t.Fatalf("plain sing-box recognized: ok=%v err=%v", ok, err)
	}
}

func TestSingBoxProfilesExpandsEnvelope(t *testing.T) {
	conf, err := os.ReadFile(filepath.Join("amneziawg", "testdata", "awg31.valid.conf"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(map[string]any{
		"v":       1,
		"app":     "nagavpn",
		"singbox": json.RawMessage(`{"outbounds":[{"type":"vless","tag":"ee"}]}`),
		"amnezia": map[string]any{"conf": string(conf)},
	})
	if err != nil {
		t.Fatal(err)
	}
	values := []Profile{{ID: "bundle", Engine: EngineSingBox, Config: raw}}
	sing := SingBoxProfiles(values)
	if len(sing) != 1 || !strings.Contains(string(sing[0].Config), `"vless"`) || strings.Contains(string(sing[0].Config), `"app"`) {
		t.Fatalf("SingBoxProfiles = %#v", sing)
	}
	awg := AmneziaWGProfiles(values)
	if len(awg) != 1 || awg[0].ID != "bundle" || !strings.Contains(string(awg[0].Config), "[Peer]") {
		t.Fatalf("AmneziaWGProfiles = %#v", awg)
	}
	if strings.Contains(string(values[0].Config), `"app":"nagavpn"`) == false {
		t.Fatal("source envelope was rewritten")
	}
}

const olcKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestParseEnvelopeOlcRTCOptional(t *testing.T) {
	base := `{"v":1,"app":"naga-network","singbox":{"outbounds":[{"type":"direct","tag":"direct"}]},"amnezia":null`
	for _, raw := range []string{base + `}`, base + `,"olcrtc":null}`} {
		env, ok, err := ParseEnvelope([]byte(raw))
		if err != nil || !ok || env.OlcRTC != nil {
			t.Fatalf("raw=%s ok=%v err=%v olc=%v", raw, ok, err, env.OlcRTC)
		}
		if OlcRTCError([]byte(raw)) != nil {
			t.Fatal(OlcRTCError([]byte(raw)))
		}
	}
}

func TestParseEnvelopeOlcRTCProfiles(t *testing.T) {
	raw := []byte(`{"v":1,"app":"naga-network","singbox":{"outbounds":[{"type":"direct","tag":"direct"}]},"amnezia":null,"olcrtc":{"label":"Naga CZ","profiles":[{"name":"jitsi","provider":"jitsi","transport":"datachannel","room":"https://meet.example/room","key":"` + olcKey + `"},{"name":"telemost","provider":"telemost","transport":"vp8channel","room":"room-1","key":"` + olcKey + `"}],"uris":["olcrtc://jitsi?datachannel@https://meet.example/room#` + olcKey + `$Naga CZ"]}}`)
	env, ok, err := ParseEnvelope(raw)
	if err != nil || !ok || env.OlcRTC == nil || len(env.OlcRTC.Profiles) != 2 {
		t.Fatalf("ok=%v err=%v olc=%v", ok, err, env.OlcRTC)
	}
	if env.OlcRTC.Label != "Naga CZ" || env.OlcRTC.Profiles[0].Provider != "jitsi" || env.OlcRTC.Profiles[1].Provider != "telemost" {
		t.Fatalf("olc=%+v", env.OlcRTC)
	}
	got := RuntimeOlcRTC(raw)
	got.Profiles[0].Name = "changed"
	if env.OlcRTC.Profiles[0].Name == "changed" {
		t.Fatal("RuntimeOlcRTC aliased profiles")
	}
}

func TestParseEnvelopeOlcRTCKeyMismatchDoesNotDropSingBox(t *testing.T) {
	other := "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"
	raw := []byte(`{"v":1,"app":"naga-network","singbox":{"outbounds":[{"type":"direct","tag":"direct"}]},"olcrtc":{"profiles":[{"name":"a","provider":"jitsi","transport":"datachannel","room":"https://meet.example/room","key":"` + olcKey + `"},{"name":"b","provider":"jitsi","transport":"datachannel","room":"https://meet.example/room","key":"` + other + `"}],"uris":[]}}`)
	env, ok, err := ParseEnvelope(raw)
	if err != nil || !ok || len(env.SingBox) == 0 || env.OlcRTC != nil {
		t.Fatalf("ok=%v err=%v sing=%d olc=%v", ok, err, len(env.SingBox), env.OlcRTC)
	}
	if OlcRTCError(raw) == nil {
		t.Fatal("expected olcrtc field error")
	}
}

func TestParseEnvelopeBrokenOlcRTCKeepsSingBox(t *testing.T) {
	raw := []byte(`{"v":1,"app":"naga-network","singbox":{"outbounds":[{"type":"direct","tag":"direct"}]},"amnezia":null,"olcrtc":{"profiles":"nope"}}`)
	env, ok, err := ParseEnvelope(raw)
	if err != nil || !ok || env.OlcRTC != nil || len(env.SingBox) == 0 {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if OlcRTCError(raw) == nil {
		t.Fatal("expected field error")
	}
}
