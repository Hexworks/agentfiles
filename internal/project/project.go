// Package project models the per-project manifest stored inside a profile. A
// project manifest records where a target repository lives, which agents are
// enabled, and which assets are selected for rendering.
package project

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/hexworks/agentfiles/internal/config"
	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/utils"
)

// Manifest stores the project-specific part of the model:
// where the repo lives, which agents are enabled, and which assets were chosen.
// NOTE: that we don't have a separate Project type as there is no separation between
// persisted and runtime state (everything is persisted).
type Manifest struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Path             string    `json:"path"`
	EnabledAgents    []string  `json:"enabled_agents"`
	SelectedAssetIDs []string  `json:"selected_asset_ids"`
	CreatedAt        time.Time `json:"created_at"`
}

// NewDraft builds a syntactically valid project manifest from form-style
// inputs. The id is derived from name via utils.Slug so callers cannot drift
// from app.Service.AddProject's id rule; CreatedAt is stamped to the current
// UTC time so the returned value passes Manifest.Validate(). EnabledAgents is
// copied defensively so later mutations on the input slice do not bleed into
// the manifest.
func NewDraft(name, path string, agents []string) *Manifest {
	return &Manifest{
		ID:            utils.Slug(name, config.DefaultProjectSlug),
		Name:          name,
		Path:          path,
		EnabledAgents: append([]string(nil), agents...),
		CreatedAt:     time.Now().UTC(),
	}
}

// Validate checks only the core project invariants.
func (m *Manifest) Validate() errs.DomainError {
	if m.ID == "" || m.Name == "" || m.Path == "" {
		return ErrProjectFieldsRequired
	}
	if len(m.EnabledAgents) == 0 {
		return ErrNoEnabledAgents
	}
	return nil
}

// Normalize transforms all paths to absolute and fixes ordering so
// the manifest stays stable in storage and comparisons.
func (m *Manifest) Normalize() errs.DomainError {
	abs, err := utils.ToAbsolute(m.Path)
	if err != nil {
		return err
	}
	m.Path = abs
	slices.Sort(m.EnabledAgents)
	slices.Sort(m.SelectedAssetIDs)
	return nil
}

// Save writes the project manifest into the owning profile's projects/
// directory.
func Save(profileRoot string, manifest *Manifest) errs.DomainError {
	if err := manifest.Normalize(); err != nil {
		return err
	}
	if err := manifest.Validate(); err != nil {
		return err
	}
	path := filepath.Join(profileRoot, config.ProjectsDirName, manifest.ID+".json")
	return utils.WriteJSON(path, manifest)
}

// Delete removes the project manifest file from the owning profile's
// projects/ directory. A missing file is treated as success so the
// operation is idempotent.
func Delete(profileRoot, projectID string) errs.DomainError {
	path := filepath.Join(profileRoot, config.ProjectsDirName, projectID+".json")
	if err := os.Remove(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return ProjectDeleteError{Path: path, Err: err}
	}
	return nil
}
