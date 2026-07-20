// Package pathselector implements a reusable Bubble Tea v2 modal for picking
// a filesystem path (folder or file) inside a caller-configurable constraint
// root. It is safety-critical: the modal enforces that the user cannot browse
// above [Options.ConstraintRoot] and rejects symlink targets that would
// resolve outside it when [Options.FollowSymlinks] is enabled.
//
// Callers open the modal through [modals.NewSelectPath] (the thin wrapper one
// directory up), then read the outcome with [ResultFromMsg] on the
// [modal.ResolvedMsg] the runtime dispatches. Runtime errors leave the
// package as [ConstraintViolationMsg] / [ReadDirErrorMsg] — the wrapper (or
// any other host) is responsible for translating those into whatever
// error-reporting mechanism the host uses (toast notification, status line,
// modal-in-a-modal, …).
package pathselector

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/tui/components/mnemonic"
	"github.com/hexworks/agentfiles/internal/tui/components/modal"
	"github.com/hexworks/agentfiles/internal/tui/components/treetable"
	"github.com/hexworks/agentfiles/internal/tui/styles"
	"github.com/hexworks/agentfiles/internal/utils"
)

const (
	// minWidth / minHeight are the smallest terminal dimensions at which the
	// modal renders its interactive body. Below the threshold a static
	// "Terminal too small" placeholder replaces the tree so the modal still
	// closes cleanly on Esc but does not try to draw a garbled listing.
	minWidth  = 40
	minHeight = 10
	// actionsColWidth fixes the per-row actions column at a width that fits
	// the single [Select] button rendered on the cursor row.
	actionsColWidth = 10
	// nameColMinWidth is the floor applied to the tree's name column when the
	// modal is squeezed but still above the (minWidth, minHeight) threshold.
	nameColMinWidth = 20
	// buttonRowHeight is the height of the screen-level mnemonic button row
	// rendered below the tree body. Named so [Content.SetSize] can split
	// the vertical budget without a bare integer.
	buttonRowHeight = 1
	// tableColumnCount is the number of columns configured on the treetable
	// (Name + Actions). Multiplied by [treetable.CellPaddingPerColumn] to
	// compute the horizontal chrome the table itself consumes.
	tableColumnCount = 2
)

// Content is the stateful [modal.Content] implementation for the path
// selector. It is exported so tests can construct it directly through [New];
// production code should always route through [modals.NewSelectPath] so the
// caller uniformly receives a [modal.Modal].
type Content struct {
	opts    resolvedOptions
	root    *os.Root // nil when opts.constraint == ""
	current string
	entries []entry

	tree         *treetable.Model
	selectBtn    *mnemonic.Button
	selectCurBtn *mnemonic.Button
	hiddenBtn    *mnemonic.Button
	cancelBtn    *mnemonic.Button
	buttons      *mnemonic.Set

	showHidden bool

	width  int
	height int

	state modal.LifecycleState
	value any
}

