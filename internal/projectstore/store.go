// Package projectstore owns the centralized per-user project selection file
// (~/.agentfiles/projects.json). It exists because a profile folder is meant
// to be shareable: keeping per-machine target-repo paths and per-user asset
// selections inside a profile leaks the author's local state on the next
// share. The store groups projects by profile id, mirrors registry.Store's
// shape (Load/Save + typed CRUD), and preserves the global one-repo-path-per-
// project invariant that was previously enforced by walking every profile
// folder.
package projectstore

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hexworks/agentfiles/internal/config"
	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/project"
	"github.com/hexworks/agentfiles/internal/utils"
)

// Version is the current projects.json schema version written by Save.
const Version = 1

// State is the serialized top-level structure stored on disk.
//
// Grouping by profile id (rather than a flat list with a profile_id field
// on each manifest) keeps the shape aligned with how the app queries: "give
// me the projects for this profile". It also keeps project.Manifest
// oblivious to which profile owns it — the grouping is the store's concern.
type State struct {
	Version  int                            `json:"version"`
	Projects map[string][]*project.Manifest `json:"projects"`
}

// OwnedProject pairs a project manifest with the id of the profile that
// owns it. Returned by AllProjects for the global repo-path uniqueness
// check.
type OwnedProject struct {
	ProfileID string
	Manifest  *project.Manifest
}

// Store owns loading and saving the centralized project store file.
type Store struct {
	Path string
}

// DefaultPath returns the conventional location of the project store,
// alongside the profile registry inside the user-config directory.
func DefaultPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, config.UserConfigDirName, config.ProjectsStoreFileName)
}

// NewStore creates a project store. An empty path means "use the default
// user-config location".
func NewStore(path string) *Store {
	if path == "" {
		path = DefaultPath()
	}
	return &Store{Path: path}
}

// Load reads the store and returns the projects grouped by profile id.
// The known set is the current registry's profile id set: any group whose
// key is not in known is reported as OrphanProfileIDError. projects.json
// is owned by the app (not user-edited), so an orphan is data corruption
// and must surface — not be silently pruned.
//
// A missing file is treated as an empty store so first-time use does not
// need special handling. Manifests are normalized and validated on the
// way in so the returned aggregate is always consistent.
func (s *Store) Load(known map[string]struct{}) (map[string][]*project.Manifest, errs.DomainError) {
	if !utils.Exists(s.Path) {
		return map[string][]*project.Manifest{}, nil
	}
	var state State
	if err := utils.ReadJSON(s.Path, &state); err != nil {
		return nil, err
	}
	if state.Projects == nil {
		state.Projects = map[string][]*project.Manifest{}
	}
	var failures errs.Errors
	for profileID, manifests := range state.Projects {
		if _, ok := known[profileID]; !ok {
			failures = append(failures, OrphanProfileIDError{ProfileID: profileID})
			continue
		}
		for _, m := range manifests {
			if err := m.Normalize(); err != nil {
				failures = append(failures, err)
				continue
			}
			if err := m.Validate(); err != nil {
				failures = append(failures, err)
			}
		}
	}
	if len(failures) > 0 {
		return nil, failures
	}
	return state.Projects, nil
}

// Save writes the given state to disk. Keys and per-profile lists are
// sorted before writing so the file stays diff-friendly. Save does not
// perform orphan validation — callers own that decision (Load enforces
// on the way in; Save trusts callers who have already loaded).
func (s *Store) Save(state map[string][]*project.Manifest) errs.DomainError {
	normalized := map[string][]*project.Manifest{}
	for profileID, manifests := range state {
		sorted := append([]*project.Manifest(nil), manifests...)
		slices.SortFunc(sorted, func(a, b *project.Manifest) int {
			return strings.Compare(a.Name, b.Name)
		})
		normalized[profileID] = sorted
	}
	return utils.WriteJSON(s.Path, State{Version: Version, Projects: normalized})
}

// loadRaw reads the file bypassing the orphan check. Internal CRUD
// operations use it so a Save that follows does not have to re-know the
// registry.
func (s *Store) loadRaw() (map[string][]*project.Manifest, errs.DomainError) {
	if !utils.Exists(s.Path) {
		return map[string][]*project.Manifest{}, nil
	}
	var state State
	if err := utils.ReadJSON(s.Path, &state); err != nil {
		return nil, err
	}
	if state.Projects == nil {
		state.Projects = map[string][]*project.Manifest{}
	}
	return state.Projects, nil
}

