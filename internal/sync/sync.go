// Package sync reconciles the render plan with the target repository. It
// classifies every managed path as create/update/drift/delete/unknown and
// performs the writes plus managed-state snapshot on apply.
package sync

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/hexworks/agentfiles/internal/config"
	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/profile"
	"github.com/hexworks/agentfiles/internal/project"
	"github.com/hexworks/agentfiles/internal/render"
	"github.com/hexworks/agentfiles/internal/surfaces"
	"github.com/hexworks/agentfiles/internal/utils"
)

// GeneratorVersion is stamped into the managed state so future format changes
// can be detected and migrated. Bumped to 2.0.0 when state.json switched from
// map[string]string entries (bare hash) to the ManagedFileEntry object shape
// (v3) to carry the AssetID/SourceRel provenance Adopt needs. See ADR 0020.
const GeneratorVersion = "2.0.0"

// SchemaVersion is the integer schema version of the state.json document
// itself. It is distinct from GeneratorVersion: GeneratorVersion is the
// semver that tracks the ManagedFileEntry payload format (bare hash vs.
// object shape, see ADR 0020), whereas SchemaVersion is the envelope
// version the persistence boundary migrates and validates (decision C of
// task 0001 — state.json carries both).
const SchemaVersion = 1

// ManagedFileEntry is one row in ManagedState.ManagedFiles. Hash is the
// SHA-256 of the last-applied body; AssetID and SourceRel are the
// reverse-mapping keys Adopt uses to write the local edit back into
// <profile>/assets/<asset_type>/<asset_id>/<source_rel>. Legacy v2
// entries loaded from disk carry Hash only; AssetID and SourceRel are
// empty until the file is re-applied under v3.
type ManagedFileEntry struct {
	Hash      string `json:"hash"`
	AssetID   string `json:"asset_id,omitempty"`
	SourceRel string `json:"source_rel,omitempty"`
}

// HasAdoptProvenance reports whether the entry carries both reverse-mapping
// keys (v3 provenance) that Adopt needs to write a local edit back into the
// owning profile asset. Legacy v2 entries carry Hash only and return false.
// It is the single source of truth for "is this entry adopt-ready", shared by
// classifyDesired (to seed the drift row's provenance) and classifyDriftAdopt
// (to guard the reverse-write). See ADR 0020.
func (e ManagedFileEntry) HasAdoptProvenance() bool {
	return e.AssetID != "" && e.SourceRel != ""
}

// UnmarshalJSON accepts either a JSON string (v2 legacy: `"deadbeef…"`
// → {Hash: "deadbeef…"}) or the v3 object shape. Marshalling always
// writes the v3 object. Adopt is disabled for v2 entries until a
// re-apply repopulates AssetID/SourceRel.
func (e *ManagedFileEntry) UnmarshalJSON(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	// Try string first — v2 legacy.
	if data[0] == '"' {
		var hash string
		if err := json.Unmarshal(data, &hash); err != nil {
			return err
		}
		*e = ManagedFileEntry{Hash: hash}
		return nil
	}
	// v3 object shape. Alias to avoid infinite recursion.
	type alias ManagedFileEntry
	var raw alias
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*e = ManagedFileEntry(raw)
	return nil
}

// ManagedState is the persisted memory of the last successful apply.
// It lets the next preview tell the difference between:
//   - a normal update (desired output changed)
//   - drift (a previously managed file was edited locally)
//   - an unknown file (lives in a managed surface but we never tracked it)
type ManagedState struct {
	// Version is the state.json schema-envelope version migrated and
	// validated at the persistence boundary. Legacy state files predate it
	// and decode to 0 (the legacy sentinel), stamped up to SchemaVersion on
	// load. It sits alongside GeneratorVersion, which keeps its distinct
	// ManagedFileEntry-format meaning (decision C, task 0001).
	Version          int       `json:"version"`
	ProfileID        string    `json:"profile_id"`
	ProjectID        string    `json:"project_id"`
	GeneratorVersion string    `json:"generator_version"`
	LastAppliedAt    time.Time `json:"last_applied_at"`
	// ManagedFiles maps the repo-relative forward-slash path of a
	// managed file to its per-entry provenance (hash + asset id +
	// asset-relative source path). Adopt reads AssetID/SourceRel to
	// resolve the reverse-write target inside the profile folder.
	ManagedFiles map[string]ManagedFileEntry `json:"managed_files"`
	// IgnoredPaths lists repo-relative folder keys the user chose to ignore.
	// Any ChangeUnknown whose path sits under one of these is suppressed on
	// the next plan, so the folder vanishes from the changes preview. Stored
	// in the same forward-slash form as ManagedFiles keys.
	IgnoredPaths []string `json:"ignored_paths"`
}

// Migrate stamps a legacy (version 0) state file up to the current schema
// version. GeneratorVersion is untouched — it tracks the entry payload
// format, a separate concern. Pointer receiver so the stamp lands at the
// persistence boundary.
func (s *ManagedState) Migrate() errs.DomainError {
	if s.Version == 0 {
		s.Version = SchemaVersion
	}
	return nil
}

// Validate rejects a state file written by a newer build than this one
// understands (forward-compat guard). Per-entry key safety is enforced
// separately in loadState, which needs the file path for its corruption
// errors.
func (s *ManagedState) Validate() errs.DomainError {
	if s.Version > SchemaVersion {
		return errs.NewerSchemaVersionError{Have: s.Version, Known: SchemaVersion}
	}
	return nil
}

