package control

import (
	"context"
	"path/filepath"
	"testing"

	"naga.network/core/engine"
	"naga.network/core/profile"
	"naga.network/core/storage"
)

func TestProbeOlcRTCSkipsMissingBinary(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const olcKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	raw := []byte(`{"v":1,"app":"naga-network","singbox":{"outbounds":[{"type":"direct","tag":"direct"}]},"olcrtc":{"label":"Naga CZ","profiles":[{"name":"jitsi","provider":"jitsi","transport":"datachannel","room":"https://meet.example/room","key":"` + olcKey + `"}]}}`)
	if err := store.Save(profile.Profile{
		ID:     "naga-envelope",
		Engine: profile.EngineSingBox,
		Config: raw,
	}); err != nil {
		t.Fatal(err)
	}
	controller := NewRuntimeController(&store, testAdapter{runtime: &testRuntime{status: engine.Stopped}})
	controller.OlcRTCBinary = filepath.Join(t.TempDir(), "missing-olcrtc")
	ran, err := controller.probeOlcRTC(context.Background())
	if ran {
		t.Fatalf("missing binary should not count as a probe run, err=%v", err)
	}
	if err != nil {
		t.Fatal(err)
	}
}
