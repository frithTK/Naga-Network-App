package amneziawg

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type Config struct {
	PrivateKey             string
	Addresses              []string
	DNS                    []string
	MTU                    int
	ListenPort             int
	Jc                     int
	Jmin                   int
	Jmax                   int
	S                      [4]int
	H                      [4]string
	I                      [5]string
	J                      [3]string
	ITime                  int
	HeaderProtectionKey    string
	ContentPaddingAddition string
	RandomTrailers         string
	DisableCookies         string
	RekeyAfterTime         string
	RekeyTimeout           string
	RejectAfterTime        string
	KeepaliveTimeout       string
	MaxHandshakeAttempts   string
	UnsupportedFields      bool
	Peers                  []Peer
}

type Peer struct {
	PublicKey                string
	PresharedKey             string
	Endpoint                 string
	AllowedIPs               []string
	PersistentKeepalive      int
	PersistentKeepaliveRange string
}

type iniSection struct {
	name   string
	values map[string]string
}

func (s *iniSection) get(key string) string {
	if s == nil {
		return ""
	}
	return s.values[strings.ToLower(strings.TrimSpace(key))]
}

type iniDoc struct {
	iface             *iniSection
	peers             []*iniSection
	unsupportedFields bool
}

func parseINI(raw []byte) (iniDoc, error) {
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	var current *iniSection
	var iface *iniSection
	peers := make([]*iniSection, 0)
	unsupportedFields := false
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			name := strings.TrimSpace(line[1 : len(line)-1])
			current = &iniSection{name: name, values: map[string]string{}}
			switch strings.ToLower(name) {
			case "interface":
				if iface != nil {
					return iniDoc{}, fmt.Errorf("amneziawg config has duplicate interface section")
				}
				iface = current
			case "peer":
				peers = append(peers, current)
			default:
				unsupportedFields = true
				current = nil
			}
			continue
		}
		if current == nil {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return iniDoc{}, fmt.Errorf("amneziawg config line %d is invalid", lineNo)
		}
		norm := strings.ToLower(strings.TrimSpace(key))
		if norm == "" {
			return iniDoc{}, fmt.Errorf("amneziawg config line %d is invalid", lineNo)
		}
		if _, exists := current.values[norm]; exists || !supportedINIField(current.name, norm) {
			unsupportedFields = true
		}
		current.values[norm] = strings.TrimSpace(value)
	}
	if err := scanner.Err(); err != nil {
		return iniDoc{}, fmt.Errorf("amneziawg config could not be read")
	}
	return iniDoc{iface: iface, peers: peers, unsupportedFields: unsupportedFields}, nil
}

// Only fields used by the userspace sidecar are accepted at runtime. Unknown
// names and repeated assignments remain importable, but cannot be silently
// treated as though their effects were applied.
func supportedINIField(section, key string) bool {
	switch strings.ToLower(section) {
	case "interface":
		switch key {
		case "privatekey", "address", "dns", "mtu", "listenport",
			"jc", "junkpacketcount", "jmin", "junkpacketmin", "junkpacketminsize",
			"jmax", "junkpacketmax", "junkpacketmaxsize", "s1", "s2", "s3", "s4",
			"h1", "h1_", "h2", "h3", "h4", "i1", "i2", "i3", "i4", "i5",
			"j1", "j2", "j3", "itime", "initpackettimeout", "headerprotectionkey", "hp1",
			"contentpaddingaddition", "randomtrailers", "disablecookies", "rekeyaftertime",
			"rekeytimeout", "rejectaftertime", "keepalivetimeout", "maxhandshakeattempts":
			return true
		}
	case "peer":
		switch key {
		case "publickey", "presharedkey", "endpoint", "allowedips", "persistentkeepalive":
			return true
		}
	}
	return false
}

