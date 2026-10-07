package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"naga.network/core/diagnostics"
	"naga.network/core/engine"
	"naga.network/core/policy"
	"naga.network/core/profile"
	"naga.network/core/storage"
	"naga.network/core/subscription"
)

type testRuntime struct {
	status engine.Status
}

func (r *testRuntime) Stop() error {
	r.status = engine.Stopped
	return nil
}

func (r *testRuntime) Status() engine.Status      { return r.status }
func (r *testRuntime) Stats() engine.TrafficStats { return engine.TrafficStats{} }

func publicSubscriptionClient(server *httptest.Server) (subscription.Client, string) {
	parsed, err := url.Parse(server.URL)
	if err != nil {
		panic(err)
	}
	base, ok := server.Client().Transport.(*http.Transport)
	if !ok {
		panic("httptest client transport")
	}
	transport := base.Clone()
	tlsConfig := base.TLSClientConfig.Clone()
	tlsConfig.ServerName = parsed.Hostname()
	transport.TLSClientConfig = tlsConfig
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		var dialer net.Dialer
		return dialer.DialContext(ctx, network, parsed.Host)
	}
	return subscription.Client{HTTPClient: &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}}, "https://example.invalid/profile"
}

type testAdapter struct {
	runtime *testRuntime
}

type flakyAdapter struct {
	mu         sync.Mutex
	runtime    *testRuntime
	failStarts int
	starts     int
}

func (a *flakyAdapter) Type() engine.Type { return engine.SingBox }

func (a *flakyAdapter) Validate(context.Context, []byte) error { return nil }

func (a *flakyAdapter) Start(context.Context, []byte) (engine.Runtime, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.starts++
	if a.failStarts > 0 {
		a.failStarts--
		return nil, errors.New("simulated runtime start failure")
	}
	a.runtime.status = engine.Connected
	return a.runtime, nil
}

func (a *flakyAdapter) setFailures(count int) {
	a.mu.Lock()
	a.failStarts = count
	a.mu.Unlock()
}

func (a *flakyAdapter) startCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.starts
}

func (testAdapter) Type() engine.Type                      { return engine.SingBox }
func (testAdapter) Validate(context.Context, []byte) error { return nil }
func (a testAdapter) Start(context.Context, []byte) (engine.Runtime, error) {
	a.runtime.status = engine.Connected
	return a.runtime, nil
}

