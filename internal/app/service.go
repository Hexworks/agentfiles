// Package app is the thin application layer that the CLI and TUI call into.
// It orchestrates the domain packages (registry, profile, asset, project,
// render, sync) but contains no business logic of its own.
package app

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/hexworks/agentfiles/internal/asset"
	"github.com/hexworks/agentfiles/internal/config"
	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/git"
	"github.com/hexworks/agentfiles/internal/profile"
	"github.com/hexworks/agentfiles/internal/project"
	"github.com/hexworks/agentfiles/internal/projectstore"
	"github.com/hexworks/agentfiles/internal/registry"
	"github.com/hexworks/agentfiles/internal/settings"
	"github.com/hexworks/agentfiles/internal/surfaces"
	llmsync "github.com/hexworks/agentfiles/internal/sync"
	"github.com/hexworks/agentfiles/internal/utils"
)

// LoadedProfile pairs a profile aggregate with the per-user project
// selections that live in a separate aggregate on disk. Callers get one
// value that carries both boundaries so they do not have to compose
// (profile + projects) themselves; the composition is a read-time
// convenience — writes still route through the owning aggregate
// (profile assets in the profile folder, project manifests in the
// projects store). See ADR 0017.
type LoadedProfile struct {
	Profile  *profile.Profile
	Projects map[string]*project.Manifest
}