// New constructs a Content ready to be embedded in a [modal.Modal]. It
// performs the option validation required by [Options.canonicalize] +
// [probe] so a misconfigured caller sees the failure immediately.
func New(opts Options) (*Content, errs.DomainError) {
	canon := opts.canonicalize()
	resolved, err := probe(canon)
	if err != nil {
		return nil, err
	}
	c := &Content{
		opts:       resolved,
		showHidden: resolved.showHiddenInitially,
		state:      modal.Active,
	}
	if resolved.constraint != "" {
		root, openErr := os.OpenRoot(resolved.constraint)
		if openErr != nil {
			return nil, ConstraintUnreadableError{Path: resolved.constraint, Err: openErr}
		}
		c.root = root
		// Runtime-managed close: the modal has no explicit Close() hook on
		// [modal.Content], and callers should not have to remember one for
		// a short-lived selector. The cleanup fires when the *Content is
		// no longer reachable, keeping the fd out of long-lived leaks.
		runtime.AddCleanup(c, func(r *os.Root) { _ = r.Close() }, c.root)
	}
	entries, entErr := buildEntries(resolved.startFolder, resolved, c.root, c.showHidden)
	if entErr != nil {
		return nil, entErr
	}
	c.entries = entries
	c.current = resolved.startFolder
	c.selectBtn = mnemonic.New("Select", 'S', func() tea.Cmd { return c.onSelectCursor() })
	c.selectCurBtn = mnemonic.New("Select current", 'c', func() tea.Cmd { return c.onSelectCurrent() })
	c.hiddenBtn = mnemonic.New(hiddenButtonLabel(c.showHidden), 'h', func() tea.Cmd { return c.onToggleHidden() })
	c.cancelBtn = mnemonic.New("Cancel", 'n', func() tea.Cmd { return c.onCancel() },
		mnemonic.WithExtraBindingKeys("esc"),
	)
	c.buttons = mnemonic.NewSet()
	c.buttons.Add(c.selectCurBtn)
	c.buttons.Add(c.hiddenBtn)
	c.buttons.Add(c.cancelBtn)
	c.tree = treetable.New(
		treetable.WithRoot(c.buildRootNode()),
		treetable.WithNameColumn(treetable.Column{Title: "Name", Width: nameColMinWidth}),
		treetable.WithActions(treetable.Column{Title: "Actions", Width: actionsColWidth}, c.treeActionsFn()),
		treetable.WithHeight(minHeight-modal.ChromeHeight),
	)
	return c, nil
}

// Init satisfies [modal.Content]. It focuses the tree so the cursor
// highlighting and per-row actions render.
func (c *Content) Init() tea.Cmd {
	return c.tree.Focus()
}

// Update satisfies [modal.Content]. It intercepts the keys the modal owns
// (Enter, Esc, mnemonic buttons) and forwards navigation keys to the
// underlying treetable.
func (c *Content) Update(msg tea.Msg) (modal.Content, tea.Cmd) {
	kp, ok := msg.(tea.KeyPressMsg)
	if !ok {
		var cmd tea.Cmd
		c.tree, cmd = c.tree.Update(msg)
		return c, cmd
	}

	switch kp.Code {
	case tea.KeyEsc:
		return c, c.onCancel()
	case tea.KeyEnter:
		return c, c.onEnter()
	}

	if btn := c.buttons.Match(kp); btn != nil {
		return c, btn.Trigger()
	}
	if c.selectBtn.Matches(kp) && c.cursorEntry().isSelectable() {
		return c, c.selectBtn.Trigger()
	}

	var cmd tea.Cmd
	c.tree, cmd = c.tree.Update(msg)
	return c, cmd
}

// View satisfies [modal.Content]. Below the minimum terminal footprint the
// tree body is replaced with a placeholder message so the modal still renders
// cleanly and the Esc / Cancel path remains reachable.
func (c *Content) View() string {
	body := c.tree.View()
	if c.width < minWidth || c.height < minHeight {
		body = styles.MutedStyle.Render("Terminal too small")
	}
	buttonRow := strings.Join([]string{
		c.selectCurBtn.View(),
		c.hiddenBtn.View(),
		c.cancelBtn.View(),
	}, " ")
	return lipgloss.JoinVertical(lipgloss.Left, body, buttonRow)
}

// SetSize satisfies [modal.Resizable]. It recomputes the tree's viewport so
// the listing tracks the terminal size. Chrome dimensions come from the
// modal and treetable packages rather than bare integers, so a change to
// either component's frame is picked up here for free.
func (c *Content) SetSize(width, height int) {
	c.width = width
	c.height = height
	if width < minWidth || height < minHeight {
		return
	}
	innerW := width - modal.ChromeWidth
	nameWidth := innerW - actionsColWidth - treetable.CellPaddingPerColumn*tableColumnCount
	if nameWidth < nameColMinWidth {
		nameWidth = nameColMinWidth
	}
	innerH := height - modal.ChromeHeight
	c.tree.SetHeight(innerH - buttonRowHeight)
}

// Lifecycle satisfies [modal.Content]. The modal reports Confirmed with a
// [Result] payload on select, Cancelled with nil on Esc / Cancel, and Active
// otherwise.
func (c *Content) Lifecycle() (modal.LifecycleState, any) {
	return c.state, c.value
}

