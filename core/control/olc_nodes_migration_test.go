package control

import (
	"strings"
	"testing"
)

func TestRemapOlcSelectionTracksDuplicateRoomsAcrossReorder(t *testing.T) {
	key := strings.Repeat("a", 64)
	first := "olcrtc://jitsi?datachannel@https://example.org/first#" + key + "$Same"
	second := "olcrtc://jitsi?datachannel@https://example.org/second#" + key + "$Same"
	old := []byte(first + "\n" + second)
	reordered := []byte(second + "\n" + first)
	mapped, err := remapOlcSelection(old, reordered, "Same 2")
	if err != nil || mapped != "Same" {
		t.Fatalf("selected second room must remain selected after reorder: mapped=%q err=%v", mapped, err)
	}
	mapped, err = remapOlcSelection(old, reordered, "profile::Same 2")
	if err != nil || mapped != "profile::Same" {
		t.Fatalf("namespaced selection not preserved: mapped=%q err=%v", mapped, err)
	}
}

func TestRemapOlcSelectionRejectsAmbiguityAndKeepsUniqueRotation(t *testing.T) {
	key := strings.Repeat("a", 64)
	first := "olcrtc://jitsi?datachannel@https://example.org/first#" + key + "$Same"
	second := "olcrtc://jitsi?datachannel@https://example.org/second#" + key + "$Same"
	rotated := "olcrtc://jitsi?datachannel@https://example.org/rotated#" + key + "$Same"
	if _, err := remapOlcSelection([]byte(first+"\n"+second), []byte(rotated), "Same 2"); err == nil {
		t.Fatal("must not guess which duplicate room survived")
	}
	if _, err := remapOlcSelection([]byte(first+"\n"+first), []byte(first), "Same 2"); err == nil {
		t.Fatal("previously identical duplicate rooms remain ambiguous")
	}
	if mapped, err := remapOlcSelection([]byte(first), []byte(rotated), "Same"); err != nil || mapped != "Same" {
		t.Fatalf("unique room rotation was not preserved: mapped=%q err=%v", mapped, err)
	}
}
