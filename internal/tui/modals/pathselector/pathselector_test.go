package pathselector

import (
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/hexworks/agentfiles/internal/tui/components/modal"
)

// TestConstraintRootUpwardNoop — AC #1. At the constraint root the `..` row
// is absent, so upward navigation is impossible; an explicit navigate() to
// the parent of the constraint root must emit a ConstraintViolationMsg and
// leave the current folder untouched.
func TestConstraintRootUpwardNoop(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "alpha"))
	c := mustNew(t, Options{ConstraintRoot: root, StartFolder: root})

	// No `..` in the listing.
	for _, e := range c.entries {
		if e.Kind == entryParent {
			t.Fatalf("entryParent should not appear at constraint root; got %+v", c.entries)
		}
	}

	// Direct escape attempt: navigate to the parent of the resolved
	// constraint root. Must emit ConstraintViolationMsg and stay put. This
	// exercises the constraint gate itself, not the treetable cursor
	// clamping — which is what the older arrow-up sanity really tested.
	before := c.current
	cmd := c.navigate(filepath.Dir(root))
	if state, _ := c.Lifecycle(); state != modal.Active {
		t.Fatalf("lifecycle = %v, want Active", state)
	}
	if c.current != before {
		t.Fatalf("current changed after escape attempt: %q → %q", before, c.current)
	}
	viols := drainConstraintViolations(t, cmd)
	if len(viols) != 1 {
		t.Fatalf("expected 1 ConstraintViolationMsg, got %d (%v)", len(viols), viols)
	}
	if viols[0].Constraint != c.opts.constraint {
		t.Fatalf("constraint = %q, want %q", viols[0].Constraint, c.opts.constraint)
	}
}

// TestSymlinkEscapeRejected — AC #2. Enter on a directory symlink whose
// target resolves outside ConstraintRoot emits a ConstraintViolationMsg
// and leaves the current folder untouched.
func TestSymlinkEscapeRejected(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	other := t.TempDir()
	if err := os.Symlink(other, filepath.Join(root, "out")); err != nil {
		t.Fatal(err)
	}
	c := mustNew(t, Options{ConstraintRoot: root, StartFolder: root, FollowSymlinks: true})

	// Move cursor onto `out@`.
	before := c.current
	moveCursorTo(t, c, "out@")
	_, cmd := c.Update(keyPress("enter", tea.KeyEnter))
	if state, _ := c.Lifecycle(); state != modal.Active {
		t.Fatalf("lifecycle = %v, want Active", state)
	}
	if c.current != before {
		t.Fatalf("current changed after escape attempt: %q → %q", before, c.current)
	}
	viols := drainConstraintViolations(t, cmd)
	if len(viols) != 1 {
		t.Fatalf("expected 1 ConstraintViolationMsg, got %d (%v)", len(viols), viols)
	}
}

// TestSymlinkNoFollow — AC #3. With FollowSymlinks=false, Enter on a
// directory symlink is a silent no-op.
func TestSymlinkNoFollow(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	target := filepath.Join(root, "inside")
	mustMkdir(t, target)
	if err := os.Symlink(target, filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}
	c := mustNew(t, Options{ConstraintRoot: root, StartFolder: root, FollowSymlinks: false})

	moveCursorTo(t, c, "alias@")
	before := c.current
	_, cmd := c.Update(keyPress("enter", tea.KeyEnter))
	if c.current != before {
		t.Fatalf("current changed: %q → %q", before, c.current)
	}
	if got := drainConstraintViolations(t, cmd); len(got) != 0 {
		t.Fatalf("expected no ConstraintViolationMsg, got %v", got)
	}
	if got := drainReadDirErrors(t, cmd); len(got) != 0 {
		t.Fatalf("expected no ReadDirErrorMsg, got %v", got)
	}
	if state, _ := c.Lifecycle(); state != modal.Active {
		t.Fatalf("lifecycle = %v, want Active", state)
	}
}

