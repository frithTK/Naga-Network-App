package olcrtc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildFlatAndFailover(t *testing.T) {
	jitsi := Profile{
		Name: "jitsi", Provider: ProviderJitsi, Transport: TransportDataChannel,
		Room: "https://meet.example/room", Key: testKey, RuntimeAllowed: true,
		Params: map[string]string{"secret": "nope"},
	}
	doc, err := Build(&Config{Profiles: []Profile{jitsi}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"mode: cnc", "provider: jitsi", `id: "https://meet.example/room"`, `key: "` + testKey + `"`, "transport: datachannel", `dns: "1.1.1.1:53"`, `host: "127.0.0.1"`, "port: 10808", "max_flows: 256", "max_session_duration: 6h"} {
		if !strings.Contains(doc.Body, want) {
			t.Fatalf("missing %q in\n%s", want, doc.Body)
		}
	}
	if strings.Contains(doc.Body, "secret") || strings.Contains(doc.Body, "profiles:") {
		t.Fatalf("unexpected body:\n%s", doc.Body)
	}

	telemost := Profile{
		Name: "telemost", Provider: ProviderTelemost, Transport: TransportDataChannel,
		Room: "room-1", Key: testKey, RuntimeAllowed: true,
	}
	doc, err = Build(&Config{Profiles: []Profile{jitsi, telemost}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(doc.Body, "auth:\n  provider:") {
		t.Fatalf("failover leaked top-level auth:\n%s", doc.Body)
	}
	if !strings.Contains(doc.Body, "transport: vp8channel") || !strings.Contains(doc.Body, "retry_delay: 2s") || !strings.Contains(doc.Body, "max_cycles: 0") {
		t.Fatalf("failover body:\n%s", doc.Body)
	}
	jitsiAt := strings.Index(doc.Body, "name: jitsi")
	telemostAt := strings.Index(doc.Body, "name: telemost")
	if jitsiAt < 0 || telemostAt < jitsiAt {
		t.Fatalf("order:\n%s", doc.Body)
	}
}

func TestBuildRejectsDisabledTransport(t *testing.T) {
	_, err := Build(&Config{Profiles: []Profile{{
		Name: "sei", Provider: ProviderJitsi, Transport: TransportSEIChannel,
		Room: "https://meet.example/room", Key: testKey, RuntimeAllowed: false,
	}}})
	if err != ErrTransportOff {
		t.Fatalf("err = %v", err)
	}
}

func TestWriteFileMode(t *testing.T) {
	dir := t.TempDir()
	path, err := WriteFile(dir, Document{Body: "mode: cnc\n"})
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("file mode = %o", info.Mode().Perm())
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode = %o", dirInfo.Mode().Perm())
	}
	body, err := os.ReadFile(path)
	if err != nil || string(body) != "mode: cnc\n" {
		t.Fatalf("body = %q err=%v", body, err)
	}
}
