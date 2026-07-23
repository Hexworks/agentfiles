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
// honoring the per-file Drift and Unknown resolutions plus the ignored
// folder keys in the input. When git integration is enabled and the
// target repo is a git repository a single scoped commit is recorded
// after the file writes succeed; the outcome rides on the returned
// CommitOutcome.
func (a *Actions) SyncProject(in SyncProjectInput) (*app.Preview, app.CommitOutcome, errs.DomainError) {
	return a.svc.Apply(in.ProfileRef, in.ProjectID, app.Resolutions{
		Drift:        in.Drift,
		Unknown:      in.Unknown,
		IgnoredPaths: in.IgnoredPaths,
	})
}
