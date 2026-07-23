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
	"sync"

	"github.com/hexworks/agentfiles/internal/appapi"
	"github.com/hexworks/agentfiles/internal/asset"
	"github.com/hexworks/agentfiles/internal/config"
	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/profile"
	"github.com/hexworks/agentfiles/internal/project"
	"github.com/hexworks/agentfiles/internal/projectstore"
	"github.com/hexworks/agentfiles/internal/registry"
	"github.com/hexworks/agentfiles/internal/settings"
	"github.com/hexworks/agentfiles/internal/surfaces"
	llmsync "github.com/hexworks/agentfiles/internal/sync"
	"github.com/hexworks/agentfiles/internal/utils"
	"time"
)

// Service is the thin application layer used by the TUI.
//
// The domain packages do the real work:
//   - registry discovers profiles
//   - profile/asset load the profile source model
//   - projectstore persists per-user project selections
//   - render computes desired outputs
//   - sync compares and writes project files
//
// Service ties those packages together into user-facing operations. It
// holds the two centralized stores (profiles + projects) plus the
// settings store and the git committer seam, and guards live-settings
// reads/writes with an RWMutex so concurrent Bubble Tea cmds (planning
// on one goroutine while UpdateSettings runs on another) do not race.
type Service struct {
	Registry     *registry.Store
	Projects     *projectstore.Store
	profilesRoot string

	settingsStore *settings.Store
	settingsMu    sync.RWMutex
	settings      settings.Settings

	committer GitCommitter
}

// NewWithStores wires the three centralized stores plus the initial
// loaded settings and the git committer seam into a Service. It is the
// only constructor: cmd/af/main.go builds every store explicitly so the
// migrator and the app read the same instances; tests build them under
// t.TempDir() for the same reason. The projects store's KnownProfiles
// and Validator seams are wired here so the store enforces its
// invariants against the live registry without importing it directly.
// committer may be nil in tests that never exercise a git-aware path;
// production wiring always supplies one.
func NewWithStores(reg *registry.Store, proj *projectstore.Store, sset *settings.Store, s settings.Settings, committer GitCommitter) *Service {
	svc := &Service{
		Registry:      reg,
		Projects:      proj,
		settingsStore: sset,
		settings:      s,
		committer:     committer,
	}
	proj.KnownProfiles = svc.knownProfileIDs
	proj.Validator = func(m *project.Manifest) errs.DomainError { return m.Validate() }
	return svc
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
// updates its last-opened timestamp, and returns both aggregates the
// caller needs: the profile (assets from disk) and the per-profile
// project selections (composed from the projects store). The
// composition is a read-time convenience — writes still route through
// the owning aggregate.
func (s *Service) LoadProfile(ref string) (*appapi.LoadedProfile, errs.DomainError) {
	profileRef, err := s.Registry.Resolve(ref)
	if err != nil {
		return nil, err
	}
	if err := s.Registry.Touch(profileRef.ID); err != nil {
		return nil, err
	}
	loaded, err := profile.Load(profileRef.Path)
	if err != nil {
		return nil, err
	}
	projects, err := s.Projects.ListByProfile(loaded.Manifest.ID)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]*project.Manifest, len(projects))
	for _, p := range projects {
		byID[p.ID] = p
	}
	return &appapi.LoadedProfile{Profile: loaded, Projects: byID}, nil
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
	for _, assetID := range assetIDs {
		if loaded.Profile.Assets[assetID] == nil {
			domainErrs = append(domainErrs, AssetNotFoundError{AssetID: assetID})
		}
	}
	if len(domainErrs) > 0 {
		return nil, domainErrs
	}
	manifest := project.NewDraft(name, path, agents)
	manifest.SelectedAssetIDs = assetIDs
	if err := s.Projects.Add(loaded.Profile.Manifest.ID, manifest); err != nil {
		return nil, []errs.DomainError{s.translateStoreError(err, loaded)}
	}
	return manifest, nil
}

