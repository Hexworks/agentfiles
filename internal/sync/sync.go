// Package sync reconciles the render plan with the target repository. It
// classifies every managed path as create/update/drift/delete/unknown and
// performs the writes plus managed-state snapshot on apply.
package sync

import (
	"encoding/json"
	"os"
	pathpkg "path"
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
}

// Preview is the bridge between render and apply.
//
// It combines:
//   - the desired files produced by render
//   - the computed change list (create/update/drift/delete/unknown)
//   - enough metadata to write a fresh managed state on apply
//   - a FirstApply flag so the TUI can surface clean-slate semantics
type Preview struct {
	ProjectPath  string
	Files        []render.RenderedFile
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
	assetDirs := assetProjectionDirs(rendered.Files)
	if state != nil {
		deletes, unknowns, detectErrs := detectDeletesAndUnknowns(proj.Path, desired, state)
		if len(detectErrs) > 0 {
			return nil, errs.Errors(detectErrs)
		}
		for _, pth := range deletes {
			changes = append(changes, FileChange{Path: pth, Kind: ChangeDelete, Reason: ReasonStateRecordedDelete})
		}
		for _, pth := range unknowns {
			changes = append(changes, FileChange{
				Path:          pth,
				Kind:          ChangeUnknown,
				Reason:        ReasonUnknown,
				OwningAssetID: owningAssetIDFor(pth, assetDirs),
			})
		}
	}
	slices.SortFunc(changes, func(a, b FileChange) int {
		return strings.Compare(a.Path, b.Path)
	})
	return &Preview{
		ProjectPath:  proj.Path,
		Files:        rendered.Files,
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
	baseline := state.ManagedFiles[file.Path].Hash
	if baseline != "" && baseline != currentHash {
		return FileChange{Path: file.Path, Kind: ChangeDrift, Reason: ReasonDriftDetected}, false, nil
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
// <profile>/assets/<asset.Type>/<AssetID>/<SourceRel>.
type AdoptRequest struct {
	Path      string
	AssetID   string
	SourceRel string
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
	bodiesByPath := map[string]render.RenderedFile{}
	// Pre-fill the state baseline with the rendered hash and provenance
	// (AssetID + SourceRel) of every desired file. This is the correct
	// baseline for clean, created, updated, and overwritten paths; only
	// the ChangeDrift branch below overrides it with the prior baseline
	// (ADR 0015).
	recordedHashes := map[string]ManagedFileEntry{}
	for _, f := range preview.Files {
		bodiesByPath[f.Path] = f
		recordedHashes[f.Path] = ManagedFileEntry{
			Hash:      utils.HashBytes(f.Body),
			AssetID:   f.AssetID,
			SourceRel: f.SourceRel,
		}
	}
	var (
		domainErrs    []errs.DomainError
		mutated       []string
		adoptRequests []AdoptRequest
	)
	track := func(rel string) {
		mutated = append(mutated, filepath.Join(preview.ProjectPath, filepath.FromSlash(rel)))
	}
	// Compute the reverse-mapping table from the rendered plan so
	// unknown-Adopt can resolve owning asset id + source_rel without
	// re-walking assets. Cheap: one pass over preview.Files.
	assetDirs := assetProjectionDirs(preview.Files)
	preserveDriftBaseline := func(path string) {
		if preview.ManagedState == nil {
			domainErrs = append(domainErrs, PreviewInvariantError{
				Kind:   "ChangeDrift",
				Reason: "ManagedState nil",
			})
			delete(recordedHashes, path)
			return
		}
		prior := preview.ManagedState.ManagedFiles[path]
		if prior.Hash == "" {
			delete(recordedHashes, path)
			return
		}
		fresh := recordedHashes[path]
		recordedHashes[path] = ManagedFileEntry{
			Hash:      prior.Hash,
			AssetID:   fresh.AssetID,
			SourceRel: fresh.SourceRel,
		}
	}
	for _, change := range preview.Changes {
		if !surfaces.IsAllowed(change.Path) {
			domainErrs = append(domainErrs, OutsideSurfaceError{Path: change.Path})
			continue
		}
		switch change.Kind {
		case ChangeCreate, ChangeUpdate:
			if err := writeRendered(preview.ProjectPath, bodiesByPath[change.Path]); err != nil {
				domainErrs = append(domainErrs, err)
				continue
			}
			track(change.Path)
			// Hash already pre-filled with rendered body.
		case ChangeDrift:
			switch driftByPath[change.Path] {
			case DriftOverwrite:
				if err := writeRendered(preview.ProjectPath, bodiesByPath[change.Path]); err != nil {
					domainErrs = append(domainErrs, err)
					continue
				}
				track(change.Path)
				// Pre-filled rendered hash is correct after overwrite.
			case DriftAdopt:
				// Adopt: the local body is authoritative. Look up the
				// reverse-mapping keys from the prior baseline; a v2
				// legacy entry (Hash only) means Adopt cannot resolve
				// the profile-side target, so we surface a typed error
				// and fall through to the preserve-baseline branch so
				// the row stays classified as drift on the next plan.
				if preview.ManagedState == nil {
					domainErrs = append(domainErrs, PreviewInvariantError{
						Kind:   "ChangeDrift",
						Reason: "ManagedState nil",
					})
					delete(recordedHashes, change.Path)
					continue
				}
				prior := preview.ManagedState.ManagedFiles[change.Path]
				if prior.AssetID == "" || prior.SourceRel == "" {
					domainErrs = append(domainErrs, AdoptUnavailableError{
						Path:   change.Path,
						Reason: "legacy v2 state entry missing asset provenance",
					})
					preserveDriftBaseline(change.Path)
					continue
				}
				adoptRequests = append(adoptRequests, AdoptRequest{
					Path:      change.Path,
					AssetID:   prior.AssetID,
					SourceRel: prior.SourceRel,
				})
				preserveDriftBaseline(change.Path)
			default:
				// DriftKeep (or unrecognized): preserve prior baseline
				// so the path stays classified as drift on the next
				// plan (ADR 0015).
				preserveDriftBaseline(change.Path)
			}
		case ChangeDelete:
			if err := removeFile(preview.ProjectPath, change.Path); err != nil {
				domainErrs = append(domainErrs, err)
				continue
			}
			track(change.Path)
			// Deleted paths are never in preview.Files, so the
			// pre-filled map already excludes them.
		case ChangeUnknown:
			switch unknownByPath[change.Path] {
			case UnknownDelete:
				if err := removeFile(preview.ProjectPath, change.Path); err != nil {
					domainErrs = append(domainErrs, err)
					continue
				}
				track(change.Path)
			case UnknownAdopt:
				if change.OwningAssetID == "" {
					domainErrs = append(domainErrs, AdoptUnavailableError{
						Path:   change.Path,
						Reason: "unknown file has no owning asset",
					})
					continue
				}
				assetID, sourceRel, ok := owningAssetSourceRelFor(change.Path, assetDirs)
				if !ok || assetID == "" || sourceRel == "" {
					domainErrs = append(domainErrs, AdoptUnavailableError{
						Path:   change.Path,
						Reason: "reverse-mapping failed",
					})
					continue
				}
				adoptRequests = append(adoptRequests, AdoptRequest{
					Path:      change.Path,
					AssetID:   assetID,
					SourceRel: sourceRel,
				})
			}
			// UnknownKeep (default) and unresolved Adopt: nothing else
			// to record on state.
		}
	}
	state := &ManagedState{
		ProfileID:        preview.ProfileID,
		ProjectID:        preview.ProjectID,
		GeneratorVersion: GeneratorVersion,
		LastAppliedAt:    time.Now().UTC(),
		ManagedFiles:     recordedHashes,
		IgnoredPaths:     normalizeIgnoredPaths(r.IgnoredPaths),
	}
	statePath := filepath.Join(preview.ProjectPath, config.StateDirName, config.StateFileName)
	if err := utils.WriteJSON(statePath, state); err != nil {
		domainErrs = append(domainErrs, err)
	}
	slices.Sort(mutated)
	slices.SortFunc(adoptRequests, func(a, b AdoptRequest) int {
		return strings.Compare(a.Path, b.Path)
	})
	result := ApplyResult{Mutated: mutated, StatePath: statePath, AdoptRequests: adoptRequests}
	if len(domainErrs) == 0 {
		return result, nil
	}
	if len(domainErrs) == 1 {
		return result, domainErrs[0]
	}
	return result, errs.Errors(domainErrs)
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
	var state ManagedState
	if err := utils.ReadJSON(pth, &state); err != nil {
		return nil, err
	}
	if state.ManagedFiles == nil {
		state.ManagedFiles = map[string]ManagedFileEntry{}
	}
	for key, entry := range state.ManagedFiles {
		if err := validatePathKey(key); err != nil {
			return nil, StateCorruptError{Path: pth, Key: key}
		}
		// A half-populated v3 entry (only one of AssetID/SourceRel set)
		// is a wiring bug: v2 entries carry neither, v3 entries carry
		// both. Surface it loudly rather than silently disable Adopt.
		if (entry.AssetID == "") != (entry.SourceRel == "") {
			return nil, StateCorruptError{Path: pth, Key: key}
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

// assetDirEntry records one directory in the rendered layout together
// with the asset that owns it and the projection-source root that maps
// the rendered dir back to <asset.Dir>/<projectionSource>.
type assetDirEntry struct {
	AssetID          string
	ProjectionSource string
	ProjectionTarget string
}

// assetProjectionDirs walks the rendered plan and returns a map keyed
// by the rendered directory each rendered file sits in, valued by the
// owning asset id + the asset-relative source root that produced the
// dir. Directories hosting rendered files from more than one asset are
// dropped as ambiguous (no owner, so Adopt is not offered).
//
// The source root is derived from the pair (RenderedFile.Path,
// RenderedFile.SourceRel): stripping the common suffix gives the
// mapping "rendered dir → asset-relative source dir". Unknown files
// inside the same rendered dir map back to the same source dir with
// their tail preserved.
func assetProjectionDirs(files []render.RenderedFile) map[string]assetDirEntry {
	type tally struct {
		AssetID      string
		SourceRoot   string
		TargetRoot   string
		Ambiguous    bool
		AssetIDCount int
	}
	tallies := map[string]*tally{}
	for _, f := range files {
		if f.AssetID == "" || f.SourceRel == "" {
			continue
		}
		targetDir := pathpkg.Dir(f.Path)
		sourceDir := pathpkg.Dir(filepath.ToSlash(f.SourceRel))
		// Walk up until the target dir's tail no longer matches the
		// source dir's tail; the shared root remainder is the
		// projection root the render pipeline used.
		targetRoot, sourceRoot := stripCommonSuffix(targetDir, sourceDir)
		key := targetDir
		existing, ok := tallies[key]
		if !ok {
			tallies[key] = &tally{
				AssetID:    f.AssetID,
				SourceRoot: sourceRoot,
				TargetRoot: targetRoot,
			}
			continue
		}
		if existing.AssetID != f.AssetID || existing.SourceRoot != sourceRoot {
			existing.Ambiguous = true
		}
	}
	out := make(map[string]assetDirEntry, len(tallies))
	for k, t := range tallies {
		if t.Ambiguous {
			continue
		}
		out[k] = assetDirEntry{
			AssetID:          t.AssetID,
			ProjectionSource: t.SourceRoot,
			ProjectionTarget: t.TargetRoot,
		}
	}
	return out
}

// stripCommonSuffix walks target and source backwards while segments
// match and returns the two root prefixes that remain. Both inputs
// are forward-slash paths. Empty strings map to ".".
func stripCommonSuffix(target, source string) (string, string) {
	tParts := splitSlash(target)
	sParts := splitSlash(source)
	i := len(tParts)
	j := len(sParts)
	for i > 0 && j > 0 && tParts[i-1] == sParts[j-1] {
		i--
		j--
	}
	return joinSlash(tParts[:i]), joinSlash(sParts[:j])
}

func splitSlash(p string) []string {
	if p == "" || p == "." {
		return nil
	}
	return strings.Split(p, "/")
}

func joinSlash(parts []string) string {
	if len(parts) == 0 {
		return "."
	}
	return strings.Join(parts, "/")
}

// owningAssetIDFor returns the AssetID owning the rendered directory
// that hosts unknownPath, or "" when no known asset's projection dir
// contains it. Walks parent dirs so an unknown at
// .claude/skills/foo/example-3.md finds the entry at
// .claude/skills/foo.
func owningAssetIDFor(unknownPath string, assetDirs map[string]assetDirEntry) string {
	dir := pathpkg.Dir(unknownPath)
	for dir != "." && dir != "/" {
		if entry, ok := assetDirs[dir]; ok {
			return entry.AssetID
		}
		if surfaces.IsAssetContainerRoot(dir) {
			return ""
		}
		next := pathpkg.Dir(dir)
		if next == dir {
			return ""
		}
		dir = next
	}
	return ""
}

// owningAssetSourceRelFor returns the projection-relative source path
// for an unknown file. Given assetDirs at rendered-dir key `dir`, the
// source path is filepath.Join(entry.ProjectionSource, path.Base(rest))
// where rest is the tail of unknownPath below entry.ProjectionTarget.
// Returns ("", "", false) when no owner exists (mirrors owningAssetIDFor
// so callers get both keys atomically).
func owningAssetSourceRelFor(unknownPath string, assetDirs map[string]assetDirEntry) (assetID, sourceRel string, ok bool) {
	dir := pathpkg.Dir(unknownPath)
	for dir != "." && dir != "/" {
		if entry, hit := assetDirs[dir]; hit {
			// Tail is the path relative to the projection target root.
			var tail string
			if entry.ProjectionTarget == "." || entry.ProjectionTarget == "" {
				tail = unknownPath
			} else if unknownPath == entry.ProjectionTarget {
				tail = ""
			} else if strings.HasPrefix(unknownPath, entry.ProjectionTarget+"/") {
				tail = unknownPath[len(entry.ProjectionTarget)+1:]
			} else {
				return "", "", false
			}
			var srcRel string
			if entry.ProjectionSource == "." || entry.ProjectionSource == "" {
				srcRel = tail
			} else if tail == "" {
				srcRel = entry.ProjectionSource
			} else {
				srcRel = entry.ProjectionSource + "/" + tail
			}
			return entry.AssetID, srcRel, true
		}
		if surfaces.IsAssetContainerRoot(dir) {
			return "", "", false
		}
		next := pathpkg.Dir(dir)
		if next == dir {
			return "", "", false
		}
		dir = next
	}
	return "", "", false
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
