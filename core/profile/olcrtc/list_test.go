package olcrtc

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseDocumentGhostlaneList(t *testing.T) {
	raw := "#name: Naga olcRTC\n#refresh: 1h\n\nolcrtc://jitsi?datachannel@https://meet.example/room#" + testKey + "$ignored\n##name: Naga CZ\n"
	cfg, err := ParseDocument([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Label != "Naga olcRTC" || cfg.Profiles[0].Name != "Naga CZ" || cfg.Profiles[0].Provider != ProviderJitsi {
		t.Fatalf("cfg = %+v profile=%+v", cfg, cfg.Profiles[0])
	}
	if cfg.ShareURI() == "" || cfg.Profiles[0].Key != testKey {
		t.Fatal("uri was not kept")
	}
}

func TestParseDocumentImportsAllRoomsAndKeepsNames(t *testing.T) {
	otherKey := strings.Repeat("f", 64)
	first := "olcrtc://jitsi?datachannel@https://example.org/first#" + testKey + "$One"
	second := "olcrtc://telemost?vp8channel@second#" + otherKey + "$Two"
	input := "#name: Example\r\n" + first + "\r\n##name: First room\r\n" + second + "\r\n##name: Second room\r\n"
	cfg, err := ParseDocument([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Profiles) != 2 || len(cfg.URIs) != 2 || cfg.Label != "Example" {
		t.Fatalf("incorrect number of profiles or label: %d / %d / %q", len(cfg.Profiles), len(cfg.URIs), cfg.Label)
	}
	if cfg.Profiles[0].Name != "First room" || cfg.Profiles[1].Name != "Second room" || cfg.Profiles[0].Key != testKey || cfg.Profiles[1].Key != otherKey {
		t.Fatal("profile names or keys were mixed")
	}
	if _, err := Build(cfg); err == nil {
		t.Fatal("group with different keys must not use the first key for both rooms")
	}
	selected, _, err := Narrow(cfg, "Second room")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := Build(selected)
	if err != nil || !strings.Contains(doc.Body, otherKey) || strings.Contains(doc.Body, testKey) {
		t.Fatal("selected room did not retain its own key")
	}
}

func TestParseFieldImportsAllURIs(t *testing.T) {
	first := "olcrtc://jitsi?datachannel@https://example.org/first#" + testKey + "$One"
	second := "olcrtc://telemost?vp8channel@second#" + testKey + "$Two"
	raw, err := json.Marshal(map[string]any{"uris": []string{first, second}})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := ParseField(raw)
	if err != nil || len(cfg.Profiles) != 2 || cfg.Profiles[0].Name != "One" || cfg.Profiles[1].Name != "Two" {
		t.Fatalf("profiles not preserved: cfg=%v err=%v", cfg != nil, err)
	}
}

func TestParseFieldDoesNotDiscardURIParameters(t *testing.T) {
	uri := "olcrtc://jitsi?datachannel&mtu=1200@https://meet.example/room#" + testKey + "$room"
	profile := map[string]string{"name": "room", "provider": "jitsi", "transport": "datachannel", "room": "https://meet.example/room", "key": testKey}
	for _, field := range []map[string]any{
		{"uris": []string{uri}},
		{"profiles": []map[string]string{profile}, "uris": []string{uri}},
	} {
		raw, err := json.Marshal(field)
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := ParseField(raw)
		if err != nil || len(cfg.Profiles) != 0 || cfg.Ignored != 1 {
			t.Fatalf("URI with unsupported parameters must be reported: cfg=%+v err=%v", cfg, err)
		}
	}
	if _, err := ParseDocument([]byte(uri)); err != ErrTransportOff {
		t.Fatalf("standalone URI with unsupported parameters should be rejected: %v", err)
	}
}

func TestParseDocumentLimitsAndReportsInvalidLaterURI(t *testing.T) {
	first := "olcrtc://jitsi?datachannel@https://example.org/first#" + testKey
	cfg, err := ParseDocument([]byte(first + "\nolcrtc://unsupported?vp8channel@room#" + testKey + "\n##name: wrong"))
	if err != nil || cfg.Ignored != 1 || len(cfg.Profiles) != 1 || cfg.Profiles[0].Name == "wrong" {
		t.Fatal("invalid room was not isolated from the supported room")
	}
	for _, input := range []string{strings.Repeat("x", maxDocumentBytes+1), first + "\n" + strings.Repeat("x", maxDocumentLineBytes+1)} {
		if _, err := ParseDocument([]byte(input)); err == nil {
			t.Fatal("accepted malformed or oversized document")
		}
	}
}