// InitAsset scaffolds a new asset inside the selected profile. The asset type
// determines the starter files that get created in the new asset directory.
// Callers may leave Manifest.ID blank — the service derives it from
// Manifest.Name via utils.Slug so the modal layer does not need to encode the
// id-derivation rule a second time.
func (s *Service) InitAsset(profileRef string, manifest asset.Manifest) (string, errs.DomainError) {
	loaded, err := s.LoadProfile(profileRef)
	if err != nil {
		return "", err
	}
	if manifest.ID == "" {
		manifest.ID = utils.Slug(manifest.Name, config.DefaultAssetSlug)
	}
	if loaded.Profile.Assets[manifest.ID] != nil {
		return "", AssetExistsError{AssetID: manifest.ID}
	}
	return asset.Init(loaded.Profile.Root, manifest)
}

// CreateAssetFromFolder creates a profile-owned asset whose content is copied
// from the project folder identified by dirKey (a project-relative,
// forward-slash key) and selects it for the project in one step. The three
// effects — create the asset, copy its files into the profile, and add it to
// the project's selection — form a single consistency boundary so a re-plan
// reclassifies those files as managed instead of unknown.
//
// The service re-plans and re-asserts the folder is registerable
// (appapi.RegisterableDirs) rather than trusting the caller, then resolves the
// absolute source by joining dirKey against the project root it loaded — a
// tampered caller cannot redirect the copy outside the repo. If the project
// save fails after the asset is written, the half-created asset is rolled back
// and both failures are accumulated so a re-run converges. Returns the new
// asset id.
func (s *Service) CreateAssetFromFolder(profileRef, projectID string, manifest asset.Manifest, dirKey string) (string, errs.DomainError) {
	loaded, p, err := s.resolveProject(profileRef, projectID)
	if err != nil {
		return "", err
	}
	syncPreview, planErr := llmsync.Plan(loaded.Profile, p)
	if planErr != nil {
		return "", planErr
	}
	leaves := appapi.LeavesFromChanges(previewFromSync(syncPreview).Changes)
	if !surfaces.RegisterableFolders(leaves)[dirKey] {
		return "", FolderNotRegisterableError{
			DirKey: dirKey,
			Reason: surfaces.ClassifyFolderRejection(dirKey, leaves),
		}
	}
	if manifest.ID == "" {
		manifest.ID = utils.Slug(manifest.Name, config.DefaultAssetSlug)
	}
	if loaded.Profile.Assets[manifest.ID] != nil {
		return "", AssetExistsError{AssetID: manifest.ID}
	}
	sourceDir := filepath.Join(p.Path, filepath.FromSlash(dirKey))
	dir, initErr := asset.InitFromFolder(loaded.Profile.Root, manifest, sourceDir)
	if initErr != nil {
		return "", initErr
	}
	p.SelectAsset(manifest.ID)
	if saveErr := s.Projects.Update(loaded.Profile.Manifest.ID, p); saveErr != nil {
		failures := errs.Errors{s.translateStoreError(saveErr, loaded)}
		if delErr := asset.Delete(dir); delErr != nil {
			failures = append(failures, delErr)
		}
		return "", failures
	}
	return manifest.ID, nil
}

// Plan builds a sync preview for one project. This is the read-only half of the
// pipeline: load profile -> render desired files -> compare with the repo.
func (s *Service) Plan(profileRef, projectID string) (*appapi.Preview, errs.DomainError) {
	syncPreview, err := s.planSync(profileRef, projectID)
	if err != nil {
		return nil, err
	}
	return previewFromSync(syncPreview), nil
}

// planSync is the internal helper that returns the full domain Preview
// needed by Apply. Public callers receive the boundary mirror via Plan
// so the TUI never has to import internal/sync.
func (s *Service) planSync(profileRef, projectID string) (*llmsync.Preview, errs.DomainError) {
	loaded, err := s.LoadProfile(profileRef)
	if err != nil {
		return nil, err
	}
	proj := loaded.Projects[projectID]
	if proj == nil {
		return nil, ProjectNotFoundError{ProjectID: projectID}
	}
	return llmsync.Plan(loaded.Profile, proj)
}

