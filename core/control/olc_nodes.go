package control

import (
	"bytes"
	"errors"
	"strings"

	"naga.network/core/profile"
	olcprofile "naga.network/core/profile/olcrtc"
)

var errOlcSelectionChanged = errors.New("выбранный узел olcRTC отсутствует или неоднозначен в обновлённой подписке; выберите узел заново")

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

// remapOlcSelection keeps the selected room tied to its previous identity.
// Names and providers are the only server-provided stable identity candidates;
// duplicates need an unchanged, unique room to avoid silently switching nodes.
func remapOlcSelection(oldRaw, newRaw []byte, mode string) (string, error) {
	oldProfiles := olcProfiles(oldRaw)
	newProfiles := olcProfiles(newRaw)
	oldIndex, ok := olcprofile.Index(oldProfiles, mode)
	if !ok || len(newProfiles) == 0 {
		return "", errOlcSelectionChanged
	}
	if oldIndex < 0 {
		return mode, nil
	}
	selected := oldProfiles[oldIndex]
	identityMatches := func(a, b olcprofile.Profile) bool {
		return strings.TrimSpace(a.Name) == strings.TrimSpace(b.Name) &&
			strings.EqualFold(strings.TrimSpace(a.Provider), strings.TrimSpace(b.Provider)) &&
			strings.EqualFold(strings.TrimSpace(a.Transport), strings.TrimSpace(b.Transport))
	}
	oldCount := 0
	for _, p := range oldProfiles {
		if identityMatches(selected, p) {
			oldCount++
		}
	}
	matches := make([]int, 0, 1)
	for i, p := range newProfiles {
		if identityMatches(selected, p) {
			matches = append(matches, i)
		}
	}
	if len(matches) == 0 {
		return "", errOlcSelectionChanged
	}
	index := matches[0]
	if oldCount != 1 || len(matches) != 1 {
		index = -1
		for _, candidate := range matches {
			if newProfiles[candidate].Room == selected.Room {
				if index != -1 {
					return "", errOlcSelectionChanged
				}
				index = candidate
			}
		}
		if index < 0 {
			return "", errOlcSelectionChanged
		}
		// An identical previous room was already ambiguous even before refresh.
		previousRoomCount := 0
		for _, p := range oldProfiles {
			if identityMatches(selected, p) && p.Room == selected.Room {
				previousRoomCount++
			}
		}
		if previousRoomCount != 1 {
			return "", errOlcSelectionChanged
		}
	}
	tag := olcprofile.Tags(newProfiles)[index]
	if sep := strings.LastIndex(mode, "::"); sep >= 0 {
		return mode[:sep+2] + tag, nil
	}
	return tag, nil
}

func olcShouldStart(value profile.Profile) bool {
	if olcMatches(value.Config, value.SelectedMode) {
		return true
	}
	return value.Engine == profile.EngineOlcRTC && strings.TrimSpace(value.SelectedMode) == ""
}

// An engine prefix keeps equally named sing-box and olcRTC rows distinct.
func olcProbeKey(profileID, tag string) string { return "olcrtc/" + profileID + "::" + tag }
