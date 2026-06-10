package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/hexworks/agentfiles/internal/render"
	llmsync "github.com/hexworks/agentfiles/internal/sync"
)

func TestRenderPreview_NoChangesShowsCleanLine(t *testing.T) {
	preview := &llmsync.Preview{ProjectPath: "/tmp/repo"}

	out := ansi.Strip(RenderPreview(preview))

	if !strings.Contains(out, "Project: /tmp/repo") {
		t.Fatalf("missing project header: %q", out)
	}
	if !strings.Contains(out, "No changes.") {
		t.Fatalf("missing clean line: %q", out)
	}
}

func TestRenderPreview_FirstApplyShowsBanner(t *testing.T) {
	preview := &llmsync.Preview{ProjectPath: "/tmp/repo", FirstApply: true}

	out := ansi.Strip(RenderPreview(preview))

	if !strings.Contains(out, "First apply") {
		t.Fatalf("missing first-apply banner: %q", out)
	}
}

func TestRenderPreview_ListsEachChangeWithIcon(t *testing.T) {
	preview := &llmsync.Preview{
		ProjectPath: "/tmp/repo",
		Files:       []render.RenderedFile{{Path: "AGENTS.md"}},
		Changes: []llmsync.FileChange{
			{Path: "AGENTS.md", Kind: llmsync.ChangeCreate, Reason: llmsync.ReasonFileMissing},
			{Path: ".claude/settings.local.json", Kind: llmsync.ChangeUpdate, Reason: llmsync.ReasonContentDiffers},
			{Path: ".codex/old.txt", Kind: llmsync.ChangeDelete, Reason: llmsync.ReasonStateRecordedDelete},
			{Path: "CLAUDE.md", Kind: llmsync.ChangeDrift, Reason: llmsync.ReasonDriftDetected},
			{Path: ".cursor/stray.md", Kind: llmsync.ChangeUnknown, Reason: llmsync.ReasonUnknown},
		},
	}

	out := ansi.Strip(RenderPreview(preview))

	for _, want := range []string{
		"+ [create] AGENTS.md: file missing",
		"~ [update] .claude/settings.local.json: content differs",
		"- [delete] .codex/old.txt: recognized llm file not selected",
		"! [drift] CLAUDE.md: managed file changed locally",
		"? [unknown] .cursor/stray.md: unknown file in managed surface",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

// TestRenderPreview_UnknownUsesDriftStyle pins that the ChangeUnknown
// branch in changeStyle wires up driftStyle (magenta), distinct from
// the mutedStyle fallback (dim grey). Asserting on the styled (non-
// stripped) output is the only way to tell the two `?` glyphs apart.
func TestRenderPreview_UnknownUsesDriftStyle(t *testing.T) {
	preview := &llmsync.Preview{
		ProjectPath: "/tmp/repo",
		Changes: []llmsync.FileChange{
			{Path: ".cursor/stray.md", Kind: llmsync.ChangeUnknown, Reason: llmsync.ReasonUnknown},
		},
	}

	out := RenderPreview(preview)

	wantLine := driftStyle.Render("? [unknown] .cursor/stray.md: unknown file in managed surface")
	if !strings.Contains(out, wantLine) {
		t.Fatalf("expected drift-styled ChangeUnknown line, got:\n%q\nwant substring:\n%q", out, wantLine)
	}
	mutedLine := mutedStyle.Render("? [unknown] .cursor/stray.md: unknown file in managed surface")
	if strings.Contains(out, mutedLine) {
		t.Fatalf("ChangeUnknown should not render with mutedStyle fallback, got:\n%q", out)
	}
}
