package diffview

import (
	"strings"
	"testing"

	"github.com/hexworks/agentfiles/internal/appapi"
)

// The colored output wraps each line with lipgloss styling but keeps the
// leading `+`/`-` glyph and the line text contiguous, so substring assertions
// on `-content` / `+content` hold whether or not the test environment emits
// ANSI color.

func TestBuildDiff_UpdateDirection(t *testing.T) {
	// given: an update row — old side is the local (current) body, new side is
	// the desired (incoming) body.
	local := []byte("common\nlocalonly\n")
	desired := []byte("common\ndesiredonly\n")

	// when
	out := BuildDiff(appapi.ChangeUpdate, local, desired)

	// then: the local-only line is removed, the desired-only line is added.
	if !strings.Contains(out, "-localonly") {
		t.Errorf("update diff missing removed local line:\n%s", out)
	}
	if !strings.Contains(out, "+desiredonly") {
		t.Errorf("update diff missing added desired line:\n%s", out)
	}
}

func TestBuildDiff_DriftDirection(t *testing.T) {
	// given: a drift row — old side is the managed (desired) baseline, new side
	// is the drifted local body. The sides flip relative to update.
	local := []byte("common\nlocalonly\n")
	desired := []byte("common\ndesiredonly\n")

	// when
	out := BuildDiff(appapi.ChangeDrift, local, desired)

	// then: the managed line is the removed side, the local edit the added side.
	if !strings.Contains(out, "-desiredonly") {
		t.Errorf("drift diff missing removed managed line:\n%s", out)
	}
	if !strings.Contains(out, "+localonly") {
		t.Errorf("drift diff missing added local line:\n%s", out)
	}
}

func TestBuildDiff_EqualBodiesShowsNoDifferences(t *testing.T) {
	body := []byte("identical\ncontent\n")

	if got := BuildDiff(appapi.ChangeUpdate, body, body); got != NoDifferencesMessage {
		t.Errorf("equal bodies = %q, want %q", got, NoDifferencesMessage)
	}
	if got := BuildDiff(appapi.ChangeDrift, body, body); got != NoDifferencesMessage {
		t.Errorf("equal bodies (drift) = %q, want %q", got, NoDifferencesMessage)
	}
}