// ChangeKind classifies a single entry in a sync preview.
type ChangeKind string

// Possible ChangeKind values. Each corresponds to a different reconciliation
// decision between the render plan and the current repository state.
const (
	// ChangeCreate means the file is absent and will be written.
	ChangeCreate ChangeKind = "create"
	// ChangeUpdate means the file exists but was updated within agentfiles
	// the contents will change
	ChangeUpdate ChangeKind = "update"
	// ChangeDrift means a previously managed file was modified outside of agentfiles
	// apply would overwrite those edits unless the user keeps the file.
	ChangeDrift ChangeKind = "drift"
	// ChangeDelete marks a recognized managed file that is no longer part of
	// the desired plan and will be removed on apply.
	ChangeDelete ChangeKind = "delete"
	// ChangeUnknown marks an unrecognized file that lives inside a managed
	// surface but was never tracked in ManagedState. Apply leaves it alone
	// unless the user opts in to delete via UnknownDelete.
	ChangeUnknown ChangeKind = "unknown"
)

// ReasonKind is the domain-level classification of why a FileChange was
// emitted. The TUI maps each value to user-facing prose; the engine
// only ever sets and compares values from this list. Tests assert on
// ReasonKind constants, not on translated strings.
type ReasonKind string

// Possible ReasonKind values. Every FileChange carries exactly one.
const (
	// ReasonFirstApply marks a ChangeCreate emitted by the first-apply
	// clean-slate path (no ManagedState yet).
	ReasonFirstApply ReasonKind = "first_apply"
	// ReasonFileMissing marks a ChangeCreate emitted because the
	// desired path does not exist on disk.
	ReasonFileMissing ReasonKind = "file_missing"
	// ReasonContentDiffers marks a ChangeUpdate emitted because the
	// rendered body differs from the on-disk file.
	ReasonContentDiffers ReasonKind = "content_differs"
	// ReasonDriftDetected marks a ChangeDrift emitted because the
	// managed-state hash does not match the on-disk file.
	ReasonDriftDetected ReasonKind = "drift_detected"
	// ReasonStateRecordedDelete marks a ChangeDelete emitted for a
	// file recorded in ManagedState but missing from desired.
	ReasonStateRecordedDelete ReasonKind = "state_recorded_delete"
	// ReasonUnknown marks a ChangeUnknown emitted for a file found
	// inside a managed surface but absent from both desired and
	// ManagedState.
	ReasonUnknown ReasonKind = "unknown_in_surface"
)

// DriftDecision is the user's per-file choice for a ChangeDrift entry.
// The zero value (empty string) is identical to DriftKeep: leave the
// on-disk content and the prior baseline alone. Adopt is the third-way
// direction (repo → profile) added by ADR 0020.
type DriftDecision string

// Possible DriftDecision values.
const (
	// DriftKeep leaves the on-disk content untouched and preserves the
	// prior managed baseline. "No decision" (path absent from
	// Resolutions.Drift) is identical. See ADR 0015.
	DriftKeep DriftDecision = "keep"
	// DriftOverwrite writes the rendered body over the drifted file.
	DriftOverwrite DriftDecision = "overwrite"
	// DriftAdopt copies the local edit back into the profile asset it
	// came from — the single sanctioned repo → profile flow (ADR 0020).
	// The on-disk repo body is left as-is; a follow-up Plan sees the
	// profile catch up so the path no longer drifts.
	DriftAdopt DriftDecision = "adopt"
)

// UnknownDecision is the user's per-file choice for a ChangeUnknown
// entry. The zero value (empty string) means "no explicit choice,
// keep the stray file" and is what Apply assumes when a path is
// missing from the resolutions slice. Adopt is available only when
// the unknown file sits inside a known asset projection dir.
type UnknownDecision string

// Possible UnknownDecision values.
const (
	// UnknownKeep leaves the stray file untouched and does not
	// record it in ManagedState.
	UnknownKeep UnknownDecision = "keep"
	// UnknownDelete removes the file from disk.
	UnknownDelete UnknownDecision = "delete"
	// UnknownAdopt copies the stray file into the owning asset. Valid
	// only when the change row carries a populated OwningAssetID
	// (populated at Plan time for unknowns nested inside a known
	// asset projection dir). See ADR 0020.
	UnknownAdopt UnknownDecision = "adopt"
)

// DriftResolution pairs a drifted path with the user's per-file
// choice. Paths absent from the slice fall back to DriftKeep.
type DriftResolution struct {
	Path     string
	Decision DriftDecision
}

// UnknownResolution pairs an unknown path with the user's per-file
// choice. Paths absent from the slice fall back to UnknownKeep.
type UnknownResolution struct {
	Path     string
	Decision UnknownDecision
}

// Resolutions bundles everything the user decided for one Apply: the
// per-file drift and unknown choices plus the folder keys to ignore.
// Bundling keeps Apply's signature stable as new resolution kinds are
// added — each becomes a field instead of another positional parameter.
type Resolutions struct {
	Drift        []DriftResolution
	Unknown      []UnknownResolution
	IgnoredPaths []string
}