// current-position helpers ----------------------------------------------

// cursorEntry returns the entry the treetable cursor is on. Index 0 (the tree
// root) maps to a synthetic [entryHeader]; children map to c.entries. A
// cursor index past the child count is a treetable invariant break and
// panics rather than degrading silently — masking the bug behind an
// [entryEmpty] fallback previously turned an off-by-one into an invisible
// no-op.
func (c *Content) cursorEntry() entry {
	i := c.tree.Cursor()
	if i <= 0 {
		return entry{Kind: entryHeader, Abs: c.current, Name: c.headerLabel()}
	}
	if i-1 >= len(c.entries) {
		panic(fmt.Sprintf("pathselector: treetable cursor %d exceeds row count %d", i, len(c.entries)))
	}
	return c.entries[i-1]
}

// key action handlers ---------------------------------------------------

// onEnter handles the Enter key. Directories navigate; files and non-
// selectable placeholders are silent no-ops. Symlink-directory rows respect
// [Options.FollowSymlinks].
func (c *Content) onEnter() tea.Cmd {
	e := c.cursorEntry()
	switch e.Kind {
	case entryHeader, entryEmpty, entryFile, entryFileSymlink:
		return nil
	case entryDirSymlink:
		if !c.opts.followSymlinks {
			return nil
		}
		return c.navigate(e.Abs)
	case entryParent, entryDir:
		return c.navigate(e.Abs)
	default:
		// Unreachable today; the switch above enumerates every entryKind.
		// A new kind added to entries.go without a case here trips this
		// panic instead of falling through to a silent no-op.
		panic(fmt.Sprintf("pathselector: unhandled entryKind %d", e.Kind))
	}
}

// onSelectCursor resolves the modal using the row currently under the cursor.
// Non-selectable rows (header, empty placeholder) are silent no-ops. The
// selection goes through [Content.selectionResult] so the same containment
// gate that protects navigate() also protects direct row selection — a
// symlink whose target escapes the constraint cannot leak out as a
// "safe-looking" absolute path in Result.Path.
func (c *Content) onSelectCursor() tea.Cmd {
	e := c.cursorEntry()
	if !e.isSelectable() {
		return nil
	}
	return c.resolveSelection(e)
}

// onSelectCurrent resolves the modal with the folder currently being browsed
// (irrespective of the cursor row). It routes through [Content.selectionResult]
// so the same containment guarantee applies — even though c.current is
// already resolved and known-safe from navigate(), keeping the two entry
// points on one gate means a future refactor cannot forget to check one.
func (c *Content) onSelectCurrent() tea.Cmd {
	return c.resolveSelection(entry{Kind: entryDir, Abs: c.current})
}

// resolveSelection is the shared safety-gated selection tail. It
// symlink-resolves e.Abs, re-checks constraint containment, and only then
// transitions the modal to [modal.Confirmed]. Failures emit a
// [ConstraintViolationMsg] and leave the modal Active on its current
// folder.
func (c *Content) resolveSelection(e entry) tea.Cmd {
	result, cmd := c.selectionResult(e)
	if cmd != nil {
		return cmd
	}
	c.state = modal.Confirmed
	c.value = result
	return nil
}

// selectionResult produces the Result payload for e, gated through the
// same containment check the navigate() path uses. Returns a non-nil cmd
// (and a zero Result) when the selection would escape [Options.ConstraintRoot].
func (c *Content) selectionResult(e entry) (Result, tea.Cmd) {
	resolved, err := utils.ResolveAbs(e.Abs, true)
	if err != nil {
		return Result{}, c.constraintViolation()
	}
	if !utils.IsUnderRoot(resolved, c.opts.constraint) {
		return Result{}, c.constraintViolation()
	}
	return Result{Path: resolved, IsDir: e.selectsAsDir()}, nil
}