func previewFromSync(p *llmsync.Preview) *appapi.Preview {
	if p == nil {
		return nil
	}
	changes := make([]appapi.FileChange, len(p.Changes))
	for i, ch := range p.Changes {
		changes[i] = appapi.FileChange{Path: ch.Path, Kind: appapi.ChangeKind(ch.Kind)}
	}
	var ignored []string
	if p.ManagedState != nil {
		ignored = slices.Clone(p.ManagedState.IgnoredPaths)
	}
	return &appapi.Preview{
		ProfileID:    p.ProfileID,
		ProjectID:    p.ProjectID,
		Changes:      changes,
		IgnoredPaths: ignored,
	}
}

// Apply executes the write half of the pipeline by first building a preview
// and then asking the sync package to materialize the desired files. The
// caller supplies per-file resolutions for drift and unknown entries.
// Defaults (no resolution for a path): drift kept, unknown kept; create/
// update/delete always apply. r.IgnoredPaths carries the complete desired
// ignored set the TUI computed this apply; sync writes it verbatim (replace,
// not merge) so un-ignoring a folder drops it from ignored_paths.
//
// Newly added ignored keys (incoming − prior) are re-asserted as registerable
// against the freshly computed plan (the same all-unknown rule the TUI button
// gate uses), so a stale or hand-built key cannot persist an ancestor of
// managed files into ignored_paths. Already-persisted keys are exempt: their
// folders have legitimately vanished from the plan.
//
// When git integration is enabled and the project's target repo is a git
// repository, a single scoped commit is recorded after sync succeeds
// covering every mutated file plus `.agentfiles/state.json`. The
// returned CommitOutcome carries the specific outcome (Committed,
// Skipped, or Failed); a hard commit failure never rolls back the file
// writes (they already happened).
func (s *Service) Apply(profileRef, projectID string, r appapi.Resolutions) (*appapi.Preview, appapi.CommitOutcome, errs.DomainError) {
	loaded, proj, projErr := s.resolveProject(profileRef, projectID)
	if projErr != nil {
		return nil, appapi.Skipped{Reason: appapi.SkipDisabled}, projErr
	}
	syncPreview, err := llmsync.Plan(loaded.Profile, proj)
	if err != nil {
		return nil, appapi.Skipped{Reason: appapi.SkipDisabled}, err
	}
	if eligErr := s.assertIgnoredRegisterable(syncPreview, r.IgnoredPaths); eligErr != nil {
		return nil, appapi.Skipped{Reason: appapi.SkipDisabled}, eligErr
	}
	syncResolutions := llmsync.Resolutions{
		Drift:        toSyncDriftResolutions(r.Drift),
		Unknown:      toSyncUnknownResolutions(r.Unknown),
		IgnoredPaths: r.IgnoredPaths,
	}
	result, applyErr := llmsync.Apply(syncPreview, syncResolutions)
	if applyErr != nil {
		return nil, appapi.Skipped{Reason: appapi.SkipDisabled}, applyErr
	}
	outcome := s.runCommit(triggerSyncProject, commitTriggerCtx{
		ProjectName:  proj.Name,
		MutatedFiles: result.Mutated,
		StatePath:    result.StatePath,
	}, proj.Path)
	return previewFromSync(syncPreview), outcome, nil
}

// commitEnabled reports whether git-aware commits should run: the
// setting is on and a committer is wired. Read under settingsMu.
func (s *Service) commitEnabled() bool {
	if s == nil || s.committer == nil {
		return false
	}
	s.settingsMu.RLock()
	defer s.settingsMu.RUnlock()
	return s.settings.Git.Enabled
}

// runCommit is the shared adapter between service methods and the
// GitCommitter seam. When the feature is disabled it returns
// Skipped{SkipDisabled}; otherwise it delegates to the injected
// committer and returns whatever discriminated outcome it produces.
// The pathspec + subject come from the injected trigger, so the three
// call sites do not spell out the template rule twice (see triggers.go).
func (s *Service) runCommit(t commitTrigger, ctx commitTriggerCtx, dir string) appapi.CommitOutcome {
	if !s.commitEnabled() {
		return appapi.Skipped{Reason: appapi.SkipDisabled}
	}
	pathspec := t.Pathspec(ctx)
	subject := t.Subject(ctx)
	return s.committer.Commit(dir, pathspec, subject, s.runHooksEnabled())
}

