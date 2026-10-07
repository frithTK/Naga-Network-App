package diagnostics

import (
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestLoggerPersistsAcrossReopenAndRedactsSecrets(t *testing.T) {
	directory := t.TempDir()
	logger, err := NewWithOptions(directory, 4096, 1)
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("runtime", "start", "connect https://secret.example/path bearer abc.def", map[string]any{
		"profile_id": "profile-safe",
		"token":      "must-not-leak",
		"detail":     "password=hunter2",
	})
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := NewWithOptions(directory, 4096, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	entries, err := reopened.Recent(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Event != "start" {
		t.Fatalf("entries = %#v", entries)
	}
	encoded, err := os.ReadFile(reopened.Path())
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, secret := range []string{"secret.example", "abc.def", "must-not-leak", "hunter2"} {
		if strings.Contains(text, secret) {
			t.Fatalf("diagnostics log leaked %q: %s", secret, text)
		}
	}
	info, err := os.Stat(reopened.Path())
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("log mode = %o", info.Mode().Perm())
	}
}

func TestLoggerRotatesAndReturnsNewestEntries(t *testing.T) {
	logger, err := NewWithOptions(t.TempDir(), 450, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Close()
	for index := 0; index < 12; index++ {
		logger.Info("runtime", "sample", strings.Repeat("x", 50), map[string]any{"index": index})
	}
	entries, err := logger.Recent(4)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 4 {
		t.Fatalf("recent entries = %d, want 4", len(entries))
	}
	if entries[len(entries)-1].Fields["index"] != float64(11) {
		t.Fatalf("last entry = %#v", entries[len(entries)-1])
	}
	if _, err := os.Stat(logger.Path() + ".1"); err != nil {
		t.Fatalf("rotated log missing: %v", err)
	}
}
