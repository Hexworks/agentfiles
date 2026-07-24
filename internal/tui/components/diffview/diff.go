// Package diffview renders a read-only unified diff between a managed
// (desired) body and its local on-disk counterpart, wired on top of
// [modal.Modal]. The domain layer returns the two raw bodies
// ([appapi.DiffBodies]); this package owns the presentation: it picks the
// diff direction by [appapi.ChangeKind], produces the unified-diff text with
// go-udiff, colors the added/removed lines, and drives a [viewport.Model] so
// the user can scroll.
package diffview

import (
	"strings"

	udiff "github.com/aymanbagabas/go-udiff"

	"github.com/hexworks/agentfiles/internal/appapi"
	"github.com/hexworks/agentfiles/internal/tui/styles"
)

// NoDifferencesMessage is shown in place of an empty pane when the two bodies
// are byte-identical (go-udiff returns an empty string).
const NoDifferencesMessage = "No differences."

// BuildDiff produces the colored unified-diff text between the local and
// desired bodies for a file row. The direction is kind-dependent so the `-`
// (old) and `+` (new) sides read naturally for what the user is about to do:
//
//   - ChangeUpdate: old = local (current on disk), new = desired (incoming
//     managed) — the diff previews what an Apply would change.
//   - ChangeDrift: old = desired (managed baseline), new = local (the drifted
//     edit) — the diff shows how the local file diverged from what agentfiles
//     wrote.
//
// Any other kind falls back to the update direction; the Plan Project screen
// only offers [Diff] on update / drift rows, so that branch is unreachable in
// practice and exists only to keep BuildDiff total. Byte-identical inputs
// yield [NoDifferencesMessage] instead of a blank pane.
func BuildDiff(kind appapi.ChangeKind, local, desired []byte) string {
	var oldLabel, newLabel, oldBody, newBody string
	switch kind {
	case appapi.ChangeDrift:
		oldLabel, newLabel = "managed", "local"
		oldBody, newBody = string(desired), string(local)
	default:
		oldLabel, newLabel = "current", "incoming"
		oldBody, newBody = string(local), string(desired)
	}
	unified := udiff.Unified(oldLabel, newLabel, oldBody, newBody)
	if unified == "" {
		return NoDifferencesMessage
	}
	return colorize(unified)
}

// colorize paints each diff line by role so added / removed content is
// distinguishable without relying on color alone — the `+` / `-` / `@@`
// glyphs stay in the text. File-header lines (`+++` / `---`) and hunk headers
// (`@@`) render muted; body additions green, deletions red.
func colorize(diff string) string {
	lines := strings.Split(diff, "\n")
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"), strings.HasPrefix(line, "@@"):
			lines[i] = styles.MutedStyle.Render(line)
		case strings.HasPrefix(line, "+"):
			lines[i] = styles.CreateStyle.Render(line)
		case strings.HasPrefix(line, "-"):
			lines[i] = styles.DeleteStyle.Render(line)
		}
	}
	return strings.Join(lines, "\n")
}