// FileChange is one human-facing diff entry shown in previews.
type FileChange struct {
	Path   string
	Kind   ChangeKind
	Reason ReasonKind
	// OwningAssetID is populated only for ChangeUnknown rows whose path
	// sits inside a known asset's rendered projection dir. When set,
	// the TUI may offer UnknownAdopt for the row and Apply resolves
	// SourceRel from the reverse-mapping table (ADR 0020).
	OwningAssetID string
	// AdoptProvenance carries the reverse-mapping keys for a ChangeDrift
	// row: the AssetID and SourceRel applyDrift needs to write the local
	// edit back into the owning profile asset. Populated only for
	// ChangeDrift rows whose state entry is v3 (both keys present); zero
	// for every other ChangeKind and for legacy v2 state entries.
	// Available() reports whether Adopt is a legal choice; the TUI degrades
	// a drift row to bilean when it is not, so users cannot pick a doomed
	// DriftAdopt. See ADR 0020 and the AdoptUnavailableError guard in
	// applyDrift.
	AdoptProvenance AdoptProvenance
}

// AdoptProvenance holds the reverse-mapping keys that make DriftAdopt legal
// for a drift row. A zero value (both keys empty) means Adopt is unavailable.
type AdoptProvenance struct {
	AssetID   string
	SourceRel string
}

// Available reports whether both reverse-mapping keys are present, i.e. the
// drift row may legally offer DriftAdopt.
func (p AdoptProvenance) Available() bool {
	return p.AssetID != "" && p.SourceRel != ""
}

// Preview is the bridge between render and apply.
//
// It combines:
//   - the desired files produced by render
//   - the computed change list (create/update/drift/delete/unknown)
//   - enough metadata to write a fresh managed state on apply
//   - a FirstApply flag so the TUI can surface clean-slate semantics
type Preview struct {
	ProjectPath string
	Files       []render.RenderedFile
	// Plan is the render plan Files were produced from. Apply reuses it
	// for ReverseLookup so the memoized reverse index is shared rather
	// than rebuilt from Files, and so a future ProjectPlan field
	// ReverseLookup comes to need is carried, not silently zeroed.
	Plan         *render.ProjectPlan
	Changes      []FileChange
	ManagedState *ManagedState
	ProfileID    string
	ProjectID    string
	// FirstApply is true when no .agentfiles/state.json existed at Plan
	// time. Every desired file is then classified as ChangeCreate with
	// ReasonFirstApply; the surface walk for deletes/unknowns is
	// skipped. The TUI uses this to show a clean-slate banner.
	FirstApply bool
}

// Plan compares the desired outputs with the current repository state. This is
// where create/update/drift/delete/unknown classification happens. Every
// failure mode (render leaves, hashing, state load, walk) is returned as an
// errs.DomainError so the TUI can render severity, icon, and color uniformly.
//
// First-apply policy: when no ManagedState exists for the project, every
// desired file is classified as ChangeCreate (overwriting any pre-existing
// file at that path) and no ChangeUnknown entries are emitted. The first
// successful Apply writes the initial state; subsequent plans then
// distinguish drift from unknown normally.
func Plan(p *profile.Profile, proj *project.Manifest) (*Preview, errs.DomainError) {
	rendered, renderErrs := render.Build(p, proj)
	if len(renderErrs) > 0 {
		return nil, errs.Errors(renderErrs)
	}
	state, stateErr := loadState(proj.Path)
	if stateErr != nil {
		// "never applied" is the common case; treat it as no prior state.
		if _, missing := stateErr.(StateMissingError); !missing {
			return nil, stateErr
		}
		state = nil
	}
	desired := map[string]string{}
	var changes []FileChange
	for _, file := range rendered.Files {
		desired[file.Path] = utils.HashBytes(file.Body)
		change, skip, classifyErr := classifyDesired(file, desired[file.Path], proj.Path, state)
		if classifyErr != nil {
			return nil, classifyErr
		}
		if skip {
			continue
		}
		changes = append(changes, change)
	}
	if state != nil {
		deletes, unknowns, detectErrs := detectDeletesAndUnknowns(proj.Path, desired, state)
		if len(detectErrs) > 0 {
			return nil, errs.Errors(detectErrs)
		}
		for _, pth := range deletes {
			changes = append(changes, FileChange{Path: pth, Kind: ChangeDelete, Reason: ReasonStateRecordedDelete})
		}
		for _, pth := range unknowns {
			// ReverseLookup delegates the repo→asset mapping to the render
			// strategy that produced the owning directory; sync no longer
			// re-derives it (task 0011). Only the owning asset id matters
			// here — the source-rel is resolved again at Apply time.
			match, _ := rendered.ReverseLookup(pth)
			changes = append(changes, FileChange{
				Path:          pth,
				Kind:          ChangeUnknown,
				Reason:        ReasonUnknown,
				OwningAssetID: match.AssetID,
			})
		}
	}
	slices.SortFunc(changes, func(a, b FileChange) int {
		return strings.Compare(a.Path, b.Path)
	})
	return &Preview{
		ProjectPath:  proj.Path,
		Files:        rendered.Files,
		Plan:         rendered,
		Changes:      changes,
		ManagedState: state,
		ProfileID:    p.Manifest.ID,
		ProjectID:    proj.ID,
		FirstApply:   state == nil,
	}, nil
}

