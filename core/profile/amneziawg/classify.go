package amneziawg

import (
	"bytes"
	"errors"
	"strings"
)

type Format int

const (
	FormatUnknown Format = iota
	FormatJSON
	FormatAmneziaWG
)

var (
	ErrEmptyConfig       = errors.New("profile is empty")
	ErrUnknownFormat     = errors.New("profile is not sing-box JSON or AmneziaWG")
	ErrPlainWireGuard    = errors.New("wireguard profile is not AmneziaWG")
	ErrMissingInterface  = errors.New("amneziawg config has no interface section")
	ErrMissingPeer       = errors.New("amneziawg config has no peer section")
	ErrPrivateKeyMissing = errors.New("amneziawg interface privatekey is required")
	ErrAddressMissing    = errors.New("amneziawg interface address is required")
	ErrPeerPublicKey     = errors.New("amneziawg peer publickey is required")
	ErrPeerEndpoint      = errors.New("amneziawg peer endpoint is required")
)

// Classify reports whether raw bytes are sing-box JSON or AmneziaWG INI.
// Plain WireGuard without obfuscation fields is rejected explicitly.
func Classify(raw []byte) (Format, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return FormatUnknown, ErrEmptyConfig
	}
	if trimmed[0] == '{' {
		return FormatJSON, nil
	}
	doc, err := parseINI(raw)
	if err != nil {
		return FormatUnknown, ErrUnknownFormat
	}
	if doc.iface == nil {
		return FormatUnknown, ErrUnknownFormat
	}
	if len(doc.peers) == 0 {
		return FormatUnknown, ErrUnknownFormat
	}
	if !hasAmneziaFields(doc.iface) {
		return FormatUnknown, ErrPlainWireGuard
	}
	return FormatAmneziaWG, nil
}

func hasAmneziaFields(section *iniSection) bool {
	if section == nil {
		return false
	}
	for _, key := range []string{
		"jc", "junkpacketcount",
		"jmin", "junkpacketmin", "junkpacketminsize",
		"jmax", "junkpacketmax", "junkpacketmaxsize",
		"s1", "s2", "s3", "s4",
		"h1", "h2", "h3", "h4",
		"i1", "i2", "i3", "i4", "i5",
		"j1", "j2", "j3", "itime",
		"headerprotectionkey", "hp1", "hp2", "hp3", "hp4",
	} {
		if strings.TrimSpace(section.get(key)) != "" {
			return true
		}
	}
	return false
}