// TestHiddenToggle — AC #8. Pressing `h` toggles dotfile visibility.
func TestHiddenToggle(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, ".secret"))
	mustWriteFile(t, filepath.Join(root, "visible.md"))
	c := mustNew(t, Options{ConstraintRoot: root, StartFolder: root, ShowFiles: true})

	if hasEntryNamed(c, ".secret") {
		t.Fatalf("initial listing must hide .secret; got %v", entryNames(c.entries))
	}
	_, _ = c.Update(keyPress("h", 'h'))
	if !hasEntryNamed(c, ".secret") {
		t.Fatalf("after `h`, .secret should be visible; got %v", entryNames(c.entries))
	}
	_, _ = c.Update(keyPress("h", 'h'))
	if hasEntryNamed(c, ".secret") {
		t.Fatalf("after second `h`, .secret should be hidden again; got %v", entryNames(c.entries))
	}
}

// TestEnterSemantics — AC #9. Enter on a folder navigates in; Enter on a file
// is a no-op.
func TestEnterSemantics(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "inside"))
	mustWriteFile(t, filepath.Join(root, "note.md"))
	c := mustNew(t, Options{ConstraintRoot: root, StartFolder: root, ShowFiles: true})

	moveCursorTo(t, c, "inside/")
	_, _ = c.Update(keyPress("enter", tea.KeyEnter))
	if c.current != filepath.Join(root, "inside") {
		t.Fatalf("current = %q, want %q", c.current, filepath.Join(root, "inside"))
	}

	// Navigate back and try Enter on file.
	moveCursorTo(t, c, "..")
	_, _ = c.Update(keyPress("enter", tea.KeyEnter))
	if c.current != root {
		t.Fatalf("after `..` Enter, current = %q, want %q", c.current, root)
	}
	moveCursorTo(t, c, "note.md")
	before := c.current
	_, _ = c.Update(keyPress("enter", tea.KeyEnter))
	if c.current != before {
		t.Fatalf("Enter on file should be no-op; current = %q, want %q", c.current, before)
	}
	if state, _ := c.Lifecycle(); state != modal.Active {
		t.Fatalf("lifecycle = %v, want Active", state)
	}
}

// TestRowSelectResolvesResult — AC #10. `s` on a file / folder / `..` row
// resolves the modal with the correct Result.
func TestRowSelectResolvesResult(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	inside := filepath.Join(root, "inside")
	mustMkdir(t, inside)
	filePath := filepath.Join(inside, "note.md")
	mustWriteFile(t, filePath)

	t.Run("file", func(t *testing.T) {
		t.Parallel()
		c := mustNew(t, Options{ConstraintRoot: root, StartFolder: inside, ShowFiles: true})
		moveCursorTo(t, c, "note.md")
		_, _ = c.Update(keyPress("s", 's'))
		assertConfirmed(t, c, Result{Path: filePath, IsDir: false})
	})

	t.Run("folder", func(t *testing.T) {
		t.Parallel()
		c := mustNew(t, Options{ConstraintRoot: root, StartFolder: root})
		moveCursorTo(t, c, "inside/")
		_, _ = c.Update(keyPress("s", 's'))
		assertConfirmed(t, c, Result{Path: inside, IsDir: true})
	})

	t.Run("parent", func(t *testing.T) {
		t.Parallel()
		c := mustNew(t, Options{ConstraintRoot: root, StartFolder: inside})
		moveCursorTo(t, c, "..")
		_, _ = c.Update(keyPress("s", 's'))
		assertConfirmed(t, c, Result{Path: root, IsDir: true})
	})
}

// TestSelectCurrentDir — AC #11. `c` resolves the modal with the folder
// currently being browsed regardless of cursor position.
func TestSelectCurrentDir(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "note.md"))
	c := mustNew(t, Options{ConstraintRoot: root, StartFolder: root, ShowFiles: true})

	moveCursorTo(t, c, "note.md")
	_, _ = c.Update(keyPress("c", 'c'))
	assertConfirmed(t, c, Result{Path: root, IsDir: true})
}

