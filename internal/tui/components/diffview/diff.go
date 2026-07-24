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

// NotDiffableMessage is shown when BuildDiff is handed a ChangeKind that lacks
// one side of the diff (create/delete/unknown). The Plan Project screen only
// offers [Diff] on update / drift rows, so this is a degrade-visibly guard for
// a future widened gate — never a normal path — rather than a one-sided diff.
const NotDiffableMessage = "This change kind cannot be diffed."

// BuildDiff produces the colored unified-diff text between the local and
// desired bodies for a file row. The direction is kind-dependent so the `-`
// (old) and `+` (new) sides read naturally, and both branches name the two
// sides with the glossary vocabulary (managed vs local) rather than net-new
// labels:
//
//   - ChangeUpdate: old = local (on disk), new = managed (desired) — the diff
//     previews what an Apply would change.
//   - ChangeDrift: old = managed (desired baseline), new = local (the drifted
//     edit) — the diff shows how the local file diverged from what agentfiles
//     wrote.
//
// Only update and drift rows carry both a desired and a local body, so any
// other kind is not diffable and returns [NotDiffableMessage] — a widened
// [Diff] gate then degrades visibly instead of rendering a one-sided diff.
// Byte-identical inputs yield [NoDifferencesMessage] instead of a blank pane.
func BuildDiff(kind appapi.ChangeKind, local, desired []byte) string {
	var oldLabel, newLabel, oldBody, newBody string
	switch kind {
	case appapi.ChangeUpdate:
		oldLabel, newLabel = "local", "managed"
		oldBody, newBody = string(local), string(desired)
	case appapi.ChangeDrift:
		oldLabel, newLabel = "managed", "local"
		oldBody, newBody = string(desired), string(local)
	default:
		return NotDiffableMessage
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
