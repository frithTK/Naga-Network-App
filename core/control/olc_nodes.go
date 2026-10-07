package control

import (
	"bytes"
	"strings"

	"naga.network/core/profile"
	olcprofile "naga.network/core/profile/olcrtc"
)

func olcProfiles(raw []byte) []olcprofile.Profile {
	if cfg := profile.RuntimeOlcRTC(raw); cfg != nil {
		return cfg.Profiles
	}
	if cfg, err := olcprofile.ParseDocument(raw); err == nil && cfg != nil {
		return cfg.Profiles
	}
	if olcprofile.IsURI(raw) {
		parsed, err := olcprofile.ParseURI(string(bytes.TrimSpace(raw)))
		if err == nil && parsed.RuntimeAllowed {
			return []olcprofile.Profile{parsed}
		}
	}
	return nil
}

// olcMatches reports whether mode selects the whole olcRTC set or one named
// profile inside raw.
func olcMatches(raw []byte, mode string) bool {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		return false
	}
	profiles := olcProfiles(raw)
	if len(profiles) == 0 {
		return false
	}
	if mode == olcprofile.NodeTag {
		return true
	}
	index, ok := olcprofile.Index(profiles, mode)
	return ok && index >= 0
}

func olcShouldStart(value profile.Profile) bool {
	if olcMatches(value.Config, value.SelectedMode) {
		return true
	}
	return value.Engine == profile.EngineOlcRTC && strings.TrimSpace(value.SelectedMode) == ""
}