func TestPreviewReturnsSafeMetadata(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Subscription-Userinfo", "upload=10; download=20; total=0; expire=1796169599")
		w.Header().Set("profile-title", "Naga RF")
		w.Header().Set("profile-update-interval", "12")
		w.Header().Set("isp-name", "NagaVPN")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"outbounds":[{"type":"direct","tag":"direct"}]}`)
	}))
	defer server.Close()

	client, publicURL := publicSubscriptionClient(server)
	api := Server{
		Subscriptions: client,
		Token:         "test-token",
		Now:           func() time.Time { return time.Unix(1700000000, 0).UTC() },
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/subscriptions/preview", strings.NewReader(`{"url":"`+publicURL+`"}`))
	request.Header.Set("Authorization", "Bearer test-token")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	api.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, forbidden := range []string{server.URL, "outbounds", "direct"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("response leaked %q: %s", forbidden, body)
		}
	}
	for _, expected := range []string{"Naga RF", "NagaVPN", "sing-box", "\"can_connect\":true", "\"update_interval_seconds\":1036800"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("response missing %q: %s", expected, body)
		}
	}
}

func TestPreviewRequiresAuthorization(t *testing.T) {
	api := Server{Token: "test-token"}
	request := httptest.NewRequest(http.MethodPost, "/v1/subscriptions/preview", strings.NewReader(`{"url":"https://example.com/profile"}`))
	response := httptest.NewRecorder()

	api.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestAuthorizationRejectsMissingWrongAndWhitespaceTokens(t *testing.T) {
	api := Server{Token: "test-token"}
	cases := []struct {
		name   string
		method string
		path   string
		auth   string
		want   int
	}{
		{name: "health missing", method: http.MethodGet, path: "/v1/health", want: http.StatusUnauthorized},
		{name: "health wrong", method: http.MethodGet, path: "/v1/health", auth: "Bearer other", want: http.StatusUnauthorized},
		{name: "health whitespace", method: http.MethodGet, path: "/v1/health", auth: "Bearer  test-token", want: http.StatusUnauthorized},
		{name: "health trailing space", method: http.MethodGet, path: "/v1/health", auth: "Bearer test-token ", want: http.StatusUnauthorized},
		{name: "health correct", method: http.MethodGet, path: "/v1/health", auth: "Bearer test-token", want: http.StatusOK},
		{name: "put missing", method: http.MethodPut, path: "/v1/policy", want: http.StatusUnauthorized},
		{name: "delete missing", method: http.MethodDelete, path: "/v1/profiles/profile-x", want: http.StatusUnauthorized},
		{name: "options skips auth", method: http.MethodOptions, path: "/v1/policy", want: http.StatusNoContent},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, nil)
			if test.auth != "" {
				request.Header.Set("Authorization", test.auth)
			}
			response := httptest.NewRecorder()
			api.ServeHTTP(response, request)
			if response.Code != test.want {
				t.Fatalf("status = %d, want %d, body = %s", response.Code, test.want, response.Body.String())
			}
			if strings.Contains(response.Body.String(), "test-token") {
				t.Fatalf("response leaked token: %s", response.Body.String())
			}
		})
	}
}

func TestCORSAllowsOnlyConfiguredOrigin(t *testing.T) {
	api := Server{AllowedOrigin: "http://127.0.0.1:8090"}

	allowed := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	allowed.Header.Set("Origin", "http://127.0.0.1:8090")
	allowedResponse := httptest.NewRecorder()
	api.ServeHTTP(allowedResponse, allowed)
	if allowedResponse.Header().Get("Access-Control-Allow-Origin") != "http://127.0.0.1:8090" {
		t.Fatalf("allowed origin header = %q", allowedResponse.Header().Get("Access-Control-Allow-Origin"))
	}

	foreign := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	foreign.Header.Set("Origin", "https://evil.example")
	foreignResponse := httptest.NewRecorder()
	api.ServeHTTP(foreignResponse, foreign)
	if value := foreignResponse.Header().Get("Access-Control-Allow-Origin"); value != "" {
		t.Fatalf("foreign origin was allowed: %q", value)
	}

	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete} {
		preflight := httptest.NewRequest(http.MethodOptions, "/v1/policy", nil)
		preflight.Header.Set("Origin", "http://127.0.0.1:8090")
		preflight.Header.Set("Access-Control-Request-Method", method)
		preflightResponse := httptest.NewRecorder()
		api.ServeHTTP(preflightResponse, preflight)
		allowed := preflightResponse.Header().Get("Access-Control-Allow-Methods")
		if preflightResponse.Code != http.StatusNoContent ||
			!strings.Contains(allowed, method) ||
			preflightResponse.Header().Get("Access-Control-Allow-Origin") != "http://127.0.0.1:8090" {
			t.Fatalf("%s preflight = %d origin=%q methods=%q", method, preflightResponse.Code, preflightResponse.Header().Get("Access-Control-Allow-Origin"), allowed)
		}
	}

	foreignPreflight := httptest.NewRequest(http.MethodOptions, "/v1/policy", nil)
	foreignPreflight.Header.Set("Origin", "https://evil.example")
	foreignPreflight.Header.Set("Access-Control-Request-Method", http.MethodPut)
	foreignPreflightResponse := httptest.NewRecorder()
	api.ServeHTTP(foreignPreflightResponse, foreignPreflight)
	if foreignPreflightResponse.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("foreign preflight origin was allowed: %q", foreignPreflightResponse.Header().Get("Access-Control-Allow-Origin"))
	}
	if foreignPreflightResponse.Code != http.StatusNoContent {
		t.Fatalf("foreign preflight status = %d, want 204", foreignPreflightResponse.Code)
	}
	if methods := foreignPreflightResponse.Header().Get("Access-Control-Allow-Methods"); methods != "" {
		t.Fatalf("foreign preflight advertised methods: %q", methods)
	}
}

func TestImportPersistsProfileAndListHidesSecrets(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Subscription-Userinfo", "upload=0; download=0; total=0; expire=0")
		w.Header().Set("profile-title", "Saved Naga")
		_, _ = io.WriteString(w, `{"outbounds":[{"type":"direct"}]}`)
	}))
	defer server.Close()

	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	client, publicURL := publicSubscriptionClient(server)
	api := Server{
		Subscriptions: client,
		Storage:       &store,
		Now:           func() time.Time { return time.Unix(1700000000, 0).UTC() },
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/profiles/import", strings.NewReader(`{"url":"`+publicURL+`"}`))
	response := httptest.NewRecorder()

	api.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), publicURL) || strings.Contains(response.Body.String(), "outbounds") {
		t.Fatalf("import response leaked profile secret: %s", response.Body.String())
	}
	profiles, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 || profiles[0].Name != "Saved Naga" {
		t.Fatalf("saved profiles = %#v", profiles)
	}
	if profiles[0].ID != profile.IDFromSourceURL(publicURL) {
		t.Fatalf("profile ID = %q", profiles[0].ID)
	}

	listRequest := httptest.NewRequest(http.MethodGet, "/v1/profiles", nil)
	listResponse := httptest.NewRecorder()
	api.ServeHTTP(listResponse, listRequest)
	if listResponse.Code != http.StatusOK || strings.Contains(listResponse.Body.String(), publicURL) {
		t.Fatalf("list leaked profile secret: %d %s", listResponse.Code, listResponse.Body.String())
	}

	shareRequest := httptest.NewRequest(http.MethodGet, "/v1/profiles/"+profiles[0].ID+"/share", nil)
	shareResponse := httptest.NewRecorder()
	api.ServeHTTP(shareResponse, shareRequest)
	if shareResponse.Code != http.StatusOK {
		t.Fatalf("share status = %d, body = %s", shareResponse.Code, shareResponse.Body.String())
	}
	if !strings.Contains(shareResponse.Body.String(), publicURL) {
		t.Fatalf("share missing URL: %s", shareResponse.Body.String())
	}
	if strings.Contains(shareResponse.Body.String(), "outbounds") {
		t.Fatalf("share leaked profile config: %s", shareResponse.Body.String())
	}
}

func TestShareProfileRequiresStoredURL(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(profile.Profile{
		ID:     "profile-local",
		Name:   "Local",
		Engine: profile.EngineSingBox,
		Config: []byte(`{"outbounds":[{"type":"direct"}]}`),
	}); err != nil {
		t.Fatal(err)
	}
	api := Server{Storage: &store}
	request := httptest.NewRequest(http.MethodGet, "/v1/profiles/profile-local/share", nil)
	response := httptest.NewRecorder()
	api.ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "outbounds") {
		t.Fatalf("missing-URL share leaked config: %s", response.Body.String())
	}
}

func TestImportOlcRTCURIAndBrokenEnvelopeField(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	journal, err := diagnostics.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	api := Server{Storage: &store, Journal: journal}
	key := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	uri := "olcrtc://jitsi?datachannel@https://meet.example/room#" + key + "$Naga"
	body, err := json.Marshal(map[string]string{"config": uri, "name": "olcrtc"})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	api.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/profiles/import", strings.NewReader(string(body))))
	if response.Code != http.StatusOK {
		t.Fatalf("import = %d %s", response.Code, response.Body.String())
	}
	var imported struct {
		ProfileID string `json:"profile_id"`
		Preview   struct {
			Engine          string `json:"engine"`
			OlcRTCAvailable bool   `json:"olcrtc_available"`
		} `json:"preview"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &imported); err != nil {
		t.Fatal(err)
	}
	if imported.Preview.Engine != string(profile.EngineOlcRTC) || !imported.Preview.OlcRTCAvailable {
		t.Fatalf("preview = %+v", imported.Preview)
	}
	saved, err := store.Load(imported.ProfileID)
	if err != nil {
		t.Fatal(err)
	}
	if string(saved.Config) != uri || saved.SelectedMode != "olcRTC" {
		t.Fatalf("saved = %+v", saved)
	}
	share := httptest.NewRecorder()
	api.ServeHTTP(share, httptest.NewRequest(http.MethodGet, "/v1/profiles/"+imported.ProfileID+"/share", nil))
	if share.Code != http.StatusOK || !strings.Contains(share.Body.String(), uri) || strings.Contains(share.Body.String(), "mode: cnc") {
		t.Fatalf("share = %d %s", share.Code, share.Body.String())
	}

	envelope := `{"v":1,"app":"naga-network","singbox":{"outbounds":[{"type":"direct","tag":"direct"}]},"olcrtc":{"profiles":"bad"}}`
	body, err = json.Marshal(map[string]string{"config": envelope})
	if err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	api.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/profiles/import", strings.NewReader(string(body))))
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), `"olcrtc_available":true`) {
		t.Fatalf("broken field import = %d %s", response.Code, response.Body.String())
	}
	entries, err := journal.Recent(20)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range entries {
		if entry.Event == "olcrtc_ignored" {
			found = true
			if strings.Contains(entry.Message, key) {
				t.Fatalf("journal leaked key: %+v", entry)
			}
		}
	}
	if !found {
		t.Fatal("expected olcrtc_ignored journal event")
	}
}

func TestImportSuppliedConfigKeepsSourceURLWithoutFetch(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	config := `{"outbounds":[{"type":"direct"}]}`
	source := "https://example.com/bundles/naga.json"
	body, err := json.Marshal(map[string]string{
		"config":                  config,
		"url":                     source,
		"profile_title":           "base64:TmFnYSBOZXR3b3Jr",
		"subscription_userinfo":   "upload=1; download=2; total=0; expire=0",
		"profile_update_interval": "2",
		"provider_name":           "Naga",
	})
	if err != nil {
		t.Fatal(err)
	}
	api := Server{Storage: &store}
	response := httptest.NewRecorder()
	api.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/profiles/import", strings.NewReader(string(body))))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), source) || strings.Contains(response.Body.String(), "outbounds") {
		t.Fatalf("import leaked profile: %s", response.Body.String())
	}
	profiles, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 {
		t.Fatalf("profiles = %d", len(profiles))
	}
	saved := profiles[0]
	if saved.SourceURL != source || saved.ID != profile.IDFromSourceURL(source) {
		t.Fatalf("source = %#v", saved)
	}
	if saved.Name != "Naga Network" || saved.ProviderName != "Naga" || saved.UpdateInterval != 48*time.Hour {
		t.Fatalf("metadata = %#v", saved)
	}
	if saved.Subscription.Upload != 1 || saved.Subscription.Download != 2 {
		t.Fatalf("usage = %#v", saved.Subscription)
	}
}

func TestImportSuppliedConfigRejectsPrivateURL(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]string{
		"config": `{"outbounds":[{"type":"direct"}]}`,
		"url":    "https://127.0.0.1/local.json",
	})
	if err != nil {
		t.Fatal(err)
	}
	api := Server{Storage: &store}
	response := httptest.NewRecorder()
	api.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/profiles/import", strings.NewReader(string(body))))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	profiles, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 0 {
		t.Fatalf("saved %d profiles", len(profiles))
	}
}

