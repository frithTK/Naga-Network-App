package olcrtc

import "testing"

func TestTagsAndNarrowSelectOneProfile(t *testing.T) {
	profiles := []Profile{
		{Name: "🇵🇱 Poland (PL) Jitsi"},
		{Name: "🇵🇱 Poland (PL) Telemost"},
		{Name: "🇨🇿 Czechia (CZ) Jitsi"},
		{Name: "🇵🇱 Poland (PL) Jitsi"},
	}
	tags := Tags(profiles)
	if tags[0] != "🇵🇱 Poland (PL) Jitsi" || tags[3] != "🇵🇱 Poland (PL) Jitsi 2" {
		t.Fatalf("tags = %#v", tags)
	}
	index, ok := Index(profiles, "profile-1::🇵🇱 Poland (PL) Telemost")
	if !ok || index != 1 {
		t.Fatalf("index = %d ok=%v", index, ok)
	}
	index, ok = Index(profiles, NodeTag)
	if !ok || index != -1 {
		t.Fatalf("legacy tag index = %d ok=%v", index, ok)
	}

	cfg := &Config{Profiles: []Profile{
		{Name: "🇵🇱 Poland (PL) Jitsi", Provider: ProviderJitsi, RuntimeAllowed: true},
		{Name: "🇨🇿 Czechia (CZ) Jitsi", Provider: ProviderJitsi, RuntimeAllowed: true},
	}}
	narrowed, tag, err := Narrow(cfg, "🇨🇿 Czechia (CZ) Jitsi")
	if err != nil || tag != "🇨🇿 Czechia (CZ) Jitsi" || len(narrowed.Profiles) != 1 || narrowed.Profiles[0].Name != "🇨🇿 Czechia (CZ) Jitsi" {
		t.Fatalf("narrow = %+v tag=%s err=%v", narrowed, tag, err)
	}
	if len(cfg.Profiles) != 2 {
		t.Fatal("narrow mutated the source config")
	}
	all, tag, err := Narrow(cfg, NodeTag)
	if err != nil || tag != NodeTag || len(all.Profiles) != 2 {
		t.Fatalf("all = %d tag=%s err=%v", len(all.Profiles), tag, err)
	}
}