// runHooksEnabled snapshots the git.run_hooks setting under the same
// lock that commitEnabled uses so a Save on another goroutine cannot
// race it.
func (s *Service) runHooksEnabled() bool {
	s.settingsMu.RLock()
	defer s.settingsMu.RUnlock()
	return s.settings.Git.RunHooks
}

// Settings returns a snapshot of the currently active settings. The
// Settings TUI screen consumes it on entry; there is no live-reload
// path from disk while the app is running (see ADR 0019).
func (s *Service) Settings() settings.Settings {
	s.settingsMu.RLock()
	defer s.settingsMu.RUnlock()
	return s.settings
}

// UpdateSettings persists next to disk and swaps it in. When enabling
// git for the first time the pre-flight rejects the save if the git
// binary is missing so the toggle can never enter an unusable state.
// The store is required — a nil settings store is a wiring bug. All
// reads and writes to the cached settings value go through
// settingsMu; concurrent commit-path reads and settings-screen writes
// therefore serialize.
func (s *Service) UpdateSettings(next settings.Settings) errs.DomainError {
	if s.settingsStore == nil {
		return SettingsUnavailableError{}
	}
	s.settingsMu.RLock()
	priorEnabled := s.settings.Git.Enabled
	s.settingsMu.RUnlock()
	if next.Git.Enabled && !priorEnabled {
		if err := s.probeGitBinary(); err != nil {
			return err
		}
	}
	if err := s.settingsStore.Save(next); err != nil {
		return err
	}
	s.settingsMu.Lock()
	s.settings = next
	s.settingsMu.Unlock()
	return nil
}

// SettingsPath exposes the on-disk location of the settings store for
// diagnostics (never for read/write bypass). Callers must not use it
// to hand-parse the file — the cached value in Service is the truth.
func (s *Service) SettingsPath() string {
	if s.settingsStore == nil {
		return ""
	}
	return s.settingsStore.Path
}

// probeGitBinary runs the injectable pre-flight through the committer
// seam. A wired committer answers on behalf of internal/git; when no
// committer is wired (test-only case) the pre-flight is a no-op — the
// commit path is disabled anyway.
func (s *Service) probeGitBinary() errs.DomainError {
	if s.committer == nil {
		return nil
	}
	return s.committer.BinaryAvailable()
}

// assertIgnoredRegisterable verifies every newly selected ignored key is an
// all-unknown folder in the freshly computed plan. Keys already persisted in
// the prior managed state are skipped because their folders no longer appear
// as unknown (they are already suppressed). Bad keys accumulate so the caller
// learns about every offending folder at once.
func (s *Service) assertIgnoredRegisterable(syncPreview *llmsync.Preview, ignoredPaths []string) errs.DomainError {
	if len(ignoredPaths) == 0 {
		return nil
	}
	var prior []string
	if syncPreview.ManagedState != nil {
		prior = syncPreview.ManagedState.IgnoredPaths
	}
	leaves := appapi.LeavesFromChanges(previewFromSync(syncPreview).Changes)
	registerable := surfaces.RegisterableFolders(leaves)
	var failures errs.Errors
	for _, p := range ignoredPaths {
		if slices.Contains(prior, p) {
			continue
		}
		if !registerable[p] {
			failures = append(failures, FolderNotRegisterableError{
				DirKey: p,
				Reason: surfaces.ClassifyFolderRejection(p, leaves),
			})
		}
	}
	if len(failures) == 0 {
		return nil
	}
	return failures
}

func toSyncDriftResolutions(in []appapi.DriftResolution) []llmsync.DriftResolution {
	if len(in) == 0 {
		return nil
	}
	out := make([]llmsync.DriftResolution, len(in))
	for i, r := range in {
		out[i] = llmsync.DriftResolution{Path: r.Path, Decision: llmsync.DriftDecision(r.Decision)}
	}
	return out
}

