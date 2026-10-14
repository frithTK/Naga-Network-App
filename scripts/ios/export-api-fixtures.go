//go:build ignore

// Export public API responses for the Swift client's contract tests.
// Run from the repository root: go run ./scripts/ios/export-api-fixtures.go
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"time"

	"naga.network/core/control"
	"naga.network/core/storage"
)

func main() {
	root, err := os.MkdirTemp("", "naga-swift-fixtures-")
	must(err)
	defer os.RemoveAll(root)
	store, err := storage.New(root)
	must(err)
	api := control.Server{
		Storage: &store, Token: "fixture-token", Version: "1.2.5",
		Now: func() time.Time { return time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC) },
	}
	request := func(method, path, body string) []byte {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer fixture-token")
		w := httptest.NewRecorder()
		api.ServeHTTP(w, r)
		if w.Code >= 400 && path != "/v1/does-not-exist" {
			panic(fmt.Sprintf("%s %s: %s", method, path, w.Body.String()))
		}
		var result any
		must(json.Unmarshal(w.Body.Bytes(), &result))
		data, err := json.MarshalIndent(result, "", "  ")
		must(err)
		return append(data, '\n')
	}
	config := `{"outbounds":[{"type":"selector","tag":"proxy","outbounds":["CZ VLESS"],"default":"CZ VLESS"},{"type":"vless","tag":"CZ VLESS","server":"example.invalid","server_port":443,"uuid":"00000000-0000-0000-0000-000000000000"}],"route":{"final":"proxy"}}`
	body, err := json.Marshal(map[string]string{"config": config, "name": "Fixture profile"})
	must(err)
	imported := request(http.MethodPost, "/v1/profiles/import", string(body))
	var saved struct {
		ID string `json:"profile_id"`
	}
	must(json.Unmarshal(imported, &saved))
	dest := filepath.Join("ios", "Tests", "NagaControlClientTests", "Fixtures")
	must(os.MkdirAll(dest, 0o755))
	fixtures := map[string][]byte{
		"import":        imported,
		"preview":       request(http.MethodPost, "/v1/subscriptions/preview", string(body)),
		"health":        request(http.MethodGet, "/v1/health", ""),
		"profiles":      request(http.MethodGet, "/v1/profiles", ""),
		"nodes":         request(http.MethodGet, "/v1/nodes", ""),
		"profile-nodes": request(http.MethodGet, "/v1/profiles/"+saved.ID+"/nodes", ""),
		"runtime":       request(http.MethodGet, "/v1/runtime", ""),
		"policy":        request(http.MethodGet, "/v1/policy", ""),
		"routing":       request(http.MethodGet, "/v1/routing", ""),
		"logs":          request(http.MethodGet, "/v1/diagnostics/logs", ""),
		"error":         request(http.MethodGet, "/v1/does-not-exist", ""),
	}
	for name, data := range fixtures {
		must(os.WriteFile(filepath.Join(dest, name+".json"), data, 0o644))
	}
	fmt.Printf("Exported %d fixtures from core/control.Server\n", len(fixtures))
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
