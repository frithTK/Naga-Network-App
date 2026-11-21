package control

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"naga.network/core/engine"
	"naga.network/core/profile"
	"naga.network/core/storage"
)

func TestProxySubscriptionImportsWithoutStoppingNeighborAndRefreshes(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	active := profile.Profile{ID: "active", Config: []byte(`{"route":{"final":"direct"},"outbounds":[{"type":"direct","tag":"direct"}]}`)}
	if err := store.Save(active); err != nil {
		t.Fatal(err)
	}
	running := &testRuntime{status: engine.Connected}
	controller := NewRuntimeController(&store, testAdapter{runtime: running})
	controller.runtime, controller.profileID = running, active.ID
	const node = "vless://00000000-0000-4000-8000-000000000001@example.invalid:443?security=tls&fp=firefox#First"
	const hy2 = "\nhy2://test@example.invalid:8443#Second"
	const unavailable = "\nvless://00000000-0000-4000-8000-000000000001@example.invalid:443?fm=%7B%7D"
	var contents atomic.Value
	contents.Store(node + hy2 + unavailable)
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("profile-title", "Test profile")
		w.Header().Set("profile-update-interval", "2")
		w.Write([]byte(base64.StdEncoding.EncodeToString([]byte(contents.Load().(string)))))
	}))
	defer origin.Close()
	client, source := publicSubscriptionClient(origin)
	api := Server{Storage: &store, Runtime: controller, Subscriptions: client}
	post := func(endpoint string, request any) *httptest.ResponseRecorder {
		t.Helper()
		var body []byte
		if request != nil {
			body, _ = json.Marshal(request)
		}
		recorder := httptest.NewRecorder()
		api.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body)))
		return recorder
	}
	preview := post("/v1/subscriptions/preview", map[string]string{"url": source})
	var metadata Preview
	if preview.Code != 200 || json.Unmarshal(preview.Body.Bytes(), &metadata) != nil ||
		metadata.Import == nil || metadata.Import.Imported != 2 || metadata.Import.Ignored["final_mask"] != 1 {
		t.Fatalf("preview failed: %s", preview.Body.String())
	}
	added := post("/v1/profiles/import", map[string]string{"url": source})
	id := profile.IDFromSourceURL(source)
	if added.Code != 200 || running.Status() != engine.Connected || !controller.IsActive(active.ID) {
		t.Fatalf("import failed or stopped the active runtime: %d / %s", added.Code, running.Status())
	}
	saved, err := store.Load(id)
	if err != nil || saved.SourceURL != source || saved.ImportFormat != "base64_uri_list" || saved.UpdateInterval.Hours() != 2 {
		t.Fatal("import did not keep source and metadata")
	}
	saved.SelectedMode = "First"
	if err := store.Save(saved); err != nil {
		t.Fatal(err)
	}
	contents.Store(strings.Replace(node, "#First", "#Renamed", 1) + hy2 + unavailable)
	refresh := post("/v1/profiles/"+id+"/refresh", nil)
	saved, _ = store.Load(id)
	if refresh.Code != 200 || saved.SelectedMode != "Renamed" || saved.Revision != 2 || running.Status() != engine.Connected {
		t.Fatalf("refresh failed to preserve selection: %d / %s", refresh.Code, saved.SelectedMode)
	}
	contents.Store(strings.TrimSpace(hy2))
	reimport := post("/v1/profiles/import", map[string]string{"url": source})
	after, _ := store.Load(id)
	if reimport.Code != 422 || !bytes.Equal(saved.Config, after.Config) || after.SelectedMode != "Renamed" ||
		running.Status() != engine.Connected {
		t.Fatalf("reimport replaced the selected node: %d", reimport.Code)
	}
}
