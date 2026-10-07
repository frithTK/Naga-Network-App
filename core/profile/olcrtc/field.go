package olcrtc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Config is the optional olcrtc object inside a Naga envelope.
type Config struct {
	Label    string
	Profiles []Profile
	URIs     []string
}

type fieldFile struct {
	Label    string         `json:"label"`
	Profiles []fieldProfile `json:"profiles"`
	URIs     []string       `json:"uris"`
}

type fieldProfile struct {
	Name      string `json:"name"`
	Provider  string `json:"provider"`
	Transport string `json:"transport"`
	Room      string `json:"room"`
	Key       string `json:"key"`
}

// ParseField parses the envelope olcrtc value. A missing value or JSON null
// returns nil, nil. Any other failure is an error of this field only.
func ParseField(raw []byte) (*Config, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, nil
	}
	var document fieldFile
	if err := json.Unmarshal(trimmed, &document); err != nil {
		return nil, errors.New("olcrtc field is invalid")
	}
	cfg := &Config{Label: strings.TrimSpace(document.Label), URIs: append([]string(nil), document.URIs...)}
	if len(document.Profiles) == 0 {
		if len(document.URIs) == 0 {
			return nil, errors.New("olcrtc field has no profiles")
		}
		profile, err := ParseURI(document.URIs[0])
		if err != nil {
			return nil, err
		}
		if !profile.RuntimeAllowed {
			return nil, ErrTransportOff
		}
		cfg.Profiles = []Profile{profile}
		return cfg, nil
	}
	key := ""
	for _, item := range document.Profiles {
		profile, err := profileFromFields(item)
		if err != nil {
			return nil, err
		}
		if key == "" {
			key = profile.Key
		} else if profile.Key != key {
			return nil, errors.New("olcrtc profiles use different keys")
		}
		cfg.Profiles = append(cfg.Profiles, profile)
	}
	if len(document.URIs) > 0 {
		first, err := ParseURI(document.URIs[0])
		if err != nil {
			return nil, err
		}
		head := cfg.Profiles[0]
		if first.Provider != head.Provider || first.Room != head.Room || first.Key != head.Key {
			return nil, errors.New("olcrtc uri does not match the first profile")
		}
	}
	return cfg, nil
}

func profileFromFields(item fieldProfile) (Profile, error) {
	name := strings.TrimSpace(item.Name)
	if name == "" || strings.TrimSpace(item.Provider) == "" || strings.TrimSpace(item.Transport) == "" || strings.TrimSpace(item.Room) == "" || strings.TrimSpace(item.Key) == "" {
		return Profile{}, errors.New("olcrtc profile is incomplete")
	}
	raw := fmt.Sprintf("olcrtc://%s?%s@%s#%s$%s", strings.TrimSpace(item.Provider), strings.TrimSpace(item.Transport), strings.TrimSpace(item.Room), strings.TrimSpace(item.Key), name)
	profile, err := ParseURI(raw)
	if err != nil {
		return Profile{}, err
	}
	if !profile.RuntimeAllowed {
		return Profile{}, ErrTransportOff
	}
	return profile, nil
}

// ShareURI is the olcrtc:// string safe to copy. Profiles stay inside the app.
func (c *Config) ShareURI() string {
	if c == nil || len(c.URIs) == 0 {
		return ""
	}
	return strings.TrimSpace(c.URIs[0])
}
