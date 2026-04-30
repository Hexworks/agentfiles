// Package project models the per-project manifest stored inside a profile. A
// project manifest records where a target repository lives, which agents are
// enabled, and which assets are selected for rendering.
package project

import (
	"path/filepath"
	"slices"
	"time"

	"github.com/addamsson/agentfiles/internal/config"
	"github.com/addamsson/agentfiles/internal/errs"
	"github.com/addamsson/agentfiles/internal/fsutil"
)

// Manifest stores the project-specific part of the model:
// where the repo lives, which agents are enabled, and which assets were chosen.
type Manifest struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Path             string    `json:"path"`
	EnabledAgents    []string  `json:"enabled_agents"`
	SelectedAssetIDs []string  `json:"selected_asset_ids"`
	CreatedAt        time.Time `json:"created_at"`
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
	abs, err := fsutil.ToAbsolute(m.Path)
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
	return fsutil.WriteJSON(path, manifest)
}