// TestReadDirDeniedNotifies — AC #12. A permission-denied ReadDir on Enter
// leaves the modal on the previous folder and emits a ReadDirErrorMsg.
func TestReadDirDeniedNotifies(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("running as root; chmod-based deny does not apply")
	}
	root := t.TempDir()
	locked := filepath.Join(root, "locked")
	mustMkdir(t, locked)
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	c := mustNew(t, Options{ConstraintRoot: root, StartFolder: root})
	moveCursorTo(t, c, "locked/")
	before := c.current
	_, cmd := c.Update(keyPress("enter", tea.KeyEnter))
	if c.current != before {
		t.Fatalf("current changed after denied Enter: %q → %q", before, c.current)
	}
	errs := drainReadDirErrors(t, cmd)
	if len(errs) != 1 {
		t.Fatalf("expected 1 ReadDirErrorMsg, got %d (%v)", len(errs), errs)
	}
	if state, _ := c.Lifecycle(); state != modal.Active {
		t.Fatalf("lifecycle = %v, want Active", state)
	}
}

// TestCancelPaths — AC #13. Both Esc and the `n` mnemonic resolve the modal
// with Confirmed=false / Value=nil.
func TestCancelPaths(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	t.Run("esc", func(t *testing.T) {
		t.Parallel()
		c := mustNew(t, Options{ConstraintRoot: root, StartFolder: root})
		_, _ = c.Update(keyPress("esc", tea.KeyEsc))
		state, value := c.Lifecycle()
		if state != modal.Cancelled {
			t.Fatalf("state = %v, want Cancelled", state)
		}
		if value != nil {
			t.Fatalf("value = %v, want nil", value)
		}
	})

	t.Run("mnemonic n", func(t *testing.T) {
		t.Parallel()
		c := mustNew(t, Options{ConstraintRoot: root, StartFolder: root})
		_, _ = c.Update(keyPress("n", 'n'))
		state, value := c.Lifecycle()
		if state != modal.Cancelled {
			t.Fatalf("state = %v, want Cancelled", state)
		}
		if value != nil {
			t.Fatalf("value = %v, want nil", value)
		}
	})
}

// TestMnemonicUniqueness — AC #14. No two labelled buttons share a mnemonic
// across every cursor state (`..`, folder, file, empty folder, folders-only).
// [Select] joins the mnemonic set on selectable rows so the check covers it
// too.
func TestMnemonicUniqueness(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "sub"))
	mustWriteFile(t, filepath.Join(root, "note.md"))

	// Folders + files present, cursor sweeps every kind.
	c := mustNew(t, Options{ConstraintRoot: root, StartFolder: root, ShowFiles: true})
	assertUnique := func(name string) {
		e := c.cursorEntry()
		labels := []string{c.selectCurBtn.Label(), c.hiddenBtn.Label(), c.cancelBtn.Label()}
		mnems := []rune{c.selectCurBtn.Mnemonic(), c.hiddenBtn.Mnemonic(), c.cancelBtn.Mnemonic()}
		if e.isSelectable() {
			labels = append(labels, c.selectBtn.Label())
			mnems = append(mnems, c.selectBtn.Mnemonic())
		}
		seen := map[rune]string{}
		for i, m := range mnems {
			if prev, ok := seen[m]; ok {
				t.Fatalf("state=%s cursor=%v: mnemonic %q collides between %q and %q", name, e.Kind, m, prev, labels[i])
			}
			seen[m] = labels[i]
		}
	}

	// Not-at-root: parent, folder, file rows all present.
	// The header row is at cursor 0; we don't require [Select] there.
	assertUnique("header")
	for _, e := range c.entries {
		moveCursorTo(t, c, e.Name)
		assertUnique(string(e.Name))
	}

	// Empty folder state.
	empty := t.TempDir() // fresh empty dir outside constraint semantics
	c2 := mustNew(t, Options{ConstraintRoot: empty, StartFolder: empty, ShowFiles: true})
	assertUnique("empty")
	moveCursorTo(t, c2, "<empty>")
	assertUnique("empty-cursor")

	// Folders-only mode.
	c3 := mustNew(t, Options{ConstraintRoot: root, StartFolder: root, ShowFiles: false})
	assertUnique("folders-only header")
	moveCursorTo(t, c3, "sub/")
	assertUnique("folders-only cursor on folder")
}

