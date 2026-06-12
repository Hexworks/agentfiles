package actions

import (
	"github.com/hexworks/agentfiles/internal/app"
	"github.com/hexworks/agentfiles/internal/asset"
)

type CreateProfileInput struct {
	Name string
	Path string
}

type RegisterProfileInput struct {
	Path string
}

// LoadProfileInput's ProfileRef follows registry resolver semantics —
// id, name, or path all resolve to the same profile.
type LoadProfileInput struct {
	ProfileRef string
}

// DeleteProfileInput is shared by both DeleteProfile (registry entry
// only) and DeleteProfileWithFolder (registry entry + folder). Splitting
// into two action methods keeps the destructive intent visible at the
// call site, mirroring app.Service.
type DeleteProfileInput struct {
	ProfileRef string
}

// AddProjectInput mirrors the fields the legacy huh form collected for
// "Add Project". EnabledAgents is the agent allow-list; AssetIDs is the
// initial selection.
type AddProjectInput struct {
	ProfileRef    string
	Name          string
	Path          string
	EnabledAgents []string
	AssetIDs      []string
}

type LoadProjectInput struct {
	ProfileRef string
	ProjectID  string
}

// UpdateProjectInput carries the user-editable fields that the Edit
// Project flow collects. ID, SelectedAssetIDs, and CreatedAt are
// preserved by the service — the TUI never sends a live
// *project.Manifest pointer through this seam.
type UpdateProjectInput struct {
	ProfileRef    string
	ProjectID     string
	Name          string
	Path          string
	EnabledAgents []string
}

type DeleteProjectInput struct {
	ProfileRef string
	ProjectID  string
}

type PlanProjectInput struct {
	ProfileRef string
	ProjectID  string
}

// SyncProjectInput carries the per-file resolutions consumed by
// Service.Apply. Drift and Unknown are kept as separate slices to match
// the actual app.Service shape; defaults (no resolution for a path)
// keep drifted files and unknown files alone.
type SyncProjectInput struct {
	ProfileRef string
	ProjectID  string
	Drift      []app.DriftResolution
	Unknown    []app.UnknownResolution
}

type LoadAssetInput struct {
	ProfileRef string
	AssetID    string
}

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

type DeleteAssetInput struct {
	ProfileRef string
	AssetID    string
}

// AddAssetFileInput carries the relative path of a file to create inside
// the asset's directory. Containment + reserved-name policy lives in the
// asset domain; the TUI never names a path the service must trust.
type AddAssetFileInput struct {
	ProfileRef string
	AssetID    string
	Rel        string
}

// RemoveAssetFileInput is the symmetric input for in-asset file deletion.
type RemoveAssetFileInput struct {
	ProfileRef string
	AssetID    string
	Rel        string
}
