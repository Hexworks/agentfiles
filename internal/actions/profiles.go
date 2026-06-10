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

// LoadProfile resolves the profile reference (id, name, or path) and
// returns the fully loaded profile model.
func (a *Actions) LoadProfile(in LoadProfileInput) (*profile.Profile, errs.DomainError) {
	return a.svc.LoadProfile(in.ProfileRef)
}

// CreateProfile scaffolds a new profile folder and registers it.
func (a *Actions) CreateProfile(in CreateProfileInput) (*registry.ProfileRef, errs.DomainError) {
	return a.svc.CreateProfile(in.Name, in.Path)
}

// RegisterProfile adopts an existing profile folder into the registry.
func (a *Actions) RegisterProfile(in RegisterProfileInput) (*registry.ProfileRef, errs.DomainError) {
	return a.svc.RegisterProfile(in.Path)
}

// DeleteProfile dispatches on FolderAction: KeepFolders removes the
// registry entry only; DeleteFolders removes both.
func (a *Actions) DeleteProfile(in DeleteProfileInput) (struct{}, errs.DomainError) {
	var err errs.DomainError
	if in.FolderAction == DeleteFolders {
		err = a.svc.DeleteProfileWithFolder(in.ProfileRef)
	} else {
		err = a.svc.DeleteProfile(in.ProfileRef)
	}
	return struct{}{}, err
}