// classifyDesired returns the FileChange (if any) for a single rendered
// file. The skip return value is true when the file matches desired
// exactly and no entry should be emitted. Splitting this out keeps Plan
// at one level of abstraction (orchestration) instead of mixing the
// per-file decision tree in.
func classifyDesired(file render.RenderedFile, desiredHash, projectPath string, state *ManagedState) (FileChange, bool, errs.DomainError) {
	if state == nil {
		return FileChange{Path: file.Path, Kind: ChangeCreate, Reason: ReasonFirstApply}, false, nil
	}
	abs := filepath.Join(projectPath, filepath.FromSlash(file.Path))
	// Reject a symlinked managed file: hashing follows the link, so a
	// classified drift would let an attacker exfiltrate an arbitrary
	// file through the Adopt reverse-write. Refuse at classify time so
	// no downstream branch (drift/update/adopt) sees the symlinked
	// entry. See task 0035 review issue #1.
	if info, err := os.Lstat(abs); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return FileChange{}, false, UnsafeSymlinkError{Path: abs}
	}
	if !utils.Exists(abs) {
		return FileChange{Path: file.Path, Kind: ChangeCreate, Reason: ReasonFileMissing}, false, nil
	}
	currentHash, hashErr := utils.HashFile(abs)
	if hashErr != nil {
		return FileChange{}, false, hashErr
	}
	if currentHash == desiredHash {
		return FileChange{}, true, nil
	}
	entry := state.ManagedFiles[file.Path]
	if entry.Hash != "" && entry.Hash != currentHash {
		change := FileChange{Path: file.Path, Kind: ChangeDrift, Reason: ReasonDriftDetected}
		if entry.HasAdoptProvenance() {
			change.AdoptProvenance = AdoptProvenance{AssetID: entry.AssetID, SourceRel: entry.SourceRel}
		}
		return change, false, nil
	}
	return FileChange{Path: file.Path, Kind: ChangeUpdate, Reason: ReasonContentDiffers}, false, nil
}

// ApplyResult is the by-mutation output of Apply. Mutated lists every
// managed file the engine created, updated, deleted, drift-overwrote,
// or unknown-deleted this call, in stable sort order. StatePath is the
// absolute path to the `.agentfiles/state.json` snapshot the engine
// rewrote after the file loop (always populated on success or partial
// success, even if no managed files changed). Callers compose the two
// into a pathspec — the split lets subject templates count real
// managed-file mutations without off-by-one arithmetic on a hardcoded
// state entry (see ADR 0019 commit trigger).
//
// AdoptRequests carries the Adopt classifications sync could not
// execute itself: the profile-side write and its commit live in
// app.Service.Apply so sync stays repo-only (ADR 0020). Empty when the
// caller did not request any Adopt resolutions.
type ApplyResult struct {
	Mutated       []string
	StatePath     string
	AdoptRequests []AdoptRequest
}

// AdoptRequest names one repo-relative file whose local content should
// replace the profile source it was rendered from. AssetID and
// SourceRel identify the target asset file:
// <profile>/assets/<asset.Type>/<AssetID>/<SourceRel>. Mode preserves
// the on-disk mode of the source file (rendered mode for drift, actual
// file mode for unknown) so the profile write is not silently
// normalized to 0o644.
type AdoptRequest struct {
	Path      string
	AssetID   string
	SourceRel string
	Mode      os.FileMode
}

