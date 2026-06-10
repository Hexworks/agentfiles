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
	// unless the user opts in to delete via ResolveDelete.
	ChangeUnknown ChangeKind = "unknown"
)

// Resolution is the per-file decision the user makes when applying a preview.
// It only matters for ChangeDrift (overwrite vs keep) and ChangeUnknown
// (delete vs keep); create/update/delete carry ResolveAuto and are always
// materialized.
type Resolution int

// Resolution values. ResolveAuto is the implicit default for create/update/
// delete entries. ResolveKeep is the implicit default for drift and unknown.
const (
	// ResolveAuto means apply the change automatically; only valid for
	// create/update/delete kinds.
	ResolveAuto Resolution = iota
	// ResolveOverwrite means write the rendered body over the drifted file.
	// Only valid for drift.
	ResolveOverwrite
	// ResolveKeep means leave the file untouched. Default for drift and
	// unknown.
	ResolveKeep
	// ResolveDelete means remove the file. Only valid for unknown.
	ResolveDelete
)

// FileResolution pairs a target path with the user's per-file choice. Paths
// absent from the resolutions slice keep the default behavior for their
// ChangeKind.
type FileResolution struct {
	Path       string
	Resolution Resolution
}

// FileChange is one human-facing diff entry shown in previews.
type FileChange struct {
	Path   string
	Kind   ChangeKind
	Reason string
}

// Preview is the bridge between render and apply.
//
// It combines:
//   - the desired files produced by render
//   - the computed change list (create/update/drift/delete/unknown)
//   - enough metadata to write a fresh managed state on apply
type Preview struct {
	ProjectPath  string
	Files        []render.RenderedFile
	Changes      []FileChange
	ManagedState *ManagedState
	ProfileID    string
	ProjectID    string
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
		if state == nil {
			// First-apply clean slate: every desired file is a create,
			// regardless of whether something already exists at that path.
			changes = append(changes, FileChange{Path: file.Path, Kind: ChangeCreate, Reason: "first apply"})
			continue
		}
		abs := filepath.Join(proj.Path, filepath.FromSlash(file.Path))
		if !utils.Exists(abs) {
			changes = append(changes, FileChange{Path: file.Path, Kind: ChangeCreate, Reason: "file missing"})
			continue
		}
		currentHash, hashErr := utils.HashFile(abs)
		if hashErr != nil {
			return nil, hashErr
		}
		if currentHash == desired[file.Path] {
			continue
		}
		if state.ManagedFiles[file.Path] != "" && state.ManagedFiles[file.Path] != currentHash {
			changes = append(changes, FileChange{Path: file.Path, Kind: ChangeDrift, Reason: "managed file changed locally"})
			continue
		}
		changes = append(changes, FileChange{Path: file.Path, Kind: ChangeUpdate, Reason: "content differs"})
	}
	if state != nil {
		deletes, unknowns, detectErrs := detectExtraneous(proj.Path, desired, state)
		if len(detectErrs) > 0 {
			return nil, errs.Errors(detectErrs)
		}
		for _, path := range deletes {
			changes = append(changes, FileChange{Path: path, Kind: ChangeDelete, Reason: "recognized llm file not selected"})
		}
		for _, path := range unknowns {
			changes = append(changes, FileChange{Path: path, Kind: ChangeUnknown, Reason: "unrecognized file in managed surface"})
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
	}, nil
}

// Apply materializes the preview into the repository using the user's
// per-file resolutions and then records a new ManagedState snapshot.
//
// Default resolution behavior:
//   - ChangeCreate, ChangeUpdate: always write (resolutions ignored).
//   - ChangeDelete: always remove (resolutions ignored; user already saw
//     the preview).
//   - ChangeDrift: keep by default; write only if ResolveOverwrite.
//   - ChangeUnknown: keep by default; remove only if ResolveDelete.
//
// Paths absent from the resolutions slice keep the default behavior for
// their ChangeKind. Duplicate paths: last entry wins.
func Apply(preview *Preview, resolutions []FileResolution) errs.DomainError {
	resolutionsByPath := map[string]Resolution{}
	for _, r := range resolutions {
		resolutionsByPath[r.Path] = r.Resolution
	}
	bodiesByPath := map[string]render.RenderedFile{}
	for _, f := range preview.Files {
		bodiesByPath[f.Path] = f
	}
	var domainErrs []errs.DomainError
	for _, change := range preview.Changes {
		switch change.Kind {
		case ChangeCreate, ChangeUpdate:
			if err := writeRendered(preview.ProjectPath, bodiesByPath[change.Path]); err != nil {
				domainErrs = append(domainErrs, err)
			}
		case ChangeDrift:
			if resolutionsByPath[change.Path] != ResolveOverwrite {
				continue
			}
			if err := writeRendered(preview.ProjectPath, bodiesByPath[change.Path]); err != nil {
				domainErrs = append(domainErrs, err)
			}
		case ChangeDelete:
			if err := removeFile(preview.ProjectPath, change.Path); err != nil {
				domainErrs = append(domainErrs, err)
			}
		case ChangeUnknown:
			if resolutionsByPath[change.Path] != ResolveDelete {
				continue
			}
			if err := removeFile(preview.ProjectPath, change.Path); err != nil {
				domainErrs = append(domainErrs, err)
			}
		}
	}
	state := &ManagedState{
		ProfileID:        preview.ProfileID,
		ProjectID:        preview.ProjectID,
		GeneratorVersion: GeneratorVersion,
		LastAppliedAt:    time.Now().UTC(),
		ManagedFiles:     map[string]string{},
	}
	for _, file := range preview.Files {
		state.ManagedFiles[file.Path] = utils.HashBytes(file.Body)
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

// writeRendered writes a single RenderedFile to the project, preserving its
// mode. The path is resolved relative to projectPath.
func writeRendered(projectPath string, file render.RenderedFile) errs.DomainError {
	abs := filepath.Join(projectPath, filepath.FromSlash(file.Path))
	return utils.WriteFile(abs, file.Body, file.Mode)
}

// removeFile deletes path inside projectPath. Missing files are tolerated so
// repeated applies stay idempotent.
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
	return &state, nil
}

// detectExtraneous splits non-desired files inside managed surfaces into two
// buckets:
//   - deletes: files recorded in state.ManagedFiles that are no longer in
//     desired. These will be auto-removed on apply.
//   - unknowns: files inside surfaces.Roots() that are neither in desired
//     nor in state.ManagedFiles. These require an explicit ResolveDelete
//     resolution to actually be removed.
//
// Callers must guarantee state != nil; first-apply skips this pass entirely.
func detectExtraneous(projectPath string, desired map[string]string, state *ManagedState) ([]string, []string, []errs.DomainError) {
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
		if !utils.Exists(abs) {
			continue
		}
		info, err := os.Stat(abs)
		if err != nil {
			domainErrs = append(domainErrs, StatError{Path: abs, Err: err})
			continue
		}
		if !info.IsDir() {
			rel := utils.ToRelative(projectPath, abs)
			classifyExtraneous(rel, desired, state, unknownSet)
			continue
		}
		walkErr := filepath.WalkDir(abs, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			rel := utils.ToRelative(projectPath, path)
			classifyExtraneous(rel, desired, state, unknownSet)
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

// classifyExtraneous routes a single file path discovered during the surface
// walk into the unknowns bucket if it is neither desired nor previously
// managed. State-recorded files missing from desired are handled by the
// caller's pre-walk pass over state.ManagedFiles.
func classifyExtraneous(rel string, desired map[string]string, state *ManagedState, unknowns map[string]bool) {
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