// Parse decodes an AmneziaWG INI config. Field values are never copied into
// returned errors.
func Parse(raw []byte) (Config, error) {
	raw = bytes.TrimSpace(bytes.TrimPrefix(bytes.TrimSpace(raw), []byte{0xEF, 0xBB, 0xBF}))
	format, err := Classify(raw)
	if err != nil {
		return Config{}, err
	}
	if format != FormatAmneziaWG {
		return Config{}, ErrUnknownFormat
	}
	doc, err := parseINI(raw)
	if err != nil {
		return Config{}, err
	}
	if doc.iface == nil {
		return Config{}, ErrMissingInterface
	}
	if len(doc.peers) == 0 {
		return Config{}, ErrMissingPeer
	}
	for _, group := range []struct {
		keys    []string
		numeric bool
	}{
		{[]string{"jc", "junkpacketcount"}, true},
		{[]string{"jmin", "junkpacketmin", "junkpacketminsize"}, true},
		{[]string{"jmax", "junkpacketmax", "junkpacketmaxsize"}, true},
		{[]string{"itime", "initpackettimeout"}, true},
		{[]string{"h1", "h1_"}, false},
		{[]string{"headerprotectionkey", "hp1"}, false},
	} {
		if conflictingAliases(doc.iface, group.keys, group.numeric) {
			doc.unsupportedFields = true
		}
	}
	cfg := Config{
		UnsupportedFields:      doc.unsupportedFields,
		PrivateKey:             doc.iface.get("privatekey"),
		Addresses:              splitList(doc.iface.get("address")),
		DNS:                    splitList(doc.iface.get("dns")),
		HeaderProtectionKey:    firstNonEmpty(doc.iface.get("headerprotectionkey"), doc.iface.get("hp1")),
		ContentPaddingAddition: doc.iface.get("contentpaddingaddition"),
		RandomTrailers:         doc.iface.get("randomtrailers"),
		DisableCookies:         doc.iface.get("disablecookies"),
		I: [5]string{
			doc.iface.get("i1"),
			doc.iface.get("i2"),
			doc.iface.get("i3"),
			doc.iface.get("i4"),
			doc.iface.get("i5"),
		},
		J: [3]string{
			doc.iface.get("j1"),
			doc.iface.get("j2"),
			doc.iface.get("j3"),
		},
		H: [4]string{
			firstNonEmpty(doc.iface.get("h1"), doc.iface.get("h1_")),
			doc.iface.get("h2"),
			doc.iface.get("h3"),
			doc.iface.get("h4"),
		},
	}
	fields := []struct {
		keys []string
		dest *int
		max  uint64
	}{
		{[]string{"mtu"}, &cfg.MTU, 65535},
		{[]string{"listenport"}, &cfg.ListenPort, 65535},
		{[]string{"jc", "junkpacketcount"}, &cfg.Jc, 128},
		{[]string{"jmin", "junkpacketmin", "junkpacketminsize"}, &cfg.Jmin, 65507},
		{[]string{"jmax", "junkpacketmax", "junkpacketmaxsize"}, &cfg.Jmax, 65507},
		{[]string{"s1"}, &cfg.S[0], 65535},
		{[]string{"s2"}, &cfg.S[1], 65535},
		{[]string{"s3"}, &cfg.S[2], 65535},
		{[]string{"s4"}, &cfg.S[3], 65535},
		{[]string{"itime", "initpackettimeout"}, &cfg.ITime, 1<<32 - 1},
	}
	for _, field := range fields {
		value, err := numericField(doc.iface, field.keys, field.max)
		if err != nil {
			return Config{}, err
		}
		*field.dest = value
	}
	for _, field := range []struct {
		key  string
		dest *string
	}{
		{"rekeyaftertime", &cfg.RekeyAfterTime},
		{"rekeytimeout", &cfg.RekeyTimeout},
		{"rejectaftertime", &cfg.RejectAfterTime},
		{"keepalivetimeout", &cfg.KeepaliveTimeout},
		{"maxhandshakeattempts", &cfg.MaxHandshakeAttempts},
	} {
		value, err := rangeField(doc.iface, field.key, 1<<32-1)
		if err != nil {
			return Config{}, err
		}
		*field.dest = value
	}
	for _, peerSec := range doc.peers {
		peer := Peer{
			PublicKey:    peerSec.get("publickey"),
			PresharedKey: peerSec.get("presharedkey"),
			Endpoint:     peerSec.get("endpoint"),
			AllowedIPs:   splitList(peerSec.get("allowedips")),
		}
		keepalive, err := rangeField(peerSec, "persistentkeepalive", 65535)
		if err != nil {
			return Config{}, err
		}
		if strings.Contains(keepalive, "-") {
			peer.PersistentKeepaliveRange = keepalive
		} else if keepalive != "" {
			peer.PersistentKeepalive, _ = strconv.Atoi(keepalive)
		}
		cfg.Peers = append(cfg.Peers, peer)
	}
	if err := Validate(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func conflictingAliases(section *iniSection, keys []string, numeric bool) bool {
	var first string
	found := false
	for _, key := range keys {
		value, exists := section.values[key]
		if !exists || strings.TrimSpace(value) == "" {
			continue
		}
		value = strings.TrimSpace(value)
		if numeric {
			if parsed, err := strconv.ParseUint(value, 10, 64); err == nil {
				value = strconv.FormatUint(parsed, 10)
			}
		}
		if found && value != first {
			return true
		}
		first, found = value, true
	}
	return false
}

// rangeField accepts the unsigned scalar and range syntax supported by the
// pinned AWG UAPI; errors never include the supplied configuration value.
func rangeField(section *iniSection, key string, maximum uint64) (string, error) {
	raw, present := section.values[key]
	if !present {
		return "", nil
	}
	raw = strings.TrimSpace(raw)
	parts := strings.Split(raw, "-")
	if len(parts) < 1 || len(parts) > 2 {
		return "", fmt.Errorf("amneziawg %s must be a valid unsigned range", key)
	}
	lo, err := strconv.ParseUint(parts[0], 10, 32)
	if err != nil || lo > maximum {
		return "", fmt.Errorf("amneziawg %s must be a valid unsigned range", key)
	}
	hi := lo
	if len(parts) == 2 {
		hi, err = strconv.ParseUint(parts[1], 10, 32)
		if err != nil || hi > maximum || hi < lo || hi-lo == 1<<32-1 {
			return "", fmt.Errorf("amneziawg %s must be a valid unsigned range", key)
		}
	}
	if len(parts) == 2 {
		return fmt.Sprintf("%d-%d", lo, hi), nil
	}
	return strconv.FormatUint(lo, 10), nil
}

func Validate(cfg Config) error {
	if strings.TrimSpace(cfg.PrivateKey) == "" {
		return ErrPrivateKeyMissing
	}
	if len(cfg.Addresses) == 0 {
		return ErrAddressMissing
	}
	if cfg.Jmin > cfg.Jmax {
		return errors.New("amneziawg jmin is greater than jmax")
	}
	if cfg.Jc < 0 || cfg.Jc > 128 {
		return errors.New("amneziawg jc is out of range")
	}
	// The native transport prepends a 16-byte header and appends a 16-byte
	// authentication tag in its fixed packet buffer. Keep the complete UDP
	// payload within the IPv4 datagram limit, including obfuscation padding.
	mtu := cfg.MTU
	if mtu == 0 {
		mtu = 1280
	}
	if mtu < 0 || cfg.S[3] < 0 || cfg.S[3]+mtu+32 > 65507 {
		return errors.New("amneziawg mtu and s4 exceed the packet size limit")
	}
	seenH := map[string]struct{}{}
	for _, header := range cfg.H {
		header = strings.TrimSpace(header)
		if header == "" {
			continue
		}
		if _, exists := seenH[header]; exists {
			return errors.New("amneziawg handshake headers must be unique")
		}
		seenH[header] = struct{}{}
	}
	if len(cfg.Peers) == 0 {
		return ErrMissingPeer
	}
	for _, field := range []struct{ key, value string }{
		{"rekeyaftertime", cfg.RekeyAfterTime}, {"rekeytimeout", cfg.RekeyTimeout},
		{"rejectaftertime", cfg.RejectAfterTime}, {"keepalivetimeout", cfg.KeepaliveTimeout},
		{"maxhandshakeattempts", cfg.MaxHandshakeAttempts},
	} {
		if field.value != "" {
			if _, err := rangeField(&iniSection{values: map[string]string{field.key: field.value}}, field.key, 1<<32-1); err != nil {
				return err
			}
		}
	}
	for _, peer := range cfg.Peers {
		if strings.TrimSpace(peer.PublicKey) == "" {
			return ErrPeerPublicKey
		}
		if strings.TrimSpace(peer.Endpoint) == "" || !strings.Contains(peer.Endpoint, ":") {
			return ErrPeerEndpoint
		}
		if peer.PersistentKeepalive < 0 || peer.PersistentKeepalive > 65535 {
			return errors.New("amneziawg persistentkeepalive out of range")
		}
		if peer.PersistentKeepaliveRange != "" {
			if _, err := rangeField(&iniSection{values: map[string]string{"persistentkeepalive": peer.PersistentKeepaliveRange}}, "persistentkeepalive", 65535); err != nil {
				return err
			}
		}
	}
	return nil
}

// ValidateRuntime rejects fields which the pinned v3 UAPI cannot carry. The
// profile remains importable, allowing other nodes in an envelope to work.
func ValidateRuntime(cfg Config) error {
	if cfg.UnsupportedFields {
		return errors.New("AmneziaWG: конфигурация содержит неподдерживаемые или повторяющиеся параметры")
	}
	for _, toggle := range []struct{ name, value string }{
		{"RandomTrailers", cfg.RandomTrailers},
		{"DisableCookies", cfg.DisableCookies},
	} {
		if toggle.value == "" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(toggle.value)) {
		case "1", "t", "true", "yes", "on", "0", "f", "false", "no", "off":
		default:
			return fmt.Errorf("AmneziaWG: неверное значение параметра %s", toggle.name)
		}
	}
	for _, value := range cfg.J {
		if strings.TrimSpace(value) != "" {
			return errors.New("AmneziaWG: параметры J1–J3 не поддерживаются текущим ядром")
		}
	}
	if cfg.ITime != 0 {
		return errors.New("AmneziaWG: параметр ITime не поддерживается текущим ядром")
	}
	return nil
}

func splitList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';'
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func numericField(section *iniSection, keys []string, maximum uint64) (int, error) {
	var result int
	found := false
	for _, key := range keys {
		raw, present := section.values[key]
		if !present {
			continue
		}
		value, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || value < 0 || uint64(value) > maximum {
			// strconv's error includes the supplied value; do not expose it.
			return 0, fmt.Errorf("amneziawg %s must be an integer in range 0..%d", key, maximum)
		}
		if !found {
			result, found = value, true
		}
	}
	return result, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
