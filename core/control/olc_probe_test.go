package control

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"naga.network/core/policy"
	olcprofile "naga.network/core/profile/olcrtc"
)

func TestOlcProbeURLUsesHTTPGenerate204(t *testing.T) {
	if olcProbeURL != "http://www.gstatic.com/generate_204" {
		t.Fatalf("olc probe URL = %q, want HTTP generate_204", olcProbeURL)
	}
	if olcMeasureTimeout != 24*time.Second {
		t.Fatalf("olc measure timeout = %s, want 24s for two shots", olcMeasureTimeout)
	}
}

func TestOlcProbeRecordsEachVariant(t *testing.T) {
	profiles := []olcprofile.Profile{
		{Name: "🇵🇱 Poland (PL) Jitsi", RuntimeAllowed: true},
		{Name: "🇵🇱 Poland (PL) Telemost", RuntimeAllowed: true},
		{Name: "🇨🇿 Czechia (CZ) Jitsi", RuntimeAllowed: true},
	}
	if tag := olcLiveProbeTag(profiles, "🇵🇱 Poland (PL) Telemost", "Auto"); tag != "🇵🇱 Poland (PL) Telemost" {
		t.Fatalf("live tag = %q", tag)
	}
	if tag := olcLiveProbeTag(profiles, olcprofile.NodeTag, "Auto"); tag != "" {
		t.Fatalf("shared session tag = %q", tag)
	}
	if tag := olcLiveProbeTag(profiles[:1], "", "olcRTC"); tag != "🇵🇱 Poland (PL) Jitsi" {
		t.Fatalf("single tag = %q", tag)
	}

	controller := &RuntimeController{}
	tags := olcprofile.Tags(profiles)
	controller.recordOlcProbe("profile-1", tags[0], 180, nil)
	controller.recordOlcProbe("profile-1", tags[2], 240, nil)
	entry, ok := probeCacheForCandidate(controller.probeCache, policy.Candidate{
		ProfileID:  "profile-1",
		SourceTag:  tags[2],
		RuntimeTag: tags[2],
	})
	if !ok || entry.LatencyMS != 240 || entry.Status != ProbeHealthy {
		t.Fatalf("czech probe = %+v ok=%v", entry, ok)
	}
	if _, ok := probeCacheForCandidate(controller.probeCache, policy.Candidate{
		ProfileID:  "profile-1",
		SourceTag:  tags[1],
		RuntimeTag: tags[1],
	}); ok {
		t.Fatal("unmeasured variant inherited another ping")
	}
}

func TestMeasureWarmHTTPLatencyReturnsSecondShot(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) == 2 {
			time.Sleep(45 * time.Millisecond)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	latency, err := measureWarmHTTPLatency(context.Background(), server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 2 {
		t.Fatalf("hits = %d, want 2", hits.Load())
	}
	if latency < 30 {
		t.Fatalf("latency = %d, want second (slower) shot", latency)
	}
}

func TestMeasureWarmHTTPLatencyFirstFailSkipsSecond(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Error(w, "fail", http.StatusInternalServerError)
	}))
	defer server.Close()

	latency, err := measureWarmHTTPLatency(context.Background(), server.Client(), server.URL)
	if err == nil {
		t.Fatal("expected first-shot error")
	}
	if latency != 0 {
		t.Fatalf("latency = %d, want 0 on first fail", latency)
	}
	if hits.Load() != 1 {
		t.Fatalf("hits = %d, want 1", hits.Load())
	}
}

func TestMeasureWarmHTTPLatencySecondFailKeepsFirst(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) == 1 {
			time.Sleep(40 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Error(w, "fail", http.StatusInternalServerError)
	}))
	defer server.Close()

	latency, err := measureWarmHTTPLatency(context.Background(), server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 2 {
		t.Fatalf("hits = %d, want 2", hits.Load())
	}
	if latency < 25 {
		t.Fatalf("latency = %d, want first shot after second 500", latency)
	}
}

func TestMeasureWarmHTTPLatencyCancelAfterFirstKeepsFirst(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	base := server.Client().Transport
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		resp, err := base.RoundTrip(req)
		cancel()
		return resp, err
	})}
	latency, err := measureWarmHTTPLatency(ctx, client, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if latency <= 0 {
		t.Fatalf("latency = %d, want first shot", latency)
	}
	if hits.Load() != 1 {
		t.Fatalf("hits = %d, want 1 after parent cancel", hits.Load())
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
