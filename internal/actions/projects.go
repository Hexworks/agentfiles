package actions

import (
	"github.com/hexworks/agentfiles/internal/app"
	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/project"
)

// AddProject creates a new per-profile project manifest. Validation
// failures from the Service accumulator are collapsed into errs.Errors.
func (a *Actions) AddProject(in AddProjectInput) (*project.Manifest, errs.DomainError) {
	manifest, es := a.svc.AddProject(in.ProfileRef, in.Name, in.Path, in.EnabledAgents, in.AssetIDs)
	return collapse(manifest, es)
}

func (a *Actions) LoadProject(in LoadProjectInput) (*project.Manifest, errs.DomainError) {
	return a.svc.LoadProject(in.ProfileRef, in.ProjectID)
}

func (a *Actions) UpdateProject(in UpdateProjectInput) (struct{}, errs.DomainError) {
	return struct{}{}, a.svc.UpdateProject(in.ProfileRef, in.ProjectID, in.Name, in.Path, in.EnabledAgents)
}

func (a *Actions) DeleteProject(in DeleteProjectInput) (struct{}, errs.DomainError) {
	return struct{}{}, a.svc.DeleteProject(in.ProfileRef, in.ProjectID)
}

func (a *Actions) PlanProject(in PlanProjectInput) (*app.Preview, errs.DomainError) {
	return a.svc.Plan(in.ProfileRef, in.ProjectID)
}

// SyncProject applies the desired files to the project repository,
// honoring the per-file Drift and Unknown resolutions in the input.
func (a *Actions) SyncProject(in SyncProjectInput) (*app.Preview, errs.DomainError) {
	return a.svc.Apply(in.ProfileRef, in.ProjectID, in.Drift, in.Unknown)
}