// ProjectList returns the loaded projects sorted by display name so
// callers get a stable order without re-implementing the rule.
func (l *LoadedProfile) ProjectList() []*project.Manifest {
	list := make([]*project.Manifest, 0, len(l.Projects))
	for _, p := range l.Projects {
		list = append(list, p)
	}
	slices.SortFunc(list, func(a, b *project.Manifest) int {
		return strings.Compare(a.Name, b.Name)
	})
	return list
}

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
// holds the two centralized stores (profiles + projects) so every
// operation runs against the same aggregate root.
type Service struct {
	Registry      *registry.Store
	Projects      *projectstore.Store
	SettingsStore *settings.Store
	settings      settings.Settings
	committer     GitCommitter
	profilesRoot  string
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
		SettingsStore: sset,
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
func (s *Service) LoadProfile(ref string) (*LoadedProfile, errs.DomainError) {
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
	return &LoadedProfile{Profile: loaded, Projects: byID}, nil
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

// RegisterableDirs returns the set of directory keys in changes that
// are eligible for asset registration. Thin adapter over
// surfaces.RegisterableFolders: copies the change-kind classification
// into surfaces.Leaf so the eligibility rule and its container-root
// data live together in surfaces, while app keeps its
// FileChange/ChangeKind vocabulary intact.
func RegisterableDirs(changes []FileChange) map[string]bool {
	return surfaces.RegisterableFolders(leavesFromChanges(changes))
}

func leavesFromChanges(changes []FileChange) []surfaces.Leaf {
	leaves := make([]surfaces.Leaf, len(changes))
	for i, ch := range changes {
		leaves[i] = surfaces.Leaf{Path: ch.Path, IsUnknown: ch.Kind == ChangeUnknown}
	}
	return leaves
}

// CreateAssetFromFolder creates a profile-owned asset whose content is copied
// from the project folder identified by dirKey (a project-relative,
// forward-slash key) and selects it for the project in one step. The three
// effects — create the asset, copy its files into the profile, and add it to
// the project's selection — form a single consistency boundary so a re-plan
// reclassifies those files as managed instead of unknown.
//
// The service re-plans and re-asserts the folder is registerable
// (RegisterableDirs) rather than trusting the caller, then resolves the
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
	leaves := leavesFromChanges(previewFromSync(syncPreview).Changes)
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
func (s *Service) Plan(profileRef, projectID string) (*Preview, errs.DomainError) {
	syncPreview, err := s.planSync(profileRef, projectID)
	if err != nil {
		return nil, err
	}
	return previewFromSync(syncPreview), nil
}

// planSync is the internal helper that returns the full domain Preview
// needed by Apply. Public callers receive the app-layer mirror via Plan
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

// ChangeKind mirrors llmsync.ChangeKind. The TUI consumes the app
// vocabulary so it never imports internal/sync directly, keeping the
// documented tui → app dependency edge true.
type ChangeKind string

// Possible ChangeKind values mirror llmsync.ChangeKind.
const (
	ChangeCreate  ChangeKind = "create"
	ChangeUpdate  ChangeKind = "update"
	ChangeDrift   ChangeKind = "drift"
	ChangeDelete  ChangeKind = "delete"
	ChangeUnknown ChangeKind = "unknown"
)

// FileChange is the app-layer mirror of llmsync.FileChange. Only the
// fields the TUI consumes are exposed; render leaves and managed-state
// metadata stay inside the domain.
type FileChange struct {
	Path string
	Kind ChangeKind
}

// Preview is the app-layer mirror of llmsync.Preview. It carries the
// change list the TUI renders plus the identifying ids; render leaves
// and managed-state stay inside the domain.
type Preview struct {
	ProfileID string
	ProjectID string
	Changes   []FileChange
	// IgnoredPaths mirrors the persisted ManagedState.IgnoredPaths: the folder
	// keys whose unknown subtree the plan suppressed. The Plan Project screen
	// surfaces these so the user can view and un-ignore them; nil when the
	// project has no managed state yet.
	IgnoredPaths []string
}

func previewFromSync(p *llmsync.Preview) *Preview {
	if p == nil {
		return nil
	}
	changes := make([]FileChange, len(p.Changes))
	for i, ch := range p.Changes {
		changes[i] = FileChange{Path: ch.Path, Kind: ChangeKind(ch.Kind)}
	}
	var ignored []string
	if p.ManagedState != nil {
		ignored = slices.Clone(p.ManagedState.IgnoredPaths)
	}
	return &Preview{
		ProfileID:    p.ProfileID,
		ProjectID:    p.ProjectID,
		Changes:      changes,
		IgnoredPaths: ignored,
	}
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

// Resolutions is the app-layer mirror of llmsync.Resolutions: the drift
// and unknown per-file choices plus the folder keys to ignore, bundled so
// Apply's signature stays stable as resolution kinds grow.
type Resolutions struct {
	Drift        []DriftResolution
	Unknown      []UnknownResolution
	IgnoredPaths []string
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
// returned CommitOutcome carries the short SHA on success; a hard
// commit failure surfaces on CommitOutcome.Err but does not roll back
// the file writes (they already happened).
func (s *Service) Apply(profileRef, projectID string, r Resolutions) (*Preview, CommitOutcome, errs.DomainError) {
	loaded, proj, projErr := s.resolveProject(profileRef, projectID)
	if projErr != nil {
		return nil, CommitOutcome{}, projErr
	}
	syncPreview, err := llmsync.Plan(loaded.Profile, proj)
	if err != nil {
		return nil, CommitOutcome{}, err
	}
	if eligErr := s.assertIgnoredRegisterable(syncPreview, r.IgnoredPaths); eligErr != nil {
		return nil, CommitOutcome{}, eligErr
	}
	syncResolutions := llmsync.Resolutions{
		Drift:        toSyncDriftResolutions(r.Drift),
		Unknown:      toSyncUnknownResolutions(r.Unknown),
		IgnoredPaths: r.IgnoredPaths,
	}
	if err := llmsync.Apply(syncPreview, syncResolutions); err != nil {
		return nil, CommitOutcome{}, err
	}
	appPreview := previewFromSync(syncPreview)
	mutated := mutatedPaths(appPreview, r)
	msg := fmt.Sprintf("chore(agentfiles): sync project %s (%d files)", proj.Name, len(mutated)-1)
	outcome := s.runCommit(proj.Path, mutated, msg)
	return appPreview, outcome, nil
}

// mutatedPaths returns the pathspec the plan-apply commit covers: every
// file the sync engine created / updated / deleted or the caller
// resolved as overwrite / delete, plus the managed-state snapshot at
// `.agentfiles/state.json`. Paths are kept as forward-slash strings so
// git's own pathspec grammar matches them directly.
func mutatedPaths(preview *Preview, r Resolutions) []string {
	if preview == nil {
		return []string{config.StateDirName + "/" + config.StateFileName}
	}
	drift := map[string]DriftDecision{}
	for _, d := range r.Drift {
		drift[d.Path] = d.Decision
	}
	unknown := map[string]UnknownDecision{}
	for _, u := range r.Unknown {
		unknown[u.Path] = u.Decision
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(preview.Changes)+1)
	add := func(p string) {
		if seen[p] {
			return
		}
		seen[p] = true
		out = append(out, p)
	}
	for _, ch := range preview.Changes {
		switch ch.Kind {
		case ChangeCreate, ChangeUpdate, ChangeDelete:
			add(ch.Path)
		case ChangeDrift:
			if drift[ch.Path] == DriftOverwrite {
				add(ch.Path)
			}
		case ChangeUnknown:
			if unknown[ch.Path] == UnknownDelete {
				add(ch.Path)
			}
		}
	}
	add(config.StateDirName + "/" + config.StateFileName)
	return out
}

// commitEnabled reports whether git-aware commits should run: the
// setting is on and a committer is wired.
func (s *Service) commitEnabled() bool {
	return s != nil && s.settings.Git.Enabled && s.committer != nil
}

// runCommit is the shared adapter between service methods and the
// GitCommitter seam. When the feature is disabled or no committer is
// wired it returns a zero-value CommitOutcome so callers still see a
// consistent shape. Typed commit failures ride on CommitOutcome.Err;
// they are never returned as domain errors from the parent method
// because the file write already succeeded.
func (s *Service) runCommit(dir string, pathspec []string, msg string) CommitOutcome {
	if !s.commitEnabled() {
		return CommitOutcome{}
	}
	sha, err := s.committer.Commit(dir, pathspec, msg)
	if err != nil {
		return CommitOutcome{Err: err}
	}
	return CommitOutcome{SHA: sha}
}

// Settings returns a read-only copy of the currently active settings.
// The Settings TUI screen consumes it on entry; there is no live-reload
// path from disk while the app is running (see ADR 0019).
func (s *Service) Settings() settings.Settings { return s.settings }

// UpdateSettings persists new to disk and swaps it in. When enabling
// git for the first time the pre-flight rejects the save if the git
// binary is missing so the toggle can never enter an unusable state.
// The store is required — a nil settings store is a wiring bug.
func (s *Service) UpdateSettings(next settings.Settings) errs.DomainError {
	if s.SettingsStore == nil {
		return SettingsUnavailableError{}
	}
	if next.Git.Enabled && !s.settings.Git.Enabled {
		if err := git.BinaryAvailable(); err != nil {
			return err
		}
	}
	if err := s.SettingsStore.Save(next); err != nil {
		return err
	}
	s.settings = next
	return nil
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
	leaves := leavesFromChanges(previewFromSync(syncPreview).Changes)
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

// DesiredIgnored computes the complete persisted ignored set a project should
// have after the user's in-session edits on the Plan Project screen:
// (persisted − unignored) ∪ newlyIgnored, deduplicated and sorted. The TUI
// sends the result on Apply and sync writes it verbatim (replace semantics), so
// dropping a key here un-ignores that folder on the next plan. The rule lives
// here, in the stable layer, rather than inside a Bubble Tea screen so it is
// testable without the TUI and sits next to assertIgnoredRegisterable, which
// guards the same set. Returns nil when the desired set is empty.
func DesiredIgnored(persisted, unignored, newlyIgnored []string) []string {
	drop := make(map[string]bool, len(unignored))
	for _, p := range unignored {
		drop[p] = true
	}
	desired := make(map[string]bool, len(persisted)+len(newlyIgnored))
	for _, p := range persisted {
		if !drop[p] {
			desired[p] = true
		}
	}
	for _, p := range newlyIgnored {
		desired[p] = true
	}
	if len(desired) == 0 {
		return nil
	}
	out := make([]string, 0, len(desired))
	for p := range desired {
		out = append(out, p)
	}
	slices.Sort(out)
	return out
}

// DriftResolutionsFromMap encodes the ADR 0015 emission contract: a
// drift row emits a resolution only when the user picked DriftOverwrite;
// DriftKeep (and "no choice") stays absent so sync preserves the prior
// baseline. Callers assembling the Apply resolutions from a change list
// and a path→decision map use this instead of open-coding the rule so
// the domain contract lives one hop from sync rather than in each UI.
// Returns nil when no row would emit.
func DriftResolutionsFromMap(changes []FileChange, decisions map[string]DriftDecision) []DriftResolution {
	var out []DriftResolution
	for _, ch := range changes {
		if ch.Kind != ChangeDrift {
			continue
		}
		if decisions[ch.Path] != DriftOverwrite {
			continue
		}
		out = append(out, DriftResolution{Path: ch.Path, Decision: DriftOverwrite})
	}
	return out
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

// translateStoreError converts a projectstore.ProjectPathOwnedError into
// the app-layer equivalent that carries human-facing profile and project
// names, so the TUI can render a specific conflict message. Any other
// error type is returned unchanged. The invariant itself lives inside
// projectstore.Store.Add/Update (see ADR 0017 Aggregate boundaries) —
// this method only translates for presentation.
func (s *Service) translateStoreError(err errs.DomainError, loaded *LoadedProfile) errs.DomainError {
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
func (s *Service) LoadProfiles() ([]*LoadedProfile, []errs.DomainError) {
	reg, err := s.Registry.Load()
	if err != nil {
		return nil, []errs.DomainError{err}
	}
	loaded := make([]*LoadedProfile, 0, len(reg.Profiles))
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
		loaded = append(loaded, &LoadedProfile{Profile: p, Projects: byID})
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
func (s *Service) resolveAsset(profileRef, assetID string) (*LoadedProfile, *asset.Asset, errs.DomainError) {
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
func (s *Service) resolveProject(profileRef, projectID string) (*LoadedProfile, *project.Manifest, errs.DomainError) {
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
// The returned CommitOutcome carries the short SHA on a successful
// commit, is zero-value when the commit path is a silent skip (feature
// disabled, dir not a repo, empty diff), and carries a typed domain
// error on a hard commit failure. A CommitOutcome.Err is never a save
// failure — the manifest is already on disk by then.
//
// Panics if manifest is nil: a nil pointer is a programmer bug per
// docs/guidelines/errors.md, not a recoverable not-found.
func (s *Service) UpdateAsset(profileRef string, manifest *asset.Manifest) (CommitOutcome, errs.DomainError) {
	if manifest == nil {
		panic("app.Service.UpdateAsset: nil manifest")
	}
	loaded, target, err := s.resolveAsset(profileRef, manifest.ID)
	if err != nil {
		return CommitOutcome{}, err
	}
	if saveErr := asset.SaveManifest(target.Dir, *manifest); saveErr != nil {
		return CommitOutcome{}, saveErr
	}
	pathspec := []string{"assets/" + manifest.ID + "/asset.json"}
	msg := fmt.Sprintf("chore(agentfiles): update asset %s manifest", manifest.ID)
	return s.runCommit(loaded.Profile.Root, pathspec, msg), nil
}

// SaveAssetFilesEdit persists the caller's manifest edits then records a
// files-scoped commit against `assets/<asset-id>/**` in the profile
// repo. It is the editor-return flow's counterpart to UpdateAsset: the
// files edit changed the on-disk content, and the single commit covers
// both the manifest and every file inside the asset directory. Same
// panic contract as UpdateAsset for a nil manifest.
func (s *Service) SaveAssetFilesEdit(profileRef string, manifest *asset.Manifest) (CommitOutcome, errs.DomainError) {
	if manifest == nil {
		panic("app.Service.SaveAssetFilesEdit: nil manifest")
	}
	loaded, target, err := s.resolveAsset(profileRef, manifest.ID)
	if err != nil {
		return CommitOutcome{}, err
	}
	if saveErr := asset.SaveManifest(target.Dir, *manifest); saveErr != nil {
		return CommitOutcome{}, saveErr
	}
	pathspec := []string{"assets/" + manifest.ID + "/**"}
	msg := fmt.Sprintf("chore(agentfiles): edit asset %s files", manifest.ID)
	return s.runCommit(loaded.Profile.Root, pathspec, msg), nil
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
