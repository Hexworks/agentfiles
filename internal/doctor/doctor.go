// Package doctor produces a read-only health summary of a profile by planning
// each of its projects. It returns structured data; the TUI is responsible for
// rendering. Doctor never writes to disk; all mutation happens through
// internal/sync.
package doctor

import (
	"fmt"

	"github.com/addamsson/agentfiles/internal/errs"
	"github.com/addamsson/agentfiles/internal/profile"
	llmsync "github.com/addamsson/agentfiles/internal/sync"
)

// ChangeKind classifies a single project change. Doctor owns its own
// vocabulary (rather than re-exporting llmsync.ChangeKind) so callers do
// not need to import sync just to read a report.
type ChangeKind string

const (
	ChangeCreate ChangeKind = "create"
	ChangeUpdate ChangeKind = "update"
	ChangeDrift  ChangeKind = "drift"
	ChangeDelete ChangeKind = "delete_candidate"
)

// ProjectChange is the doctor-owned representation of a pending change.
// It mirrors the fields of llmsync.FileChange that are useful to render
// without dragging the sync vocabulary across doctor's API boundary.
type ProjectChange struct {
	Path   string
	Kind   ChangeKind
	Reason string
}

// Report is the structured result of a profile health check. It contains one
// ProjectStatus per project that the profile owns.
type Report struct {
	ProfileName string
	Projects    []ProjectStatus
}

// ProjectStatus carries the per-project view of a Report. An empty Changes
// slice means the project is in sync; otherwise it lists every pending
// change the next apply would make.
type ProjectStatus struct {
	Name    string
	Changes []ProjectChange
}

// IsClean reports whether the project has no pending changes.
func (s ProjectStatus) IsClean() bool {
	return len(s.Changes) == 0
}

// ProjectCheckError reports a per-project plan failure encountered while
// building the report. Err preserves the underlying cause so callers can
// inspect domain leaves with errs.Collect.
type ProjectCheckError struct {
	ProjectName string
	Err         error
}

func (e ProjectCheckError) Error() string {
	return fmt.Sprintf("project %s: %s", e.ProjectName, e.Err.Error())
}

func (ProjectCheckError) Severity() errs.Severity {
	return errs.SeverityError
}

func (e ProjectCheckError) Unwrap() error {
	return e.Err
}

// CheckProfile runs a sync plan for every project owned by the profile and
// builds a structured Report. Per-project failures are accumulated as
// ProjectCheckError values so the user sees every problem at once;
// successful projects still appear in the returned report.
func CheckProfile(p *profile.Profile) (*Report, []errs.DomainError) {
	report := &Report{ProfileName: p.Manifest.Name}
	var domainErrs []errs.DomainError
	for _, proj := range p.ProjectList() {
		preview, err := llmsync.Plan(p, proj)
		if err != nil {
			domainErrs = append(domainErrs, ProjectCheckError{ProjectName: proj.Name, Err: err})
			continue
		}
		report.Projects = append(report.Projects, ProjectStatus{
			Name:    proj.Name,
			Changes: convertChanges(preview.Changes),
		})
	}
	return report, domainErrs
}

func convertChanges(in []llmsync.FileChange) []ProjectChange {
	if len(in) == 0 {
		return nil
	}
	out := make([]ProjectChange, len(in))
	for i, c := range in {
		out[i] = ProjectChange{
			Path:   c.Path,
			Kind:   convertKind(c.Kind),
			Reason: c.Reason,
		}
	}
	return out
}

func convertKind(k llmsync.ChangeKind) ChangeKind {
	switch k {
	case llmsync.ChangeCreate:
		return ChangeCreate
	case llmsync.ChangeUpdate:
		return ChangeUpdate
	case llmsync.ChangeDrift:
		return ChangeDrift
	case llmsync.ChangeDelete:
		return ChangeDelete
	}
	return ChangeKind(string(k))
}
