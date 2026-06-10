// Package app is the thin application layer that the CLI and TUI call into.
// It orchestrates the domain packages (registry, profile, asset, project,
// render, sync) but contains no business logic of its own.
package app

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/hexworks/agentfiles/internal/asset"
	"github.com/hexworks/agentfiles/internal/config"
	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/profile"
	"github.com/hexworks/agentfiles/internal/project"
	"github.com/hexworks/agentfiles/internal/registry"
	llmsync "github.com/hexworks/agentfiles/internal/sync"
	"github.com/hexworks/agentfiles/internal/utils"
)

// FolderAction controls whether DeleteProfile also removes the on-disk
// profile folder.
type FolderAction int

const (
	// KeepFolders leaves the on-disk profile folder in place after
	// DeleteProfile removes the registry entry.
	KeepFolders FolderAction = iota
	// DeleteFolders removes the on-disk profile folder during
	// DeleteProfile. A pre-missing folder is not treated as an error.
	DeleteFolders
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

// DriftDecision is the app-layer mirror of llmsync.DriftDecision. The
// TUI consumes the app vocabulary so it never imports `internal/sync`
// directly, keeping the documented `tui → app` dependency edge true.
type DriftDecision string

// Possible DriftDecision values mirror llmsync.DriftDecision.
const (
	DriftKeep      DriftDecision = "keep"
	DriftOverwrite DriftDecision = "overwrite"
)

// UnknownDecision is the app-layer mirror of llmsync.UnknownDecision.
type UnknownDecision string

// Possible UnknownDecision values mirror llmsync.UnknownDecision.
const (
	UnknownKeep   UnknownDecision = "keep"
	UnknownDelete UnknownDecision = "delete"
)

// DriftResolution pairs a drifted path with the user's per-file
// decision. Service.Apply translates these into the corresponding
// sync types before invoking the engine.
type DriftResolution struct {
	Path     string
	Decision DriftDecision
}

// UnknownResolution pairs an unknown path with the user's per-file
// decision.
type UnknownResolution struct {
	Path     string
	Decision UnknownDecision
}

// Apply executes the write half of the pipeline by first building a preview
// and then asking the sync package to materialize the desired files. The
// caller supplies per-file resolutions for drift and unknown entries.
// Defaults (no resolution for a path): drift kept, unknown kept; create/
// update/delete always apply.
func (s *Service) Apply(profileRef, projectID string, driftResolutions []DriftResolution, unknownResolutions []UnknownResolution) (*llmsync.Preview, errs.DomainError) {
	preview, err := s.Plan(profileRef, projectID)
	if err != nil {
		return nil, err
	}
	if err := llmsync.Apply(preview, toSyncDriftResolutions(driftResolutions), toSyncUnknownResolutions(unknownResolutions)); err != nil {
		return nil, err
	}
	return preview, nil
}

func toSyncDriftResolutions(in []DriftResolution) []llmsync.DriftResolution {
	if len(in) == 0 {
		return nil
	}
	out := make([]llmsync.DriftResolution, len(in))
	for i, r := range in {
		out[i] = llmsync.DriftResolution{Path: r.Path, Decision: llmsync.DriftDecision(r.Decision)}
	}
	return out
}

func toSyncUnknownResolutions(in []UnknownResolution) []llmsync.UnknownResolution {
	if len(in) == 0 {
		return nil
	}
	out := make([]llmsync.UnknownResolution, len(in))
	for i, r := range in {
		out[i] = llmsync.UnknownResolution{Path: r.Path, Decision: llmsync.UnknownDecision(r.Decision)}
	}
	return out
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

// LoadProfiles returns every registered profile, fully loaded (assets and
// projects scanned). Profiles that fail to load are skipped from the result
// and their errors are aggregated into the returned errs.DomainError so the
// TUI can show every problem at once without losing healthy profiles.
//
// The returned slice keeps the registry order (already sorted by name by
// registry.Store.Save).
func (s *Service) LoadProfiles() ([]*profile.Profile, errs.DomainError) {
	reg, err := s.Registry.Load()
	if err != nil {
		return nil, err
	}
	loaded := make([]*profile.Profile, 0, len(reg.Profiles))
	var loadErrs []errs.DomainError
	for _, ref := range reg.Profiles {
		p, loadErr := profile.Load(ref.Path)
		if loadErr != nil {
			loadErrs = append(loadErrs, loadErr)
			continue
		}
		loaded = append(loaded, p)
	}
	if len(loadErrs) > 0 {
		return loaded, errs.Errors(loadErrs)
	}
	return loaded, nil
}

// DeleteProfile removes the profile from the global registry. When
// folderAction is DeleteFolders the on-disk profile folder is removed as
// well; a pre-missing folder is not an error so the registry side always
// succeeds even when the folder is already gone.
func (s *Service) DeleteProfile(profileRef string, folderAction FolderAction) errs.DomainError {
	ref, err := s.Registry.Resolve(profileRef)
	if err != nil {
		return err
	}
	if err := s.Registry.Remove(ref.ID); err != nil {
		return err
	}
	if folderAction != DeleteFolders {
		return nil
	}
	if rmErr := os.RemoveAll(ref.Path); rmErr != nil && !errors.Is(rmErr, fs.ErrNotExist) {
		return ProfileFolderRemoveError{Path: ref.Path, Err: rmErr}
	}
	return nil
}

// LoadAsset returns the asset identified by assetID inside the given
// profile. Missing asset → AssetNotFoundError.
func (s *Service) LoadAsset(profileRef, assetID string) (*asset.Asset, errs.DomainError) {
	loaded, err := s.LoadProfile(profileRef)
	if err != nil {
		return nil, err
	}
	a := loaded.Assets[assetID]
	if a == nil {
		return nil, AssetNotFoundError{AssetID: assetID}
	}
	return a, nil
}

// UpdateAsset overwrites the asset manifest on disk with the caller's
// edits. The asset must already exist in the profile; this method is not
// a scaffold path (use InitAsset for that).
//
// Note: the task description mentions recomputing per-file content
// hashes. The current asset model does not store hashes — internal/sync
// computes them on the fly from disk and the project's state.json — so
// the drift-vs-update distinction already works without an asset-side
// hash store. If a later change adds an in-memory hash field on
// asset.Asset, recompute it here.
func (s *Service) UpdateAsset(profileRef string, a *asset.Asset) errs.DomainError {
	if a == nil {
		return AssetNotFoundError{AssetID: ""}
	}
	loaded, err := s.LoadProfile(profileRef)
	if err != nil {
		return err
	}
	if loaded.Assets[a.ID] == nil {
		return AssetNotFoundError{AssetID: a.ID}
	}
	if validateErr := a.Manifest.Validate(); validateErr != nil {
		return validateErr
	}
	return utils.WriteJSON(filepath.Join(a.Dir, config.AssetManifestFileName), a.Manifest)
}

// DeleteAsset removes the asset folder from the profile and unselects the
// asset id from every project in the profile. Already-synced files in
// project repos are not touched and remain orphaned.
func (s *Service) DeleteAsset(profileRef, assetID string) errs.DomainError {
	loaded, err := s.LoadProfile(profileRef)
	if err != nil {
		return err
	}
	target := loaded.Assets[assetID]
	if target == nil {
		return AssetNotFoundError{AssetID: assetID}
	}
	for _, p := range loaded.ProjectList() {
		idx := slices.Index(p.SelectedAssetIDs, assetID)
		if idx < 0 {
			continue
		}
		p.SelectedAssetIDs = slices.Delete(p.SelectedAssetIDs, idx, idx+1)
		if saveErr := project.Save(loaded.Root, p); saveErr != nil {
			return saveErr
		}
	}
	if rmErr := os.RemoveAll(target.Dir); rmErr != nil && !errors.Is(rmErr, fs.ErrNotExist) {
		return AssetFolderRemoveError{Dir: target.Dir, Err: rmErr}
	}
	return nil
}

// LoadProject returns the project manifest identified by projectID inside
// the given profile. Missing project → ProjectNotFoundError.
func (s *Service) LoadProject(profileRef, projectID string) (*project.Manifest, errs.DomainError) {
	loaded, err := s.LoadProfile(profileRef)
	if err != nil {
		return nil, err
	}
	p := loaded.Projects[projectID]
	if p == nil {
		return nil, ProjectNotFoundError{ProjectID: projectID}
	}
	return p, nil
}

// UpdateProject overwrites the project manifest on disk with the caller's
// edits. The project must already exist in the profile; this is not a
// create path (use AddProject for that).
func (s *Service) UpdateProject(profileRef string, p *project.Manifest) errs.DomainError {
	if p == nil {
		return ProjectNotFoundError{ProjectID: ""}
	}
	loaded, err := s.LoadProfile(profileRef)
	if err != nil {
		return err
	}
	if loaded.Projects[p.ID] == nil {
		return ProjectNotFoundError{ProjectID: p.ID}
	}
	return project.Save(loaded.Root, p)
}

// DeleteProject removes the project manifest from the profile. Files in
// the project's target repository are not touched.
func (s *Service) DeleteProject(profileRef, projectID string) errs.DomainError {
	loaded, err := s.LoadProfile(profileRef)
	if err != nil {
		return err
	}
	if loaded.Projects[projectID] == nil {
		return ProjectNotFoundError{ProjectID: projectID}
	}
	return project.Delete(loaded.Root, projectID)
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
