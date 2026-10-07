package olcrtc

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

const Scheme = "olcrtc://"

const NodeTag = "olcRTC"

const (
	ProviderJitsi    = "jitsi"
	ProviderTelemost = "telemost"
	ProviderWBStream = "wbstream"

	TransportDataChannel  = "datachannel"
	TransportVP8Channel   = "vp8channel"
	TransportSEIChannel   = "seichannel"
	TransportVideoChannel = "videochannel"
)

var (
	ErrURI          = errors.New("olcrtc uri is invalid")
	ErrTransportOff = errors.New("olcrtc transport is disabled")
)

// Profile is one olcRTC room extracted from an olcrtc:// URI.
type Profile struct {
	Name      string
	Provider  string
	Transport string
	Room      string
	Key       string
	Params    map[string]string
	// RuntimeAllowed is false for seichannel and videochannel until the
	// envelope is allowed to start them.
	RuntimeAllowed bool
}

// IsURI reports whether raw is a single olcrtc:// line.
func IsURI(raw []byte) bool {
	text := strings.TrimSpace(string(raw))
	return strings.HasPrefix(strings.ToLower(text), Scheme) && !strings.Contains(text, "\n")
}

// ParseURI parses olcrtc://provider?transport[@params]@room#key$name.
// A missing $, @ or # in its place, or an empty provider, transport, room or
// key, makes the URI invalid. An empty name after $ is valid.
func ParseURI(raw string) (Profile, error) {
	text := strings.TrimSpace(raw)
	if !strings.HasPrefix(strings.ToLower(text), Scheme) {
		return Profile{}, fmt.Errorf("%w: scheme", ErrURI)
	}
	body := text[len(Scheme):]
	provider, rest, ok := cut(body, "?")
	if !ok || strings.TrimSpace(provider) == "" {
		return Profile{}, fmt.Errorf("%w: provider", ErrURI)
	}
	transportBlock, roomAndKey, ok := cut(rest, "@")
	if !ok || transportBlock == "" {
		return Profile{}, fmt.Errorf("%w: transport", ErrURI)
	}
	room, keyAndName, ok := cut(roomAndKey, "#")
	if !ok || strings.TrimSpace(room) == "" {
		return Profile{}, fmt.Errorf("%w: room", ErrURI)
	}
	key, name, _ := cut(keyAndName, "$")
	key = strings.ToLower(strings.TrimSpace(key))
	if !validKey(key) {
		return Profile{}, fmt.Errorf("%w: key", ErrURI)
	}
	transport, params, err := splitTransport(transportBlock)
	if err != nil {
		return Profile{}, err
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	switch provider {
	case ProviderJitsi, ProviderTelemost, ProviderWBStream:
	default:
		return Profile{}, fmt.Errorf("%w: provider", ErrURI)
	}
	room = strings.TrimSpace(room)
	if err := validateRoom(provider, room); err != nil {
		return Profile{}, err
	}
	allowed := transport == TransportDataChannel || transport == TransportVP8Channel
	return Profile{
		Name:           name,
		Provider:       provider,
		Transport:      transport,
		Room:           room,
		Key:            key,
		Params:         params,
		RuntimeAllowed: allowed,
	}, nil
}

func splitTransport(block string) (string, map[string]string, error) {
	block = strings.TrimSpace(block)
	var transport string
	for _, candidate := range []string{TransportDataChannel, TransportVP8Channel, TransportSEIChannel, TransportVideoChannel} {
		if strings.HasPrefix(block, candidate) {
			transport = candidate
			break
		}
	}
	if transport == "" {
		return "", nil, fmt.Errorf("%w: transport", ErrURI)
	}
	rest := strings.TrimPrefix(block, transport)
	params := map[string]string{}
	if rest == "" {
		return transport, params, nil
	}
	rest = strings.TrimPrefix(rest, "?")
	rest = strings.TrimPrefix(rest, "&")
	if rest == "" {
		return transport, params, nil
	}
	for _, pair := range strings.Split(rest, "&") {
		if pair == "" {
			continue
		}
		k, v, ok := strings.Cut(pair, "=")
		if !ok || strings.TrimSpace(k) == "" {
			return "", nil, fmt.Errorf("%w: params", ErrURI)
		}
		params[strings.TrimSpace(k)] = v
	}
	return transport, params, nil
}

func validateRoom(provider, room string) error {
	if strings.Contains(room, "://") {
		parsed, err := url.Parse(room)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
			return fmt.Errorf("%w: room", ErrURI)
		}
		return nil
	}
	if provider == ProviderJitsi {
		return fmt.Errorf("%w: room", ErrURI)
	}
	if room == "" {
		return fmt.Errorf("%w: room", ErrURI)
	}
	return nil
}

func validKey(key string) bool {
	if len(key) != 64 {
		return false
	}
	for _, r := range key {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'f':
		default:
			return false
		}
	}
	return true
}

func cut(s, sep string) (string, string, bool) {
	i := strings.Index(s, sep)
	if i < 0 {
		return s, "", false
	}
	return s[:i], s[i+len(sep):], true
}

// Redact removes key and room text from an error so logs stay safe.
func Redact(err error, secret Profile) error {
	if err == nil {
		return nil
	}
	return errors.New(redactText(err.Error(), secret.Key, secret.Room))
}

// RedactURI removes a key and room embedded in raw text.
func RedactURI(raw string) string {
	profile, err := ParseURI(raw)
	if err != nil {
		return raw
	}
	return redactText(raw, profile.Key, profile.Room)
}

func RedactLine(line string, profiles []Profile) string {
	for _, profile := range profiles {
		line = redactText(line, profile.Key, profile.Room)
	}
	return line
}

func redactText(text, key, room string) string {
	if key != "" {
		text = strings.ReplaceAll(text, key, "[key]")
		text = strings.ReplaceAll(text, strings.ToUpper(key), "[key]")
	}
	if room != "" {
		text = strings.ReplaceAll(text, room, "[room]")
		if parsed, err := url.Parse(room); err == nil && parsed.Host != "" {
			text = strings.ReplaceAll(text, parsed.Host, "[room]")
		}
	}
	return text
}
