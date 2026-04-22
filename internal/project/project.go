package project

import (
	"fmt"
	"path/filepath"
	"slices"
	"time"

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
func (m *Manifest) Validate() error {
	if m.ID == "" || m.Name == "" || m.Path == "" {
		return fmt.Errorf("project id, name, and path are required")
	}
	if len(m.EnabledAgents) == 0 {
		return fmt.Errorf("at least one agent must be enabled")
	}
	return nil
}

// Normalize canonicalizes path and ordering so the manifest stays stable in
// storage and comparisons.
func (m *Manifest) Normalize() error {
	abs, err := fsutil.CleanAbs(m.Path)
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
func Save(profileRoot string, manifest *Manifest) error {
	if err := manifest.Normalize(); err != nil {
		return err
	}
	if err := manifest.Validate(); err != nil {
		return err
	}
	path := filepath.Join(profileRoot, "projects", manifest.ID+".json")
	return fsutil.WriteJSON(path, manifest)
}
