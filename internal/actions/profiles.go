package actions

import (
	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/profile"
	"github.com/hexworks/agentfiles/internal/registry"
)

// LoadProfiles returns every registered profile, fully loaded. Per-
// profile load errors are collapsed into a single errs.Errors so the
// action surface stays uniform.
func (a *Actions) LoadProfiles() ([]*profile.Profile, errs.DomainError) {
	profiles, es := a.svc.LoadProfiles()
	return collapse(profiles, es)
}

func (a *Actions) LoadProfile(in LoadProfileInput) (*profile.Profile, errs.DomainError) {
	return a.svc.LoadProfile(in.ProfileRef)
}

func (a *Actions) CreateProfile(in CreateProfileInput) (*registry.ProfileRef, errs.DomainError) {
	return a.svc.CreateProfile(in.Name, in.Path)
}

func (a *Actions) RegisterProfile(in RegisterProfileInput) (*registry.ProfileRef, errs.DomainError) {
	return a.svc.RegisterProfile(in.Path)
}

// DeleteProfile removes the registry entry. The on-disk profile folder
// is left untouched; use DeleteProfileWithFolder for the destructive
// variant.
func (a *Actions) DeleteProfile(in DeleteProfileInput) (struct{}, errs.DomainError) {
	return struct{}{}, a.svc.DeleteProfile(in.ProfileRef)
}

// DeleteProfileWithFolder removes both the registry entry and the
// on-disk profile folder. Guarded by Service-side safety checks.
func (a *Actions) DeleteProfileWithFolder(in DeleteProfileInput) (struct{}, errs.DomainError) {
	return struct{}{}, a.svc.DeleteProfileWithFolder(in.ProfileRef)
}
