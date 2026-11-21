package olcrtc

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestYAMLQuotedValuesPreserveSpecialCharacters(t *testing.T) {
	for _, value := range []string{"room\\path", "name\n  injected: true", "a\r\tb", "quote\"\\tail", "ключ"} {
		quoted := yamlQuote(value)
		if strings.ContainsAny(quoted, "\n\r\t") {
			t.Fatal("quoted value contains an unescaped control character")
		}
		var decoded string
		if err := json.Unmarshal([]byte(quoted), &decoded); err != nil || decoded != value {
			t.Fatalf("quoted value did not round trip: %q, %v", quoted, err)
		}
	}
	for _, value := range []string{"name\rnext", "name\tnext", "name\\next"} {
		if !strings.HasPrefix(yamlScalar(value), `"`) {
			t.Fatal("unsafe plain scalar was not quoted")
		}
	}
}

func TestBuildFlatAndFailover(t *testing.T) {
	jitsi := Profile{
		Name: "jitsi", Provider: ProviderJitsi, Transport: TransportDataChannel,
		Room: "https://meet.example/room", Key: testKey, RuntimeAllowed: true,
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
	if strings.Contains(doc.Body, "profiles:") {
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

func TestBuildRejectsParametersThatWouldBeDiscarded(t *testing.T) {
	item := Profile{
		Provider: ProviderJitsi, Transport: TransportDataChannel,
		Room: "https://meet.example/room", Key: testKey, RuntimeAllowed: true,
		Params: map[string]string{"mtu": "1200"},
	}
	if _, err := Build(&Config{Profiles: []Profile{item}}); err != ErrTransportOff {
		t.Fatalf("unsupported parameter was not rejected: %v", err)
	}
}

func TestBuildAtPortKeepsLiveDefaultAndIsolatesFlatAndFailover(t *testing.T) {
	item := Profile{Name: "jitsi", Provider: ProviderJitsi, Transport: TransportDataChannel, Room: "room", Key: testKey, RuntimeAllowed: true}
	for _, items := range [][]Profile{{item}, {item, item}} {
		cfg := &Config{Profiles: items}
		live, err := Build(cfg)
		if err != nil {
			t.Fatal(err)
		}
		probe, err := BuildAtPort(cfg, 32123)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(live.Body, "port: 10808") || !strings.Contains(probe.Body, "port: 32123") || strings.Contains(probe.Body, "port: 10808") {
			t.Fatal("probe shares live port")
		}
	}
	if _, err := BuildAtPort(&Config{Profiles: []Profile{item}}, 0); err == nil {
		t.Fatal("invalid port accepted")
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
	// Windows reports synthetic Unix mode bits; ACL validation is separate.
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("file mode = %o", info.Mode().Perm())
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode = %o", dirInfo.Mode().Perm())
	}
	body, err := os.ReadFile(path)
	if err != nil || string(body) != "mode: cnc\n" {
		t.Fatalf("body = %q err=%v", body, err)
	}
}
