package tui

import (
	"strings"
	"testing"

	"github.com/addamsson/agentfiles/internal/doctor"
)

func TestRenderReport_CleanProjectShowsCleanLine(t *testing.T) {
	report := &doctor.Report{
		ProfileName: "Personal",
		Projects: []doctor.ProjectStatus{
			{Name: "app"},
		},
	}

	out := RenderReport(report)

	if !strings.Contains(out, "Profile: Personal") {
		t.Fatalf("missing profile header: %q", out)
	}
	if !strings.Contains(out, "[app]") {
		t.Fatalf("missing project header: %q", out)
	}
	if !strings.Contains(out, "clean") {
		t.Fatalf("missing clean line: %q", out)
	}
}

func TestRenderReport_DirtyProjectListsChanges(t *testing.T) {
	report := &doctor.Report{
		ProfileName: "Personal",
		Projects: []doctor.ProjectStatus{
			{
				Name: "app",
				Changes: []doctor.ProjectChange{
					{Path: "AGENTS.md", Kind: doctor.ChangeUpdate},
					{Path: "CLAUDE.md", Kind: doctor.ChangeDrift},
				},
			},
		},
	}

	out := RenderReport(report)

	if !strings.Contains(out, "~ update AGENTS.md") {
		t.Fatalf("missing update line: %q", out)
	}
	if !strings.Contains(out, "! drift CLAUDE.md") {
		t.Fatalf("missing drift line: %q", out)
	}
	if strings.Contains(out, "clean") {
		t.Fatalf("dirty report should not include clean: %q", out)
	}
}

func TestRenderReport_MixedCleanAndDirtyProjects(t *testing.T) {
	report := &doctor.Report{
		ProfileName: "Personal",
		Projects: []doctor.ProjectStatus{
			{Name: "app-clean"},
			{
				Name: "app-dirty",
				Changes: []doctor.ProjectChange{
					{Path: "AGENTS.md", Kind: doctor.ChangeUpdate},
				},
			},
		},
	}

	out := RenderReport(report)

	if !strings.Contains(out, "[app-clean]") {
		t.Fatalf("missing clean project header: %q", out)
	}
	if !strings.Contains(out, "[app-dirty]") {
		t.Fatalf("missing dirty project header: %q", out)
	}
	cleanIdx := strings.Index(out, "[app-clean]")
	dirtyIdx := strings.Index(out, "[app-dirty]")
	if cleanIdx > dirtyIdx {
		t.Fatalf("expected clean before dirty in output:\n%s", out)
	}
	cleanLine := strings.Index(out, "clean")
	if cleanLine < cleanIdx || cleanLine > dirtyIdx {
		t.Fatalf("expected clean line under app-clean, output:\n%s", out)
	}
	if !strings.Contains(out[dirtyIdx:], "~ update AGENTS.md") {
		t.Fatalf("expected update line under app-dirty, output:\n%s", out)
	}
}