func TestRefreshUsesSuppliedConfigWithoutFetch(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source := "https://127.0.0.1/would-fail-if-fetched.json"
	original := profile.Profile{
		ID:        profile.IDFromSourceURL(source),
		Name:      "Old",
		SourceURL: source,
		Engine:    profile.EngineSingBox,
		Config:    []byte(`{"outbounds":[{"type":"direct","tag":"old"}]}`),
	}
	if err := store.Save(original); err != nil {
		t.Fatal(err)
	}
	config := `{"outbounds":[{"type":"direct","tag":"new"}]}`
	body, err := json.Marshal(map[string]string{
		"config":        config,
		"profile_title": "Fresh",
		"provider_name": "Naga",
	})
	if err != nil {
		t.Fatal(err)
	}
	api := Server{Storage: &store}
	response := httptest.NewRecorder()
	api.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/profiles/"+original.ID+"/refresh", strings.NewReader(string(body))))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	saved, err := store.Load(original.ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(saved.Config) != config || saved.Name != "Fresh" || saved.SourceURL != source || saved.ProviderName != "Naga" {
		t.Fatalf("saved = %#v", saved)
	}
	if saved.UpdateInterval != 24*time.Hour {
		t.Fatalf("interval = %s", saved.UpdateInterval)
	}
}

