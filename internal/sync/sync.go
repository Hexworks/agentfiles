// Package sync reconciles the render plan with the target repository. It
// classifies every managed path as create/update/drift/delete/unknown and
// performs the writes plus managed-state snapshot on apply.
package sync

import (
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
// can be detected and migrated.
const GeneratorVersion = "1.0.0"

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
	// ManagedFiles contains the path -> hash mapping
	ManagedFiles map[string]string `json:"managed_files"`
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
// The zero value (empty string) means "no explicit choice, keep the
// local edits" and is what Apply assumes when a path is missing from
// the resolutions slice.
type DriftDecision string

// Possible DriftDecision values.
const (
	// DriftKeep leaves the on-disk content untouched and adopts the
	// current on-disk hash as the new managed baseline so future plans
	// no longer report the path as drift. The next Plan will still
	// emit ChangeUpdate if the rendered body diverges, which the user
	// can choose to apply or skip again.
	DriftKeep DriftDecision = "keep"
	// DriftOverwrite writes the rendered body over the drifted file.
	DriftOverwrite DriftDecision = "overwrite"
)

// UnknownDecision is the user's per-file choice for a ChangeUnknown
// entry. The zero value (empty string) means "no explicit choice,
// keep the stray file" and is what Apply assumes when a path is
// missing from the resolutions slice.
type UnknownDecision string

// Possible UnknownDecision values.
const (
	// UnknownKeep leaves the stray file untouched and does not
	// record it in ManagedState.
	UnknownKeep UnknownDecision = "keep"
	// UnknownDelete removes the file from disk.
	UnknownDelete UnknownDecision = "delete"
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

// FileChange is one human-facing diff entry shown in previews.
type FileChange struct {
	Path   string
	Kind   ChangeKind
	Reason ReasonKind
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
	if state != nil {
		deletes, unknowns, detectErrs := detectDeletesAndUnknowns(proj.Path, desired, state)
		if len(detectErrs) > 0 {
			return nil, errs.Errors(detectErrs)
		}
		for _, path := range deletes {
			changes = append(changes, FileChange{Path: path, Kind: ChangeDelete, Reason: ReasonStateRecordedDelete})
		}
		for _, path := range unknowns {
			changes = append(changes, FileChange{Path: path, Kind: ChangeUnknown, Reason: ReasonUnknown})
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
	if state.ManagedFiles[file.Path] != "" && state.ManagedFiles[file.Path] != currentHash {
		return FileChange{Path: file.Path, Kind: ChangeDrift, Reason: ReasonDriftDetected}, false, nil
	}
	return FileChange{Path: file.Path, Kind: ChangeUpdate, Reason: ReasonContentDiffers}, false, nil
}

// Apply materializes the preview into the repository using the user's
// per-file resolutions and then records a new ManagedState snapshot.
//
// Default decisions per ChangeKind:
//   - ChangeCreate, ChangeUpdate: always write (resolutions ignored).
//   - ChangeDelete: always remove (resolutions ignored; user already saw
//     the preview).
//   - ChangeDrift: DriftKeep by default; DriftOverwrite writes the body.
//     DriftKeep adopts the current on-disk hash into ManagedState so
//     future plans no longer flag the path as drift.
//   - ChangeUnknown: UnknownKeep by default; UnknownDelete removes the
//     file.
//
// All resolution paths are validated up front; invalid (absolute, OS-
// separated) paths produce InvalidPathError. Targets that fall outside
// surfaces.IsAllowed produce OutsideSurfaceError. Errors accumulate;
// state is still rewritten so the recorded baseline reflects whatever
// the apply loop actually wrote.
func Apply(preview *Preview, driftResolutions []DriftResolution, unknownResolutions []UnknownResolution) errs.DomainError {
	driftByPath, validationErrs := indexDriftResolutions(driftResolutions)
	unknownByPath, unknownErrs := indexUnknownResolutions(unknownResolutions)
	validationErrs = append(validationErrs, unknownErrs...)
	if len(validationErrs) > 0 {
		return errs.Errors(validationErrs)
	}
	bodiesByPath := map[string]render.RenderedFile{}
	// Pre-fill the state baseline with the rendered hash of every
	// desired file. The switch below only needs to overwrite paths
	// where the on-disk content diverges (kept drift adopts current
	// hash); clean files (not in Changes) retain their rendered hash
	// so drift detection still works on the next plan.
	recordedHashes := map[string]string{}
	for _, f := range preview.Files {
		bodiesByPath[f.Path] = f
		recordedHashes[f.Path] = utils.HashBytes(f.Body)
	}
	var domainErrs []errs.DomainError
	for _, change := range preview.Changes {
		if !surfaces.IsAllowed(change.Path) {
			domainErrs = append(domainErrs, OutsideSurfaceError{Path: change.Path})
			continue
		}
		switch change.Kind {
		case ChangeCreate, ChangeUpdate:
			if err := writeRendered(preview.ProjectPath, bodiesByPath[change.Path]); err != nil {
				domainErrs = append(domainErrs, err)
			}
			// Hash already pre-filled with rendered body.
		case ChangeDrift:
			if driftByPath[change.Path] == DriftOverwrite {
				if err := writeRendered(preview.ProjectPath, bodiesByPath[change.Path]); err != nil {
					domainErrs = append(domainErrs, err)
				}
				// Pre-filled rendered hash is correct after overwrite.
				continue
			}
			// DriftKeep (default): adopt the on-disk hash as the new
			// managed baseline so the path no longer trips drift
			// detection next plan.
			abs := filepath.Join(preview.ProjectPath, filepath.FromSlash(change.Path))
			currentHash, hashErr := utils.HashFile(abs)
			if hashErr != nil {
				domainErrs = append(domainErrs, hashErr)
				continue
			}
			recordedHashes[change.Path] = currentHash
		case ChangeDelete:
			if err := removeFile(preview.ProjectPath, change.Path); err != nil {
				domainErrs = append(domainErrs, err)
			}
			// Deleted paths are never in preview.Files, so the
			// pre-filled map already excludes them.
		case ChangeUnknown:
			if unknownByPath[change.Path] != UnknownDelete {
				continue
			}
			if err := removeFile(preview.ProjectPath, change.Path); err != nil {
				domainErrs = append(domainErrs, err)
			}
			// Unknown files were never in preview.Files; nothing to
			// record either way.
		}
	}
	state := &ManagedState{
		ProfileID:        preview.ProfileID,
		ProjectID:        preview.ProjectID,
		GeneratorVersion: GeneratorVersion,
		LastAppliedAt:    time.Now().UTC(),
		ManagedFiles:     recordedHashes,
	}
	if err := utils.WriteJSON(filepath.Join(preview.ProjectPath, config.StateDirName, config.StateFileName), state); err != nil {
		domainErrs = append(domainErrs, err)
	}
	if len(domainErrs) == 0 {
		return nil
	}
	if len(domainErrs) == 1 {
		return domainErrs[0]
	}
	return errs.Errors(domainErrs)
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
	path := filepath.Join(projectPath, config.StateDirName, config.StateFileName)
	if !utils.Exists(path) {
		return nil, StateMissingError{Path: path}
	}
	var state ManagedState
	if err := utils.ReadJSON(path, &state); err != nil {
		return nil, err
	}
	if state.ManagedFiles == nil {
		state.ManagedFiles = map[string]string{}
	}
	for key := range state.ManagedFiles {
		if err := validatePathKey(key); err != nil {
			return nil, StateCorruptError{Path: path, Key: key}
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
	for path := range state.ManagedFiles {
		if desired[path] == "" {
			deleteSet[path] = true
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
	if state.ManagedFiles[rel] != "" {
		return
	}
	unknowns[rel] = true
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
