// Package project models the per-project manifest that pairs a target
// repository path with the agents and assets selected for it. The manifest
// itself is content-only: persistence lives in `internal/projectstore` so
// the profile folder can be shared without leaking per-machine selections.
package project

import (
	"slices"
	"time"

	"github.com/hexworks/agentfiles/internal/agent"
	"github.com/hexworks/agentfiles/internal/config"
	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/utils"
)

// Manifest stores the project-specific part of the model:
// where the repo lives, which agents are enabled, and which assets were chosen.
// NOTE: that we don't have a separate Project type as there is no separation between
// persisted and runtime state (everything is persisted).
type Manifest struct {
	ID               string        `json:"id"`
	Name             string        `json:"name"`
	Path             string        `json:"path"`
	EnabledAgents    []agent.Agent `json:"enabled_agents"`
	SelectedAssetIDs []string      `json:"selected_asset_ids"`
	CreatedAt        time.Time     `json:"created_at"`
}

// NewDraft builds a syntactically valid project manifest from form-style
// inputs. The id is derived from name via utils.Slug so callers cannot drift
// from app.Service.AddProject's id rule; CreatedAt is stamped to the current
// UTC time so the returned value passes Manifest.Validate(). EnabledAgents is
// copied defensively so later mutations on the input slice do not bleed into
// the manifest.
func NewDraft(name, path string, agents []agent.Agent) *Manifest {
	return &Manifest{
		ID:            utils.Slug(name, config.DefaultProjectSlug),
		Name:          name,
		Path:          path,
		EnabledAgents: append([]agent.Agent(nil), agents...),
		CreatedAt:     time.Now().UTC(),
	}
}

// SelectAsset appends id to SelectedAssetIDs if not already present,
// reporting whether it was added. It owns the append-if-absent rule on the
// in-memory manifest so callers (Service.SelectAsset,
// Service.CreateAssetFromFolder) do not reimplement the dedup and drift apart.
func (m *Manifest) SelectAsset(id string) bool {
	if slices.Contains(m.SelectedAssetIDs, id) {
		return false
	}
	m.SelectedAssetIDs = append(m.SelectedAssetIDs, id)
	return true
}

// Validate checks only the core project invariants.
func (m *Manifest) Validate() errs.DomainError {
	if m.ID == "" || m.Name == "" || m.Path == "" {
		return ErrProjectFieldsRequired
	}
	if len(m.EnabledAgents) == 0 {
		return ErrNoEnabledAgents
	}
	// agent.Unknown owns the order-preserving dedup rule shared with asset
	// validation; project has a single source (EnabledAgents) to feed it.
	if unknown := agent.Unknown(m.EnabledAgents); len(unknown) > 0 {
		return UnknownEnabledAgentError{Agents: unknown}
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
