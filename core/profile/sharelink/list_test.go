package sharelink

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

const uuid = "00000000-0000-4000-8000-000000000001"
const vlessWS = "vless://" + uuid + "@example.invalid:443?security=tls&type=ws&sni=example.invalid&host=cdn.example.invalid&path=%2Fweb&fp=firefox#Europe"
const hysteria = "hy2://example%3Apass@[2001:db8::10]:8443?sni=example.invalid&obfs=salamander&obfs-password=test#Hysteria"

func TestBase64ListPreservesParametersAndReportsIncompatibleNodes(t *testing.T) {
	input := strings.Join([]string{vlessWS, hysteria, vlessWS,
		"vless://" + uuid + "@example.invalid:443?fm=%7B%7D",
		"vless://" + uuid + "@example.invalid:443?fp=unsupported&security=tls",
		"vless://invalid@example.invalid:443",
		"ssr://example",
	}, "\r\n")
	raw := base64.RawURLEncoding.EncodeToString([]byte(input))
	result, recognized, err := Parse([]byte(raw[:50] + "\r\n" + raw[50:]))
	if err != nil || !recognized || result.Summary.Format != "base64_uri_list" || result.Summary.Imported != 2 {
		t.Fatalf("unexpected import result: %#v, recognized %t, error %v", result.Summary, recognized, err)
	}
	for _, reason := range []string{FinalMask, Duplicate, UnsupportedFingerprint, InvalidLink, UnsupportedProtocol} {
		if result.Summary.Ignored[reason] != 1 {
			t.Errorf("failed to classify %s", reason)
		}
	}
	b, err := result.Config()
	if err != nil || !json.Valid(b) {
		t.Fatal("invalid sing-box config")
	}
	for _, expected := range []string{`"Host":"cdn.example.invalid"`, `"path":"/web"`, `"fingerprint":"firefox"`,
		`"server":"2001:db8::10"`, `"password":"example:pass"`, `"type":"salamander"`} {
		if !strings.Contains(string(b), expected) {
			t.Errorf("missing test field %s", expected)
		}
	}
}

func TestTLSFingerprintsRemainExplicit(t *testing.T) {
	key := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	reality := "vless://" + uuid + "@example.invalid:443?security=reality&type=tcp&sni=example.invalid&flow=xtls-rprx-vision&sid=02&pbk=" +
		key + "&fp=hellochrome_120#Reality"
	result, _, err := Parse([]byte(reality + "\n" + hysteria))
	if err != nil || result.Summary.Imported != 2 {
		t.Fatalf("parse: %v", err)
	}
	stock := result.WithoutChrome120()
	if stock.Summary.Imported != 1 || stock.Summary.Ignored[UnsupportedFingerprint] != 1 || result.Summary.Imported != 2 {
		t.Fatalf("unexpected fallback: %#v", stock.Summary)
	}
}

func TestSelectionFollowsRenamedNodeAndRejectsMissingNode(t *testing.T) {
	before, _, _ := Parse([]byte(vlessWS + "\n" + hysteria))
	after, _, _ := Parse([]byte(hysteria + "\n" + strings.Replace(vlessWS, "#Europe", "#New%20name", 1)))
	previous, _ := before.Config()
	next, _ := after.Config()
	if name, ok := RemapSelection(previous, next, "Europe"); !ok || name != "New name" {
		t.Fatalf("selection not remapped: %q / %t", name, ok)
	}
	remaining, _, _ := Parse([]byte(hysteria))
	shortened, _ := remaining.Config()
	if name, ok := RemapSelection(previous, shortened, "Europe"); ok || name != "" {
		t.Fatal("remapped to a different outbound")
	}
}

func TestClashImportsInlineProxiesAndReportsRoutingBoundary(t *testing.T) {
	raw := `proxies:
  - {name: Web, type: vless, server: example.invalid, port: 443, uuid: ` + uuid + `, tls: true, network: ws, ws-opts: {path: /proxy, headers: {Host: example.invalid}}}
  - {name: Test, type: hysteria2, server: example.invalid, port: 443, password: test}
  - {name: Unknown, type: snell, server: example.invalid, port: 443}
rules: [MATCH, DIRECT]
`
	result, recognized, err := ParseClash([]byte(raw))
	if err != nil || !recognized || result.Summary.Imported != 2 || !result.Summary.ProxyOnly ||
		result.Summary.Ignored[UnsupportedProtocol] != 1 {
		t.Fatalf("Clash result: %#v, %t, %v", result.Summary, recognized, err)
	}
	b, _ := result.Config()
	if strings.Contains(string(b), "MATCH") || !strings.Contains(string(b), `"path":"/proxy"`) {
		t.Fatal("wrong Clash conversion")
	}
}
