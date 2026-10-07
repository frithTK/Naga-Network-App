package olcrtc

import (
	"bytes"
	"strings"
)

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
	text := string(bytes.TrimSpace(raw))
	if text == "" {
		return nil, ErrURI
	}
	if IsURI(raw) {
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
	var uri string
	var lineName string
	var sawURI bool
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		switch {
		case strings.HasPrefix(lower, "##name:"):
			lineName = strings.TrimSpace(line[len("##name:"):])
		case strings.HasPrefix(lower, "#name:"):
			title = strings.TrimSpace(line[len("#name:"):])
		case strings.HasPrefix(line, "#"):
			continue
		case strings.HasPrefix(lower, Scheme):
			sawURI = true
			if uri == "" {
				uri = line
			}
		default:
			return nil, ErrURI
		}
	}
	if !sawURI {
		return nil, ErrURI
	}
	profile, err := ParseURI(uri)
	if err != nil {
		return nil, err
	}
	if !profile.RuntimeAllowed {
		return nil, ErrTransportOff
	}
	if lineName != "" {
		profile.Name = lineName
	}
	if title == "" {
		title = profile.Name
	}
	return &Config{
		Label:    title,
		Profiles: []Profile{profile},
		URIs:     []string{uri},
	}, nil
}