// Apply materializes the preview into the repository using the user's
// per-file resolutions and then records a new ManagedState snapshot.
//
// Default decisions per ChangeKind:
//   - ChangeCreate, ChangeUpdate: always write (resolutions ignored).
//   - ChangeDelete: always remove (resolutions ignored; user already saw
//     the preview).
//   - ChangeDrift: DriftKeep by default; DriftOverwrite writes the body.
//     DriftKeep preserves the prior managed baseline so the path stays
//     classified as drift on subsequent plans (ADR 0015).
//   - ChangeUnknown: UnknownKeep by default; UnknownDelete removes the
//     file.
//
// All resolution paths are validated up front; invalid (absolute, OS-
// separated) paths produce InvalidPathError. Targets that fall outside
// surfaces.IsAllowed produce OutsideSurfaceError. Errors accumulate;
// state is still rewritten so the recorded baseline reflects whatever
// the apply loop actually wrote.
//
// The returned ApplyResult records what actually changed on disk so
// downstream steps (e.g. the ADR 0019 auto-commit) do not have to
// reconstruct the drift/unknown decision matrix a second time.
func Apply(preview *Preview, r Resolutions) (ApplyResult, errs.DomainError) {
	driftByPath, validationErrs := indexDriftResolutions(r.Drift)
	unknownByPath, unknownErrs := indexUnknownResolutions(r.Unknown)
	validationErrs = append(validationErrs, unknownErrs...)
	for _, p := range r.IgnoredPaths {
		if err := validatePathKey(p); err != nil {
			validationErrs = append(validationErrs, err)
		}
	}
	if len(validationErrs) > 0 {
		return ApplyResult{}, errs.Errors(validationErrs)
	}
	loop := newApplyLoop(preview, driftByPath, unknownByPath)
	for _, change := range preview.Changes {
		if !surfaces.IsAllowed(change.Path) {
			loop.domainErrs = append(loop.domainErrs, OutsideSurfaceError{Path: change.Path})
			continue
		}
		switch change.Kind {
		case ChangeCreate, ChangeUpdate:
			loop.applyCreateUpdate(change)
		case ChangeDrift:
			loop.applyDrift(change)
		case ChangeDelete:
			loop.applyDelete(change)
		case ChangeUnknown:
			loop.applyUnknown(change)
		}
	}
	ignoredNormalized := normalizeIgnoredPaths(r.IgnoredPaths)
	// Preserve the prior LastAppliedAt when nothing effectively
	// changed: no managed-file mutations, no adopt requests, the
	// recorded baseline map matches the prior one, and the ignored set
	// matches. That leaves state.json byte-identical on disk so the
	// git commit's empty-diff path skips naturally, avoiding a spurious
	// project-repo commit whose only diff would be the timestamp bump.
	// See task 0035 review issue #11.
	lastAppliedAt := time.Now().UTC()
	if len(loop.mutated) == 0 && len(loop.adoptRequests) == 0 && preview.ManagedState != nil &&
		managedFilesEqual(loop.recordedHashes, preview.ManagedState.ManagedFiles) &&
		slices.Equal(ignoredNormalized, preview.ManagedState.IgnoredPaths) {
		lastAppliedAt = preview.ManagedState.LastAppliedAt
	}
	state := &ManagedState{
		ProfileID:        preview.ProfileID,
		ProjectID:        preview.ProjectID,
		GeneratorVersion: GeneratorVersion,
		LastAppliedAt:    lastAppliedAt,
		ManagedFiles:     loop.recordedHashes,
		IgnoredPaths:     ignoredNormalized,
	}
	statePath := filepath.Join(preview.ProjectPath, config.StateDirName, config.StateFileName)
	if err := utils.WriteJSON(statePath, *state); err != nil {
		loop.domainErrs = append(loop.domainErrs, err)
	}
	slices.Sort(loop.mutated)
	slices.SortFunc(loop.adoptRequests, func(a, b AdoptRequest) int {
		return strings.Compare(a.Path, b.Path)
	})
	result := ApplyResult{Mutated: loop.mutated, StatePath: statePath, AdoptRequests: loop.adoptRequests}
	if len(loop.domainErrs) == 0 {
		return result, nil
	}
	if len(loop.domainErrs) == 1 {
		return result, loop.domainErrs[0]
	}
	return result, errs.Errors(loop.domainErrs)
}

// applyLoop holds the per-Apply mutable state so the per-ChangeKind
// helpers can be plain methods instead of closures. Splitting the
// dispatch this way lets each branch sit at one level of abstraction —
// the outer switch reads as pure dispatch, each helper owns the
// classification rules for its kind (see task 0035 review issue #7).
type applyLoop struct {
	preview        *Preview
	driftByPath    map[string]DriftDecision
	unknownByPath  map[string]UnknownDecision
	bodiesByPath   map[string]render.RenderedFile
	recordedHashes map[string]ManagedFileEntry
	plan           *render.ProjectPlan
	mutated        []string
	adoptRequests  []AdoptRequest
	domainErrs     []errs.DomainError
}

func newApplyLoop(preview *Preview, drift map[string]DriftDecision, unknown map[string]UnknownDecision) *applyLoop {
	bodies := make(map[string]render.RenderedFile, len(preview.Files))
	// Pre-fill the state baseline with the rendered hash and provenance
	// (AssetID + SourceRel) of every desired file. This is the correct
	// baseline for clean, created, updated, and overwritten paths; only
	// the ChangeDrift branch overrides it with the prior baseline (ADR
	// 0015).
	recorded := make(map[string]ManagedFileEntry, len(preview.Files))
	for _, f := range preview.Files {
		bodies[f.Path] = f
		recorded[f.Path] = ManagedFileEntry{
			Hash:      utils.HashBytes(f.Body),
			AssetID:   f.AssetID,
			SourceRel: f.SourceRel,
		}
	}
	// Reuse the render plan threaded through Preview so ReverseLookup
	// shares its memoized reverse index. Fall back to a Files-only
	// reconstruction for a hand-built Preview that carries no plan.
	plan := preview.Plan
	if plan == nil {
		plan = &render.ProjectPlan{Files: preview.Files}
	}
	return &applyLoop{
		preview:        preview,
		driftByPath:    drift,
		unknownByPath:  unknown,
		bodiesByPath:   bodies,
		recordedHashes: recorded,
		plan:           plan,
	}
}

// track records rel as an absolute pathspec entry for the sync commit.
func (a *applyLoop) track(rel string) {
	a.mutated = append(a.mutated, filepath.Join(a.preview.ProjectPath, filepath.FromSlash(rel)))
}

// preserveDriftBaseline keeps the prior baseline hash for path so a
// kept drift stays classified as drift on the next plan (ADR 0015),
// while still upgrading legacy v2 provenance from the freshly rendered
// plan. Callers reach for it from every branch that leaves the on-disk
// file unchanged.
func (a *applyLoop) preserveDriftBaseline(path string) {
	if a.preview.ManagedState == nil {
		a.domainErrs = append(a.domainErrs, PreviewInvariantError{
			Kind:   "ChangeDrift",
			Reason: "ManagedState nil",
		})
		delete(a.recordedHashes, path)
		return
	}
	prior := a.preview.ManagedState.ManagedFiles[path]
	if prior.Hash == "" {
		delete(a.recordedHashes, path)
		return
	}
	fresh := a.recordedHashes[path]
	a.recordedHashes[path] = ManagedFileEntry{
		Hash:      prior.Hash,
		AssetID:   fresh.AssetID,
		SourceRel: fresh.SourceRel,
	}
}

