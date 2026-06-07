// Package sync reconciles the render plan with the target repository. It
// classifies every managed path as create/update/drift/delete and
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
	// apply would overwrite those edits.
	ChangeDrift ChangeKind = "drift"
	// ChangeDelete marks a recognized managed file that is no longer part of
	// the desired plan and may be removed on apply.
	ChangeDelete ChangeKind = "delete"
	// ChangeUnknown marks an unrecognized file
	ChangeUnknown = "unknown"
)

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
//   - the computed change list
//   - the delete candidates found in the current repo
//   - enough metadata to write a fresh managed state on apply
type Preview struct {
	ProjectPath string
	Files       []render.RenderedFile
	Changes     []FileChange
	// TODO: delete this, use Changes instead
	DeleteCandidates []string
	ManagedState     *ManagedState
	ProfileID        string
	ProjectID        string
}

// Plan compares the desired outputs with the current repository state. This is
// where create/update/drift/delete-candidate classification happens. Every
// failure mode (render leaves, hashing, state load, walk) is returned as an
// errs.DomainError so the TUI can render severity, icon, and color uniformly.
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
		if state != nil && state.ManagedFiles[file.Path] != "" && state.ManagedFiles[file.Path] != currentHash {
			changes = append(changes, FileChange{Path: file.Path, Kind: ChangeDrift, Reason: "managed file changed locally"})
			continue
		}
		changes = append(changes, FileChange{Path: file.Path, Kind: ChangeUpdate, Reason: "content differs"})
	}
	deleteCandidates, detectErrs := detectDeleteCandidates(proj.Path, desired, state)
	if len(detectErrs) > 0 {
		return nil, errs.Errors(detectErrs)
	}
	for _, candidate := range deleteCandidates {
		changes = append(changes, FileChange{Path: candidate, Kind: ChangeDelete, Reason: "recognized llm file not selected"})
	}
	slices.SortFunc(changes, func(a, b FileChange) int {
		return strings.Compare(a.Path, b.Path)
	})
	return &Preview{
		ProjectPath:      proj.Path,
		Files:            rendered.Files,
		Changes:          changes,
		DeleteCandidates: deleteCandidates,
		ManagedState:     state,
		ProfileID:        p.Manifest.ID,
		ProjectID:        proj.ID,
	}, nil
}

// Apply materializes the preview into the repository and then records a new
// ManagedState snapshot. Preview generation and file writing are separated so
// the user can inspect changes first.
func Apply(preview *Preview, deleteCandidates bool) errs.DomainError {
	var domainErrs []errs.DomainError
	for _, file := range preview.Files {
		abs := filepath.Join(preview.ProjectPath, filepath.FromSlash(file.Path))
		if err := utils.WriteFile(abs, file.Body, file.Mode); err != nil {
			domainErrs = append(domainErrs, err)
		}
	}
	if deleteCandidates {
		for _, path := range preview.DeleteCandidates {
			abs := filepath.Join(preview.ProjectPath, filepath.FromSlash(path))
			if err := os.Remove(abs); err != nil && !os.IsNotExist(err) {
				domainErrs = append(domainErrs, DeleteError{Path: abs, Err: err})
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

// detectDeleteCandidates looks for recognized LLM-tooling files that are inside
// managed surfaces but not part of the new desired state.
func detectDeleteCandidates(projectPath string, desired map[string]string, state *ManagedState) ([]string, []errs.DomainError) {
	candidates := map[string]bool{}
	if state != nil {
		for path := range state.ManagedFiles {
			if desired[path] == "" {
				candidates[path] = true
			}
		}
	}
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
			if desired[rel] == "" {
				candidates[rel] = true
			}
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
			if desired[rel] == "" {
				candidates[rel] = true
			}
			return nil
		})
		if walkErr != nil {
			domainErrs = append(domainErrs, SurfaceWalkError{Root: abs, Err: walkErr})
		}
	}
	var list []string
	for path := range candidates {
		list = append(list, path)
	}
	slices.Sort(list)
	return list, domainErrs
}
