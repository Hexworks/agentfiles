// Package projectstore owns the centralized per-user project selection file
// (~/.agentfiles/projects.json). It exists because a profile folder is meant
// to be shareable: keeping per-machine target-repo paths and per-user asset
// selections inside a profile leaks the author's local state on the next
// share. The store groups projects by profile id, mirrors registry.Store's
// shape (Load/Save + typed CRUD), and enforces the global one-repo-path-per-
// project invariant directly on Add/Update so any caller — the TUI, a
// future scripting path, a migration — sees the same rule.
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

// Migrate stamps a legacy (version 0) store up to the current schema
// version at the persistence boundary. Pointer receiver so the stamp lands
// on the decoded value. Per-project Normalize/Validate is a separate
// orphan-and-shape pass (validateState) that needs the wired seams.
func (s *State) Migrate() errs.DomainError {
	if s.Version == 0 {
		s.Version = Version
	}
	return nil
}

// Validate rejects a store written by a newer build than this one
// understands (forward-compat guard). Pointer receiver so *State satisfies
// utils.Persisted alongside Migrate.
func (s *State) Validate() errs.DomainError {
	if s.Version > Version {
		return errs.NewerSchemaVersionError{Have: s.Version, Known: Version}
	}
	return nil
}

// OwnedProject pairs a project manifest with the id of the profile that
// owns it. Returned by AllProjects for the global repo-path uniqueness
// check.
type OwnedProject struct {
	ProfileID string
	Manifest  *project.Manifest
}

// KnownProvider returns the set of profile ids that the registry currently
// knows about, so the store can flag orphan groups. Its return type
// mirrors the shape callers already pass to Load; a nil provider means
// "orphan check off" and is used by migrate.Run before the registry
// exists.
type KnownProvider func() (map[string]struct{}, errs.DomainError)

// Validator is the domain seam through which the store validates a
// manifest without importing the rule itself. Keeps Store a pure
// persistence type; the invariant (`project.Manifest.Validate`) lives
// with the domain and is wired at construction. Nil means "skip".
type Validator func(m *project.Manifest) errs.DomainError

// Store owns loading and saving the centralized project store file.
// KnownProfiles and Validator are optional seams: when nil, the
// corresponding check is skipped (used by migrate, which has neither a
// live registry nor a validation contract at that phase).
type Store struct {
	Path          string
	KnownProfiles KnownProvider
	Validator     Validator
}

// DefaultPath returns the conventional location of the project store,
// alongside the profile registry inside the user-config directory.
// Returns HomeDirUnavailableError when os.UserHomeDir fails, so callers
// surface the environment problem instead of writing to a CWD-relative
// fallback.
func DefaultPath() (string, errs.DomainError) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", HomeDirUnavailableError{Err: err}
	}
	return filepath.Join(home, config.UserConfigDirName, config.ProjectsStoreFileName), nil
}

// NewStore creates a project store. An empty path means "use the default
// user-config location"; when DefaultPath itself fails the returned
// store's Path is empty and every I/O method surfaces the underlying
// filesystem error naturally.
func NewStore(path string) *Store {
	if path == "" {
		path, _ = DefaultPath()
	}
	return &Store{Path: path}
}

// WithKnownProfiles wires the orphan-check callback and returns the
// store for fluent construction at wiring time.
func (s *Store) WithKnownProfiles(k KnownProvider) *Store {
	s.KnownProfiles = k
	return s
}

// WithValidator wires the manifest-validation seam and returns the store
// for fluent construction at wiring time.
func (s *Store) WithValidator(v Validator) *Store {
	s.Validator = v
	return s
}

// Load reads the store and returns the full state. Any group whose key
// is not in known is reported as OrphanProfileIDError. projects.json
// is owned by the app (not user-edited), so an orphan is data corruption
// and must surface — not be silently pruned. Missing file → empty
// state.
//
// The caller-supplied known set overrides s.KnownProfiles. Pass nil to
// use the wired provider (or skip the check when no provider is wired).
func (s *Store) Load(known map[string]struct{}) (*State, errs.DomainError) {
	state, err := s.readState()
	if err != nil {
		return nil, err
	}
	if known == nil && s.KnownProfiles != nil {
		wired, kerr := s.KnownProfiles()
		if kerr != nil {
			return nil, kerr
		}
		known = wired
	}
	return s.validateState(state, known)
}

