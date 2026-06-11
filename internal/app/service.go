// Package app is the thin application layer that the CLI and TUI call into.
// It orchestrates the domain packages (registry, profile, asset, project,
// render, sync) but contains no business logic of its own.
package app

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
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
		ID:               utils.Slug(name),
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
// projects scanned). Per-profile load failures are accumulated and
// returned alongside the healthy subset so the TUI can list every working
// profile while still showing each problem at once.
//
// The returned profile slice keeps the registry order (already sorted by
// name by registry.Store.Save). The error slice is the accumulator shape
// described in docs/guidelines/errors.md §"Accumulator Functions Return
// `[]errs.DomainError`"; an empty slice means full success.
func (s *Service) LoadProfiles() ([]*profile.Profile, []errs.DomainError) {
	reg, err := s.Registry.Load()
	if err != nil {
		return nil, []errs.DomainError{err}
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
	return loaded, loadErrs
}

// DeleteProfile removes the profile from the global registry only. The
// on-disk profile folder is left untouched. Use DeleteProfileWithFolder
// for the destructive variant; splitting the two operations makes caller
// intent visible at the call site.
func (s *Service) DeleteProfile(profileRef string) errs.DomainError {
	ref, err := s.Registry.Resolve(profileRef)
	if err != nil {
		return err
	}
	return s.Registry.Remove(ref.ID)
}

// DeleteProfileWithFolder removes both the registry entry and the on-disk
// profile folder. The folder is removed first; if that fails the registry
// entry is left in place so the user can retry from the TUI. A
// pre-missing folder is treated as success.
//
// Two safety checks gate the recursive removal: ref.Path must still
// contain a profile.json (so the folder still looks like a profile root),
// and it must not be a pathological deletion target (empty, the
// filesystem root, the user's home directory, or an ancestor of the
// global registry file). Both checks defend against tampered registry
// state — a corrupted ~/.agentprofiles.json must not turn this call into
// a wipe of an arbitrary directory.
func (s *Service) DeleteProfileWithFolder(profileRef string) errs.DomainError {
	ref, err := s.Registry.Resolve(profileRef)
	if err != nil {
		return err
	}
	if reason, unsafe := s.isUnsafeProfilePath(ref.Path); unsafe {
		return UnsafeProfilePathError{Path: ref.Path, Reason: reason}
	}
	if !utils.Exists(ref.Path) {
		// Folder already gone — proceed straight to deregistering so the
		// registry side always converges, matching project.Delete /
		// asset.Delete idempotence.
		return s.Registry.Remove(ref.ID)
	}
	if !utils.Exists(filepath.Join(ref.Path, config.ProfileManifestFileName)) {
		return ProfileFolderNotARootError{Path: ref.Path}
	}
	if rmErr := os.RemoveAll(ref.Path); rmErr != nil && !errors.Is(rmErr, fs.ErrNotExist) {
		return ProfileFolderRemoveError{Path: ref.Path, Err: rmErr}
	}
	return s.Registry.Remove(ref.ID)
}

// isUnsafeProfilePath rejects pathological deletion targets independent
// of who tampered with the registry: the empty string, the filesystem
// root, the user's home directory, or any path that is the registry
// file's directory or one of its ancestors (which would also delete the
// registry alongside the profile).
func (s *Service) isUnsafeProfilePath(p string) (string, bool) {
	if p == "" {
		return "empty path", true
	}
	clean := filepath.Clean(p)
	if clean == "/" || clean == "." {
		return "filesystem or working-directory root", true
	}
	if home, _ := os.UserHomeDir(); home != "" && clean == filepath.Clean(home) {
		return "user home directory", true
	}
	regDir := filepath.Clean(filepath.Dir(s.Registry.Path))
	if regDir != "" && regDir != "." && (clean == regDir || isAncestor(clean, regDir)) {
		return "ancestor of the profile registry file", true
	}
	return "", false
}

// isAncestor reports whether ancestor strictly contains descendant.
func isAncestor(ancestor, descendant string) bool {
	sep := string(filepath.Separator)
	return strings.HasPrefix(descendant+sep, ancestor+sep) && ancestor != descendant
}

// resolveAsset is the shared prelude for every asset CRUD method: load
// the profile then look up the asset id, returning the canonical typed
// error if either step fails. Keeps the public CRUD methods readable as
// "load → act → return" one-liners.
func (s *Service) resolveAsset(profileRef, assetID string) (*profile.Profile, *asset.Asset, errs.DomainError) {
	loaded, err := s.LoadProfile(profileRef)
	if err != nil {
		return nil, nil, err
	}
	a := loaded.Assets[assetID]
	if a == nil {
		return nil, nil, AssetNotFoundError{AssetID: assetID}
	}
	return loaded, a, nil
}

// resolveProject mirrors resolveAsset for the project CRUD methods.
func (s *Service) resolveProject(profileRef, projectID string) (*profile.Profile, *project.Manifest, errs.DomainError) {
	loaded, err := s.LoadProfile(profileRef)
	if err != nil {
		return nil, nil, err
	}
	p := loaded.Projects[projectID]
	if p == nil {
		return nil, nil, ProjectNotFoundError{ProjectID: projectID}
	}
	return loaded, p, nil
}

// LoadAsset returns the asset identified by assetID inside the given
// profile. Missing asset → AssetNotFoundError.
func (s *Service) LoadAsset(profileRef, assetID string) (*asset.Asset, errs.DomainError) {
	_, a, err := s.resolveAsset(profileRef, assetID)
	return a, err
}

// UpdateAsset overwrites the asset manifest on disk with the caller's
// edits. The asset must already exist in the profile; this method is not
// a scaffold path (use InitAsset for that). The on-disk destination is
// resolved from the loaded profile, so a tampered caller cannot redirect
// the write outside the profile root.
//
// Note: the task description mentions recomputing per-file content
// hashes. The current asset model does not store hashes — internal/sync
// computes them on the fly from disk and the project's state.json — so
// the drift-vs-update distinction already works without an asset-side
// hash store. If a later change adds an in-memory hash field on
// asset.Asset, recompute it here.
//
// Panics if manifest is nil: a nil pointer is a programmer bug per
// docs/guidelines/errors.md, not a recoverable not-found.
func (s *Service) UpdateAsset(profileRef string, manifest *asset.Manifest) errs.DomainError {
	if manifest == nil {
		panic("app.Service.UpdateAsset: nil manifest")
	}
	_, target, err := s.resolveAsset(profileRef, manifest.ID)
	if err != nil {
		return err
	}
	return asset.SaveManifest(target.Dir, *manifest)
}

// DeleteAsset removes the asset folder from the profile and unselects the
// asset id from every project in the profile. Already-synced files in
// project repos remain on disk as orphaned files (see docs/glossary.md);
// the next Plan/Apply on those projects reports them as ChangeDelete via
// the managed-state baseline.
//
// Failures in the per-project unselect loop and the folder removal are
// both accumulated into errs.Errors so a partial failure surfaces every
// problem and re-running the method converges idempotently.
func (s *Service) DeleteAsset(profileRef, assetID string) errs.DomainError {
	loaded, target, err := s.resolveAsset(profileRef, assetID)
	if err != nil {
		return err
	}
	var failures errs.Errors
	if unselectErr := loaded.UnselectAsset(assetID); unselectErr != nil {
		failures = append(failures, unselectErr)
	}
	if delErr := asset.Delete(target.Dir); delErr != nil {
		failures = append(failures, delErr)
	}
	if len(failures) == 0 {
		return nil
	}
	return failures
}

// LoadProject returns the project manifest identified by projectID inside
// the given profile. Missing project → ProjectNotFoundError.
func (s *Service) LoadProject(profileRef, projectID string) (*project.Manifest, errs.DomainError) {
	_, p, err := s.resolveProject(profileRef, projectID)
	return p, err
}

// UpdateProject overwrites the project manifest on disk with the caller's
// edits. The project must already exist in the profile; this is not a
// create path (use AddProject for that).
//
// Panics if p is nil: a nil pointer is a programmer bug per
// docs/guidelines/errors.md, not a recoverable not-found.
func (s *Service) UpdateProject(profileRef string, p *project.Manifest) errs.DomainError {
	if p == nil {
		panic("app.Service.UpdateProject: nil manifest")
	}
	loaded, _, err := s.resolveProject(profileRef, p.ID)
	if err != nil {
		return err
	}
	return project.Save(loaded.Root, p)
}

// DeleteProject removes the project manifest from the profile. Files in
// the project's target repository remain on disk as orphaned files (see
// docs/glossary.md); the project's previous repo is not touched.
func (s *Service) DeleteProject(profileRef, projectID string) errs.DomainError {
	loaded, _, err := s.resolveProject(profileRef, projectID)
	if err != nil {
		return err
	}
	return project.Delete(loaded.Root, projectID)
}
