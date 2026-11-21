package olcrtc

import (
	"bytes"
	"fmt"
	"strings"
)

const maxDocumentBytes = 4 << 20
const maxDocumentProfiles = 1024
const maxDocumentLineBytes = 64 << 10

// IsDocument reports whether raw is an olcrtc:// URI or a Ghostlane text list.
// JSON envelopes are not documents even if they embed a URI string.
func IsDocument(raw []byte) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] == '{' {
		return false
	}
	return bytes.Contains(bytes.ToLower(trimmed), []byte(Scheme))
}

// ParseDocument reads either one olcrtc:// URI or a Ghostlane text list:
// header comments (#name:, #refresh:), one or more URI lines, and an
// optional ##name: under a URI. Bytes are not rewritten.
func ParseDocument(raw []byte) (*Config, error) {
	if len(raw) > maxDocumentBytes {
		return nil, fmt.Errorf("%w: document too large", ErrURI)
	}
	text := string(bytes.TrimSpace(raw))
	if text == "" {
		return nil, ErrURI
	}
	if IsURI(raw) {
		if len(text) > maxDocumentLineBytes {
			return nil, fmt.Errorf("%w: line too large", ErrURI)
		}
		profile, err := ParseURI(text)
		if err != nil {
			return nil, err
		}
		if !profile.RuntimeAllowed {
			return nil, ErrTransportOff
		}
		return &Config{
			Label:    profile.Name,
			Profiles: []Profile{profile},
			URIs:     []string{text},
		}, nil
	}
	var title string
	cfg := &Config{}
	lastProfile := -1
	entries := 0
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if len(line) > maxDocumentLineBytes {
			return nil, fmt.Errorf("%w: line too large", ErrURI)
		}
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		switch {
		case strings.HasPrefix(lower, "##name:"):
			if lastProfile >= 0 {
				if name := strings.TrimSpace(line[len("##name:"):]); name != "" {
					cfg.Profiles[lastProfile].Name = name
				}
			}
		case strings.HasPrefix(lower, "#name:"):
			title = strings.TrimSpace(line[len("#name:"):])
		case strings.HasPrefix(line, "#"):
			continue
		case strings.HasPrefix(lower, Scheme):
			entries++
			if entries > maxDocumentProfiles {
				return nil, fmt.Errorf("%w: too many profiles", ErrURI)
			}
			profile, err := ParseURI(line)
			if err != nil || !profile.RuntimeAllowed {
				cfg.Ignored++
				lastProfile = -1
				continue
			}
			cfg.Profiles = append(cfg.Profiles, profile)
			cfg.URIs = append(cfg.URIs, line)
			lastProfile = len(cfg.Profiles) - 1
		default:
			return nil, ErrURI
		}
	}
	if len(cfg.Profiles) == 0 {
		return nil, ErrURI
	}
	if title == "" {
		title = cfg.Profiles[0].Name
	}
	cfg.Label = title
	return cfg, nil
}