func toSyncUnknownResolutions(in []appapi.UnknownResolution) []llmsync.UnknownResolution {
	if len(in) == 0 {
		return nil
	}
	out := make([]llmsync.UnknownResolution, len(in))
	for i, r := range in {
		out[i] = llmsync.UnknownResolution{Path: r.Path, Decision: llmsync.UnknownDecision(r.Decision)}
	}
	return out
}

// translateStoreError converts a projectstore.ProjectPathOwnedError into
// the app-layer equivalent that carries human-facing profile and project
// names, so the TUI can render a specific conflict message. Any other
// error type is returned unchanged. The invariant itself lives inside
// projectstore.Store.Add/Update (see ADR 0017 Aggregate boundaries) —
// this method only translates for presentation.
func (s *Service) translateStoreError(err errs.DomainError, loaded *appapi.LoadedProfile) errs.DomainError {
	if err == nil {
		return nil
	}
	var owned projectstore.ProjectPathOwnedError
	if !errors.As(err, &owned) {
		return err
	}
	profileName := ""
	projectName := ""
	if loaded != nil && owned.ExistingProfileID == loaded.Profile.Manifest.ID {
		profileName = loaded.Profile.Manifest.Name
		if p := loaded.Projects[owned.ExistingProjectID]; p != nil {
			projectName = p.Name
		}
	}
	if profileName == "" || projectName == "" {
		names := s.profileNameByID()
		if profileName == "" {
			profileName = names[owned.ExistingProfileID]
		}
		if projectName == "" {
			all, listErr := s.Projects.AllProjects()
			if listErr == nil {
				for _, op := range all {
					if op.ProfileID == owned.ExistingProfileID && op.Manifest.ID == owned.ExistingProjectID {
						projectName = op.Manifest.Name
						break
					}
				}
			}
		}
	}
	return ProjectPathOwnedError{
		Path:        owned.Path,
		ProfileName: profileName,
		ProjectName: projectName,
	}
}

// knownProfileIDs is the projectstore.KnownProvider that lets the
// projects store detect orphan groups without importing internal/registry.
// A registry read failure surfaces to the store, which returns it from
// its next CRUD call.
func (s *Service) knownProfileIDs() (map[string]struct{}, errs.DomainError) {
	reg, err := s.Registry.Load()
	if err != nil {
		return nil, err
	}
	out := make(map[string]struct{}, len(reg.Profiles))
	for _, ref := range reg.Profiles {
		out[ref.ID] = struct{}{}
	}
	return out, nil
}