func (a *applyLoop) applyCreateUpdate(change FileChange) {
	if err := writeRendered(a.preview.ProjectPath, a.bodiesByPath[change.Path]); err != nil {
		a.domainErrs = append(a.domainErrs, err)
		return
	}
	a.track(change.Path)
}

func (a *applyLoop) applyDrift(change FileChange) {
	switch a.driftByPath[change.Path] {
	case DriftOverwrite:
		if err := writeRendered(a.preview.ProjectPath, a.bodiesByPath[change.Path]); err != nil {
			a.domainErrs = append(a.domainErrs, err)
			return
		}
		a.track(change.Path)
	case DriftAdopt:
		a.classifyDriftAdopt(change)
	default:
		// DriftKeep (or unrecognized): preserve prior baseline so the
		// path stays classified as drift on the next plan (ADR 0015).
		a.preserveDriftBaseline(change.Path)
	}
}

// classifyDriftAdopt turns a DriftAdopt resolution into an AdoptRequest
// after cross-checking the state-recorded reverse-mapping keys against
// the freshly rendered plan. Without the cross-check a tampered
// state.json entry could redirect the profile-side write into an
// unrelated asset (task 0035 review issue #2).
func (a *applyLoop) classifyDriftAdopt(change FileChange) {
	if a.preview.ManagedState == nil {
		a.domainErrs = append(a.domainErrs, PreviewInvariantError{
			Kind:   "ChangeDrift",
			Reason: "ManagedState nil",
		})
		delete(a.recordedHashes, change.Path)
		return
	}
	prior := a.preview.ManagedState.ManagedFiles[change.Path]
	if !prior.HasAdoptProvenance() {
		a.domainErrs = append(a.domainErrs, AdoptUnavailableError{
			Path:   change.Path,
			Reason: "legacy v2 state entry missing asset provenance",
		})
		a.preserveDriftBaseline(change.Path)
		return
	}
	rendered, ok := a.bodiesByPath[change.Path]
	if !ok || rendered.AssetID != prior.AssetID || rendered.SourceRel != prior.SourceRel {
		a.domainErrs = append(a.domainErrs, AdoptUnavailableError{
			Path:   change.Path,
			Reason: "state provenance stale, re-plan",
		})
		a.preserveDriftBaseline(change.Path)
		return
	}
	a.adoptRequests = append(a.adoptRequests, AdoptRequest{
		Path:      change.Path,
		AssetID:   prior.AssetID,
		SourceRel: prior.SourceRel,
		Mode:      rendered.Mode,
	})
	a.preserveDriftBaseline(change.Path)
}

func (a *applyLoop) applyDelete(change FileChange) {
	if err := removeFile(a.preview.ProjectPath, change.Path); err != nil {
		a.domainErrs = append(a.domainErrs, err)
		return
	}
	a.track(change.Path)
	// Deleted paths are never in preview.Files, so the pre-filled map
	// already excludes them.
}

func (a *applyLoop) applyUnknown(change FileChange) {
	switch a.unknownByPath[change.Path] {
	case UnknownDelete:
		if err := removeFile(a.preview.ProjectPath, change.Path); err != nil {
			a.domainErrs = append(a.domainErrs, err)
			return
		}
		a.track(change.Path)
	case UnknownAdopt:
		a.classifyUnknownAdopt(change)
	}
	// UnknownKeep (default) and unresolved Adopt: nothing else to
	// record on state.
}

func (a *applyLoop) classifyUnknownAdopt(change FileChange) {
	if change.OwningAssetID == "" {
		a.domainErrs = append(a.domainErrs, AdoptUnavailableError{
			Path:   change.Path,
			Reason: "unknown file has no owning asset",
		})
		return
	}
	match, ok := a.plan.ReverseLookup(change.Path)
	if !ok || match.AssetID == "" || match.SourceRel == "" {
		a.domainErrs = append(a.domainErrs, AdoptUnavailableError{
			Path:   change.Path,
			Reason: "reverse-mapping failed",
		})
		return
	}
	a.adoptRequests = append(a.adoptRequests, AdoptRequest{
		Path:      change.Path,
		AssetID:   match.AssetID,
		SourceRel: match.SourceRel,
		Mode:      unknownAdoptMode(a.preview.ProjectPath, change.Path),
	})
}

// managedFilesEqual reports whether two ManagedFileEntry maps are
// element-wise identical. Used to detect a no-op Apply so state.json
// stays byte-identical on disk (task 0035 review issue #11).
func managedFilesEqual(a, b map[string]ManagedFileEntry) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		bv, ok := b[k]
		if !ok || bv != v {
			return false
		}
	}
	return true
}

// normalizeIgnoredPaths writes the incoming ignored set verbatim (replace, not
// merge): the TUI now sees the full persisted set and sends the complete
// desired set on every Apply, so un-ignoring a folder must be able to drop it.
// The result is deduplicated and sorted for a stable on-disk form. Returns nil
// when the input is empty; with no omitempty tag that serializes as
// "ignored_paths": null, matching managed_files' treatment of an empty map.
func normalizeIgnoredPaths(selected []string) []string {
	if len(selected) == 0 {
		return nil
	}
	return utils.DeduplicateAndSort(selected)
}

