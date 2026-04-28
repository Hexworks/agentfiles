package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/addamsson/agentfiles/internal/app"
	"github.com/addamsson/agentfiles/internal/errs"
	"github.com/addamsson/agentfiles/internal/render"
)

func TestRenderError_TypedErrorRendersWithIcon(t *testing.T) {
	err := render.AssetNotFoundError{AssetID: "review"}

	out := RenderError(err)

	if !strings.Contains(out, "✗") {
		t.Fatalf("expected error icon, got %q", out)
	}
	if !strings.Contains(out, "review") {
		t.Fatalf("expected asset id in output, got %q", out)
	}
}

func TestRenderError_WarningSeverityForOwnedPath(t *testing.T) {
	err := app.ProjectPathOwnedError{Path: "/tmp/repo", ProfileName: "Other", ProjectName: "Repo"}

	out := RenderError(err)

	if !strings.Contains(out, "⚠") {
		t.Fatalf("expected warning icon, got %q", out)
	}
}

func TestRenderError_JoinedErrorsRenderEachOnItsOwnLine(t *testing.T) {
	err := errors.Join(
		render.AssetNotFoundError{AssetID: "review"},
		render.AssetNotFoundError{AssetID: "missing"},
	)

	out := RenderError(err)

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d:\n%s", len(lines), out)
	}
	if !strings.Contains(out, "review") || !strings.Contains(out, "missing") {
		t.Fatalf("expected both ids, got %q", out)
	}
}

func TestRenderError_UnknownErrorFallsBackToErrorIcon(t *testing.T) {
	err := errors.New("something else")

	out := RenderError(err)

	if !strings.Contains(out, "✗") {
		t.Fatalf("expected error icon for unknown error, got %q", out)
	}
	if !strings.Contains(out, "something else") {
		t.Fatalf("expected message preserved, got %q", out)
	}
}

func TestRenderError_NilReturnsEmptyString(t *testing.T) {
	if out := RenderError(nil); out != "" {
		t.Fatalf("expected empty string for nil error, got %q", out)
	}
}

func TestRenderErrors_RendersSliceOfDomainErrors(t *testing.T) {
	out := RenderErrors([]errs.DomainError{
		render.AssetNotFoundError{AssetID: "a"},
		app.ProjectPathOwnedError{Path: "/p", ProfileName: "X", ProjectName: "Y"},
	})

	if !strings.Contains(out, "✗") || !strings.Contains(out, "⚠") {
		t.Fatalf("expected both icons, got %q", out)
	}
}
