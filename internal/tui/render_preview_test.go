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

func TestRenderPreview_ListsEachChangeWithIcon(t *testing.T) {
	preview := &llmsync.Preview{
		ProjectPath: "/tmp/repo",
		Files:       []render.RenderedFile{{Path: "AGENTS.md"}},
		Changes: []llmsync.FileChange{
			{Path: "AGENTS.md", Kind: llmsync.ChangeCreate, Reason: "file missing"},
			{Path: ".claude/settings.local.json", Kind: llmsync.ChangeUpdate, Reason: "content differs"},
			{Path: ".codex/old.txt", Kind: llmsync.ChangeDelete, Reason: "recognized llm file not selected"},
			{Path: "CLAUDE.md", Kind: llmsync.ChangeDrift, Reason: "managed file changed locally"},
			{Path: ".cursor/stray.md", Kind: llmsync.ChangeUnknown, Reason: "unrecognized file in managed surface"},
		},
	}

	out := ansi.Strip(RenderPreview(preview))

	for _, want := range []string{
		"+ [create] AGENTS.md: file missing",
		"~ [update] .claude/settings.local.json: content differs",
		"- [delete] .codex/old.txt: recognized llm file not selected",
		"! [drift] CLAUDE.md: managed file changed locally",
		"? [unknown] .cursor/stray.md: unrecognized file in managed surface",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}
