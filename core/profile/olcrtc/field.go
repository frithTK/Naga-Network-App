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
	Ignored  int
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
	if len(raw) > maxDocumentBytes {
		return nil, fmt.Errorf("%w: document too large", ErrURI)
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, nil
	}
	var document fieldFile
	if err := json.Unmarshal(trimmed, &document); err != nil {
		return nil, errors.New("olcrtc field is invalid")
	}
	cfg := &Config{Label: strings.TrimSpace(document.Label)}
	if len(document.Profiles) == 0 {
		if len(document.URIs) == 0 {
			return nil, errors.New("olcrtc field has no profiles")
		}
		if len(document.URIs) > maxDocumentProfiles {
			return nil, fmt.Errorf("%w: too many profiles", ErrURI)
		}
		for _, uri := range document.URIs {
			if len(uri) > maxDocumentLineBytes {
				return nil, fmt.Errorf("%w: line too large", ErrURI)
			}
			profile, err := ParseURI(uri)
			if err != nil || !profile.RuntimeAllowed {
				cfg.Ignored++
				continue
			}
			cfg.Profiles = append(cfg.Profiles, profile)
			cfg.URIs = append(cfg.URIs, uri)
		}
		return cfg, nil
	}
	if len(document.Profiles) > maxDocumentProfiles || len(document.URIs) > maxDocumentProfiles {
		return nil, fmt.Errorf("%w: too many profiles", ErrURI)
	}
	if len(document.URIs) > len(document.Profiles) {
		return nil, errors.New("olcrtc uri list exceeds profiles")
	}
	for i, item := range document.Profiles {
		profile, err := profileFromFields(item)
		if err != nil {
			cfg.Ignored++
			continue
		}
		if i < len(document.URIs) {
			uri := document.URIs[i]
			if len(uri) > maxDocumentLineBytes {
				return nil, fmt.Errorf("%w: line too large", ErrURI)
			}
			parsed, err := ParseURI(uri)
			if err != nil || parsed.Provider != profile.Provider || parsed.Room != profile.Room || parsed.Key != profile.Key || parsed.Transport != profile.Transport {
				return nil, errors.New("olcrtc uri does not match its profile")
			}
			if !parsed.RuntimeAllowed {
				cfg.Ignored++
				continue
			}
			profile.Params = parsed.Params
			cfg.URIs = append(cfg.URIs, uri)
		}
		cfg.Profiles = append(cfg.Profiles, profile)
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
