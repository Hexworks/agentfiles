package actions

import (
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

func (a *Actions) UpdateAsset(in UpdateAssetInput) (struct{}, errs.DomainError) {
	return struct{}{}, a.svc.UpdateAsset(in.ProfileRef, in.Manifest)
}

// DeleteAsset removes the asset from the profile and unselects it from
// every project that referenced it.
func (a *Actions) DeleteAsset(in DeleteAssetInput) (struct{}, errs.DomainError) {
	return struct{}{}, a.svc.DeleteAsset(in.ProfileRef, in.AssetID)
}
