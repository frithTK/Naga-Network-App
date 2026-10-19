//go:build darwin && !ios

package singbox

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDarwinBridgeChild(t *testing.T) {
	kind := os.Getenv("NAGA_TEST_BRIDGE_EVENT")
	if kind == "" {
		return
	}
	switch kind {
	case "clean":
		fmt.Println(`{"type":"ready","protocol_version":1}`)
		fmt.Println(`{"type":"stopped","cleanup":true}`)
	case "missing-cleanup":
		fmt.Println(`{"type":"ready","protocol_version":1}`)
	case "wrong-version":
		fmt.Println(`{"type":"ready","protocol_version":2}`)
	case "error":
		fmt.Println(`{"type":"error","message":"permission refused"}`)
	case "oversized":
		fmt.Println(strings.Repeat("x", 20000))
	}
	os.Exit(0)
}

func TestDarwinBridgeRequiresVersionAndCleanupAcknowledgement(t *testing.T) {
	for _, kind := range []string{"clean", "missing-cleanup", "wrong-version", "error", "oversized"} {
		t.Run(kind, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestDarwinBridgeChild$")
			cmd.Env = append(os.Environ(), "NAGA_TEST_BRIDGE_EVENT="+kind)
			input, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			output, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			p := &darwinProcess{cmd: cmd, input: input, done: make(chan struct{}), ready: make(chan error, 1)}
			go p.read(output)
			readyErr := <-p.ready
			waitErr := p.Wait()
			_ = p.SignalStop()
			if kind == "clean" && (readyErr != nil || waitErr != nil) {
				t.Fatalf("clean stop rejected: ready=%v wait=%v", readyErr, waitErr)
			}
			if kind != "clean" && readyErr == nil && waitErr == nil {
				t.Fatal("incompatible or unconfirmed bridge exit was accepted")
			}
		})
	}
}

func TestDarwinRuleSetsBecomeInlineBeforeHelper(t *testing.T) {
	path := filepath.Join(t.TempDir(), "правила с пробелами.json")
	if err := os.WriteFile(path, []byte(`{"version":3,"rules":[{"domain_suffix":["example.invalid"]}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	set := map[string]any{"type": "local", "tag": "test", "format": "source", "path": path}
	doc := map[string]any{"route": map[string]any{"rule_set": []any{set}}}
	if err := inlineDarwinRuleSets(context.Background(), doc, ""); err != nil {
		t.Fatal(err)
	}
	if set["type"] != "inline" || set["path"] != nil || set["format"] != nil {
		t.Fatalf("root still receives a file reference: %v", set)
	}
	rules, _ := json.Marshal(set["rules"])
	if !strings.Contains(string(rules), "example.invalid") {
		t.Fatal("rule content was lost")
	}
}
