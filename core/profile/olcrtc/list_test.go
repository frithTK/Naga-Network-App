package olcrtc

import "testing"

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