// helpers ----------------------------------------------------------------

func mustNew(t *testing.T, opts Options) *Content {
	t.Helper()
	c, err := New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Focus the tree so its Update pipeline processes navigation.
	if cmd := c.Init(); cmd != nil {
		_ = cmd()
	}
	return c
}

// moveCursorTo drives arrow-down presses until the cursor entry's display
// name matches want. Fails with a diagnostic if the row is not reached
// after exactly len(c.entries) presses — the exact upper bound needed to
// visit every child from the initial header position.
func moveCursorTo(t *testing.T, c *Content, want string) {
	t.Helper()
	presses := len(c.entries)
	for range presses {
		e := c.cursorEntry()
		if e.Kind != entryHeader && e.Name == want {
			return
		}
		_, _ = c.Update(keyPress("j", 'j'))
	}
	e := c.cursorEntry()
	if e.Kind != entryHeader && e.Name == want {
		return
	}
	t.Fatalf("row %q not reachable after %d presses; visible entries: %v", want, presses, entryNames(c.entries))
}

// hasEntryNamed reports whether the current listing carries a row whose
// display name equals want. Used by TestHiddenToggle to observe the state
// change without depending on cursor position.
func hasEntryNamed(c *Content, want string) bool {
	for _, e := range c.entries {
		if e.Name == want {
			return true
		}
	}
	return false
}

// keyPress builds a tea.KeyPressMsg that matches on both Code and Text (the
// pattern used by bubbles/key: k.String() consults Text first, so passing it
// makes rune keys like `j` route through the table's LineDown binding).
func keyPress(text string, code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code, Text: text}
}

// drainConstraintViolations executes cmd and returns every
// [ConstraintViolationMsg] the runtime would receive. Batched commands
// are walked. Non-violation messages are discarded.
func drainConstraintViolations(t *testing.T, cmd tea.Cmd) []ConstraintViolationMsg {
	t.Helper()
	var out []ConstraintViolationMsg
	for _, msg := range drainMessages(cmd) {
		if v, ok := msg.(ConstraintViolationMsg); ok {
			out = append(out, v)
		}
	}
	return out
}

// drainReadDirErrors executes cmd and returns every [ReadDirErrorMsg]
// the runtime would receive.
func drainReadDirErrors(t *testing.T, cmd tea.Cmd) []ReadDirErrorMsg {
	t.Helper()
	var out []ReadDirErrorMsg
	for _, msg := range drainMessages(cmd) {
		if e, ok := msg.(ReadDirErrorMsg); ok {
			out = append(out, e)
		}
	}
	return out
}

// drainMessages executes cmd and flattens any BatchMsg into the returned
// slice of individual tea.Msg values.
func drainMessages(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		out := make([]tea.Msg, 0, len(batch))
		for _, sub := range batch {
			if sub == nil {
				continue
			}
			out = append(out, sub())
		}
		return out
	}
	return []tea.Msg{msg}
}

// assertConfirmed asserts the modal has resolved with the expected [Result]
// payload. Uses the Lifecycle() accessor directly rather than driving through
// a full modal.Modal wrapper — this test file targets the Content in
// isolation.
func assertConfirmed(t *testing.T, c *Content, want Result) {
	t.Helper()
	state, value := c.Lifecycle()
	if state != modal.Confirmed {
		t.Fatalf("state = %v, want Confirmed", state)
	}
	got, ok := value.(Result)
	if !ok {
		t.Fatalf("value = %T (%v), want Result", value, value)
	}
	if got != want {
		t.Fatalf("Result = %#v, want %#v", got, want)
	}
}
