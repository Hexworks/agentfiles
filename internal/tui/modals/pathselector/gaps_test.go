package pathselector

import (
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/hexworks/agentfiles/internal/tui/components/modal"
)

// TestClassifyDanglingSymlinkFallsBackToFileSymlink covers the branch in
// classify where os.Stat on a symlink target fails (dangling symlink).
// The dirent must still surface — as an entryFileSymlink — so the user
// can see the broken link rather than have it silently disappear.
func TestClassifyDanglingSymlinkFallsBackToFileSymlink(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.Symlink(filepath.Join(dir, "gone"), filepath.Join(dir, "dangling")); err != nil {
		t.Fatal(err)
	}
	dirents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirents) != 1 {
		t.Fatalf("dirents = %d, want 1", len(dirents))
	}
	kind := classify(dirents[0], filepath.Join(dir, dirents[0].Name()))
	if kind != entryFileSymlink {
		t.Fatalf("kind = %v, want entryFileSymlink", kind)
	}
}

// TestSelectCurrentOnEmptyFolder covers [Select current] resolving with the
// empty folder's path — an edge case that TestSelectCurrentDir does not
// exercise because it opens on a non-empty folder.
func TestSelectCurrentOnEmptyFolder(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	c := mustNew(t, Options{ConstraintRoot: root, StartFolder: root})
	// Listing is a single entryEmpty placeholder.
	if len(c.entries) != 1 || c.entries[0].Kind != entryEmpty {
		t.Fatalf("entries = %v, want single entryEmpty", c.entries)
	}
	_, _ = c.Update(keyPress("c", 'c'))
	assertConfirmed(t, c, Result{Path: root, IsDir: true})
}

// TestHeaderRowEnterAndSelectAreNoOps covers Enter and `s` while the cursor
// sits on the synthetic header row: both are silent no-ops that leave the
// modal Active. Mnemonic uniqueness asserts this indirectly by refusing to
// register [Select] on the header row; this test proves the runtime
// behavior directly.
func TestHeaderRowEnterAndSelectAreNoOps(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "sub"))
	c := mustNew(t, Options{ConstraintRoot: root, StartFolder: root})
	// Cursor starts on the header (index 0) after Init.
	if got := c.cursorEntry().Kind; got != entryHeader {
		t.Fatalf("initial cursor kind = %v, want entryHeader", got)
	}
	before := c.current
	_, _ = c.Update(keyPress("enter", tea.KeyEnter))
	if state, _ := c.Lifecycle(); state != modal.Active {
		t.Fatalf("Enter on header changed state to %v", state)
	}
	if c.current != before {
		t.Fatalf("Enter on header changed current: %q → %q", before, c.current)
	}
	_, _ = c.Update(keyPress("s", 's'))
	if state, _ := c.Lifecycle(); state != modal.Active {
		t.Fatalf("`s` on header changed state to %v", state)
	}
}

// TestUnconstrainedNavigation covers the Constraint == "" branch: the modal
// should let the user navigate freely (no `..` suppression) and never emit
// a ConstraintViolationMsg.
func TestUnconstrainedNavigation(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	inner := filepath.Join(root, "inner")
	mustMkdir(t, inner)
	// StartFolder set to inner; no ConstraintRoot means no fence. The
	// listing must carry a `..` row (there is no constraint root to
	// suppress it against).
	c := mustNew(t, Options{StartFolder: inner})
	if c.opts.constraint != "" {
		t.Fatalf("constraint = %q, want empty", c.opts.constraint)
	}
	if c.root != nil {
		t.Fatalf("*os.Root must be nil when unconstrained")
	}
	hasParent := false
	for _, e := range c.entries {
		if e.Kind == entryParent {
			hasParent = true
			break
		}
	}
	if !hasParent {
		t.Fatalf("expected `..` row in unconstrained mode; got %v", entryNames(c.entries))
	}
	// Navigating up must not emit a violation.
	cmd := c.navigate(root)
	if got := drainConstraintViolations(t, cmd); len(got) != 0 {
		t.Fatalf("unconstrained navigation must not violate constraint; got %v", got)
	}
	if c.current != root {
		t.Fatalf("current = %q, want %q", c.current, root)
	}
}

// TestHiddenButtonLabelSwapsOnToggle covers the visible side of AC #8: the
// button label flips between "Show hidden" and "Hide hidden" on each press.
// TestHiddenToggle only checks that the listing changes, not the label.
func TestHiddenButtonLabelSwapsOnToggle(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	c := mustNew(t, Options{ConstraintRoot: root, StartFolder: root})
	if got := c.hiddenBtn.Label(); got != "Show hidden" {
		t.Fatalf("initial label = %q, want %q", got, "Show hidden")
	}
	_, _ = c.Update(keyPress("h", 'h'))
	if got := c.hiddenBtn.Label(); got != "Hide hidden" {
		t.Fatalf("after 1x h, label = %q, want %q", got, "Hide hidden")
	}
	_, _ = c.Update(keyPress("h", 'h'))
	if got := c.hiddenBtn.Label(); got != "Show hidden" {
		t.Fatalf("after 2x h, label = %q, want %q", got, "Show hidden")
	}
}