// indexDriftResolutions validates each entry's path and folds the
// slice into a path → decision lookup. Duplicate paths: last entry
// wins. Validation accumulates so all bad paths report at once.
func indexDriftResolutions(resolutions []DriftResolution) (map[string]DriftDecision, []errs.DomainError) {
	out := map[string]DriftDecision{}
	var errsOut []errs.DomainError
	for _, r := range resolutions {
		if err := validatePathKey(r.Path); err != nil {
			errsOut = append(errsOut, err)
			continue
		}
		out[r.Path] = r.Decision
	}
	return out, errsOut
}

// indexUnknownResolutions validates each entry's path and folds the
// slice into a path → decision lookup. Duplicate paths: last entry
// wins.
func indexUnknownResolutions(resolutions []UnknownResolution) (map[string]UnknownDecision, []errs.DomainError) {
	out := map[string]UnknownDecision{}
	var errsOut []errs.DomainError
	for _, r := range resolutions {
		if err := validatePathKey(r.Path); err != nil {
			errsOut = append(errsOut, err)
			continue
		}
		out[r.Path] = r.Decision
	}
	return out, errsOut
}

// validatePathKey rejects paths that do not fit the FileChange.Path
// convention: forward-slash relative keys only. Absolute paths and
// paths containing an OS separator different from "/" are refused.
// Paths containing ".." segments after Clean are also refused so they
// cannot escape the project root through filepath.Join.
func validatePathKey(path string) errs.DomainError {
	if path == "" {
		return InvalidPathError{Path: path, Reason: "empty path"}
	}
	if filepath.IsAbs(path) {
		return InvalidPathError{Path: path, Reason: "must be relative"}
	}
	if filepath.Separator != '/' && strings.ContainsRune(path, filepath.Separator) {
		return InvalidPathError{Path: path, Reason: "must use forward slashes"}
	}
	cleaned := filepath.ToSlash(filepath.Clean(path))
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return InvalidPathError{Path: path, Reason: "escapes project root"}
	}
	return nil
}

// ValidatePathKey exposes the FileChange path-key guard to callers outside
// the sync package (e.g. app.DiffFile) that join a slash key with a project
// root and need the same boundary check against absolute paths, a wrong OS
// separator, and ".." escapes before reaching filepath.Join.
func ValidatePathKey(path string) errs.DomainError {
	return validatePathKey(path)
}

// writeRendered writes a single RenderedFile to the project, preserving its
// mode. The path is resolved relative to projectPath. If the target
// exists and is a symbolic link the write is refused with
// UnsafeSymlinkError; the open also passes O_NOFOLLOW so a final-
// segment race between Lstat and open is rejected by the kernel.
func writeRendered(projectPath string, file render.RenderedFile) errs.DomainError {
	if err := validatePathKey(file.Path); err != nil {
		return err
	}
	abs := filepath.Join(projectPath, filepath.FromSlash(file.Path))
	if info, err := os.Lstat(abs); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return UnsafeSymlinkError{Path: abs}
	}
	if dirErr := utils.EnsureDir(filepath.Dir(abs)); dirErr != nil {
		return dirErr
	}
	f, err := os.OpenFile(abs, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|safeWriteFlags, file.Mode)
	if err != nil {
		return utils.WriteFileError{Path: abs, Err: err}
	}
	defer f.Close()
	if _, err := f.Write(file.Body); err != nil {
		return utils.WriteFileError{Path: abs, Err: err}
	}
	return nil
}

// removeFile deletes path inside projectPath. Missing files are tolerated so
// repeated applies stay idempotent. The path must already have passed
// validatePathKey via the caller's Change list.
func removeFile(projectPath, path string) errs.DomainError {
	abs := filepath.Join(projectPath, filepath.FromSlash(path))
	if err := os.Remove(abs); err != nil && !os.IsNotExist(err) {
		return DeleteError{Path: abs, Err: err}
	}
	return nil
}

// loadState reads the previous managed snapshot from the target repository.
// A missing snapshot is reported as StateMissingError (info-severity) so
// callers can distinguish first-time applies from corrupt state files.
// Keys in ManagedFiles are validated against the slash-key convention;
// any unsafe key returns StateCorruptError so traversal through the
// state file cannot trigger a write or delete outside the project root.
func loadState(projectPath string) (*ManagedState, errs.DomainError) {
	pth := filepath.Join(projectPath, config.StateDirName, config.StateFileName)
	if !utils.Exists(pth) {
		return nil, StateMissingError{Path: pth}
	}
	state, err := utils.ReadJSON[ManagedState](pth)
	if err != nil {
		return nil, err
	}
	if state.ManagedFiles == nil {
		state.ManagedFiles = map[string]ManagedFileEntry{}
	}
	for key, entry := range state.ManagedFiles {
		if err := validatePathKey(key); err != nil {
			return nil, StateCorruptError{Path: pth, Key: key}
		}
		trimmedAsset := strings.TrimSpace(entry.AssetID)
		trimmedSource := strings.TrimSpace(entry.SourceRel)
		// A half-populated v3 entry (only one of AssetID/SourceRel set)
		// is a wiring bug: v2 entries carry neither, v3 entries carry
		// both. Whitespace-only counts as empty for the same reason.
		if (trimmedAsset == "") != (trimmedSource == "") {
			return nil, StateCorruptError{Path: pth, Key: key}
		}
		if trimmedAsset != entry.AssetID || trimmedSource != entry.SourceRel {
			return nil, StateCorruptError{Path: pth, Key: key}
		}
		// SourceRel is written into <profile>/assets/<type>/<AssetID>/<SourceRel>
		// by Adopt. Validate up front with the same forward-slash
		// relative-key rule that scopes every other on-disk path so a
		// tampered "../evil" can never reach asset.ResolveRelative.
		if entry.SourceRel != "" {
			if err := validatePathKey(entry.SourceRel); err != nil {
				return nil, StateCorruptError{Path: pth, Key: key}
			}
		}
	}
	for _, key := range state.IgnoredPaths {
		if err := validatePathKey(key); err != nil {
			return nil, StateCorruptError{Path: pth, Key: key}
		}
	}
	return &state, nil
}

