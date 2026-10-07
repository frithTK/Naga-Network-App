package profile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"naga.network/core/profile/olcrtc"
)

const (
	EnvelopeApp       = "naga-network"
	EnvelopeAppLegacy = "nagavpn"
	EnvelopeVersion   = 1
)

// Envelope is the Naga Network subscription wrapper. The stored profile keeps
// the original bytes; sing-box JSON and optional AmneziaWG conf are extracted
// only for validation and runtime.
type Envelope struct {
	Version     int
	App         string
	SingBox     []byte
	AmneziaConf []byte
	OlcRTC      *olcrtc.Config
}

type envelopeFile struct {
	V       int             `json:"v"`
	App     string          `json:"app"`
	SingBox json.RawMessage `json:"singbox"`
	Amnezia json.RawMessage `json:"amnezia"`
	OlcRTC  json.RawMessage `json:"olcrtc"`
}

type amneziaEnvelope struct {
	Conf string `json:"conf"`
}

// ParseEnvelope reports whether raw is a Naga Network envelope. recognized is
// true when app is naga-network or legacy nagavpn, even if the document is invalid.
func ParseEnvelope(raw []byte) (Envelope, bool, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return Envelope{}, false, nil
	}
	var document envelopeFile
	if err := json.Unmarshal(trimmed, &document); err != nil {
		return Envelope{}, false, nil
	}
	app := strings.ToLower(strings.TrimSpace(document.App))
	if !isEnvelopeApp(app) {
		return Envelope{}, false, nil
	}
	if document.V != EnvelopeVersion {
		return Envelope{}, true, fmt.Errorf("unsupported naga-network envelope version %d", document.V)
	}
	singbox := bytes.TrimSpace(document.SingBox)
	if len(singbox) == 0 || bytes.Equal(singbox, []byte("null")) {
		return Envelope{}, true, errors.New("naga-network envelope is missing singbox")
	}
	olc, _ := olcrtc.ParseField(document.OlcRTC)
	env := Envelope{
		Version: document.V,
		App:     app,
		SingBox: append([]byte(nil), singbox...),
		OlcRTC:  olc,
	}
	amnezia := bytes.TrimSpace(document.Amnezia)
	if len(amnezia) == 0 || bytes.Equal(amnezia, []byte("null")) {
		return env, true, nil
	}
	var wrapper amneziaEnvelope
	if err := json.Unmarshal(amnezia, &wrapper); err != nil {
		return Envelope{}, true, errors.New("naga-network amnezia field is invalid")
	}
	conf := strings.TrimSpace(wrapper.Conf)
	if conf == "" {
		return env, true, nil
	}
	env.AmneziaConf = []byte(conf)
	return env, true, nil
}

// OlcRTCError reports a broken olcrtc field without failing the envelope.
// A missing field and JSON null are not errors.
func OlcRTCError(raw []byte) error {
	_, ok, err := ParseEnvelope(raw)
	if !ok || err != nil {
		return nil
	}
	var document envelopeFile
	if json.Unmarshal(bytes.TrimSpace(raw), &document) != nil {
		return nil
	}
	_, fieldErr := olcrtc.ParseField(document.OlcRTC)
	return fieldErr
}

// RuntimeOlcRTC returns a copy of the parsed olcrtc field. Stored bytes stay unchanged.
func RuntimeOlcRTC(raw []byte) *olcrtc.Config {
	env, ok, err := ParseEnvelope(raw)
	if !ok || err != nil || env.OlcRTC == nil {
		return nil
	}
	copy := *env.OlcRTC
	copy.Profiles = append([]olcrtc.Profile(nil), env.OlcRTC.Profiles...)
	copy.URIs = append([]string(nil), env.OlcRTC.URIs...)
	return &copy
}

func isEnvelopeApp(app string) bool {
	return app == EnvelopeApp || app == EnvelopeAppLegacy
}

// RuntimeSingBoxConfig returns the sing-box JSON used at runtime. Envelope
// bytes stay unchanged on disk.
func RuntimeSingBoxConfig(raw []byte) []byte {
	env, ok, err := ParseEnvelope(raw)
	if ok && err == nil {
		return env.SingBox
	}
	return raw
}

// RuntimeAmneziaConf returns the AmneziaWG INI from an envelope, if present.
func RuntimeAmneziaConf(raw []byte) []byte {
	env, ok, err := ParseEnvelope(raw)
	if ok && err == nil {
		return env.AmneziaConf
	}
	return nil
}
