package modals

import (
	"strings"
	"testing"

	"charm.land/huh/v2"

	"github.com/hexworks/agentfiles/internal/tui/styles"
)

// TestPathDisplayNote_SanitisesControlRunes — task 0041: the picked path
// is raw filesystem bytes, so a control rune (raw ESC) could otherwise
// reach the rendered frame and hijack the terminal. pathDisplayNote must
// pass the value through styles.Safe, which quotes the whole string on
// any hostile rune. The rendered view must contain the escaped literal
// `\x1b` (four chars), proving Safe ran, and must not embed the raw ESC
// byte.
func TestPathDisplayNote_SanitisesControlRunes(t *testing.T) {
	const hostile = "~/re\x1bpo"

	form := huh.NewForm(huh.NewGroup(pathDisplayNote(hostile, "picked"))).
		WithTheme(styles.HuhTheme())
	form.Init()
	view := form.View()

	// Positive proof: the full sanitised path — `re\x1bpo` with a literal
	// backslash-x-1-b — must appear, pinning that styles.Safe quoted the
	// injected bytes rather than four stray chars coinciding elsewhere.
	if !strings.Contains(view, `re\x1bpo`) {
		t.Errorf("view missing escaped path %q; styles.Safe did not run:\n%s", `re\x1bpo`, view)
	}
	// Negative proof runs unconditionally. A lipgloss theme legitimately
	// emits ESC-based SGR colour codes, so a bare ESC in the frame is
	// expected; what must never appear is the *injected* raw sequence — the
	// path bytes `re<ESC>po` verbatim.
	if strings.Contains(view, "re\x1bpo") {
		t.Errorf("view contains the raw injected control sequence %q:\n%q", "re\x1bpo", view)
	}
}

// TestPathDisplayNote_CleanPathRendersVerbatim — Safe leaves a safe
// string untouched, so a control-free path still appears exactly as
// picked (no quoting, no stripping).
func TestPathDisplayNote_CleanPathRendersVerbatim(t *testing.T) {
	const clean = "/tmp/seed"

	form := huh.NewForm(huh.NewGroup(pathDisplayNote(clean, "picked"))).
		WithTheme(styles.HuhTheme())
	form.Init()

	if view := form.View(); !strings.Contains(view, clean) {
		t.Errorf("view missing clean path %q:\n%s", clean, view)
	}
}
