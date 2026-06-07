// Package doctor produces a read-only health summary of a profile by planning
// each of its projects. It returns structured data; the TUI is responsible for
// rendering. Doctor never writes to disk; all mutation happens through
// internal/sync.
package doctor

import (
	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/profile"
	llmsync "github.com/hexworks/agentfiles/internal/sync"
)

// ProjectChange is the doctor-owned representation of a pending change.
// It mirrors the fields of llmsync.FileChange that are useful to render
// without dragging the sync vocabulary across doctor's API boundary.
type ProjectChange struct {
	Path   string
	Kind   llmsync.ChangeKind
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

func convertKind(k llmsync.ChangeKind) llmsync.ChangeKind {
	switch k {
	case llmsync.ChangeCreate:
		return llmsync.ChangeCreate
	case llmsync.ChangeUpdate:
		return llmsync.ChangeUpdate
	case llmsync.ChangeDrift:
		return llmsync.ChangeDrift
	case llmsync.ChangeDelete:
		return llmsync.ChangeDelete
	}
	return llmsync.ChangeKind(string(k))
}
