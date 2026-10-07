// Package storage persists profiles outside the Flutter process.
package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"naga.network/core/policy"
	"naga.network/core/profile"
	"naga.network/core/profile/olcrtc"
)

const fileMode os.FileMode = 0o600

const CurrentSchemaVersion = 1

// Store persists profiles and settings. After New, do not copy the value:
// settings methods use a mutex.
type Store struct {
	Root string
	mu   *sync.Mutex
}

func (s *Store) settingsRoot() string { return filepath.Join(s.Root, ".settings") }

var errSettingsUnchanged = errors.New("settings unchanged")

func (s *Store) LoadConnectionPolicy() (policy.ConnectionPolicy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	settings, err := s.loadSettings()
	return settings.Connection, err
}

func (s *Store) SaveConnectionPolicy(value policy.ConnectionPolicy) error {
	if value.TrafficMode == "" {
		value.TrafficMode = policy.TrafficTUN
	}
	if err := value.Validate(); err != nil {
		return err
	}
	return s.withSettings(func(settings *storedSettings) error {
		value.SchemaVersion = 1
		value.Revision = settings.Connection.Revision + 1
		settings.Connection = value
		return nil
	})
}

// PatchConnectionPolicy mutates the stored connection policy under the settings
// mutex. A no-op patch does not write the file or bump revision.
func (s *Store) PatchConnectionPolicy(fn func(*policy.ConnectionPolicy) error) error {
	if fn == nil {
		return errors.New("connection policy patch is nil")
	}
	return s.withSettings(func(settings *storedSettings) error {
		before := settings.Connection
		if err := fn(&settings.Connection); err != nil {
			return err
		}
		if settings.Connection.TrafficMode == "" {
			settings.Connection.TrafficMode = policy.TrafficTUN
		}
		if err := settings.Connection.Validate(); err != nil {
			return err
		}
		patched := settings.Connection
		patched.Revision = before.Revision
		patched.SchemaVersion = before.SchemaVersion
		if patched == before {
			settings.Connection = before
			return errSettingsUnchanged
		}
		settings.Connection.SchemaVersion = 1
		settings.Connection.Revision = before.Revision + 1
		return nil
	})
}

func (s *Store) LoadRoutingPolicy() (policy.RoutingPolicy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	settings, err := s.loadSettings()
	return settings.Routing, err
}

func (s *Store) SaveRoutingPolicy(value policy.RoutingPolicy) error {
	if err := value.Validate(); err != nil {
		return err
	}
	return s.withSettings(func(settings *storedSettings) error {
		value.SchemaVersion = 2
		value.Revision = settings.Routing.Revision + 1
		if value.Apps == nil {
			value.Apps = []policy.AppRoute{}
		}
		value.RUDirect = value.BuiltinRU
		settings.Routing = value
		return nil
	})
}

func (s *Store) withSettings(fn func(*storedSettings) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	settings, err := s.loadSettings()
	if err != nil {
		return err
	}
	if err := fn(&settings); err != nil {
		if errors.Is(err, errSettingsUnchanged) {
			return nil
		}
		return err
	}
	return s.saveSettings(settings)
}

type storedSettings struct {
	Connection policy.ConnectionPolicy `json:"connection"`
	Routing    policy.RoutingPolicy    `json:"routing"`
}

func (s *Store) loadSettings() (storedSettings, error) {
	path := filepath.Join(s.settingsRoot(), "settings.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return storedSettings{
			Connection: policy.DefaultConnectionPolicy(),
			Routing:    policy.DefaultRoutingPolicy(),
		}, nil
	}
	if err != nil {
		return storedSettings{}, fmt.Errorf("read settings: %w", err)
	}
	var settings storedSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return storedSettings{}, fmt.Errorf("decode settings: %w", err)
	}
	if settings.Connection.Mode == "" {
		settings.Connection = policy.DefaultConnectionPolicy()
	} else if settings.Connection.TrafficMode == "" {
		settings.Connection.TrafficMode = policy.TrafficTUN
	}
	if settings.Routing.Mode == "" {
		settings.Routing = policy.DefaultRoutingPolicy()
	}
	return settings, nil
}

func (s *Store) saveSettings(settings storedSettings) error {
	if err := os.MkdirAll(s.settingsRoot(), 0o700); err != nil {
		return fmt.Errorf("create settings root: %w", err)
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}
	return atomicWrite(filepath.Join(s.settingsRoot(), "settings.json"), data)
}

func New(root string) (Store, error) {
	if strings.TrimSpace(root) == "" {
		return Store{}, errors.New("storage root is empty")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return Store{}, fmt.Errorf("create storage root: %w", err)
	}
	return Store{Root: root, mu: &sync.Mutex{}}, nil
}

func NewDefault() (Store, error) {
	root, err := DefaultRoot()
	if err != nil {
		return Store{}, err
	}
	return New(filepath.Join(root, "profiles"))
}

