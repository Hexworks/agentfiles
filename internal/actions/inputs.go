package actions

import (
	"github.com/hexworks/agentfiles/internal/appapi"
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
	Drift      []appapi.DriftResolution
	Unknown    []appapi.UnknownResolution
	// IgnoredPaths carries repo-relative folder keys the user chose to ignore;
	// they are persisted so future plans suppress unknowns under them.
	IgnoredPaths []string
}

// DiffFileInput carries the ids for the Plan Project [Diff] row action:
// the profile + project plus the forward-slash project-relative path of the
// file to diff. The service re-renders read-only and reads the on-disk body;
// the TUI never hands over a pre-resolved absolute path.
type DiffFileInput struct {
	ProfileRef string
	ProjectID  string
	Path       string
}

type LoadAssetInput struct {
	ProfileRef string
	AssetID    string
}

type CreateAssetInput struct {
	ProfileRef string
	Manifest   asset.Manifest
}

// CreateAssetFromFolderInput carries the data for the Plan Project
// "Register as Asset" action: the manifest the user filled in plus the
// project-relative, forward-slash directory key whose files become the
// asset's content. The service resolves the key against the project root,
// re-asserts the folder is registerable, and selects the new asset for the
// project — the TUI never hands over a pre-resolved absolute path.
type CreateAssetFromFolderInput struct {
	ProfileRef string
	ProjectID  string
	Manifest   asset.Manifest
	DirKey     string
}

// UpdateAssetInput carries the profile reference plus the edited
// manifest. The Service derives the on-disk directory; Dir is not part
// of this input.
type UpdateAssetInput struct {
	ProfileRef string
	Manifest   *asset.Manifest
}

// SaveAssetFilesEditInput mirrors UpdateAssetInput; the separate name
// keeps the intent visible at the call site (Save button vs. return
// from external editor) so the shell test doubles can differentiate the
// two commit shapes without inspecting arguments.
type SaveAssetFilesEditInput struct {
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

// SelectAssetInput carries the ids for the row-level select action on the
// Select Project Assets screen. The service is idempotent on duplicate
// ids; callers do not need to filter.
type SelectAssetInput struct {
	ProfileRef string
	ProjectID  string
	AssetID    string
}

// UnselectAssetInput is the symmetric input for removing an asset id from
// a project's selection. Idempotent on missing ids.
type UnselectAssetInput struct {
	ProfileRef string
	ProjectID  string
	AssetID    string
}
