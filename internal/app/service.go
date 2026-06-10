// Package app is the thin application layer that the CLI and TUI call into.
// It orchestrates the domain packages (registry, profile, asset, project,
// render, sync) but contains no business logic of its own.
package app

import (
	"strings"
	"time"

	"github.com/hexworks/agentfiles/internal/asset"
	"github.com/hexworks/agentfiles/internal/config"
	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/utils"
	"github.com/hexworks/agentfiles/internal/profile"
	"github.com/hexworks/agentfiles/internal/project"
	"github.com/hexworks/agentfiles/internal/registry"
	llmsync "github.com/hexworks/agentfiles/internal/sync"
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
func (s *Service) CreateProfile(name, path string) (*registry.ProfileRef, errs.DomainError) {
	path, absErr := utils.ToAbsolute(path)
	if absErr != nil {
		return nil, absErr
	}
	manifest, initErr := profile.Init(path, name)
	if initErr != nil {
		return nil, initErr
	}
	ref := registry.ProfileRef{
		ID:           manifest.ID,
		Name:         manifest.Name,
		Path:         path,
		Source:       config.DefaultProfileSource,
		ManagedBy:    config.DefaultProfileManagedBy,
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
func (s *Service) RegisterProfile(path string) (*registry.ProfileRef, errs.DomainError) {
	loaded, loadErr := profile.Load(path)
	if loadErr != nil {
		return nil, loadErr
	}
	ref := registry.ProfileRef{
		ID:           loaded.Manifest.ID,
		Name:         loaded.Manifest.Name,
		Path:         loaded.Root,
		Source:       config.DefaultProfileSource,
		ManagedBy:    config.DefaultProfileManagedBy,
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
func (s *Service) LoadProfile(ref string) (*profile.Profile, errs.DomainError) {
	profileRef, err := s.Registry.Resolve(ref)
	if err != nil {
		return nil, err
	}
	if err := s.Registry.Touch(profileRef.ID); err != nil {
		return nil, err
	}
	return profile.Load(profileRef.Path)
}

// AddProject creates a per-profile project manifest. Domain failures
// (path collisions, unknown asset ids) are accumulated and returned as a
// slice of typed domain errors so the caller can display every problem
// at once. A non-empty slice means the project was not saved.
//
// A project manifest does not store rendered files. It stores only the project
// path plus the asset/agent selection used later by render + sync.
func (s *Service) AddProject(profileRef, name, path string, agents, assetIDs []string) (*project.Manifest, []errs.DomainError) {
	loaded, loadErr := s.LoadProfile(profileRef)
	if loadErr != nil {
		return nil, []errs.DomainError{loadErr}
	}
	path, absErr := utils.ToAbsolute(path)
	if absErr != nil {
		return nil, []errs.DomainError{absErr}
	}
	var domainErrs []errs.DomainError
	domainErrs = append(domainErrs, s.ensureProjectPathAvailable(path, loaded)...)
	for _, assetID := range assetIDs {
		if loaded.Assets[assetID] == nil {
			domainErrs = append(domainErrs, AssetNotFoundError{AssetID: assetID})
		}
	}
	if len(domainErrs) > 0 {
		return nil, domainErrs
	}
	manifest := &project.Manifest{
		ID:               slug(name),
		Name:             name,
		Path:             path,
		EnabledAgents:    agents,
		SelectedAssetIDs: assetIDs,
		CreatedAt:        time.Now().UTC(),
	}
	if err := project.Save(loaded.Root, manifest); err != nil {
		return nil, []errs.DomainError{err}
	}
	return manifest, nil
}

// InitAsset scaffolds a new asset inside the selected profile. The asset type
// determines the starter files that get created in the new asset directory.
func (s *Service) InitAsset(profileRef string, manifest asset.Manifest) (string, errs.DomainError) {
	loaded, err := s.LoadProfile(profileRef)
	if err != nil {
		return "", err
	}
	if loaded.Assets[manifest.ID] != nil {
		return "", AssetExistsError{AssetID: manifest.ID}
	}
	return asset.Init(loaded.Root, manifest)
}

// Plan builds a sync preview for one project. This is the read-only half of the
// pipeline: load profile -> render desired files -> compare with the repo.
func (s *Service) Plan(profileRef, projectID string) (*llmsync.Preview, errs.DomainError) {
	loaded, err := s.LoadProfile(profileRef)
	if err != nil {
		return nil, err
	}
	proj := loaded.Projects[projectID]
	if proj == nil {
		return nil, ProjectNotFoundError{ProjectID: projectID}
	}
	return llmsync.Plan(loaded, proj)
}

// Apply executes the write half of the pipeline by first building a preview and
// then asking the sync package to materialize the desired files. The caller
// supplies per-file resolutions for drift and unknown entries; create/update/
// delete kinds carry the implicit ResolveAuto.
func (s *Service) Apply(profileRef, projectID string, resolutions []llmsync.FileResolution) (*llmsync.Preview, errs.DomainError) {
	preview, err := s.Plan(profileRef, projectID)
	if err != nil {
		return nil, err
	}
	if err := llmsync.Apply(preview, resolutions); err != nil {
		return nil, err
	}
	return preview, nil
}

// ensureProjectPathAvailable enforces the ownership rule that one repository
// path may belong to only one project. Without this check, two projects could
// fight over the same generated files and the same .agentfiles/state.json.
// Both same-profile and cross-profile conflicts are reported; all conflicts
// are accumulated so the caller learns about every owning project in one
// pass. Project iteration is sorted so the returned slice is deterministic.
func (s *Service) ensureProjectPathAvailable(projectPath string, active *profile.Profile) []errs.DomainError {
	var conflicts []errs.DomainError
	for _, proj := range active.ProjectList() {
		if proj.Path == projectPath {
			conflicts = append(conflicts, ProjectPathOwnedError{
				Path:        projectPath,
				ProfileName: active.Manifest.Name,
				ProjectName: proj.Name,
			})
		}
	}
	reg, err := s.Registry.Load()
	if err != nil {
		return append(conflicts, err)
	}
	for _, profileRef := range reg.Profiles {
		if profileRef.ID == active.Manifest.ID {
			continue
		}
		loaded, loadErr := profile.Load(profileRef.Path)
		if loadErr != nil {
			continue
		}
		for _, proj := range loaded.ProjectList() {
			if proj.Path == projectPath {
				conflicts = append(conflicts, ProjectPathOwnedError{
					Path:        projectPath,
					ProfileName: profileRef.Name,
					ProjectName: proj.Name,
				})
			}
		}
	}
	return conflicts
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