func (s *Store) Save(value profile.Profile) error {
	if err := validateID(value.ID); err != nil {
		return err
	}
	if len(value.Config) == 0 {
		return errors.New("profile config is empty")
	}
	if value.SchemaVersion == 0 {
		value.SchemaVersion = CurrentSchemaVersion
	}
	if value.SchemaVersion > CurrentSchemaVersion {
		return fmt.Errorf("profile schema version %d is newer than supported version %d", value.SchemaVersion, CurrentSchemaVersion)
	}
	if value.Revision == 0 {
		value.Revision = 1
	}
	if value.UpdatedAt.IsZero() {
		value.UpdatedAt = time.Now().UTC()
	}
	if err := os.MkdirAll(s.Root, 0o700); err != nil {
		return fmt.Errorf("create storage root: %w", err)
	}

	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode profile: %w", err)
	}
	path := filepath.Join(s.Root, value.ID+".json")
	if err := atomicWrite(path, data); err != nil {
		return fmt.Errorf("save profile: %w", err)
	}
	return nil
}

// SaveVersioned stores a new known-good profile and keeps the previous
// profile in a private history directory. Ordinary Save remains intentionally
// history-free because selecting a node should not create a new config
// version.
func (s *Store) SaveVersioned(value profile.Profile) error {
	if err := validateID(value.ID); err != nil {
		return err
	}
	path := filepath.Join(s.Root, value.ID+".json")
	if previous, err := os.ReadFile(path); err == nil {
		var previousProfile profile.Profile
		if decodeErr := json.Unmarshal(previous, &previousProfile); decodeErr == nil && previousProfile.Revision > 0 {
			value.Revision = previousProfile.Revision + 1
		}
		historyDir := filepath.Join(s.Root, ".history", value.ID)
		if err := os.MkdirAll(historyDir, 0o700); err != nil {
			return fmt.Errorf("create profile history: %w", err)
		}
		historyPath := filepath.Join(historyDir, fmt.Sprintf("%020d.json", time.Now().UTC().UnixNano()))
		if err := atomicWrite(historyPath, previous); err != nil {
			return fmt.Errorf("save previous profile version: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read previous profile version: %w", err)
	}
	if value.Revision <= 0 {
		value.Revision = 1
	}
	value.UpdatedAt = time.Now().UTC()
	return s.Save(value)
}

// Rollback restores the most recent version saved before the current one.
// The restored version is itself saved atomically as the current version.
func (s *Store) Rollback(id string) (profile.Profile, error) {
	if err := validateID(id); err != nil {
		return profile.Profile{}, err
	}
	historyDir := filepath.Join(s.Root, ".history", id)
	entries, err := os.ReadDir(historyDir)
	if err != nil {
		return profile.Profile{}, fmt.Errorf("read profile history: %w", err)
	}
	if len(entries) == 0 {
		return profile.Profile{}, errors.New("profile history is empty")
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() > entries[j].Name() })
	data, err := os.ReadFile(filepath.Join(historyDir, entries[0].Name()))
	if err != nil {
		return profile.Profile{}, fmt.Errorf("read rollback profile: %w", err)
	}
	var value profile.Profile
	if err := json.Unmarshal(data, &value); err != nil {
		return profile.Profile{}, fmt.Errorf("decode rollback profile: %w", err)
	}
	if value.ID != id {
		return profile.Profile{}, errors.New("rollback profile ID mismatch")
	}
	if err := s.Save(value); err != nil {
		return profile.Profile{}, err
	}
	return value, nil
}

func (s *Store) Load(id string) (profile.Profile, error) {
	if err := validateID(id); err != nil {
		return profile.Profile{}, err
	}
	data, err := os.ReadFile(filepath.Join(s.Root, id+".json"))
	if err != nil {
		return profile.Profile{}, fmt.Errorf("read profile: %w", err)
	}
	var value profile.Profile
	if err := json.Unmarshal(data, &value); err != nil {
		return profile.Profile{}, fmt.Errorf("decode profile: %w", err)
	}
	if value.SchemaVersion == 0 {
		value.SchemaVersion = CurrentSchemaVersion
	}
	if value.SchemaVersion > CurrentSchemaVersion {
		return profile.Profile{}, fmt.Errorf("profile schema version %d is newer than supported version %d", value.SchemaVersion, CurrentSchemaVersion)
	}
	return value, nil
}

func (s *Store) Delete(id string) error {
	if err := validateID(id); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(s.Root, id+".json")); err != nil {
		return fmt.Errorf("delete profile: %w", err)
	}
	if err := os.RemoveAll(filepath.Join(s.Root, ".history", id)); err != nil {
		return fmt.Errorf("delete profile history: %w", err)
	}
	if err := os.RemoveAll(olcrtc.ConfigDir(s.Root, id)); err != nil {
		return fmt.Errorf("delete olcrtc config: %w", err)
	}
	return nil
}

func (s *Store) List() ([]profile.Profile, error) {
	entries, err := os.ReadDir(s.Root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("list profiles: %w", err)
	}

	profiles := make([]profile.Profile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		value, err := s.Load(id)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, value)
	}
	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].ImportedAt.Before(profiles[j].ImportedAt)
	})
	return profiles, nil
}

func atomicWrite(path string, data []byte) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".naga-profile-*.tmp")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(fileMode); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryName, path)
}

func validateID(id string) error {
	if id == "" || filepath.Base(id) != id || strings.Contains(id, string(filepath.Separator)) {
		return errors.New("profile ID is invalid")
	}
	return nil
}
