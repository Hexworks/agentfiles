// Package registry persists the global list of known profiles and resolves
// user-facing references (id, name, or path) to a concrete profile folder.
package registry

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/hexworks/agentfiles/internal/config"
	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/utils"
)

// Version is the current registry file schema version written by Save.
const Version = 1

// ProfileRef is the lightweight, global metadata stored in ~/.agentfiles.json.
//
// It intentionally does not contain the whole profile model; it only provides
// enough information to discover and resolve a profile folder quickly.
type ProfileRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
	// Source tells where the profileRef came from.
	// currently only valid value is "local" as we don't support
	// other sources yet
	Source       string    `json:"source"`
	ManagedBy    string    `json:"managed_by"`
	CreatedAt    time.Time `json:"created_at"`
	LastOpenedAt time.Time `json:"last_opened_at"`
}

// Registry is the serialized top-level structure stored in the global file.
type Registry struct {
	Version  int          `json:"version"`
	Profiles []ProfileRef `json:"profiles"`
}

// Store owns loading and saving the global profile registry file.
type Store struct {
	Path string
}

// DefaultPath returns the conventional location of the global registry.
func DefaultPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, config.RegistryFileName)
}

// NewStore creates a registry store. An empty path means "use the default
// global registry location".
func NewStore(path string) *Store {
	if path == "" {
		path = DefaultPath()
	}
	return &Store{Path: path}
}

// Load returns the current registry contents. Missing registry files are treated
// as an empty registry so first-time use does not need special handling.
func (s *Store) Load() (*Registry, errs.DomainError) {
	if !utils.Exists(s.Path) {
		return &Registry{Version: Version, Profiles: []ProfileRef{}}, nil
	}
	var reg Registry
	if err := utils.ReadJSON(s.Path, &reg); err != nil {
		return nil, err
	}
	if reg.Version == 0 {
		reg.Version = Version
	}
	if reg.Profiles == nil {
		reg.Profiles = []ProfileRef{}
	}
	return &reg, nil
}

// Save sorts profiles by name before writing so the registry remains stable and
// diff-friendly in Git.
func (s *Store) Save(reg *Registry) errs.DomainError {
	reg.Version = Version
	slices.SortFunc(reg.Profiles, func(a, b ProfileRef) int {
		return strings.Compare(a.Name, b.Name)
	})
	return utils.WriteJSON(s.Path, reg)
}

// Add appends a profile reference after checking the registry-wide uniqueness
// rules for id, display name, and filesystem path.
func (s *Store) Add(ref ProfileRef) errs.DomainError {
	reg, err := s.Load()
	if err != nil {
		return err
	}
	for _, existing := range reg.Profiles {
		if existing.ID == ref.ID {
			return ProfileIDExistsError{ID: ref.ID}
		}
		if strings.EqualFold(existing.Name, ref.Name) {
			return ProfileNameExistsError{Name: ref.Name}
		}
		if existing.Path == ref.Path {
			return ProfilePathExistsError{Path: ref.Path}
		}
	}
	reg.Profiles = append(reg.Profiles, ref)
	return s.Save(reg)
}

// Remove deletes the profile reference matching profileID from the global
// registry. A missing id is reported as ProfileNotFoundError so callers
// can distinguish "nothing to do" from "remove succeeded".
func (s *Store) Remove(profileID string) errs.DomainError {
	reg, err := s.Load()
	if err != nil {
		return err
	}
	for i, existing := range reg.Profiles {
		if existing.ID == profileID {
			reg.Profiles = append(reg.Profiles[:i], reg.Profiles[i+1:]...)
			return s.Save(reg)
		}
	}
	return ProfileNotFoundError{Ref: profileID}
}

// Touch updates the last-opened timestamp for one profile. The timestamp is
// operational metadata only; it does not affect rendering behavior.
func (s *Store) Touch(profileID string) errs.DomainError {
	reg, err := s.Load()
	if err != nil {
		return err
	}
	for i := range reg.Profiles {
		if reg.Profiles[i].ID == profileID {
			reg.Profiles[i].LastOpenedAt = time.Now().UTC()
			return s.Save(reg)
		}
	}
	return ProfileNotFoundError{Ref: profileID}
}

// Resolve finds a profile by any user-facing identifier the TUI accepts:
// id, name, or exact path.
func (s *Store) Resolve(ref string) (*ProfileRef, errs.DomainError) {
	reg, err := s.Load()
	if err != nil {
		return nil, err
	}
	for _, profile := range reg.Profiles {
		if profile.ID == ref || profile.Name == ref || profile.Path == ref {
			copy := profile
			return &copy, nil
		}
	}
	return nil, ProfileNotFoundError{Ref: ref}
}