// Save writes state to disk atomically with owner-only permissions. Keys
// and per-profile lists are sorted before writing so the file stays
// diff-friendly.
func (s *Store) Save(state *State) errs.DomainError {
	normalized := &State{Version: Version, Projects: map[string][]*project.Manifest{}}
	if state != nil {
		for profileID, manifests := range state.Projects {
			sorted := append([]*project.Manifest(nil), manifests...)
			slices.SortFunc(sorted, func(a, b *project.Manifest) int {
				return strings.Compare(a.Name, b.Name)
			})
			normalized.Projects[profileID] = sorted
		}
	}
	return utils.WriteJSONAtomic(s.Path, *normalized, 0o700, 0o600)
}

// readState reads the file into a State value. A missing file is treated
// as an empty state so first-time use does not need special handling.
// Nil groups are replaced with an empty map so callers never nil-check.
func (s *Store) readState() (*State, errs.DomainError) {
	if !utils.Exists(s.Path) {
		return &State{Version: Version, Projects: map[string][]*project.Manifest{}}, nil
	}
	state, err := utils.ReadJSON[State](s.Path)
	if err != nil {
		return nil, err
	}
	if state.Projects == nil {
		state.Projects = map[string][]*project.Manifest{}
	}
	return &state, nil
}

// validateState runs the orphan check + Normalize + Validator seam over
// every manifest in state. Failures accumulate so a corrupt file surfaces
// every issue at once.
func (s *Store) validateState(state *State, known map[string]struct{}) (*State, errs.DomainError) {
	var failures errs.Errors
	for profileID, manifests := range state.Projects {
		if known != nil {
			if _, ok := known[profileID]; !ok {
				failures = append(failures, OrphanProfileIDError{ProfileID: profileID})
				continue
			}
		}
		for _, m := range manifests {
			if err := m.Normalize(); err != nil {
				failures = append(failures, err)
				continue
			}
			if s.Validator != nil {
				if err := s.Validator(m); err != nil {
					failures = append(failures, err)
				}
			}
		}
	}
	if len(failures) > 0 {
		return nil, failures
	}
	return state, nil
}

// Add appends a project manifest under the given profile id. Normalize
// runs first so paths are absolute; the wired Validator (if any) then
// checks domain rules. Duplicate ids inside the same profile group
// produce DuplicateProjectIDError; a target repo path already owned by
// another group returns ProjectPathOwnedError — the single-ownership
// invariant lives inside the aggregate that persists it.
func (s *Store) Add(profileID string, m *project.Manifest) errs.DomainError {
	if err := m.Normalize(); err != nil {
		return err
	}
	if s.Validator != nil {
		if err := s.Validator(m); err != nil {
			return err
		}
	}
	state, err := s.readAndCheck()
	if err != nil {
		return err
	}
	for _, existing := range state.Projects[profileID] {
		if existing.ID == m.ID {
			return DuplicateProjectIDError{ProfileID: profileID, ProjectID: m.ID}
		}
	}
	if owned := s.pathOwner(state, m.Path, profileID, m.ID); owned != nil {
		return *owned
	}
	state.Projects[profileID] = append(state.Projects[profileID], m)
	return s.Save(state)
}

// Update replaces the project manifest whose id matches m.ID in the given
// profile group. Missing project → ProjectNotFoundError. Same
// path-uniqueness rule as Add, excluding the project being updated.
func (s *Store) Update(profileID string, m *project.Manifest) errs.DomainError {
	if err := m.Normalize(); err != nil {
		return err
	}
	if s.Validator != nil {
		if err := s.Validator(m); err != nil {
			return err
		}
	}
	state, err := s.readAndCheck()
	if err != nil {
		return err
	}
	group, ok := state.Projects[profileID]
	if !ok {
		return ProjectNotFoundError{ProfileID: profileID, ProjectID: m.ID}
	}
	if owned := s.pathOwner(state, m.Path, profileID, m.ID); owned != nil {
		return *owned
	}
	for i, existing := range group {
		if existing.ID == m.ID {
			group[i] = m
			state.Projects[profileID] = group
			return s.Save(state)
		}
	}
	return ProjectNotFoundError{ProfileID: profileID, ProjectID: m.ID}
}

