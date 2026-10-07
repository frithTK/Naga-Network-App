//go:build windows

package control

import (
	"strings"
	"testing"
)

func TestDiscoverUserProcessesReturnsUniqueApps(t *testing.T) {
	got := DiscoverUserProcesses()
	if got == nil {
		t.Fatal("discovered apps must be an empty slice, not nil")
	}
	if len(got) == 0 {
		t.Fatal("expected at least one user process on Windows")
	}
	t.Logf("discovered %d apps, first=%q path=%q", len(got), got[0].Name, got[0].ProcessPath)
	withPath := 0
	for _, app := range got {
		if app.ProcessPath != "" {
			withPath++
		}
	}
	if withPath == 0 {
		t.Fatal("QueryFullProcessImageName returned no image paths")
	}
	t.Logf("apps with full path: %d/%d", withPath, len(got))
	seen := make(map[string]struct{}, len(got))
	for _, app := range got {
		if app.Process == "" {
			t.Fatalf("empty process: %#v", app)
		}
		if shouldSkipDiscoveredProcess(app.Process) {
			t.Fatalf("skipped process leaked: %#v", app)
		}
		key := strings.ToLower(app.ProcessPath)
		if key == "" {
			key = strings.ToLower(app.Process)
		}
		if _, exists := seen[key]; exists {
			t.Fatalf("duplicate discovered app %q", key)
		}
		seen[key] = struct{}{}
	}
}
