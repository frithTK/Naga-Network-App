package singbox

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"naga.network/core/engine"
	"naga.network/core/profile"
)

func TestConnectivityProbeURLUsesHTTPGenerate204(t *testing.T) {
	if connectivityProbeURL != "http://www.gstatic.com/generate_204" {
		t.Fatalf("probe URL = %q, want HTTP generate_204", connectivityProbeURL)
	}
}

func TestRuntimeAPIReadsHTTPTrafficStreamAndProxySelection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("test server does not support flushing")
		}
		switch r.URL.Path {
		case "/traffic":
			fmt.Fprintln(w, `{"up":10,"down":100}`)
			flusher.Flush()
			<-r.Context().Done()
		case "/proxies":
			fmt.Fprintln(w, `{"proxies":{"Mode":{"type":"Selector","now":"Auto","history":[{"delay":12}]},"Auto":{"type":"URLTest","now":"🇪🇪 Estonia (EE) TUIC","history":[{"delay":15}]},"🇪🇪 Estonia (EE) TUIC":{"type":"TUIC","history":[{"delay":87}]}}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	api := &runtimeAPI{
		baseURL: server.URL,
		secret:  "secret",
		client: http.Client{
			Transport: &http.Transport{Proxy: nil},
			Timeout:   500 * time.Millisecond,
		},
		modeTag: "Mode",
		nodeMetadata: map[string]profile.Node{
			"🇪🇪 Estonia (EE) TUIC": profile.NodeMetadata("🇪🇪 Estonia (EE) TUIC", "tuic"),
		},
		details: engine.RuntimeDetails{Status: "checking"},
	}
	api.Start()
	defer api.Close()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		stats := api.Stats()
		details := api.Details()
		if stats.Available && stats.Upload == 10 && stats.Download == 100 &&
			details.Node == "🇪🇪 Estonia (EE) TUIC" && details.Country == "Estonia" &&
			details.Protocol == "TUIC" && details.Status == "active" && details.Latency == 87 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("telemetry did not converge: stats=%+v details=%+v", api.Stats(), api.Details())
}

func TestRuntimeAPIProbesSpecificOutbound(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !validProbeRequest(w, r) {
			return
		}
		requests++
		delay := 240
		if requests == 2 {
			delay = 52
		}
		fmt.Fprintf(w, "{\"delay\":%d}\n", delay)
	}))
	defer server.Close()

	latency, err := probeTestAPI(server).Probe(context.Background(), "profile::node", 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if latency != 52 {
		t.Fatalf("latency = %d, want 52", latency)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2", requests)
	}
}

func TestRuntimeAPIProbeFirstHTTPErrorSkipsSecond(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !validProbeRequest(w, r) {
			return
		}
		requests++
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	_, err := probeTestAPI(server).Probe(context.Background(), "profile::node", 2*time.Second)
	if err == nil {
		t.Fatal("expected first-shot HTTP error")
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
}

func TestRuntimeAPIProbeFirstZeroDelaySkipsSecond(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !validProbeRequest(w, r) {
			return
		}
		requests++
		fmt.Fprintln(w, `{"delay":0}`)
	}))
	defer server.Close()

	_, err := probeTestAPI(server).Probe(context.Background(), "profile::node", 2*time.Second)
	if err == nil {
		t.Fatal("expected first-shot zero delay error")
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
}

func TestRuntimeAPIProbeSecondFailureKeepsFirst(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !validProbeRequest(w, r) {
			return
		}
		requests++
		if requests == 1 {
			fmt.Fprintln(w, `{"delay":240}`)
			return
		}
		http.Error(w, "failed", http.StatusInternalServerError)
	}))
	defer server.Close()

	latency, err := probeTestAPI(server).Probe(context.Background(), "profile::node", 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if latency != 240 {
		t.Fatalf("latency = %d, want 240", latency)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2", requests)
	}
}

func TestRuntimeAPIProbeCancelAfterFirstKeepsFirst(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !validProbeRequest(w, r) {
			return
		}
		requests++
		if requests == 1 {
			fmt.Fprintln(w, `{"delay":240}`)
			return
		}
		http.Error(w, "second probe should not be required", http.StatusInternalServerError)
	}))
	defer server.Close()

	api := probeTestAPI(server)
	api.client.Transport = &cancelAfterFirstTransport{
		base:   api.client.Transport,
		cancel: cancel,
	}
	latency, err := api.Probe(ctx, "profile::node", 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if latency != 240 {
		t.Fatalf("latency = %d, want 240", latency)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
}

type cancelAfterFirstTransport struct {
	base   http.RoundTripper
	cancel context.CancelFunc
	n      int
}

func (t *cancelAfterFirstTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	resp, err := base.RoundTrip(req)
	if err != nil {
		return resp, err
	}
	t.n++
	if t.n != 1 {
		return resp, nil
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, err
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	t.cancel()
	return resp, nil
}

func probeTestAPI(server *httptest.Server) *runtimeAPI {
	return &runtimeAPI{
		baseURL: server.URL,
		secret:  "secret",
		client: http.Client{
			Transport: &http.Transport{Proxy: nil},
			Timeout:   500 * time.Millisecond,
		},
	}
}

func validProbeRequest(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get("Authorization") != "Bearer secret" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return false
	}
	if r.URL.Path != "/proxies/profile%3A%3Anode/delay" && r.URL.Path != "/proxies/profile::node/delay" {
		http.NotFound(w, r)
		return false
	}
	if r.URL.Query().Get("url") != connectivityProbeURL || r.URL.Query().Get("timeout") != "2000" {
		http.Error(w, "unexpected query", http.StatusBadRequest)
		return false
	}
	return true
}

func TestClashLeafSelectionIgnoresSelectorHistory(t *testing.T) {
	leaf, latency := clashLeafSelection(map[string]clashProxyState{
		"Mode": {
			Type: "Selector",
			Now:  "Auto",
			History: []struct {
				Delay int `json:"delay"`
			}{{Delay: 12}},
		},
		"Auto": {
			Type: "URLTest",
			Now:  "Estonia TUIC",
			History: []struct {
				Delay int `json:"delay"`
			}{{Delay: 180}},
		},
		"Estonia TUIC": {
			Type: "TUIC",
			History: []struct {
				Delay int `json:"delay"`
			}{{Delay: 42}},
		},
	}, "Mode")
	if leaf != "Estonia TUIC" || latency != 42 {
		t.Fatalf("leaf=%q latency=%d", leaf, latency)
	}

	leaf, latency = clashLeafSelection(map[string]clashProxyState{
		"Mode": {Type: "Selector", Now: "Auto"},
		"Auto": {
			Type: "URLTest",
			Now:  "Estonia TUIC",
			History: []struct {
				Delay int `json:"delay"`
			}{{Delay: 180}},
		},
		"Estonia TUIC": {Type: "TUIC"},
	}, "Mode")
	if leaf != "Estonia TUIC" || latency != 0 {
		t.Fatalf("missing leaf history: leaf=%q latency=%d", leaf, latency)
	}
}