// onToggleHidden flips dotfile visibility, rebuilds the listing, and swaps
// the button label between [Show hidden] and [Hide hidden]. The label
// update is unconditional so a failed refresh does not desync the button
// state from the internal showHidden flag.
func (c *Content) onToggleHidden() tea.Cmd {
	c.showHidden = !c.showHidden
	c.hiddenBtn.SetLabel(hiddenButtonLabel(c.showHidden))
	return c.refresh(c.current)
}

// onCancel resolves the modal in the Cancelled state.
func (c *Content) onCancel() tea.Cmd {
	c.state = modal.Cancelled
	c.value = nil
	return nil
}

// navigate moves the modal into target. Constraint violations and I/O errors
// leave the current folder untouched and emit an error message.
func (c *Content) navigate(target string) tea.Cmd {
	resolved, err := utils.ResolveAbs(target, c.opts.followSymlinks)
	if err != nil {
		return c.constraintViolation()
	}
	if !utils.IsUnderRoot(resolved, c.opts.constraint) {
		return c.constraintViolation()
	}
	return c.refresh(resolved)
}

// refresh sets the modal's current folder to target, rebuilds the entry list,
// and refreshes the tree. Returns a [ReadDirErrorMsg] command when the
// underlying read fails.
func (c *Content) refresh(target string) tea.Cmd {
	entries, err := buildEntries(target, c.opts, c.root, c.showHidden)
	if err != nil {
		return c.readDirError(target, err)
	}
	c.current = target
	c.entries = entries
	c.tree.SetRoot(c.buildRootNode())
	return nil
}

// constraintViolation emits a [ConstraintViolationMsg] carrying the
// modal's resolved constraint root so the host can compose a message
// that names the fence the user tried to cross.
func (c *Content) constraintViolation() tea.Cmd {
	constraint := c.opts.constraint
	return func() tea.Msg {
		return ConstraintViolationMsg{Constraint: constraint}
	}
}

// readDirError emits a [ReadDirErrorMsg] for a runtime read failure.
// The path and error are captured by value so the closure remains valid
// after the caller returns.
func (c *Content) readDirError(path string, err error) tea.Cmd {
	return func() tea.Msg {
		return ReadDirErrorMsg{Path: path, Err: err}
	}
}

// treetable adapter ------------------------------------------------------

// treeActionsFn returns the per-row action list. Only selectable rows get a
// [Select] button; the header and empty placeholder rows return nil so the
// mnemonic surface stays uniform across cursor positions.
func (c *Content) treeActionsFn() treetable.ActionsFunc {
	return func(n *treetable.Node) []*mnemonic.Button {
		data, _ := n.Data.(entry)
		if !data.isSelectable() {
			return nil
		}
		return []*mnemonic.Button{c.selectBtn}
	}
}

// buildRootNode assembles the treetable root reflecting the current folder
// and the filtered entries. The root row is the "you are here" header; each
// child node carries the corresponding [entry] value in its Data field so the
// actions-func lookup is a plain type assertion.
func (c *Content) buildRootNode() *treetable.Node {
	root := &treetable.Node{
		Label: c.headerLabel(),
		Data:  entry{Kind: entryHeader, Abs: c.current, Name: c.headerLabel()},
	}
	for _, e := range c.entries {
		root.Children = append(root.Children, &treetable.Node{
			Label: e.Name,
			Data:  e,
		})
	}
	return root
}

// headerLabel produces the string displayed on the root row: the current
// folder path made relative to the constraint (so long paths do not blow
// out the modal width). At the constraint root the label is ".".
func (c *Content) headerLabel() string {
	if c.opts.constraint == "" {
		return c.current
	}
	rel, err := filepath.Rel(c.opts.constraint, c.current)
	if err != nil {
		return c.current
	}
	if rel == "." {
		return "."
	}
	return "./" + rel
}

// static helpers ---------------------------------------------------------

func hiddenButtonLabel(showing bool) string {
	if showing {
		return "Hide hidden"
	}
	return "Show hidden"
}

// verify: Content satisfies modal.Content and modal.Resizable.
var (
	_ modal.Content   = (*Content)(nil)
	_ modal.Resizable = (*Content)(nil)
)
