// Package app is the thin application layer that the CLI and TUI call into.
// It orchestrates the domain packages (registry, profile, asset, project,
// render, sync) but contains no business logic of its own.
package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/addamsson/agentfiles/internal/asset"
	"github.com/addamsson/agentfiles/internal/fsutil"
	"github.com/addamsson/agentfiles/internal/profile"
	"github.com/addamsson/agentfiles/internal/project"
	"github.com/addamsson/agentfiles/internal/registry"
	llmsync "github.com/addamsson/agentfiles/internal/sync"
)

// Service is the thin application layer used by the TUI.
//
// The domain packages do the real work:
//   - registry discovers profiles
//   - profile/asset/project load the source model
//   - render computes desired outputs
//   - sync compares and writes project files
//
// Service ties those packages together into user-facing operations.
type Service struct {
	Registry *registry.Store
}

// New builds a Service backed by the registry store at registryPath. An empty
// path selects the default registry location.
func New(registryPath string) *Service {
	return &Service{Registry: registry.NewStore(registryPath)}
}

// CreateProfile initializes a new profile folder on disk and registers it in
// the global profile registry. The profile folder is the authoritative source
// of truth; project files are generated later from its contents.
func (s *Service) CreateProfile(name, path string) (*registry.ProfileRef, error) {
	path, err := fsutil.ToAbsolute(path)
	if err != nil {
		return nil, err
	}
	manifest, err := profile.Init(path, name)
	if err != nil {
		return nil, err
	}
	ref := registry.ProfileRef{
		ID:   manifest.ID,
		Name: manifest.Name,
		Path: path,
		// FIX: task#0004 move these values to global config
		Source:       "local",
		ManagedBy:    "self",
		CreatedAt:    time.Now().UTC(),
		LastOpenedAt: time.Now().UTC(),
	}
	if err := s.Registry.Add(ref); err != nil {
		return nil, err
	}
	return &ref, nil
}

// RegisterProfile adds an already-existing profile folder to the global
// registry. This is the "adopt existing local folder" path as opposed to
// CreateProfile's "scaffold a fresh one" path.
func (s *Service) RegisterProfile(path string) (*registry.ProfileRef, error) {
	loaded, err := profile.Load(path)
	if err != nil {
		return nil, err
	}
	ref := registry.ProfileRef{
		ID:   loaded.Manifest.ID,
		Name: loaded.Manifest.Name,
		Path: loaded.Root,
		// FIX: task#0004 move these values to global config
		Source:       "local",
		ManagedBy:    "self",
		CreatedAt:    loaded.Manifest.CreatedAt,
		LastOpenedAt: time.Now().UTC(),
	}
	if err := s.Registry.Add(ref); err != nil {
		return nil, err
	}
	return &ref, nil
}

// LoadProfile resolves a user-facing profile reference (id, name, or path),
// updates its last-opened timestamp, and returns the fully loaded profile model.
func (s *Service) LoadProfile(ref string) (*profile.Profile, error) {
	profileRef, err := s.Registry.Resolve(ref)
	if err != nil {
		return nil, err
	}
	if err := s.Registry.Touch(profileRef.ID); err != nil {
		return nil, err
	}
	return profile.Load(profileRef.Path)
}

// AddProject creates a per-profile project manifest.
//
// A project manifest does not store rendered files. It stores only the project
// path plus the asset/agent selection used later by render + sync.
func (s *Service) AddProject(profileRef, name, path string, agents, assetIDs []string) (*project.Manifest, error) {
	loaded, err := s.LoadProfile(profileRef)
	if err != nil {
		return nil, err
	}
	path, err = fsutil.ToAbsolute(path)
	if err != nil {
		return nil, err
	}
	if err := s.ensureProjectPathAvailable(path, loaded.Manifest.ID); err != nil {
		return nil, err
	}
	manifest := &project.Manifest{
		ID:               slug(name),
		Name:             name,
		Path:             path,
		EnabledAgents:    agents,
		SelectedAssetIDs: assetIDs,
		CreatedAt:        time.Now().UTC(),
	}
	// FIX: task#0005: Accumulate errors into an error list and return it
	// instead of returning an error message
	for _, assetID := range assetIDs {
		if loaded.Assets[assetID] == nil {
			return nil, fmt.Errorf("unknown asset: %s", assetID)
		}
	}
	if err := project.Save(loaded.Root, manifest); err != nil {
		return nil, err
	}
	return manifest, nil
}

// InitAsset scaffolds a new asset inside the selected profile. The asset type
// determines the starter files that get created in the new asset directory.
func (s *Service) InitAsset(profileRef string, manifest asset.Manifest) (string, error) {
	loaded, err := s.LoadProfile(profileRef)
	if err != nil {
		return "", err
	}
	if loaded.Assets[manifest.ID] != nil {
		// FIX: task#0005 return metadata for error instead of hard-coded error message
		return "", fmt.Errorf("asset already exists: %s", manifest.ID)
	}
	return asset.Init(loaded.Root, manifest)
}

// Plan builds a sync preview for one project. This is the read-only half of the
// pipeline: load profile -> render desired files -> compare with the repo.
func (s *Service) Plan(profileRef, projectID string) (*llmsync.Preview, error) {
	loaded, err := s.LoadProfile(profileRef)
	if err != nil {
		return nil, err
	}
	proj := loaded.Projects[projectID]
	if proj == nil {
		// FIX: task#0005 return metadata for error instead of hard-coded error message
		return nil, fmt.Errorf("project not found: %s", projectID)
	}
	return llmsync.Plan(loaded, proj)
}

// Apply executes the write half of the pipeline by first building a preview and
// then asking the sync package to materialize the desired files.
func (s *Service) Apply(profileRef, projectID string, deleteCandidates bool) (*llmsync.Preview, error) {
	preview, err := s.Plan(profileRef, projectID)
	if err != nil {
		return nil, err
	}
	if err := llmsync.Apply(preview, deleteCandidates); err != nil {
		return nil, err
	}
	return preview, nil
}

// ensureProjectPathAvailable enforces the ownership rule that one repository
// path may belong to only one profile. Without this check, two profiles could
// fight over the same generated files.
func (s *Service) ensureProjectPathAvailable(projectPath, activeProfileID string) error {
	reg, err := s.Registry.Load()
	if err != nil {
		return err
	}
	for _, profileRef := range reg.Profiles {
		loaded, err := profile.Load(profileRef.Path)
		if err != nil {
			continue
		}
		// FIX: task#0005 accumulate error metadata (eg: {Path, Profile}) as opposed to rendering
		// and return all of them instead of failing fast on the first one.
		for _, proj := range loaded.Projects {
			if proj.Path == projectPath && profileRef.ID != activeProfileID {
				return fmt.Errorf("project path already owned by profile %s", profileRef.Name)
			}
		}
	}
	return nil
}

// slug creates a stable file/id friendly name from user-facing input.
func slug(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	v = strings.ReplaceAll(v, " ", "-")
	v = strings.ReplaceAll(v, "_", "-")
	var b strings.Builder
	for _, r := range v {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "item"
	}
	return b.String()
}
