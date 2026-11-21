package singbox

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"naga.network/core/profile"
)

func TestConnectionsKeepsBidirectionalCountersAndMapsRoute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/connections" || r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"connections":[{"metadata":{"network":"udp","destinationIP":"2001:db8::1","destinationPort":"27031","processPath":"/home/example/bin/dota (1000)"},"upload":900,"download":150,"chains":["Mode","runtime-tag"],"rule":"final","start":"2026-10-01T12:00:00Z"}]}`))
	}))
	defer server.Close()
	api := runtimeAPI{baseURL: server.URL, secret: "test-token", nodeMetadata: map[string]profile.Node{"runtime-tag": {Tag: "Test VPN"}}}
	result, err := api.connections(context.Background())
	if err != nil || !result.Available || result.Total != 1 || len(result.Rows) != 1 {
		t.Fatalf("snapshot: %#v, %v", result, err)
	}
	row := result.Rows[0]
	if row.Network != "udp" || row.Process != "dota" || row.Destination != "[2001:db8::1]:27031" ||
		row.Upload != 900 || row.Download != 150 || len(row.Chain) != 2 || row.Chain[1] != "Test VPN" {
		t.Fatalf("row: %#v", row)
	}
}