// Add appends a project manifest under the given profile id. Normalize +
// Validate run first so a bad manifest never reaches disk. Duplicate ids
// within the same profile group produce DuplicateProjectIDError.
func (s *Store) Add(profileID string, m *project.Manifest) errs.DomainError {
	if err := m.Normalize(); err != nil {
		return err
	}
	if err := m.Validate(); err != nil {
		return err
	}
	state, err := s.loadRaw()
	if err != nil {
		return err
	}
	for _, existing := range state[profileID] {
		if existing.ID == m.ID {
			return DuplicateProjectIDError{ProfileID: profileID, ProjectID: m.ID}
		}
	}
	state[profileID] = append(state[profileID], m)
	return s.Save(state)
}

// Update replaces the project manifest whose id matches m.ID in the given
// profile group. Missing project → ProjectNotFoundError.
func (s *Store) Update(profileID string, m *project.Manifest) errs.DomainError {
	if err := m.Normalize(); err != nil {
		return err
	}
	if err := m.Validate(); err != nil {
		return err
	}
	state, err := s.loadRaw()
	if err != nil {
		return err
	}
	group, ok := state[profileID]
	if !ok {
		return ProjectNotFoundError{ProfileID: profileID, ProjectID: m.ID}
	}
	for i, existing := range group {
		if existing.ID == m.ID {
			group[i] = m
			state[profileID] = group
			return s.Save(state)
		}
	}
	return ProjectNotFoundError{ProfileID: profileID, ProjectID: m.ID}
}

// Remove deletes the project with projectID from the given profile group.
// Missing project → ProjectNotFoundError. Removing the last project under
// a profile drops the empty group so the file stays tidy.
func (s *Store) Remove(profileID, projectID string) errs.DomainError {
	state, err := s.loadRaw()
	if err != nil {
		return err
	}
	group, ok := state[profileID]
	if !ok {
		return ProjectNotFoundError{ProfileID: profileID, ProjectID: projectID}
	}
	for i, existing := range group {
		if existing.ID == projectID {
			group = append(group[:i], group[i+1:]...)
			if len(group) == 0 {
				delete(state, profileID)
			} else {
				state[profileID] = group
			}
			return s.Save(state)
		}
	}
	return ProjectNotFoundError{ProfileID: profileID, ProjectID: projectID}
}

// ListByProfile returns every project manifest under profileID, sorted by
// display name so callers get a stable order for presentation. Returns an
// empty slice when the profile has no projects.
func (s *Store) ListByProfile(profileID string) ([]*project.Manifest, errs.DomainError) {
	state, err := s.loadRaw()
	if err != nil {
		return nil, err
	}
	group := state[profileID]
	out := append([]*project.Manifest(nil), group...)
	slices.SortFunc(out, func(a, b *project.Manifest) int {
		return strings.Compare(a.Name, b.Name)
	})
	return out, nil
}

// RemoveByProfile deletes every project owned by profileID. Used by
// app.Service on profile deletion so per-profile projects cascade away
// with their owner. Missing profile is not an error — the cascade is
// idempotent by design.
func (s *Store) RemoveByProfile(profileID string) errs.DomainError {
	state, err := s.loadRaw()
	if err != nil {
		return err
	}
	if _, ok := state[profileID]; !ok {
		return nil
	}
	delete(state, profileID)
	return s.Save(state)
}

// AllProjects returns every stored project paired with its owning profile
// id. Iteration is sorted first by profile id and then by project name so
// callers (repo-path uniqueness check) get a deterministic order.
func (s *Store) AllProjects() ([]OwnedProject, errs.DomainError) {
	state, err := s.loadRaw()
	if err != nil {
		return nil, err
	}
	profileIDs := make([]string, 0, len(state))
	for id := range state {
		profileIDs = append(profileIDs, id)
	}
	slices.Sort(profileIDs)
	var out []OwnedProject
	for _, id := range profileIDs {
		group := append([]*project.Manifest(nil), state[id]...)
		slices.SortFunc(group, func(a, b *project.Manifest) int {
			return strings.Compare(a.Name, b.Name)
		})
		for _, m := range group {
			out = append(out, OwnedProject{ProfileID: id, Manifest: m})
		}
	}
	return out, nil
}
