package actions

import (
	"github.com/hexworks/agentfiles/internal/app"
	"github.com/hexworks/agentfiles/internal/asset"
	"github.com/hexworks/agentfiles/internal/project"
)

// FolderAction selects whether DeleteProfile also removes the on-disk
// profile folder. The zero value (KeepFolders) is the safe default:
// callers must opt in to destruction.
type FolderAction int

const (
	// KeepFolders deletes the registry entry only, leaving the profile
	// folder on disk untouched.
	KeepFolders FolderAction = iota
	// DeleteFolders removes both the registry entry and the on-disk
	// profile folder. Guarded by Service-side safety checks.
	DeleteFolders
)

// CreateProfileInput is the input for Actions.CreateProfile.
type CreateProfileInput struct {
	Name string
	Path string
}

// RegisterProfileInput is the input for Actions.RegisterProfile.
type RegisterProfileInput struct {
	Path string
}

// LoadProfileInput is the input for Actions.LoadProfile. ProfileRef
// follows registry resolver semantics — id, name, or path all resolve
// to the same profile.
type LoadProfileInput struct {
	ProfileRef string
}

// DeleteProfileInput is the input for Actions.DeleteProfile.
type DeleteProfileInput struct {
	ProfileRef   string
	FolderAction FolderAction
}

// RegisterProjectInput mirrors the fields the legacy huh form collected
// for "Add Project". EnabledAgents is the agent allow-list; AssetIDs is
// the initial selection.
type RegisterProjectInput struct {
	ProfileRef    string
	Name          string
	Path          string
	EnabledAgents []string
	AssetIDs      []string
}

// LoadProjectInput pairs profile + project ids for Actions.LoadProject.
type LoadProjectInput struct {
	ProfileRef string
	ProjectID  string
}

// UpdateProjectInput carries the profile reference plus the edited
// manifest. The manifest's ID identifies the project to overwrite.
type UpdateProjectInput struct {
	ProfileRef string
	Project    *project.Manifest
}

// DeleteProjectInput pairs profile + project ids for
// Actions.DeleteProject.
type DeleteProjectInput struct {
	ProfileRef string
	ProjectID  string
}

// PlanProjectInput pairs profile + project ids for
// Actions.PlanProject.
type PlanProjectInput struct {
	ProfileRef string
	ProjectID  string
}

// SyncProjectInput carries the per-file resolutions consumed by
// Service.Apply. Drift and Unknown are kept as separate slices to
// match the actual app.Service shape; defaults (no resolution for a
// path) keep drifted files and unknown files alone.
type SyncProjectInput struct {
	ProfileRef string
	ProjectID  string
	Drift      []app.DriftResolution
	Unknown    []app.UnknownResolution
}

// LoadAssetInput pairs profile + asset ids for Actions.LoadAsset.
type LoadAssetInput struct {
	ProfileRef string
	AssetID    string
}

// CreateAssetInput pairs profile reference with the asset manifest
// scaffolded into the profile.
type CreateAssetInput struct {
	ProfileRef string
	Manifest   asset.Manifest
}

// UpdateAssetInput carries the profile reference plus the edited
// manifest. The Service derives the on-disk directory; Dir is not part
// of this input.
type UpdateAssetInput struct {
	ProfileRef string
	Manifest   *asset.Manifest
}

// DeleteAssetInput pairs profile + asset ids for Actions.DeleteAsset.
type DeleteAssetInput struct {
	ProfileRef string
	AssetID    string
}