// profileNameByID indexes registered profile refs by id so path-conflict
// messages can display a friendly name without a second lookup per hit.
// A registry read failure returns an empty map; callers get an ok
// message shape with a blank profile name rather than a fatal error at
// what is really a lookup-side concern.
func (s *Service) profileNameByID() map[string]string {
	reg, err := s.Registry.Load()
	if err != nil {
		return map[string]string{}
	}
	names := make(map[string]string, len(reg.Profiles))
	for _, ref := range reg.Profiles {
		names[ref.ID] = ref.Name
	}
	return names
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
func (s *Service) LoadProfiles() ([]*appapi.LoadedProfile, []errs.DomainError) {
	reg, err := s.Registry.Load()
	if err != nil {
		return nil, []errs.DomainError{err}
	}
	loaded := make([]*appapi.LoadedProfile, 0, len(reg.Profiles))
	var loadErrs []errs.DomainError
	for _, ref := range reg.Profiles {
		p, loadErr := profile.Load(ref.Path)
		if loadErr != nil {
			loadErrs = append(loadErrs, loadErr)
			continue
		}
		projects, listErr := s.Projects.ListByProfile(p.Manifest.ID)
		if listErr != nil {
			loadErrs = append(loadErrs, listErr)
			continue
		}
		byID := make(map[string]*project.Manifest, len(projects))
		for _, pj := range projects {
			byID[pj.ID] = pj
		}
		loaded = append(loaded, &appapi.LoadedProfile{Profile: p, Projects: byID})
	}
	return loaded, loadErrs
}

// DeleteProfile removes the profile from the global registry and cascades
// through the projects store so per-user selections for that profile do
// not linger as orphans. The on-disk profile folder is left untouched;
// use DeleteProfileWithFolder for the destructive variant.
//
// Projects are removed before the registry entry so a mid-operation crash
// leaves the registry entry (and therefore recovery possible) rather than
// leaving orphaned project groups that the next Load would surface as an
// OrphanProfileIDError.
func (s *Service) DeleteProfile(profileRef string) errs.DomainError {
	ref, err := s.Registry.Resolve(profileRef)
	if err != nil {
		return err
	}
	if cascadeErr := s.Projects.RemoveByProfile(ref.ID); cascadeErr != nil {
		return cascadeErr
	}
	return s.Registry.Remove(ref.ID)
}

// SetProfilesRoot configures the allow-list root that constrains where
// DeleteProfileWithFolder can recurse. Any registered ref whose path
// does not sit under this root refuses deletion — the safety fence
// cannot be bypassed by a symlink, a tampered registry entry, or a
// hand-edited profiles.json pointing at arbitrary user territory. An
// empty root disables the fence (used by tests that share a t.TempDir()
// for both stores and profile folders); production wiring in main.go
// always sets it.
func (s *Service) SetProfilesRoot(root string) {
	if root == "" {
		s.profilesRoot = ""
		return
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		s.profilesRoot = filepath.Clean(root)
		return
	}
	s.profilesRoot = abs
}

// DeleteProfileWithFolder removes both the registry entry and the on-disk
// profile folder. The folder is removed first; if that fails the registry
// entry is left in place so the user can retry from the TUI. A
// pre-missing folder is treated as success.
//
// Three safety checks gate the recursive removal: ref.Path must sit
// under the configured profiles root (see SetProfilesRoot) so deletion
// cannot escape into arbitrary user territory even with a symlinked or
// tampered entry; it must still contain a profile.json (so the folder
// still looks like a profile root); and it must not be a pathological
// deletion target (empty, the filesystem root, the user's home
// directory, or an ancestor of the global registry file).
func (s *Service) DeleteProfileWithFolder(profileRef string) errs.DomainError {
	ref, err := s.Registry.Resolve(profileRef)
	if err != nil {
		return err
	}
	if reason, unsafe := s.isUnsafeProfilePath(ref.Path); unsafe {
		return UnsafeProfilePathError{Path: ref.Path, Reason: reason}
	}
	if !utils.Exists(ref.Path) {
		if cascadeErr := s.Projects.RemoveByProfile(ref.ID); cascadeErr != nil {
			return cascadeErr
		}
		return s.Registry.Remove(ref.ID)
	}
	if !utils.Exists(filepath.Join(ref.Path, config.ProfileManifestFileName)) {
		return ProfileFolderNotARootError{Path: ref.Path}
	}
	if rmErr := os.RemoveAll(ref.Path); rmErr != nil && !errors.Is(rmErr, fs.ErrNotExist) {
		return ProfileFolderRemoveError{Path: ref.Path, Err: rmErr}
	}
	if cascadeErr := s.Projects.RemoveByProfile(ref.ID); cascadeErr != nil {
		return cascadeErr
	}
	return s.Registry.Remove(ref.ID)
}

// isUnsafeProfilePath rejects pathological deletion targets independent
// of who tampered with the registry: the empty string, the filesystem
// root, the user's home directory, any path that is the registry file's
// directory or one of its ancestors, and any path that sits outside the
// configured profiles-root allow-list. When a profiles root is
// configured the allow-list check is authoritative; the older
// string-comparison checks remain as belt-and-suspenders for
// installations without a configured root.
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
	if regDir != "" && regDir != "." && (clean == regDir || utils.IsAncestor(clean, regDir)) {
		return "ancestor of the profile registry file", true
	}
	if s.profilesRoot != "" && !utils.IsUnderRoot(clean, s.profilesRoot) {
		return "outside the configured profiles root", true
	}
	return "", false
}

