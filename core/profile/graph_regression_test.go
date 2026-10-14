package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestSelectionPathsMatchGraphSearch(t *testing.T) {
	config := []byte(`{"route":{"final":"Mode"},"outbounds":[{"tag":"Mode","type":"selector","outbounds":["auto","country","branch"]},{"tag":"auto","type":"urltest","outbounds":["a","b"]},{"tag":"country","type":"selector","outbounds":["a","cycle"]},{"tag":"branch","type":"selector","outbounds":["country","b"]},{"tag":"cycle","type":"selector","outbounds":["country"]},{"tag":"a","type":"vless"},{"tag":"b","type":"vless"},{"tag":"isolated","type":"vless"}]}`)
	document, err := decodeConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	root, index, err := modeGraph(document)
	if err != nil {
		t.Fatal(err)
	}
	paths, err := SelectionPaths(config)
	if err != nil {
		t.Fatal(err)
	}
	for tag := range index {
		oldPath := findNodePath(root, tag, index, make(map[string]bool))
		if len(oldPath) == 0 {
			if _, ok := paths[tag]; ok {
				t.Fatalf("unreachable %q was indexed", tag)
			}
			continue
		}
		var want []SelectorChoice
		for i := 0; i+1 < len(oldPath); i++ {
			kind, _ := index[oldPath[i]]["type"].(string)
			if kind == "selector" || kind == "urltest" {
				want = append(want, SelectorChoice{oldPath[i], oldPath[i+1]})
			}
		}
		if !reflect.DeepEqual(paths[tag], want) {
			t.Fatalf("%s got %v want %v", tag, paths[tag], want)
		}
	}
}
func TestInspectModeBoundsSharedSubgraphExpansion(t *testing.T) {
	outbounds := []any{map[string]any{"tag": "Mode", "type": "selector", "outbounds": []string{"a0", "b0"}}}
	for level := 0; level < 13; level++ {
		for _, prefix := range []string{"a", "b"} {
			node := map[string]any{"tag": fmt.Sprintf("%s%d", prefix, level), "type": "selector", "outbounds": []string{fmt.Sprintf("a%d", level+1), fmt.Sprintf("b%d", level+1)}}
			if level == 12 {
				node["type"] = "direct"
				delete(node, "outbounds")
			}
			outbounds = append(outbounds, node)
		}
	}
	raw, err := json.Marshal(map[string]any{"outbounds": outbounds})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = InspectMode(raw); !errors.Is(err, ErrProfileTooComplex) {
		t.Fatalf("small input expands without budget: %v", err)
	}
	// The index remains bounded by the 27 actual graph nodes.
	paths, err := SelectionPaths(raw)
	if err != nil || len(paths) != 27 {
		t.Fatalf("paths=%d err=%v", len(paths), err)
	}
}

func TestGraphRejectsOversizedMetadataBeforeExpansion(t *testing.T) {
	long := strings.Repeat("x", maxModeTagBytes+1)
	for _, raw := range []string{
		`{"outbounds":[{"type":"direct","tag":"` + long + `"}]}`,
		`{"outbounds":[{"type":"selector","tag":"Mode","outbounds":["` + long + `"]}]}`,
	} {
		if err := ValidateSingBoxConfig([]byte(raw)); !errors.Is(err, ErrProfileTooComplex) {
			t.Fatalf("oversized graph accepted: %v", err)
		}
	}
}
