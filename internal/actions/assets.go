package actions

import (
	"github.com/hexworks/agentfiles/internal/app"
	"github.com/hexworks/agentfiles/internal/asset"
	"github.com/hexworks/agentfiles/internal/errs"
)

func (a *Actions) LoadAsset(in LoadAssetInput) (*asset.Asset, errs.DomainError) {
	return a.svc.LoadAsset(in.ProfileRef, in.AssetID)
}

// CreateAsset scaffolds a new asset inside the profile and returns the
// on-disk asset directory.
func (a *Actions) CreateAsset(in CreateAssetInput) (string, errs.DomainError) {
	return a.svc.InitAsset(in.ProfileRef, in.Manifest)
}

// CreateAssetFromFolder creates an asset from an unmanaged project folder
// and selects it for the project. Returns the new asset id.
func (a *Actions) CreateAssetFromFolder(in CreateAssetFromFolderInput) (string, errs.DomainError) {
	return a.svc.CreateAssetFromFolder(in.ProfileRef, in.ProjectID, in.Manifest, in.DirKey)
}

// UpdateAsset persists the manifest edit and, when git integration is
// enabled and the profile is a git repo, records a manifest-scoped
// commit. The returned CommitOutcome carries the short SHA on success
// and Err on a hard commit failure; a zero-value CommitOutcome means
// "no commit path was attempted" (feature disabled or dir not a repo).
func (a *Actions) UpdateAsset(in UpdateAssetInput) (app.CommitOutcome, errs.DomainError) {
	return a.svc.UpdateAsset(in.ProfileRef, in.Manifest)
}

// SaveAssetFilesEdit persists the manifest edit alongside a
// files-scoped commit against `assets/<asset-id>/**`, used by the
// editor-return flow after the user finishes editing an asset file. See
// UpdateAsset for the CommitOutcome semantics.
func (a *Actions) SaveAssetFilesEdit(in SaveAssetFilesEditInput) (app.CommitOutcome, errs.DomainError) {
	return a.svc.SaveAssetFilesEdit(in.ProfileRef, in.Manifest)
}

// DeleteAsset removes the asset from the profile and unselects it from
// every project that referenced it.
func (a *Actions) DeleteAsset(in DeleteAssetInput) (struct{}, errs.DomainError) {
	return struct{}{}, a.svc.DeleteAsset(in.ProfileRef, in.AssetID)
}

// AddAssetFile creates an empty file inside the asset's directory.
func (a *Actions) AddAssetFile(in AddAssetFileInput) (struct{}, errs.DomainError) {
	return struct{}{}, a.svc.AddAssetFile(in.ProfileRef, in.AssetID, in.Rel)
}

// RemoveAssetFile deletes a file inside the asset's directory.
func (a *Actions) RemoveAssetFile(in RemoveAssetFileInput) (struct{}, errs.DomainError) {
	return struct{}{}, a.svc.RemoveAssetFile(in.ProfileRef, in.AssetID, in.Rel)
}

// SelectAsset adds the asset id to the project's SelectedAssetIDs and
// returns the post-persistence selection slice. Idempotent on duplicate
// ids. Callers never project the next state TUI-side — use the returned
// slice as the new truth.
func (a *Actions) SelectAsset(in SelectAssetInput) ([]string, errs.DomainError) {
	return a.svc.SelectAsset(in.ProfileRef, in.ProjectID, in.AssetID)
}

// UnselectAsset removes the asset id from the project's SelectedAssetIDs
// and returns the post-persistence selection slice. Idempotent on
// missing ids.
func (a *Actions) UnselectAsset(in UnselectAssetInput) ([]string, errs.DomainError) {
	return a.svc.UnselectAsset(in.ProfileRef, in.ProjectID, in.AssetID)
}
