package storage

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"naga.network/core/policy"
	"naga.network/core/profile"
)

func TestStoreSaveLoadAndList(t *testing.T) {
	root := filepath.Join(t.TempDir(), "profiles")
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	want := profile.Profile{
		ID:             "profile-test",
		Name:           "Naga Network",
		SourceURL:      "https://example.com/profile",
		Engine:         profile.EngineSingBox,
		Config:         []byte(`{"outbounds":[{"type":"direct"}]}`),
		ImportedAt:     time.Unix(1700000000, 0).UTC(),
		UpdateInterval: 24 * time.Hour,
	}
	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}

	got, err := store.Load(want.ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Config) != string(want.Config) || got.SourceURL != want.SourceURL {
		t.Fatalf("loaded profile differs: %#v", got)
	}
	entries, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].ID != want.ID {
		t.Fatalf("list = %#v", entries)
	}
	info, err := os.Stat(filepath.Join(root, want.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != fileMode {
		t.Fatalf("mode = %o, want %o", info.Mode().Perm(), fileMode)
	}
}

func TestStoreVersionedSaveAndRollback(t *testing.T) {
	store, err := New(filepath.Join(t.TempDir(), "profiles"))
	if err != nil {
		t.Fatal(err)
	}
	first := profile.Profile{
		ID:     "profile-versioned",
		Engine: profile.EngineSingBox,
		Config: []byte(`{"version":1}`),
	}
	if err := store.Save(first); err != nil {
		t.Fatal(err)
	}
	second := first
	second.Config = []byte(`{"version":2}`)
	if err := store.SaveVersioned(second); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(loaded.Config) != string(second.Config) || loaded.Revision != 2 {
		t.Fatalf("versioned profile = %#v", loaded)
	}
	restored, err := store.Rollback(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(restored.Config) != string(first.Config) {
		t.Fatalf("rollback config = %s", restored.Config)
	}
	if err := store.Delete(first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(store.Root, ".history", first.ID)); !os.IsNotExist(err) {
		t.Fatalf("profile history still exists after delete: %v", err)
	}
}

func TestStoreDeleteRejectsPathTraversalAndRepeatedDelete(t *testing.T) {
	store, err := New(filepath.Join(t.TempDir(), "profiles"))
	if err != nil {
		t.Fatal(err)
	}
	value := profile.Profile{
		ID:     "profile-delete",
		Engine: profile.EngineSingBox,
		Config: []byte(`{"outbounds":[{"type":"direct"}]}`),
	}
	if err := store.Save(value); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveVersioned(value); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete("profile-delete"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load("profile-delete"); err == nil {
		t.Fatal("deleted profile still loads")
	}
	if err := store.Delete("profile-delete"); err == nil {
		t.Fatal("repeated delete succeeded")
	}
	if _, err := store.Rollback("profile-delete"); err == nil {
		t.Fatal("rollback after delete succeeded")
	}
	for _, id := range []string{"../escape", "a/b", ".", "..", ""} {
		if err := store.Delete(id); err == nil {
			t.Fatalf("path traversal id %q was accepted", id)
		}
	}
	missing := profile.Profile{
		ID:     "profile-no-history",
		Engine: profile.EngineSingBox,
		Config: []byte(`{"outbounds":[{"type":"direct"}]}`),
	}
	if err := store.Save(missing); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(missing.ID); err != nil {
		t.Fatalf("delete without history: %v", err)
	}
}

func TestStoreConcurrentPatchConnectionAndSaveRouting(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if err := store.PatchConnectionPolicy(func(current *policy.ConnectionPolicy) error {
			current.NetworkClass = policy.NetworkEthernet
			return nil
		}); err != nil {
			t.Errorf("patch connection: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		routing := policy.DefaultRoutingPolicy()
		routing.Mode = policy.RoutingAllDirect
		if err := store.SaveRoutingPolicy(routing); err != nil {
			t.Errorf("save routing: %v", err)
		}
	}()
	wg.Wait()

	connection, err := store.LoadConnectionPolicy()
	if err != nil {
		t.Fatal(err)
	}
	routing, err := store.LoadRoutingPolicy()
	if err != nil {
		t.Fatal(err)
	}
	if connection.NetworkClass != policy.NetworkEthernet {
		t.Fatalf("network class = %q, want ethernet", connection.NetworkClass)
	}
	if routing.Mode != policy.RoutingAllDirect {
		t.Fatalf("routing mode = %q, want all_direct", routing.Mode)
	}
	if connection.Revision < 1 {
		t.Fatalf("connection revision = %d", connection.Revision)
	}
	if routing.Revision < 1 {
		t.Fatalf("routing revision = %d", routing.Revision)
	}
}

func TestLoadRoutingPolicyMigratesLegacyRUDirect(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, ".settings")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	legacy := []byte(`{"routing":{"mode":"all_vpn","ru_direct":false,"apps":[]}}`)
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	routing, err := store.LoadRoutingPolicy()
	if err != nil {
		t.Fatal(err)
	}
	if routing.ProviderRules || routing.BuiltinRU || !routing.BuiltinPrivate {
		t.Fatalf("migrated routing = %#v", routing)
	}
}
