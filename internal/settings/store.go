package settings

import (
	"os"
	"path/filepath"

	"github.com/hexworks/agentfiles/internal/config"
	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/utils"
)

// Store owns loading and saving the centralized settings file. Shape
// mirrors projectstore.Store / registry.Store so wiring in cmd/af/main
// stays symmetrical.
type Store struct {
	Path string
}

// DefaultPath returns the conventional location of settings.json next to
// profiles.json and projects.json under ~/.agentfiles.
func DefaultPath() (string, errs.DomainError) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", HomeDirUnavailableError{Err: err}
	}
	return filepath.Join(home, config.UserConfigDirName, config.SettingsStoreFileName), nil
}

// NewStore builds a Store rooted at path. An empty path defers to
// DefaultPath; when that fails the returned store's Path is empty and
// I/O calls surface the underlying filesystem error naturally.
func NewStore(path string) *Store {
	if path == "" {
		path, _ = DefaultPath()
	}
	return &Store{Path: path}
}

// Load reads the store and returns the persisted Settings. A missing
// file returns Default() with no error so first-time users start with
// the safe (git-disabled) defaults.
func (s *Store) Load() (Settings, errs.DomainError) {
	if !utils.Exists(s.Path) {
		return Default(), nil
	}
	var v Settings
	if err := utils.ReadJSON(s.Path, &v); err != nil {
		return Settings{}, err
	}
	if v.Version == 0 {
		v.Version = Version
	}
	return v, nil
}

// Save writes settings to disk atomically with owner-only permissions,
// matching the projects/registry stores.
func (s *Store) Save(v Settings) errs.DomainError {
	if v.Version == 0 {
		v.Version = Version
	}
	return utils.WriteJSONAtomic(s.Path, v, 0o700, 0o600)
}