// resolveAsset is the shared prelude for every asset CRUD method: load
// the profile then look up the asset id, returning the canonical typed
// error if either step fails. Keeps the public CRUD methods readable as
// "load → act → return" one-liners.
func (s *Service) resolveAsset(profileRef, assetID string) (*appapi.LoadedProfile, *asset.Asset, errs.DomainError) {
	loaded, err := s.LoadProfile(profileRef)
	if err != nil {
		return nil, nil, err
	}
	a := loaded.Profile.Assets[assetID]
	if a == nil {
		return nil, nil, AssetNotFoundError{AssetID: assetID}
	}
	return loaded, a, nil
}

// resolveProject mirrors resolveAsset for the project CRUD methods.
func (s *Service) resolveProject(profileRef, projectID string) (*appapi.LoadedProfile, *project.Manifest, errs.DomainError) {
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
// edits and, when git integration is enabled and the profile folder is a
// git repository, records a scoped commit against the manifest file. The
// asset must already exist in the profile; this method is not a scaffold
// path (use InitAsset for that). The on-disk destination is resolved
// from the loaded profile, so a tampered caller cannot redirect the
// write outside the profile root.
//
// The returned CommitOutcome is one of appapi.Committed, appapi.Skipped
// (feature disabled, dir not a repo, empty diff), or appapi.Failed for
// a hard commit failure. A Failed outcome is never a save failure —
// the manifest is already on disk by then.
//
// Panics if manifest is nil: a nil pointer is a programmer bug per
// docs/guidelines/errors.md, not a recoverable not-found.
func (s *Service) UpdateAsset(profileRef string, manifest *asset.Manifest) (appapi.CommitOutcome, errs.DomainError) {
	if manifest == nil {
		panic("app.Service.UpdateAsset: nil manifest")
	}
	loaded, target, err := s.resolveAsset(profileRef, manifest.ID)
	if err != nil {
		return appapi.Skipped{Reason: appapi.SkipDisabled}, err
	}
	if saveErr := asset.SaveManifest(target.Dir, *manifest); saveErr != nil {
		return appapi.Skipped{Reason: appapi.SkipDisabled}, saveErr
	}
	outcome := s.runCommit(triggerAssetManifest, commitTriggerCtx{
		AssetID:      manifest.ID,
		AssetDir:     target.Dir,
		ManifestPath: filepath.Join(target.Dir, config.AssetManifestFileName),
	}, loaded.Profile.Root)
	return outcome, nil
}

// SaveAssetFilesEdit persists the caller's manifest edits then records a
// files-scoped commit against the asset directory in the profile repo.
// It is the editor-return flow's counterpart to UpdateAsset: the files
// edit changed the on-disk content, and the single commit covers both
// the manifest and every file inside the asset directory via a
// recursive pathspec. Same panic contract as UpdateAsset for a nil
// manifest.
func (s *Service) SaveAssetFilesEdit(profileRef string, manifest *asset.Manifest) (appapi.CommitOutcome, errs.DomainError) {
	if manifest == nil {
		panic("app.Service.SaveAssetFilesEdit: nil manifest")
	}
	loaded, target, err := s.resolveAsset(profileRef, manifest.ID)
	if err != nil {
		return appapi.Skipped{Reason: appapi.SkipDisabled}, err
	}
	if saveErr := asset.SaveManifest(target.Dir, *manifest); saveErr != nil {
		return appapi.Skipped{Reason: appapi.SkipDisabled}, saveErr
	}
	outcome := s.runCommit(triggerAssetFiles, commitTriggerCtx{
		AssetID:  manifest.ID,
		AssetDir: target.Dir,
	}, loaded.Profile.Root)
	return outcome, nil
}

// AddAssetFile creates an empty file at rel inside the asset folder.
// Containment + reserved-name policy lives in asset.AddFile; the service
// only resolves the asset directory from the loaded profile so a tampered
// caller cannot redirect the write outside the profile root.
func (s *Service) AddAssetFile(profileRef, assetID, rel string) errs.DomainError {
	_, target, err := s.resolveAsset(profileRef, assetID)
	if err != nil {
		return err
	}
	return asset.AddFile(target.Dir, rel)
}

// RemoveAssetFile deletes the file at rel inside the asset folder.
// Symmetric with AddAssetFile; pre-missing file → success.
func (s *Service) RemoveAssetFile(profileRef, assetID, rel string) errs.DomainError {
	_, target, err := s.resolveAsset(profileRef, assetID)
	if err != nil {
		return err
	}
	return asset.RemoveFile(target.Dir, rel)
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
	for _, p := range loaded.ProjectList() {
		idx := slices.Index(p.SelectedAssetIDs, assetID)
		if idx < 0 {
			continue
		}
		p.SelectedAssetIDs = slices.Delete(p.SelectedAssetIDs, idx, idx+1)
		if saveErr := s.Projects.Update(loaded.Profile.Manifest.ID, p); saveErr != nil {
			failures = append(failures, saveErr)
		}
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

// UpdateProject applies user-editable fields (name, repo path, enabled
// agents) to the project identified by projectID and persists the result.
// ID, SelectedAssetIDs, and CreatedAt are preserved by loading the
// on-disk manifest first and only overwriting the editable fields — the
// merge contract lives here so the TUI never holds a live aggregate
// pointer it has half-mutated.
func (s *Service) UpdateProject(profileRef, projectID, name, path string, enabledAgents []string) errs.DomainError {
	loaded, p, err := s.resolveProject(profileRef, projectID)
	if err != nil {
		return err
	}
	p.Name = name
	p.Path = path
	p.EnabledAgents = append([]string(nil), enabledAgents...)
	return s.translateStoreError(s.Projects.Update(loaded.Profile.Manifest.ID, p), loaded)
}

// SelectAsset appends assetID to the project's SelectedAssetIDs if not
// already present and persists the change. The returned slice is the
// authoritative selection after the call so callers do not project the
// next state TUI-side; idempotent on duplicates. Returns
// AssetNotFoundError when the asset is unknown in the profile and
// ProjectNotFoundError when the project is missing.
func (s *Service) SelectAsset(profileRef, projectID, assetID string) ([]string, errs.DomainError) {
	loaded, p, err := s.resolveProject(profileRef, projectID)
	if err != nil {
		return nil, err
	}
	if loaded.Profile.Assets[assetID] == nil {
		return nil, AssetNotFoundError{AssetID: assetID}
	}
	if p.SelectAsset(assetID) {
		if saveErr := s.Projects.Update(loaded.Profile.Manifest.ID, p); saveErr != nil {
			return nil, saveErr
		}
	}
	return append([]string(nil), p.SelectedAssetIDs...), nil
}

// UnselectAsset removes assetID from the project's SelectedAssetIDs if
// present and persists the change. The returned slice is the
// authoritative selection after the call (idempotent on a not-present
// id). Returns AssetNotFoundError when the asset is unknown in the
// profile and ProjectNotFoundError when the project is missing.
func (s *Service) UnselectAsset(profileRef, projectID, assetID string) ([]string, errs.DomainError) {
	loaded, p, err := s.resolveProject(profileRef, projectID)
	if err != nil {
		return nil, err
	}
	if loaded.Profile.Assets[assetID] == nil {
		return nil, AssetNotFoundError{AssetID: assetID}
	}
	idx := -1
	for i, existing := range p.SelectedAssetIDs {
		if existing == assetID {
			idx = i
			break
		}
	}
	if idx == -1 {
		return append([]string(nil), p.SelectedAssetIDs...), nil
	}
	p.SelectedAssetIDs = append(p.SelectedAssetIDs[:idx], p.SelectedAssetIDs[idx+1:]...)
	if saveErr := s.Projects.Update(loaded.Profile.Manifest.ID, p); saveErr != nil {
		return nil, saveErr
	}
	return append([]string(nil), p.SelectedAssetIDs...), nil
}

// DeleteProject removes the project manifest from the profile. Files in
// the project's target repository remain on disk as orphaned files (see
// docs/glossary.md); the project's previous repo is not touched.
func (s *Service) DeleteProject(profileRef, projectID string) errs.DomainError {
	loaded, _, err := s.resolveProject(profileRef, projectID)
	if err != nil {
		return err
	}
	return s.Projects.Remove(loaded.Profile.Manifest.ID, projectID)
}