func TestImportConfigFromFileOmitsSourceURL(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	config := `{"outbounds":[{"type":"direct"}]}`
	body, err := json.Marshal(map[string]string{
		"config": config,
		"name":   `C:\Users\me\Downloads\estonia.json`,
	})
	if err != nil {
		t.Fatal(err)
	}
	api := Server{
		Storage: &store,
		Now:     func() time.Time { return time.Unix(1700000000, 0).UTC() },
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/profiles/import", strings.NewReader(string(body)))
	response := httptest.NewRecorder()
	api.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "outbounds") || strings.Contains(response.Body.String(), "C:\\") {
		t.Fatalf("import leaked file contents: %s", response.Body.String())
	}
	profiles, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 || profiles[0].Name != "estonia" || profiles[0].SourceURL != "" {
		t.Fatalf("saved = %#v", profiles)
	}
	if profiles[0].ID != profile.IDFromConfig([]byte(config)) {
		t.Fatalf("id = %q", profiles[0].ID)
	}

	list := httptest.NewRecorder()
	api.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/v1/profiles", nil))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"has_source_url":false`) || !strings.Contains(list.Body.String(), `"updated_at":"`) {
		t.Fatalf("list = %d %s", list.Code, list.Body.String())
	}

	refresh := httptest.NewRecorder()
	api.ServeHTTP(refresh, httptest.NewRequest(http.MethodPost, "/v1/profiles/"+profiles[0].ID+"/refresh", nil))
	if refresh.Code != http.StatusUnprocessableEntity {
		t.Fatalf("refresh = %d %s", refresh.Code, refresh.Body.String())
	}
}

func TestPreviewConfigRejectsInvalidFile(t *testing.T) {
	api := Server{}
	body, err := json.Marshal(map[string]string{"config": "not-a-vpn-profile"})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/subscriptions/preview", strings.NewReader(string(body)))
	response := httptest.NewRecorder()
	api.ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "not-a-vpn-profile") {
		t.Fatalf("preview leaked config: %s", response.Body.String())
	}
}

func TestShareProfileMissingID(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	api := Server{Storage: &store}
	request := httptest.NewRequest(http.MethodGet, "/v1/profiles/missing-profile/share", nil)
	response := httptest.NewRecorder()
	api.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestHealthIsAvailable(t *testing.T) {
	api := Server{}
	request := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	response := httptest.NewRecorder()

	api.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", response.Code, response.Body.String())
	}
	var payload healthResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Service != "naga-control" || payload.Status != "ok" || payload.RuntimeReady {
		t.Fatalf("health = %#v", payload)
	}
	if payload.Version == "" {
		t.Fatal("health version is empty")
	}
}

func TestHealthReportsConfiguredVersion(t *testing.T) {
	api := Server{Version: "1.2.4"}
	request := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	response := httptest.NewRecorder()
	api.ServeHTTP(response, request)
	var payload healthResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Version != "1.2.4" {
		t.Fatalf("version = %q", payload.Version)
	}
}

func TestHealthReportsRuntimeReady(t *testing.T) {
	api := Server{RuntimeReady: true, OlcRTCReady: true}
	request := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	response := httptest.NewRecorder()
	api.ServeHTTP(response, request)
	var payload healthResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.RuntimeReady || !payload.OlcRTCReady {
		t.Fatalf("health = %#v", payload)
	}
}

func TestClientEventEndpointRecordsUIFailures(t *testing.T) {
	journal, err := diagnostics.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()

	api := Server{Journal: journal}
	body := `{"level":"error","event":"subscription_download_failed","message":"загрузка подписки не удалась"}`
	request := httptest.NewRequest(http.MethodPost, "/v1/diagnostics/client", strings.NewReader(body))
	response := httptest.NewRecorder()
	api.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("client event status = %d, body = %s", response.Code, response.Body.String())
	}

	logs := httptest.NewRequest(http.MethodGet, "/v1/diagnostics/logs?limit=20", nil)
	logsResponse := httptest.NewRecorder()
	api.ServeHTTP(logsResponse, logs)
	logged := logsResponse.Body.String()
	if !strings.Contains(logged, `"event":"subscription_download_failed"`) ||
		!strings.Contains(logged, `"component":"client"`) {
		t.Fatalf("client failure not recorded: %s", logged)
	}
}

func TestDiagnosticsLogEndpointKeepsEventsAfterRuntimeStop(t *testing.T) {
	journal, err := diagnostics.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()
	journal.Info("runtime", "previous_session", "VPN был остановлен ранее", nil)

	api := Server{Journal: journal}
	stopRequest := httptest.NewRequest(http.MethodPost, "/v1/runtime/stop", nil)
	stopResponse := httptest.NewRecorder()
	api.ServeHTTP(stopResponse, stopRequest)
	if stopResponse.Code != http.StatusOK {
		t.Fatalf("stop status = %d, body = %s", stopResponse.Code, stopResponse.Body.String())
	}

	request := httptest.NewRequest(http.MethodGet, "/v1/diagnostics/logs?limit=20", nil)
	response := httptest.NewRecorder()
	api.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("diagnostics status = %d, body = %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if !strings.Contains(body, `"event":"previous_session"`) ||
		!strings.Contains(body, `"event":"mutation_completed"`) {
		t.Fatalf("persistent diagnostics missing events: %s", body)
	}
}

func TestPolicyRoutingAndApplicationsAPI(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	api := Server{Storage: &store}

	getPolicy := httptest.NewRequest(http.MethodGet, "/v1/policy", nil)
	getPolicyResponse := httptest.NewRecorder()
	api.ServeHTTP(getPolicyResponse, getPolicy)
	if getPolicyResponse.Code != http.StatusOK ||
		!strings.Contains(getPolicyResponse.Body.String(), `"mode":"auto"`) {
		t.Fatalf("default policy = %d %s", getPolicyResponse.Code, getPolicyResponse.Body.String())
	}

	putPolicy := httptest.NewRequest(http.MethodPut, "/v1/policy", strings.NewReader(`{"mode":"manual_strict","network_class":"wifi","tuic_fallback_enabled":false}`))
	putPolicy.Header.Set("Content-Type", "application/json")
	putPolicyResponse := httptest.NewRecorder()
	api.ServeHTTP(putPolicyResponse, putPolicy)
	if putPolicyResponse.Code != http.StatusOK ||
		!strings.Contains(putPolicyResponse.Body.String(), `"mode":"manual_strict"`) {
		t.Fatalf("updated policy = %d %s", putPolicyResponse.Code, putPolicyResponse.Body.String())
	}

	putRoutingEmpty := httptest.NewRequest(http.MethodPut, "/v1/routing", strings.NewReader(`{"mode":"selected_vpn","ru_direct":true,"apps":[]}`))
	putRoutingEmpty.Header.Set("Content-Type", "application/json")
	putRoutingEmptyResponse := httptest.NewRecorder()
	api.ServeHTTP(putRoutingEmptyResponse, putRoutingEmpty)
	if putRoutingEmptyResponse.Code != http.StatusUnprocessableEntity {
		t.Fatalf("empty selected_vpn = %d %s", putRoutingEmptyResponse.Code, putRoutingEmptyResponse.Body.String())
	}

	putRouting := httptest.NewRequest(http.MethodPut, "/v1/routing", strings.NewReader(`{"mode":"selected_vpn","ru_direct":true,"apps":[{"id":"firefox","display_name":"Firefox","platform":"linux","package_name_or_process_id":"firefox","route":"direct","enabled":true}]}`))
	putRouting.Header.Set("Content-Type", "application/json")
	putRoutingResponse := httptest.NewRecorder()
	api.ServeHTTP(putRoutingResponse, putRouting)
	if putRoutingResponse.Code != http.StatusOK ||
		!strings.Contains(putRoutingResponse.Body.String(), `"mode":"selected_vpn"`) {
		t.Fatalf("updated routing = %d %s", putRoutingResponse.Code, putRoutingResponse.Body.String())
	}

	putApp := httptest.NewRequest(http.MethodPut, "/v1/apps/firefox", strings.NewReader(`{"display_name":"Firefox","platform":"linux","package_name_or_process_id":"firefox","route":"direct","enabled":true}`))
	putApp.Header.Set("Content-Type", "application/json")
	putAppResponse := httptest.NewRecorder()
	api.ServeHTTP(putAppResponse, putApp)
	if putAppResponse.Code != http.StatusOK ||
		!strings.Contains(putAppResponse.Body.String(), `"id":"firefox"`) {
		t.Fatalf("saved app = %d %s", putAppResponse.Code, putAppResponse.Body.String())
	}

	getApps := httptest.NewRequest(http.MethodGet, "/v1/apps", nil)
	getAppsResponse := httptest.NewRecorder()
	api.ServeHTTP(getAppsResponse, getApps)
	if getAppsResponse.Code != http.StatusOK ||
		!strings.Contains(getAppsResponse.Body.String(), `"display_name":"Firefox"`) {
		t.Fatalf("saved apps = %d %s", getAppsResponse.Code, getAppsResponse.Body.String())
	}

	deleteApp := httptest.NewRequest(http.MethodDelete, "/v1/apps/firefox", nil)
	deleteAppResponse := httptest.NewRecorder()
	api.ServeHTTP(deleteAppResponse, deleteApp)
	if deleteAppResponse.Code != http.StatusNoContent {
		t.Fatalf("delete last selected app = %d %s", deleteAppResponse.Code, deleteAppResponse.Body.String())
	}
	getRouting := httptest.NewRequest(http.MethodGet, "/v1/routing", nil)
	getRoutingResponse := httptest.NewRecorder()
	api.ServeHTTP(getRoutingResponse, getRouting)
	if getRoutingResponse.Code != http.StatusOK ||
		!strings.Contains(getRoutingResponse.Body.String(), `"mode":"all_vpn"`) {
		t.Fatalf("delete last selected app must fall back to all_vpn = %d %s", getRoutingResponse.Code, getRoutingResponse.Body.String())
	}
}

func TestConnectionPolicyPUTIgnoresClientNetworkClass(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	connection := policy.DefaultConnectionPolicy()
	connection.NetworkClass = policy.NetworkEthernet
	if err := store.SaveConnectionPolicy(connection); err != nil {
		t.Fatal(err)
	}
	api := Server{Storage: &store}
	putPolicy := httptest.NewRequest(http.MethodPut, "/v1/policy", strings.NewReader(`{"mode":"manual_fallback","network_class":"wifi","tuic_fallback_enabled":false}`))
	putPolicy.Header.Set("Content-Type", "application/json")
	putPolicyResponse := httptest.NewRecorder()
	api.ServeHTTP(putPolicyResponse, putPolicy)
	if putPolicyResponse.Code != http.StatusOK {
		t.Fatalf("PUT policy = %d %s", putPolicyResponse.Code, putPolicyResponse.Body.String())
	}
	saved, err := store.LoadConnectionPolicy()
	if err != nil {
		t.Fatal(err)
	}
	if saved.NetworkClass != policy.NetworkEthernet {
		t.Fatalf("stored network class = %q, want ethernet", saved.NetworkClass)
	}
	if saved.Mode != policy.ConnectionManualFallback {
		t.Fatalf("stored mode = %q, want manual_fallback", saved.Mode)
	}
}

func TestConnectionPolicyMutationRestoresRuntimeAfterReconnectFailure(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	value := profile.Profile{
		ID:     "profile-policy-rollback",
		Engine: profile.EngineSingBox,
		Config: []byte(`{"outbounds":[{"type":"direct","tag":"direct"}]}`),
	}
	if err := store.Save(value); err != nil {
		t.Fatal(err)
	}
	adapter := &flakyAdapter{runtime: &testRuntime{status: engine.Stopped}}
	controller := NewRuntimeController(&store, adapter)
	if _, err := controller.Start(value.ID); err != nil {
		t.Fatal(err)
	}
	adapter.setFailures(1)

	api := Server{Storage: &store, Runtime: controller}
	request := httptest.NewRequest(http.MethodPut, "/v1/policy", strings.NewReader(`{"mode":"manual_strict","traffic_mode":"system_proxy"}`))
	response := httptest.NewRecorder()
	api.ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("policy update status = %d, body = %s", response.Code, response.Body.String())
	}
	saved, err := store.LoadConnectionPolicy()
	if err != nil {
		t.Fatal(err)
	}
	if saved.Mode != policy.ConnectionAuto {
		t.Fatalf("policy was not restored: %#v", saved)
	}
	if status := controller.Status(); status.Status != string(engine.Connected) {
		t.Fatalf("runtime was not restored: %#v", status)
	}
	if got := adapter.startCount(); got != 3 {
		t.Fatalf("start attempts = %d, want initial + failed reconnect + restore", got)
	}
	if _, err := controller.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestConnectionPolicyRestoreFailureLeavesStoppedState(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	value := profile.Profile{
		ID:     "profile-policy-restore-fail",
		Engine: profile.EngineSingBox,
		Config: []byte(`{"outbounds":[{"type":"direct","tag":"direct"}]}`),
	}
	if err := store.Save(value); err != nil {
		t.Fatal(err)
	}
	adapter := &flakyAdapter{runtime: &testRuntime{status: engine.Stopped}}
	controller := NewRuntimeController(&store, adapter)
	if _, err := controller.Start(value.ID); err != nil {
		t.Fatal(err)
	}
	adapter.setFailures(2)

	api := Server{Storage: &store, Runtime: controller}
	request := httptest.NewRequest(http.MethodPut, "/v1/policy", strings.NewReader(`{"mode":"manual_strict","traffic_mode":"system_proxy"}`))
	response := httptest.NewRecorder()
	api.ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("policy update status = %d, body = %s", response.Code, response.Body.String())
	}
	saved, err := store.LoadConnectionPolicy()
	if err != nil {
		t.Fatal(err)
	}
	if saved.Mode != policy.ConnectionAuto {
		t.Fatalf("policy was not restored in storage: %#v", saved)
	}
	if status := controller.Status(); status.Status == string(engine.Connected) {
		t.Fatalf("runtime should stay stopped after restore failure: %#v", status)
	}
}

func TestRoutingAndAppMutationsRestoreStateAfterReconnectFailure(t *testing.T) {
	tests := []struct {
		name  string
		path  string
		body  string
		check func(*testing.T, policy.RoutingPolicy)
	}{
		{
			name: "routing",
			path: "/v1/routing",
			body: `{"mode":"all_direct","ru_direct":false,"apps":[]}`,
			check: func(t *testing.T, saved policy.RoutingPolicy) {
				if saved.Mode != policy.RoutingAllVPN || !saved.RUDirect {
					t.Fatalf("routing policy was not restored: %#v", saved)
				}
			},
		},
		{
			name: "app",
			path: "/v1/apps/firefox",
			body: `{"display_name":"Firefox","platform":"linux","package_name_or_process_id":"firefox","route":"direct","enabled":true}`,
			check: func(t *testing.T, saved policy.RoutingPolicy) {
				if len(saved.Apps) != 0 {
					t.Fatalf("app route was not restored: %#v", saved.Apps)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, err := storage.New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			value := profile.Profile{
				ID:     "profile-routing-rollback-" + test.name,
				Engine: profile.EngineSingBox,
				Config: []byte(`{"outbounds":[{"type":"direct","tag":"direct"}]}`),
			}
			if err := store.Save(value); err != nil {
				t.Fatal(err)
			}
			adapter := &flakyAdapter{runtime: &testRuntime{status: engine.Stopped}}
			controller := NewRuntimeController(&store, adapter)
			if _, err := controller.Start(value.ID); err != nil {
				t.Fatal(err)
			}
			adapter.setFailures(1)

			api := Server{Storage: &store, Runtime: controller}
			request := httptest.NewRequest(http.MethodPut, test.path, strings.NewReader(test.body))
			response := httptest.NewRecorder()
			api.ServeHTTP(response, request)
			if response.Code != http.StatusConflict {
				t.Fatalf("mutation status = %d, body = %s", response.Code, response.Body.String())
			}
			saved, err := store.LoadRoutingPolicy()
			if err != nil {
				t.Fatal(err)
			}
			test.check(t, saved)
			if status := controller.Status(); status.Status != string(engine.Connected) {
				t.Fatalf("runtime was not restored: %#v", status)
			}
			if _, err := controller.Stop(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestProfileNodesAndModeSelection(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	value := profile.Profile{
		ID:         "profile-nodes",
		Name:       "Nodes",
		Config:     []byte(`{"route":{"final":"Mode"},"outbounds":[{"type":"selector","tag":"Mode","outbounds":["fast","direct"],"default":"direct"},{"type":"shadowsocks","tag":"fast"},{"type":"direct","tag":"direct"}]}`),
		ImportedAt: time.Now().UTC(),
	}
	if err := store.Save(value); err != nil {
		t.Fatal(err)
	}
	api := Server{Storage: &store}

	get := httptest.NewRequest(http.MethodGet, "/v1/profiles/profile-nodes/nodes", nil)
	getResponse := httptest.NewRecorder()
	api.ServeHTTP(getResponse, get)
	if getResponse.Code != http.StatusOK {
		t.Fatalf("list nodes status = %d, body = %s", getResponse.Code, getResponse.Body.String())
	}
	if body := getResponse.Body.String(); !strings.Contains(body, `"tag":"fast"`) || !strings.Contains(body, `"selected":false`) {
		t.Fatalf("unexpected nodes response: %s", body)
	}
	if strings.Contains(getResponse.Body.String(), "outbounds") {
		t.Fatal("nodes response exposed the raw outbound list")
	}

	selectRequest := httptest.NewRequest(http.MethodPost, "/v1/profiles/profile-nodes/mode", strings.NewReader(`{"mode":"fast"}`))
	selectRequest.Header.Set("Content-Type", "application/json")
	selectResponse := httptest.NewRecorder()
	api.ServeHTTP(selectResponse, selectRequest)
	if selectResponse.Code != http.StatusOK || !strings.Contains(selectResponse.Body.String(), `"selected":true`) {
		t.Fatalf("select mode response = %d, %s", selectResponse.Code, selectResponse.Body.String())
	}
	saved, err := store.Load(value.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.SelectedMode != "fast" {
		t.Fatalf("selected mode = %q", saved.SelectedMode)
	}
}

func TestOlcRTCNodesUseSubscriptionNames(t *testing.T) {
	key := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	raw := []byte(`{"v":1,"app":"naga-network","singbox":{"route":{"final":"Mode"},"outbounds":[{"type":"selector","tag":"Mode","outbounds":["direct"],"default":"direct"},{"type":"direct","tag":"direct"}]},"olcrtc":{"label":"Naga olcRTC","profiles":[{"name":"🇵🇱 Poland (PL) Jitsi","provider":"jitsi","transport":"datachannel","room":"https://meet.example/room","key":"` + key + `"},{"name":"🇨🇿 Czechia (CZ) Jitsi","provider":"jitsi","transport":"datachannel","room":"https://meet.example/cz","key":"` + key + `"}]}}`)
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	value := profile.Profile{ID: "profile-olc", Name: "Naga", Config: raw, ImportedAt: time.Now().UTC()}
	if err := store.Save(value); err != nil {
		t.Fatal(err)
	}
	candidates := olcRTCUnifiedCandidates([]profile.Profile{value})
	if len(candidates) != 2 || candidates[0].SourceTag != "🇵🇱 Poland (PL) Jitsi" || candidates[0].Country != "Poland" || candidates[0].Protocol != "olcRTC" || candidates[1].Country != "Czechia" {
		t.Fatalf("candidates = %+v", candidates)
	}

	api := Server{Storage: &store}
	get := httptest.NewRequest(http.MethodGet, "/v1/profiles/profile-olc/nodes", nil)
	getResponse := httptest.NewRecorder()
	api.ServeHTTP(getResponse, get)
	if getResponse.Code != http.StatusOK || !strings.Contains(getResponse.Body.String(), "🇵🇱 Poland (PL) Jitsi") || !strings.Contains(getResponse.Body.String(), "🇨🇿 Czechia (CZ) Jitsi") || strings.Contains(getResponse.Body.String(), `"tag":"olcRTC"`) {
		t.Fatalf("nodes = %d %s", getResponse.Code, getResponse.Body.String())
	}

	selectRequest := httptest.NewRequest(http.MethodPost, "/v1/profiles/profile-olc/mode", strings.NewReader(`{"mode":"🇨🇿 Czechia (CZ) Jitsi"}`))
	selectRequest.Header.Set("Content-Type", "application/json")
	selectResponse := httptest.NewRecorder()
	api.ServeHTTP(selectResponse, selectRequest)
	if selectResponse.Code != http.StatusOK {
		t.Fatalf("select = %d %s", selectResponse.Code, selectResponse.Body.String())
	}
	saved, err := store.Load(value.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.SelectedMode != "🇨🇿 Czechia (CZ) Jitsi" {
		t.Fatalf("selected mode = %q", saved.SelectedMode)
	}
}

func TestRefreshAndDeleteProfile(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Subscription-Userinfo", "upload=10; download=20; total=100; expire=1796169599")
		w.Header().Set("profile-title", "Refreshed Naga")
		_, _ = io.WriteString(w, `{"outbounds":[{"type":"direct","tag":"direct"}]}`)
	}))
	defer server.Close()

	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	client, publicURL := publicSubscriptionClient(server)
	value := profile.Profile{
		ID:         "profile-refresh",
		Name:       "Old Naga",
		SourceURL:  publicURL,
		Config:     []byte(`{"outbounds":[{"type":"direct","tag":"old"}]}`),
		ImportedAt: time.Unix(1700000000, 0).UTC(),
	}
	if err := store.Save(value); err != nil {
		t.Fatal(err)
	}
	api := Server{
		Subscriptions: client,
		Storage:       &store,
		Now:           func() time.Time { return time.Unix(1700001000, 0).UTC() },
	}

	refresh := httptest.NewRequest(http.MethodPost, "/v1/profiles/profile-refresh/refresh", nil)
	refreshResponse := httptest.NewRecorder()
	api.ServeHTTP(refreshResponse, refresh)
	if refreshResponse.Code != http.StatusOK || !strings.Contains(refreshResponse.Body.String(), "Refreshed Naga") {
		t.Fatalf("refresh status = %d, body = %s", refreshResponse.Code, refreshResponse.Body.String())
	}
	saved, err := store.Load(value.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Name != "Refreshed Naga" || string(saved.Config) == string(value.Config) {
		t.Fatalf("refreshed profile = %+v", saved)
	}

	remove := httptest.NewRequest(http.MethodDelete, "/v1/profiles/profile-refresh", nil)
	removeResponse := httptest.NewRecorder()
	api.ServeHTTP(removeResponse, remove)
	if removeResponse.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, body = %s", removeResponse.Code, removeResponse.Body.String())
	}
	if _, err := store.Load(value.ID); err == nil {
		t.Fatal("profile still exists after delete")
	}
	if _, err := os.Stat(filepath.Join(store.Root, ".history", value.ID)); !os.IsNotExist(err) {
		t.Fatalf("profile history still exists after API delete: %v", err)
	}
	if _, err := store.Rollback(value.ID); err == nil {
		t.Fatal("rollback resurrected a deleted profile")
	}

	again := httptest.NewRequest(http.MethodDelete, "/v1/profiles/profile-refresh", nil)
	againResponse := httptest.NewRecorder()
	api.ServeHTTP(againResponse, again)
	if againResponse.Code == http.StatusNoContent {
		t.Fatal("repeated delete succeeded after the profile was already gone")
	}
}

func TestActiveProfileRefreshUsesUpdatePathAndModeSwitchRemainsHot(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runtime := &testRuntime{status: engine.Stopped}
	value := profile.Profile{
		ID:         "profile-active",
		Name:       "Active",
		Config:     []byte(`{"route":{"final":"Mode"},"outbounds":[{"type":"selector","tag":"Mode","outbounds":["direct"],"default":"direct"},{"type":"direct","tag":"direct"}]}`),
		ImportedAt: time.Now().UTC(),
	}
	if err := store.Save(value); err != nil {
		t.Fatal(err)
	}
	controller := NewRuntimeController(&store, testAdapter{runtime: runtime})
	if _, err := controller.Start(value.ID); err != nil {
		t.Fatal(err)
	}
	api := Server{Storage: &store, Runtime: controller}

	refresh := httptest.NewRequest(http.MethodPost, "/v1/profiles/profile-active/refresh", nil)
	refreshResponse := httptest.NewRecorder()
	api.ServeHTTP(refreshResponse, refresh)
	if refreshResponse.Code != http.StatusUnprocessableEntity {
		t.Fatalf("active refresh should reach profile validation: %s", refreshResponse.Body.String())
	}

	selectRequest := httptest.NewRequest(http.MethodPost, "/v1/profiles/profile-active/mode", strings.NewReader(`{"mode":"direct"}`))
	selectResponse := httptest.NewRecorder()
	api.ServeHTTP(selectResponse, selectRequest)
	if selectResponse.Code != http.StatusConflict {
		t.Fatalf("active mode change status = %d, body = %s", selectResponse.Code, selectResponse.Body.String())
	}

	remove := httptest.NewRequest(http.MethodDelete, "/v1/profiles/profile-active", nil)
	removeResponse := httptest.NewRecorder()
	api.ServeHTTP(removeResponse, remove)
	if removeResponse.Code != http.StatusNoContent {
		t.Fatalf("active delete status = %d, body = %s", removeResponse.Code, removeResponse.Body.String())
	}
	if runtime.Status() != engine.Stopped {
		t.Fatalf("runtime status after delete = %s", runtime.Status())
	}
}

func TestReplaceProfileControlledReconnectsRuntime(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runtime := &testRuntime{status: engine.Stopped}
	old := profile.Profile{
		ID:     "profile-replace",
		Engine: profile.EngineSingBox,
		Config: []byte(`{"route":{"final":"Mode"},"outbounds":[{"type":"selector","tag":"Mode","outbounds":["old"],"default":"old"},{"type":"vless","tag":"old"}]}`),
	}
	if err := store.Save(old); err != nil {
		t.Fatal(err)
	}
	controller := NewRuntimeController(&store, testAdapter{runtime: runtime})
	if _, err := controller.Start(old.ID); err != nil {
		t.Fatal(err)
	}
	updated := old
	updated.Config = []byte(`{"route":{"final":"Mode"},"outbounds":[{"type":"selector","tag":"Mode","outbounds":["new"],"default":"new"},{"type":"vless","tag":"new"}]}`)
	if _, err := controller.ReplaceProfile(updated); err != nil {
		t.Fatal(err)
	}
	saved, err := store.Load(old.ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(saved.Config) != string(updated.Config) || saved.Revision != 2 {
		t.Fatalf("saved replacement = %#v", saved)
	}
	if runtime.Status() != engine.Connected {
		t.Fatalf("runtime status after replacement = %s", runtime.Status())
	}
	if _, err := controller.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestNodesProbeWithoutProfilesReturnsUnprocessable(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	controller := NewRuntimeController(&store, testAdapter{runtime: &testRuntime{status: engine.Stopped}})
	api := Server{Storage: &store, Runtime: controller}
	request := httptest.NewRequest(http.MethodPost, "/v1/nodes/probe", nil)
	response := httptest.NewRecorder()
	api.ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("probe without profiles = %d %s", response.Code, response.Body.String())
	}
}

func TestNodesProbeReturnsLatencyWhileRuntimeIsStopped(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	value := profile.Profile{
		ID:     "profile-select",
		Engine: profile.EngineSingBox,
		Config: []byte(`{"route":{"final":"Mode"},"outbounds":[{"type":"selector","tag":"Mode","outbounds":["fast","slow"],"default":"fast"},{"type":"vless","tag":"fast"},{"type":"vless","tag":"slow"}]}`),
	}
	if err := store.Save(value); err != nil {
		t.Fatal(err)
	}
	runtime := &probingRuntime{outcomes: map[string]probeOutcome{
		value.ID + "::fast": {latency: 12},
		value.ID + "::slow": {latency: 88},
	}}
	controller := NewRuntimeController(&store, probingAdapter{runtime: runtime})
	api := Server{Storage: &store, Runtime: controller}
	request := httptest.NewRequest(http.MethodPost, "/v1/nodes/probe", nil)
	response := httptest.NewRecorder()
	api.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("idle probe = %d %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if !strings.Contains(body, `"latency_ms":12`) || !strings.Contains(body, `"latency_ms":88`) {
		t.Fatalf("idle probe response missing latencies: %s", body)
	}
	if controller.runtime != nil {
		t.Fatal("idle probe left VPN runtime running")
	}
	if runtime.stops() != 1 {
		t.Fatalf("idle runtime stops = %d", runtime.stops())
	}
}

func TestNodesProbeReturnsLatencyWhenRuntimeIsNotMerged(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	value := profile.Profile{
		ID:     "profile-select",
		Engine: profile.EngineSingBox,
		Config: []byte(`{"route":{"final":"Mode"},"outbounds":[{"type":"selector","tag":"Mode","outbounds":["fast","slow"],"default":"fast"},{"type":"vless","tag":"fast"},{"type":"vless","tag":"slow"}]}`),
	}
	if err := store.Save(value); err != nil {
		t.Fatal(err)
	}
	runtime := &probingRuntime{outcomes: map[string]probeOutcome{
		"fast": {latency: 12},
		"slow": {latency: 88},
	}}
	controller := NewRuntimeController(&store, probingAdapter{runtime: runtime})
	if _, err := controller.Start(value.ID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = controller.Stop() })
	api := Server{Storage: &store, Runtime: controller}
	request := httptest.NewRequest(http.MethodPost, "/v1/nodes/probe", nil)
	response := httptest.NewRecorder()
	api.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("probe = %d %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if !strings.Contains(body, `"latency_ms":12`) || !strings.Contains(body, `"latency_ms":88`) {
		t.Fatalf("probe response missing latencies: %s", body)
	}
}

func TestNodesIncludeProbeCacheAfterStart(t *testing.T) {
	store, value := saveAutoProbeProfile(t)
	vlessTag := value.ID + "::vless"
	tuicTag := value.ID + "::tuic"
	runtime := &probingRuntime{outcomes: map[string]probeOutcome{
		vlessTag: {latency: 42},
		tuicTag:  {latency: 5},
	}}
	controller := NewRuntimeController(&store, probingAdapter{runtime: runtime})
	if _, err := controller.Start(value.ID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = controller.Stop() })
	api := Server{Storage: &store, Runtime: controller}
	request := httptest.NewRequest(http.MethodGet, "/v1/nodes", nil)
	response := httptest.NewRecorder()
	api.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("nodes = %d %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if !strings.Contains(body, `"probe_status":"healthy"`) || !strings.Contains(body, `"latency_ms":42`) {
		t.Fatalf("nodes missing probe cache: %s", body)
	}
}

func TestRuntimeSelectAutoUsesPolicyNotClashGroup(t *testing.T) {
	store, value := saveAutoProbeProfile(t)
	vlessTag := value.ID + "::vless"
	tuicTag := value.ID + "::tuic"
	runtime := &probingRuntime{outcomes: map[string]probeOutcome{
		vlessTag: {latency: 42},
		tuicTag:  {latency: 5},
	}}
	controller := NewRuntimeController(&store, probingAdapter{runtime: runtime})
	if _, err := controller.Start(value.ID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = controller.Stop() })
	api := Server{Storage: &store, Runtime: controller}
	request := httptest.NewRequest(http.MethodPost, "/v1/runtime/select", strings.NewReader(`{"auto":true}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	api.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("select auto = %d %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "not selectable") {
		t.Fatalf("auto select leaked clash error: %s", response.Body.String())
	}
	if runtime.selectedTag() != vlessTag {
		t.Fatalf("selected = %q", runtime.selectedTag())
	}
}