// Remove deletes the project with projectID from the given profile group.
// Missing project → ProjectNotFoundError. Removing the last project under
// a profile drops the empty group so the file stays tidy.
func (s *Store) Remove(profileID, projectID string) errs.DomainError {
	state, err := s.readAndCheck()
	if err != nil {
		return err
	}
	group, ok := state.Projects[profileID]
	if !ok {
		return ProjectNotFoundError{ProfileID: profileID, ProjectID: projectID}
	}
	for i, existing := range group {
		if existing.ID == projectID {
			group = append(group[:i], group[i+1:]...)
			if len(group) == 0 {
				delete(state.Projects, profileID)
			} else {
				state.Projects[profileID] = group
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
	state, err := s.readAndCheck()
	if err != nil {
		return nil, err
	}
	group := state.Projects[profileID]
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
	state, err := s.readAndCheck()
	if err != nil {
		return err
	}
	if _, ok := state.Projects[profileID]; !ok {
		return nil
	}
	delete(state.Projects, profileID)
	return s.Save(state)
}

// AllProjects returns every stored project paired with its owning profile
// id. Iteration is sorted first by profile id and then by project name so
// callers get a deterministic order.
func (s *Store) AllProjects() ([]OwnedProject, errs.DomainError) {
	state, err := s.readAndCheck()
	if err != nil {
		return nil, err
	}
	profileIDs := make([]string, 0, len(state.Projects))
	for id := range state.Projects {
		profileIDs = append(profileIDs, id)
	}
	slices.Sort(profileIDs)
	var out []OwnedProject
	for _, id := range profileIDs {
		group := append([]*project.Manifest(nil), state.Projects[id]...)
		slices.SortFunc(group, func(a, b *project.Manifest) int {
			return strings.Compare(a.Name, b.Name)
		})
		for _, m := range group {
			out = append(out, OwnedProject{ProfileID: id, Manifest: m})
		}
	}
	return out, nil
}

// readAndCheck is the shared CRUD prelude: read the file, run the orphan
// check against the wired KnownProfiles provider (skipped when none is
// wired), and re-run the Validator against every already-persisted
// manifest so a corrupt or tampered file surfaces on the next mutation.
// Every internal path uses it so the orphan check no longer sits only on
// Load.
func (s *Store) readAndCheck() (*State, errs.DomainError) {
	state, err := s.readState()
	if err != nil {
		return nil, err
	}
	if s.KnownProfiles == nil && s.Validator == nil {
		return state, nil
	}
	var known map[string]struct{}
	if s.KnownProfiles != nil {
		wired, kerr := s.KnownProfiles()
		if kerr != nil {
			return nil, kerr
		}
		known = wired
	}
	return s.validateState(state, known)
}

// pathOwner returns a ProjectPathOwnedError pointer when path is already
// claimed by a project other than (excludeProfileID, excludeProjectID),
// or nil when the path is free. Runs against the full state passed by the
// caller so no extra I/O happens per mutation.
func (s *Store) pathOwner(state *State, path, excludeProfileID, excludeProjectID string) *ProjectPathOwnedError {
	if path == "" {
		return nil
	}
	profileIDs := make([]string, 0, len(state.Projects))
	for id := range state.Projects {
		profileIDs = append(profileIDs, id)
	}
	slices.Sort(profileIDs)
	for _, id := range profileIDs {
		for _, existing := range state.Projects[id] {
			if id == excludeProfileID && existing.ID == excludeProjectID {
				continue
			}
			if existing.Path == path {
				return &ProjectPathOwnedError{
					Path:              path,
					ExistingProfileID: id,
					ExistingProjectID: existing.ID,
				}
			}
		}
	}
	return nil
}
