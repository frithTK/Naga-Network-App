package singbox

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"naga.network/core/diagnostics"
	"naga.network/core/profile"
)

func TestAdapterCheckConfigUsesConfigDir(t *testing.T) {
	parent := t.TempDir()
	adapter := Adapter{BinaryPath: "true", ConfigDir: parent}
	if err := adapter.checkConfig(context.Background(), []byte(`{"outbounds":[{"type":"direct","tag":"direct"}]}`)); err != nil {
		t.Fatal(err)
	}
}

func TestAdapterValidatesConfig(t *testing.T) {
	adapter := Adapter{}
	if err := adapter.Validate(context.Background(), []byte(`{"outbounds":[{"type":"direct"}]}`)); err != nil {
		t.Fatal(err)
	}
}

func TestAdapterRequiresBinaryForRuntime(t *testing.T) {
	_, err := (Adapter{}).Start(context.Background(), []byte(`{"outbounds":[{"type":"direct"}]}`))
	if err != ErrBinaryNotConfigured {
		t.Fatalf("expected ErrBinaryNotConfigured, got %v", err)
	}
}

func TestRuntimeLogWriterPersistsCompleteAndPartialLines(t *testing.T) {
	journal, err := diagnostics.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()
	writer := newRuntimeLogWriter(journal, "stderr")
	_, _ = writer.Write([]byte("INFO[0000] outbound started\nDEBUG[0000] dns response NOERROR\nERROR[0001] request https://secret.example/path token=hidden"))
	writer.Flush()

	entries, err := journal.Recent(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("entries = %#v", entries)
	}
	if entries[0].Level != "info" || entries[1].Level != "info" || entries[2].Level != "error" {
		t.Fatalf("levels = %q, %q, %q", entries[0].Level, entries[1].Level, entries[2].Level)
	}
	encoded := entries[2].Message
	for _, secret := range []string{"secret.example", "hidden"} {
		if strings.Contains(encoded, secret) {
			t.Fatalf("runtime log leaked %q: %s", secret, encoded)
		}
	}
}

func TestLookupSelectionPathResolvesAmneziaWGDisplayTag(t *testing.T) {
	raw, err := json.Marshal(map[string]any{
		"outbounds": []any{
			map[string]any{"type": "selector", "tag": "Naga-Policy", "outbounds": []any{"ee-vless", "awg-1::AmneziaWG"}},
			map[string]any{"type": "vless", "tag": "ee-vless"},
			map[string]any{"type": "socks", "tag": "awg-1::AmneziaWG"},
		},
		"route": map[string]any{"final": "Naga-Policy"},
	})
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := profile.InspectOutboundMetadata(raw)
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string][]profile.SelectorChoice{}
	for tag := range metadata {
		path, err := profile.SelectionPath(raw, tag)
		if err == nil && len(path) > 0 {
			paths[tag] = path
		}
	}
	tag, path, ok := lookupSelectionPath(paths, metadata, "AmneziaWG")
	if !ok || tag != "awg-1::AmneziaWG" || len(path) == 0 {
		t.Fatalf("lookup AmneziaWG = %q %#v ok=%v", tag, path, ok)
	}
}

func TestAnnotateReadyTimeoutReadsSingBoxLog(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sing-box.log"), []byte("FATAL[0001] create tun: Access is denied\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := annotateReadyTimeout(&processRuntime{configDir: dir}, "sing-box did not become ready in time")
	if err == nil || !strings.Contains(err.Error(), "Access is denied") {
		t.Fatalf("error = %v", err)
	}
}
