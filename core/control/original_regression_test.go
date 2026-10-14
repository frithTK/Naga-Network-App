package control

import (
	"bytes"
	"encoding/json"
	"naga.network/core/profile"
	"naga.network/core/storage"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestImportStoresOriginalAmneziaWGBytes(t *testing.T) {
	conf, err := os.ReadFile("../profile/amneziawg/testdata/awg31.valid.conf")
	if err != nil {
		t.Fatal(err)
	}
	original := append([]byte{0xef, 0xbb, 0xbf}, conf...)
	original = append(original, []byte("\n  \n")...)
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]string{"config": string(original), "name": "original.conf"})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	(Server{Storage: &store}).ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/profiles/import", bytes.NewReader(body)))
	if response.Code != http.StatusOK {
		t.Fatalf("import=%d %s", response.Code, response.Body.String())
	}
	var imported importResponse
	if err = json.Unmarshal(response.Body.Bytes(), &imported); err != nil {
		t.Fatal(err)
	}
	saved, err := store.Load(imported.ProfileID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Engine != profile.EngineAmneziaWG || !bytes.Equal(saved.Config, original) {
		t.Fatal("changed original bytes or engine")
	}
}
