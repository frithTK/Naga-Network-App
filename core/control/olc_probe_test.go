package control

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"naga.network/core/engine"
	"naga.network/core/profile"
	"naga.network/core/storage"
)

func TestHTTPProbeRequires204(t *testing.T) {
	for _, status := range []int{http.StatusNoContent, http.StatusOK, http.StatusFound, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
			}))
			defer server.Close()
			_, err := measureHTTPLatency(context.Background(), server.Client(), server.URL)
			if (err == nil) != (status == http.StatusNoContent) {
				t.Fatalf("status %d: error = %v", status, err)
			}
		})
	}
}

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