func TestDiscoveredAppsNever500(t *testing.T) {
	api := Server{}
	request := httptest.NewRequest(http.MethodGet, "/v1/apps/discovered", nil)
	response := httptest.NewRecorder()
	api.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"apps"`) {
		t.Fatalf("discovered apps = %d %s", response.Code, response.Body.String())
	}
}

func awgTestdata(t *testing.T, name string) []byte {
	t.Helper()
	payload, err := os.ReadFile(filepath.Join("..", "profile", "amneziawg", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func TestPreviewAndImportAmneziaWG(t *testing.T) {
	conf := awgTestdata(t, "awg31.valid.conf")
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("profile-title", "Naga AWG")
		_, _ = w.Write(conf)
	}))
	defer server.Close()

	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	client, publicURL := publicSubscriptionClient(server)
	api := Server{
		Subscriptions: client,
		Storage:       &store,
		Now:           func() time.Time { return time.Unix(1700000000, 0).UTC() },
	}

	preview := httptest.NewRecorder()
	api.ServeHTTP(preview, httptest.NewRequest(http.MethodPost, "/v1/subscriptions/preview", strings.NewReader(`{"url":"`+publicURL+`"}`)))
	if preview.Code != http.StatusOK {
		t.Fatalf("preview = %d %s", preview.Code, preview.Body.String())
	}
	if !strings.Contains(preview.Body.String(), `"engine":"amneziawg"`) {
		t.Fatalf("preview missing engine: %s", preview.Body.String())
	}
	if strings.Contains(preview.Body.String(), "PrivateKey") || strings.Contains(preview.Body.String(), string(conf)) {
		t.Fatalf("preview leaked conf: %s", preview.Body.String())
	}

	imported := httptest.NewRecorder()
	api.ServeHTTP(imported, httptest.NewRequest(http.MethodPost, "/v1/profiles/import", strings.NewReader(`{"url":"`+publicURL+`"}`)))
	if imported.Code != http.StatusOK {
		t.Fatalf("import = %d %s", imported.Code, imported.Body.String())
	}
	profiles, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 || profiles[0].Engine != profile.EngineAmneziaWG {
		t.Fatalf("saved = %#v", profiles)
	}
	if string(profiles[0].Config) != string(conf) {
		t.Fatal("imported AWG config was rewritten")
	}

	nodes := httptest.NewRecorder()
	api.ServeHTTP(nodes, httptest.NewRequest(http.MethodGet, "/v1/profiles/"+profiles[0].ID+"/nodes", nil))
	if nodes.Code != http.StatusOK || !strings.Contains(nodes.Body.String(), `"tag":"AmneziaWG"`) {
		t.Fatalf("profile nodes = %d %s", nodes.Code, nodes.Body.String())
	}

	unified := httptest.NewRecorder()
	api.ServeHTTP(unified, httptest.NewRequest(http.MethodGet, "/v1/nodes", nil))
	if unified.Code != http.StatusOK {
		t.Fatalf("unified nodes = %d %s", unified.Code, unified.Body.String())
	}
	if !strings.Contains(unified.Body.String(), `"protocol":"AmneziaWG"`) {
		t.Fatalf("AWG-only nodes missing leaf: %s", unified.Body.String())
	}
}

func TestPreviewAndImportNagaEnvelope(t *testing.T) {
	conf := awgTestdata(t, "awg31.valid.conf")
	singbox := json.RawMessage(`{"route":{"final":"Mode"},"outbounds":[{"type":"selector","tag":"Mode","outbounds":["ee"],"default":"ee"},{"type":"vless","tag":"ee"}]}`)
	body, err := json.Marshal(map[string]any{
		"v":       1,
		"app":     "nagavpn",
		"singbox": singbox,
		"amnezia": map[string]any{"conf": string(conf)},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("profile-title", "Naga Network")
		_, _ = w.Write(body)
	}))
	defer server.Close()

	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	client, publicURL := publicSubscriptionClient(server)
	api := Server{
		Subscriptions: client,
		Storage:       &store,
		Now:           func() time.Time { return time.Unix(1700000000, 0).UTC() },
	}
	deep := "nagavpn://install-config?url=" + url.QueryEscape(publicURL)
	payload, err := json.Marshal(map[string]string{"url": deep})
	if err != nil {
		t.Fatal(err)
	}

	preview := httptest.NewRecorder()
	api.ServeHTTP(preview, httptest.NewRequest(http.MethodPost, "/v1/subscriptions/preview", strings.NewReader(string(payload))))
	if preview.Code != http.StatusOK {
		t.Fatalf("preview = %d %s", preview.Code, preview.Body.String())
	}
	if !strings.Contains(preview.Body.String(), `"engine":"sing-box"`) {
		t.Fatalf("preview = %s", preview.Body.String())
	}
	if strings.Contains(preview.Body.String(), "PrivateKey") || strings.Contains(preview.Body.String(), `"app"`) {
		t.Fatalf("preview leaked envelope: %s", preview.Body.String())
	}

	imported := httptest.NewRecorder()
	api.ServeHTTP(imported, httptest.NewRequest(http.MethodPost, "/v1/profiles/import", strings.NewReader(string(payload))))
	if imported.Code != http.StatusOK {
		t.Fatalf("import = %d %s", imported.Code, imported.Body.String())
	}
	profiles, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 || profiles[0].Engine != profile.EngineSingBox {
		t.Fatalf("saved = %#v", profiles)
	}
	if string(profiles[0].Config) != string(body) {
		t.Fatal("imported envelope was rewritten")
	}
	if profiles[0].SourceURL != publicURL {
		t.Fatalf("SourceURL = %q", profiles[0].SourceURL)
	}

	nodes := httptest.NewRecorder()
	api.ServeHTTP(nodes, httptest.NewRequest(http.MethodGet, "/v1/profiles/"+profiles[0].ID+"/nodes", nil))
	if nodes.Code != http.StatusOK || !strings.Contains(nodes.Body.String(), `"tag":"ee"`) || !strings.Contains(nodes.Body.String(), `"tag":"AmneziaWG"`) {
		t.Fatalf("profile nodes = %d %s", nodes.Code, nodes.Body.String())
	}

	unified := httptest.NewRecorder()
	api.ServeHTTP(unified, httptest.NewRequest(http.MethodGet, "/v1/nodes", nil))
	if unified.Code != http.StatusOK {
		t.Fatalf("unified nodes = %d %s", unified.Code, unified.Body.String())
	}
	if !strings.Contains(unified.Body.String(), `"protocol":"AmneziaWG"`) {
		t.Fatalf("unified missing AWG: %s", unified.Body.String())
	}
}

func TestPreviewRejectsPlainWireGuard(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(awgTestdata(t, "awg31.plain-wg.conf"))
	}))
	defer server.Close()
	client, publicURL := publicSubscriptionClient(server)
	api := Server{Subscriptions: client}
	response := httptest.NewRecorder()
	api.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/subscriptions/preview", strings.NewReader(`{"url":"`+publicURL+`"}`)))
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "wireguard profile is not AmneziaWG") {
		t.Fatalf("error = %s", response.Body.String())
	}
	if strings.Contains(response.Body.String(), "PrivateKey") {
		t.Fatalf("error leaked key: %s", response.Body.String())
	}
}

func TestUnifiedNodesKeepSingBoxWhenAmneziaWGIsStored(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	jsonProfile := profile.Profile{
		ID:     "profile-json",
		Engine: profile.EngineSingBox,
		Config: []byte(`{"route":{"final":"Mode"},"outbounds":[{"type":"selector","tag":"Mode","outbounds":["ee-vless"],"default":"ee-vless"},{"type":"vless","tag":"ee-vless"},{"type":"direct","tag":"direct"}]}`),
	}
	awgProfile := profile.Profile{
		ID:     "profile-awg",
		Engine: profile.EngineAmneziaWG,
		Config: awgTestdata(t, "awg31.valid.conf"),
	}
	if err := store.Save(jsonProfile); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(awgProfile); err != nil {
		t.Fatal(err)
	}
	api := Server{Storage: &store}
	response := httptest.NewRecorder()
	api.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/nodes", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if !strings.Contains(body, "ee-vless") {
		t.Fatalf("missing VLESS leaf: %s", body)
	}
	if !strings.Contains(body, `"protocol":"AmneziaWG"`) {
		t.Fatalf("AWG missing from mixed unified nodes: %s", body)
	}
}

func TestSafeFetchErrorMapsTransportFailures(t *testing.T) {
	cases := []struct {
		err  string
		want string
	}{
		{"fetch subscription: x509: certificate signed by unknown authority", "subscription tls certificate could not be verified"},
		{"fetch subscription: dial tcp: lookup example.invalid: no such host", "subscription dns lookup failed"},
		{"fetch subscription: dial tcp: i/o timeout", "subscription endpoint timed out"},
		{"fetch subscription: connectex: A connection attempt failed because the connected party did not properly respond after a period of time", "subscription endpoint timed out"},
		{"fetch subscription: subscription destination is not allowed", "subscription URL host is not allowed"},
		{"fetch subscription: connectex: No connection could be made because the target machine actively refused it", "subscription endpoint blocked"},
		{"fetch subscription: read tcp: connection reset by peer", "subscription endpoint blocked"},
	}
	for _, tc := range cases {
		got := safeFetchError(errors.New(tc.err))
		if got != tc.want {
			t.Fatalf("%q -> %q, want %q", tc.err, got, tc.want)
		}
	}
}

func TestLogFetchErrorWritesClassWithoutURL(t *testing.T) {
	journal, err := diagnostics.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = journal.Close() })
	api := Server{Journal: journal}
	api.logFetchError(errors.New("fetch subscription: dial tcp: connectex: No connection could be made because the target machine actively refused it https://secret.example/path.json"))
	entries, err := journal.Recent(10)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range entries {
		if entry.Event != "fetch_failed" {
			continue
		}
		found = true
		if entry.Message != "subscription endpoint blocked" {
			t.Fatalf("message = %q", entry.Message)
		}
		if got, _ := entry.Fields["class"].(string); got != "blocked" {
			t.Fatalf("class = %v", entry.Fields["class"])
		}
		if strings.Contains(entry.Message, "http") || strings.Contains(fmt.Sprint(entry.Fields), "secret.example") {
			t.Fatalf("leaked url: %+v", entry)
		}
	}
	if !found {
		t.Fatal("missing fetch_failed")
	}
}

func TestSafeFetchErrorRedactsAmneziaKeys(t *testing.T) {
	err := errors.New("amneziawg parse failed PrivateKey = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	got := safeFetchError(err)
	if strings.Contains(got, "AAAAAAAA") {
		t.Fatalf("leaked key: %q", got)
	}
	if !strings.Contains(got, "amneziawg") {
		t.Fatalf("dropped AWG error: %q", got)
	}
}

func TestRuntimeInspectRequiresAuthorization(t *testing.T) {
	api := Server{Token: "test-token"}
	request := httptest.NewRequest(http.MethodGet, "/v1/runtime/inspect", nil)
	response := httptest.NewRecorder()
	api.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestRuntimeInspectStoppedIsOKEmpty(t *testing.T) {
	api := Server{Token: "test-token"}
	request := httptest.NewRequest(http.MethodGet, "/v1/runtime/inspect", nil)
	request.Header.Set("Authorization", "Bearer test-token")
	response := httptest.NewRecorder()
	api.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if !strings.Contains(body, `"status":"stopped"`) {
		t.Fatalf("body = %s", body)
	}
	if strings.Contains(body, `"outbounds"`) {
		t.Fatalf("stopped inspect leaked outbounds: %s", body)
	}
}

func TestRuntimeInspectConnectedOmitsSecrets(t *testing.T) {
	store, value := saveAutoProbeProfile(t)
	vlessTag := value.ID + "::vless"
	tuicTag := value.ID + "::tuic"
	runtime := &probingRuntime{outcomes: map[string]probeOutcome{
		vlessTag: {latency: 42},
		tuicTag:  {latency: 5},
	}}
	controller := NewRuntimeController(&store, probingAdapter{runtime: runtime})
	if _, err := controller.Start(value.ID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = controller.Stop() })

	api := Server{Token: "test-token", Runtime: controller}
	request := httptest.NewRequest(http.MethodGet, "/v1/runtime/inspect", nil)
	request.Header.Set("Authorization", "Bearer test-token")
	response := httptest.NewRecorder()
	api.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if !strings.Contains(body, "Naga-Policy") {
		t.Fatalf("missing Naga-Policy: %s", body)
	}
	for _, forbidden := range []string{"://", "uuid", "password", "clash_api"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("inspect leaked %q: %s", forbidden, body)
		}
	}
}
