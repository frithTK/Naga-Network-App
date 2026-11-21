// Package sharelink converts proxy subscriptions to sing-box outbounds.
package sharelink

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxLinks     = 4096
	maxLinkBytes = 64 << 10
)

const (
	InvalidLink            = "invalid_link"
	UnsupportedProtocol    = "unsupported_protocol"
	UnsupportedParameter   = "unsupported_parameter"
	UnsupportedFingerprint = "unsupported_fingerprint"
	FinalMask              = "final_mask"
	Duplicate              = "duplicate"
)

type Summary struct {
	Format    string         `json:"format"`
	Imported  int            `json:"imported"`
	Ignored   map[string]int `json:"ignored,omitempty"`
	ProxyOnly bool           `json:"proxy_only,omitempty"`
}

type Result struct {
	Summary Summary
	nodes   []map[string]any
}

func (r Result) Config() ([]byte, error) {
	tags := make([]string, 0, len(r.nodes))
	outbounds := make([]map[string]any, 1, len(r.nodes)+2)
	for _, node := range r.nodes {
		tags = append(tags, node["tag"].(string))
		outbounds = append(outbounds, node)
	}
	if len(tags) == 0 {
		return nil, errors.New("subscription contains no compatible proxy nodes")
	}
	outbounds[0] = map[string]any{"type": "selector", "tag": "Mode", "outbounds": tags}
	outbounds = append(outbounds, map[string]any{"type": "direct", "tag": "direct"})
	return json.Marshal(map[string]any{
		"route":     map[string]any{"final": "Mode"},
		"outbounds": outbounds,
	})
}

// WithoutChrome120 keeps the subset that works with an unmodified sing-box.
func (r Result) WithoutChrome120() Result {
	copy := Result{Summary: Summary{Format: r.Summary.Format, ProxyOnly: r.Summary.ProxyOnly, Ignored: make(map[string]int)}}
	for reason, count := range r.Summary.Ignored {
		copy.Summary.Ignored[reason] = count
	}
	for _, node := range r.nodes {
		tls, _ := node["tls"].(map[string]any)
		utls, _ := tls["utls"].(map[string]any)
		if utls["fingerprint"] == "hellochrome_120" {
			copy.Summary.Ignored[UnsupportedFingerprint]++
			continue
		}
		copy.nodes = append(copy.nodes, node)
	}
	copy.Summary.Imported = len(copy.nodes)
	return copy
}

// Parse recognizes plain and base64-encoded lists. Other formats continue
// through the native profile parsers.
func Parse(raw []byte) (Result, bool, error) {
	format := "uri_list"
	raw = bytes.TrimSpace(bytes.TrimPrefix(raw, []byte{0xef, 0xbb, 0xbf}))
	if len(raw) == 0 || bytes.ContainsRune([]byte("{[<"), rune(raw[0])) {
		return Result{}, false, nil
	}
	if !hasLinkLine(raw) {
		decoded, err := decodeBase64(string(raw))
		if err != nil || !hasLinkLine(decoded) {
			return Result{}, false, nil
		}
		raw, format = decoded, "base64_uri_list"
	}
	result := newResult(format)
	seen := make(map[string]bool)
	count := 0
	for line := range bytes.SplitSeq(raw, []byte{'\n'}) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 || line[0] == '#' {
			continue
		}
		count++
		if count > maxLinks {
			return Result{}, true, errors.New("subscription has too many proxy links")
		}
		if len(line) > maxLinkBytes || !utf8.Valid(line) {
			result.Summary.Ignored[InvalidLink]++
			continue
		}
		node, label, reason := parseURI(string(line))
		result.add(node, label, reason, seen)
	}
	result.finish()
	return result, true, nil
}

func newResult(format string) Result {
	return Result{Summary: Summary{Format: format, Ignored: make(map[string]int)}}
}

func (r *Result) add(node map[string]any, label, reason string, seen map[string]bool) {
	if reason != "" {
		r.Summary.Ignored[reason]++
		return
	}
	key := identity(node)
	if seen[key] {
		r.Summary.Ignored[Duplicate]++
		return
	}
	seen[key] = true
	node["tag"] = cleanLabel(label, node["type"].(string))
	r.nodes = append(r.nodes, node)
}

func (r *Result) finish() {
	counts := make(map[string]int)
	for _, node := range r.nodes {
		counts[strings.ToLower(node["tag"].(string))]++
	}
	used := map[string]bool{"mode": true, "direct": true, "auto": true}
	for _, node := range r.nodes {
		label := node["tag"].(string)
		tag := label
		if counts[strings.ToLower(label)] > 1 || used[strings.ToLower(tag)] {
			tag = fmt.Sprintf("%s [%s]", label, identity(node)[:16])
		}
		for suffix := 2; used[strings.ToLower(tag)]; suffix++ {
			tag = fmt.Sprintf("%s [%s-%d]", label, identity(node)[:16], suffix)
		}
		used[strings.ToLower(tag)] = true
		node["tag"] = tag
	}
	r.Summary.Imported = len(r.nodes)
}

func cleanLabel(raw, protocol string) string {
	if !utf8.ValidString(raw) || strings.Contains(raw, "://") {
		return strings.ToUpper(protocol)
	}
	var label []rune
	for _, ch := range strings.TrimSpace(raw) {
		if unicode.IsControl(ch) || unicode.Is(unicode.Bidi_Control, ch) {
			continue
		}
		if len(label) == 120 {
			break
		}
		label = append(label, ch)
	}
	if len(label) == 0 {
		return strings.ToUpper(protocol)
	}
	return string(label)
}

func identity(node map[string]any) string {
	config := make(map[string]any, len(node))
	for key, value := range node {
		if key != "tag" {
			config[key] = value
		}
	}
	data, _ := json.Marshal(config)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// RemapSelection follows the same outbound when a provider renames it.
func RemapSelection(previous, next []byte, selected string) (string, bool) {
	find := func(config []byte) []map[string]any {
		var doc struct {
			Outbounds []map[string]any `json:"outbounds"`
		}
		_ = json.Unmarshal(config, &doc)
		return doc.Outbounds
	}
	for _, old := range find(previous) {
		if old["tag"] != selected {
			continue
		}
		key := identity(old)
		for _, candidate := range find(next) {
			tag, _ := candidate["tag"].(string)
			if tag != "" && identity(candidate) == key {
				return tag, true
			}
		}
		break
	}
	return "", false
}

func hasLinkLine(raw []byte) bool {
	for line := range bytes.SplitSeq(raw, []byte{'\n'}) {
		line = bytes.TrimSpace(line)
		separator := bytes.Index(line, []byte("://"))
		if separator > 0 && separator <= 24 && isScheme(line[:separator]) {
			return true
		}
	}
	return false
}

func isScheme(raw []byte) bool {
	for i, ch := range raw {
		if ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' {
			continue
		}
		if i == 0 || !(ch >= '0' && ch <= '9' || ch == '+' || ch == '-' || ch == '.') {
			return false
		}
	}
	return len(raw) > 0
}

func decodeBase64(raw string) ([]byte, error) {
	compacted := strings.Map(func(ch rune) rune {
		if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' {
			return -1
		}
		return ch
	}, raw)
	standard := strings.NewReplacer("-", "+", "_", "/").Replace(compacted)
	if strings.ContainsRune(standard, '=') {
		return base64.StdEncoding.Strict().DecodeString(standard)
	}
	return base64.RawStdEncoding.Strict().DecodeString(standard)
}
