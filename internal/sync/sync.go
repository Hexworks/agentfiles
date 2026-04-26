// Package sync reconciles the render plan with the target repository. It
// classifies every managed path as create/update/drift/delete_candidate and
// performs the writes plus managed-state snapshot on apply.
package sync

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/addamsson/agentfiles/internal/config"
	"github.com/addamsson/agentfiles/internal/fsutil"
	"github.com/addamsson/agentfiles/internal/profile"
	"github.com/addamsson/agentfiles/internal/project"
	"github.com/addamsson/agentfiles/internal/render"
	"github.com/addamsson/agentfiles/internal/surfaces"
)

// GeneratorVersion is stamped into the managed state so future format changes
// can be detected and migrated.
const GeneratorVersion = "1.0.0"

// ManagedState is the persisted memory of the last successful apply.
// It lets the next preview tell the difference between:
//   - a normal update (desired output changed)
//   - drift (a previously managed file was edited locally)
type ManagedState struct {
	ProfileID        string            `json:"profile_id"`
	ProjectID        string            `json:"project_id"`
	GeneratorVersion string            `json:"generator_version"`
	LastAppliedAt    time.Time         `json:"last_applied_at"`
	ManagedFiles     map[string]string `json:"managed_files"`
}

// ChangeKind classifies a single entry in a sync preview.
type ChangeKind string

// Possible ChangeKind values. Each corresponds to a different reconciliation
// decision between the render plan and the current repository state.
const (
	// ChangeCreate means the file is absent and will be written.
	ChangeCreate ChangeKind = "create"
	// ChangeUpdate means the file exists with different content and will be
	// overwritten.
	ChangeUpdate ChangeKind = "update"
	// ChangeDrift means a previously managed file was modified locally; apply
	// would overwrite those edits.
	ChangeDrift ChangeKind = "drift"
	// ChangeDelete marks a recognized managed file that is no longer part of
	// the desired plan and may be removed on apply.
	ChangeDelete ChangeKind = "delete_candidate"
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
	ProjectPath         string
	Files               []render.RenderedFile
	Changes             []FileChange
	DeleteCandidates    []string
	ManagedState        *ManagedState
	ProfileID           string
	ProjectID           string
	UnmanagedRecognized []string
}

// Plan compares the desired outputs with the current repository state. This is
// where create/update/drift/delete-candidate classification happens.
func Plan(p *profile.Profile, proj *project.Manifest) (*Preview, error) {
	rendered, err := render.Build(p, proj)
	if err != nil {
		return nil, err
	}
	state, _ := loadState(proj.Path)
	desired := map[string]string{}
	var changes []FileChange
	for _, file := range rendered.Files {
		desired[file.Path] = fsutil.HashBytes(file.Body)
		abs := filepath.Join(proj.Path, filepath.FromSlash(file.Path))
		if !fsutil.Exists(abs) {
			changes = append(changes, FileChange{Path: file.Path, Kind: ChangeCreate, Reason: "file missing"})
			continue
		}
		currentHash, err := fsutil.HashFile(abs)
		if err != nil {
			return nil, err
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
	deleteCandidates, err := detectDeleteCandidates(proj.Path, desired, state)
	if err != nil {
		return nil, err
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
func Apply(preview *Preview, deleteCandidates bool) error {
	for _, file := range preview.Files {
		abs := filepath.Join(preview.ProjectPath, filepath.FromSlash(file.Path))
		if err := fsutil.WriteFile(abs, file.Body, file.Mode); err != nil {
			return err
		}
	}
	if deleteCandidates {
		for _, path := range preview.DeleteCandidates {
			abs := filepath.Join(preview.ProjectPath, filepath.FromSlash(path))
			if err := os.Remove(abs); err != nil && !os.IsNotExist(err) {
				return err
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
		state.ManagedFiles[file.Path] = fsutil.HashBytes(file.Body)
	}
	return fsutil.WriteJSON(filepath.Join(preview.ProjectPath, config.StateDirName, config.StateFileName), state)
}

// loadState reads the previous managed snapshot from the target repository.
func loadState(projectPath string) (*ManagedState, error) {
	path := filepath.Join(projectPath, config.StateDirName, config.StateFileName)
	if !fsutil.Exists(path) {
		return nil, os.ErrNotExist
	}
	var state ManagedState
	if err := fsutil.ReadJSON(path, &state); err != nil {
		return nil, err
	}
	if state.ManagedFiles == nil {
		state.ManagedFiles = map[string]string{}
	}
	return &state, nil
}

// detectDeleteCandidates looks for recognized LLM-tooling files that are inside
// managed surfaces but not part of the new desired state.
func detectDeleteCandidates(projectPath string, desired map[string]string, state *ManagedState) ([]string, error) {
	candidates := map[string]bool{}
	if state != nil {
		for path := range state.ManagedFiles {
			if desired[path] == "" {
				candidates[path] = true
			}
		}
	}
	// .agentfiles is intentionally outside surfaces.Roots(), so the walk
	// below never enters the managed-state directory; no skip check needed.
	for _, root := range surfaces.Roots() {
		abs := filepath.Join(projectPath, root)
		if !fsutil.Exists(abs) {
			continue
		}
		info, err := os.Stat(abs)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			rel := fsutil.ToRelative(projectPath, abs)
			if desired[rel] == "" {
				candidates[rel] = true
			}
			continue
		}
		err = filepath.WalkDir(abs, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			rel := fsutil.ToRelative(projectPath, path)
			if desired[rel] == "" {
				candidates[rel] = true
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	var list []string
	for path := range candidates {
		list = append(list, path)
	}
	slices.Sort(list)
	return list, nil
}

// FormatPreview renders the preview into a compact CLI-friendly text summary.
// FIX: task#0005
func FormatPreview(preview *Preview) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Project: %s\n", preview.ProjectPath)
	if len(preview.Changes) == 0 {
		b.WriteString("No changes.\n")
		return b.String()
	}
	for _, change := range preview.Changes {
		fmt.Fprintf(&b, "- [%s] %s: %s\n", change.Kind, change.Path, change.Reason)
	}
	return b.String()
}
