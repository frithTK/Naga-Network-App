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
	Peers                  []Peer
}

type Peer struct {
	PublicKey           string
	PresharedKey        string
	Endpoint            string
	AllowedIPs          []string
	PersistentKeepalive int
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
	iface *iniSection
	peers []*iniSection
}

func parseINI(raw []byte) (iniDoc, error) {
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	var current *iniSection
	var iface *iniSection
	peers := make([]*iniSection, 0)
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
		current.values[norm] = strings.TrimSpace(value)
	}
	if err := scanner.Err(); err != nil {
		return iniDoc{}, fmt.Errorf("amneziawg config could not be read")
	}
	return iniDoc{iface: iface, peers: peers}, nil
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
	cfg := Config{
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
	for _, peerSec := range doc.peers {
		peer := Peer{
			PublicKey:    peerSec.get("publickey"),
			PresharedKey: peerSec.get("presharedkey"),
			Endpoint:     peerSec.get("endpoint"),
			AllowedIPs:   splitList(peerSec.get("allowedips")),
		}
		keepalive, err := numericField(peerSec, []string{"persistentkeepalive"}, 65535)
		if err != nil {
			return Config{}, err
		}
		peer.PersistentKeepalive = keepalive
		cfg.Peers = append(cfg.Peers, peer)
	}
	if err := Validate(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
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
	for _, peer := range cfg.Peers {
		if strings.TrimSpace(peer.PublicKey) == "" {
			return ErrPeerPublicKey
		}
		if strings.TrimSpace(peer.Endpoint) == "" || !strings.Contains(peer.Endpoint, ":") {
			return ErrPeerEndpoint
		}
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
