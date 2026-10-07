package olcrtc

import (
	"fmt"
	"strings"
)

// Tags assigns the list name of each profile. An empty name uses NodeTag.
// A repeated name gets a numeric suffix so every row stays selectable.
func Tags(profiles []Profile) []string {
	used := make(map[string]struct{}, len(profiles))
	tags := make([]string, len(profiles))
	for i, profile := range profiles {
		base := strings.TrimSpace(profile.Name)
		if base == "" {
			base = NodeTag
		}
		tag := base
		for n := 2; ; n++ {
			if _, exists := used[tag]; !exists {
				break
			}
			tag = fmt.Sprintf("%s %d", base, n)
		}
		used[tag] = struct{}{}
		tags[i] = tag
	}
	return tags
}

// Index locates the profile selected by mode. NodeTag means every profile
// and returns -1, true. A profileID::tag prefix is stripped. An unknown mode
// returns -1, false.
func Index(profiles []Profile, mode string) (int, bool) {
	mode = strings.TrimSpace(mode)
	if sep := strings.LastIndex(mode, "::"); sep >= 0 {
		mode = strings.TrimSpace(mode[sep+2:])
	}
	if mode == "" {
		return -1, false
	}
	if mode == NodeTag {
		return -1, true
	}
	tags := Tags(profiles)
	for i, tag := range tags {
		if tag == mode {
			return i, true
		}
	}
	return -1, false
}

// Narrow keeps the selected profile. NodeTag and an empty mode keep every
// profile. The returned tag is the list name of a single profile, or NodeTag
// when several profiles stay together.
func Narrow(cfg *Config, mode string) (*Config, string, error) {
	if cfg == nil || len(cfg.Profiles) == 0 {
		return nil, "", fmt.Errorf("%w: profiles", ErrURI)
	}
	tags := Tags(cfg.Profiles)
	mode = strings.TrimSpace(mode)
	if mode == "" || mode == NodeTag {
		if len(tags) == 1 {
			return cfg, tags[0], nil
		}
		return cfg, NodeTag, nil
	}
	index, ok := Index(cfg.Profiles, mode)
	if !ok || index < 0 {
		return nil, "", fmt.Errorf("%w: profile", ErrURI)
	}
	next := *cfg
	next.Profiles = []Profile{cfg.Profiles[index]}
	return &next, tags[index], nil
}