// detectDeletesAndUnknowns splits non-desired files inside managed
// surfaces into two buckets:
//   - deletes: files recorded in state.ManagedFiles that are no longer
//     in desired. These will be auto-removed on apply.
//   - unknowns: files inside surfaces.Roots() that are neither in
//     desired nor in state.ManagedFiles. These require an explicit
//     UnknownDelete resolution to actually be removed.
//
// Callers must guarantee state != nil; first-apply skips this pass
// entirely. Symbolic links are never followed: if a surface root is a
// symlink a SurfaceSymlinkError is emitted and that root is skipped;
// symlinked entries inside a root are similarly skipped so the user
// cannot inadvertently delete cross-boundary files via UnknownDelete.
func detectDeletesAndUnknowns(projectPath string, desired map[string]string, state *ManagedState) ([]string, []string, []errs.DomainError) {
	deleteSet := map[string]bool{}
	for pth := range state.ManagedFiles {
		if desired[pth] == "" {
			deleteSet[pth] = true
		}
	}
	unknownSet := map[string]bool{}
	var domainErrs []errs.DomainError
	// .agentfiles is intentionally outside surfaces.Roots(), so the walk
	// below never enters the managed-state directory; no skip check needed.
	for _, root := range surfaces.Roots() {
		abs := filepath.Join(projectPath, root)
		info, err := os.Lstat(abs)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			domainErrs = append(domainErrs, StatError{Path: abs, Err: err})
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			domainErrs = append(domainErrs, SurfaceSymlinkError{Root: abs})
			continue
		}
		if !info.IsDir() {
			rel := utils.ToRelative(projectPath, abs)
			classifyDeleteOrUnknown(rel, desired, state, unknownSet)
			continue
		}
		walkErr := filepath.WalkDir(abs, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			entryInfo, infoErr := d.Info()
			if infoErr != nil {
				domainErrs = append(domainErrs, StatError{Path: path, Err: infoErr})
				return nil
			}
			if entryInfo.Mode()&os.ModeSymlink != 0 {
				// Skip symlinked entries silently; deleting them would
				// follow the link out of the managed surface.
				return nil
			}
			rel := utils.ToRelative(projectPath, path)
			classifyDeleteOrUnknown(rel, desired, state, unknownSet)
			return nil
		})
		if walkErr != nil {
			domainErrs = append(domainErrs, SurfaceWalkError{Root: abs, Err: walkErr})
		}
	}
	deletes := setToSortedSlice(deleteSet)
	unknowns := setToSortedSlice(unknownSet)
	return deletes, unknowns, domainErrs
}

// classifyDeleteOrUnknown routes a single file path discovered during the
// surface walk into the unknowns bucket if it is neither desired nor
// previously managed. State-recorded files missing from desired are
// handled by the caller's pre-walk pass over state.ManagedFiles.
func classifyDeleteOrUnknown(rel string, desired map[string]string, state *ManagedState, unknowns map[string]bool) {
	if desired[rel] != "" {
		return
	}
	if state.ManagedFiles[rel].Hash != "" {
		return
	}
	if isUnderIgnored(rel, state.IgnoredPaths) {
		return
	}
	unknowns[rel] = true
}

// isUnderIgnored suppresses ChangeUnknown entries for folders the user
// chose to ignore. The ig+"/" boundary stops an ignored key from matching
// a sibling whose name it is merely a string prefix of (".codex/ig" must
// not swallow ".codex/ignore-me").
func isUnderIgnored(rel string, ignored []string) bool {
	for _, ig := range ignored {
		if rel == ig || strings.HasPrefix(rel, ig+"/") {
			return true
		}
	}
	return false
}

// unknownAdoptMode returns the on-disk mode of the repo-relative file
// so an UnknownAdopt reverse-write does not silently downgrade an
// executable bit. Falls back to 0o644 when the stat fails; the
// subsequent read/write pair inside app.Service will surface any real
// I/O failure as AdoptReadError.
func unknownAdoptMode(projectPath, rel string) os.FileMode {
	info, err := os.Stat(filepath.Join(projectPath, filepath.FromSlash(rel)))
	if err != nil {
		return 0o644
	}
	return info.Mode().Perm()
}

func setToSortedSlice(set map[string]bool) []string {
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for path := range set {
		out = append(out, path)
	}
	slices.Sort(out)
	return out
}
