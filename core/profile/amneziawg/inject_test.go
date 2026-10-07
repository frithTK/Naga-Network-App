package amneziawg

import (
	"strings"
	"testing"

	"naga.network/core/profile"
)

func TestInjectSOCKSLeafAddsSelectorMemberWithoutTUN(t *testing.T) {
	base := []byte(`{
  "outbounds": [
    {"type":"selector","tag":"Naga-Policy","outbounds":["ee-vless"],"default":"ee-vless"},
    {"type":"vless","tag":"ee-vless"},
    {"type":"direct","tag":"direct"}
  ],
  "route": {"final": "Naga-Policy"}
}`)
	raw, err := InjectSOCKSLeaf(base, "127.0.0.1", 9050, "profile-awg::AmneziaWG")
	if err != nil {
		t.Fatal(err)
	}
	if err := profile.ValidateSingBoxConfig(raw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"type": "tun"`) {
		t.Fatalf("inject must not add TUN: %s", raw)
	}
	options, err := profile.InspectMode(raw)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, node := range options.Nodes {
		if node.Tag == "AmneziaWG" && node.Protocol == "AmneziaWG" {
			found = true
		}
	}
	if !found {
		t.Fatalf("injected leaf missing from InspectMode: %+v", options.Nodes)
	}
	path, err := profile.SelectionPath(raw, "profile-awg::AmneziaWG")
	if err != nil || len(path) == 0 {
		t.Fatalf("SelectionPath namespaced AWG: %v %#v", err, path)
	}
	metadata, err := profile.InspectOutboundMetadata(raw)
	if err != nil {
		t.Fatal(err)
	}
	node, ok := metadata["profile-awg::AmneziaWG"]
	if !ok || node.Protocol != "AmneziaWG" {
		t.Fatalf("overlay metadata = %#v", metadata["profile-awg::AmneziaWG"])
	}
}

func TestInjectSOCKSLeafFallsBackToMode(t *testing.T) {
	base := []byte(`{
  "outbounds": [
    {"type":"selector","tag":"Mode","outbounds":["fast"],"default":"fast"},
    {"type":"vless","tag":"fast"},
    {"type":"direct","tag":"direct"}
  ],
  "route": {"final": "Mode"}
}`)
	raw, err := InjectSOCKSLeaf(base, "127.0.0.1", 9050, "awg-1::AmneziaWG")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"awg-1::AmneziaWG"`) {
		t.Fatalf("missing injected tag: %s", raw)
	}
}
