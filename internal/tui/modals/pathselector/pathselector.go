// Package pathselector implements a reusable Bubble Tea v2 modal for picking
// a filesystem path (folder or file) inside a caller-configurable constraint
// root. It is safety-critical: the modal enforces that the user cannot browse
// above [Options.Constraint] and rejects symlink targets that would resolve
// outside it when [Options.FollowSymlinks] is enabled.
//
// Callers open the modal through [modals.NewSelectPath] (the thin wrapper one
// directory up), then read the outcome with [ResultFromMsg] on the
// [modal.ResolvedMsg] the runtime dispatches.
package pathselector

import (
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/tui/components/mnemonic"
	"github.com/hexworks/agentfiles/internal/tui/components/modal"
	"github.com/hexworks/agentfiles/internal/tui/components/treetable"
	"github.com/hexworks/agentfiles/internal/tui/notifications"
	"github.com/hexworks/agentfiles/internal/tui/styles"
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
)

// Content is the stateful [modal.Content] implementation for the path
// selector. It is exported so tests can construct it directly through [New];
// production code should always route through [modals.NewSelectPath] so the
// caller uniformly receives a [modal.Modal].
type Content struct {
	opts    resolvedOptions
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
// performs the option validation required by [Options.normalize] so a
// misconfigured caller sees the failure immediately.
func New(opts Options) (*Content, errs.DomainError) {
	resolved, err := opts.normalize()
	if err != nil {
		return nil, err
	}
	c := &Content{
		opts:       resolved,
		showHidden: resolved.showHiddenInitially,
		state:      modal.Active,
	}
	if entries, entErr := buildEntries(resolved.startFolder, resolved, c.showHidden); entErr != nil {
		return nil, StartUnreadableError{Path: resolved.startFolder, Err: entErr}
	} else {
		c.entries = entries
	}
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
		treetable.WithHeight(minHeight-4),
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
// the listing tracks the terminal size.
func (c *Content) SetSize(width, height int) {
	c.width = width
	c.height = height
	if width < minWidth || height < minHeight {
		return
	}
	nameWidth := width - actionsColWidth - 8 // 8 ≈ borders + separators + padding
	if nameWidth < nameColMinWidth {
		nameWidth = nameColMinWidth
	}
	c.tree.SetHeight(height - 4)
	// bubbles/table does not expose column-width mutation post-construction
	// here; rebuild the root to nudge column measurement through the current
	// row set. This is a no-op for a name-only single-column treetable but
	// keeps behavior predictable if the tree grows extra columns later.
	c.tree.SetRoot(c.buildRootNode())
}

// Lifecycle satisfies [modal.Content]. The modal reports Confirmed with a
// [Result] payload on select, Cancelled with nil on Esc / Cancel, and Active
// otherwise.
func (c *Content) Lifecycle() (modal.LifecycleState, any) {
	return c.state, c.value
}

// current-position helpers ----------------------------------------------

// cursorEntry returns the entry the treetable cursor is on. Index 0 (the tree
// root) maps to a synthetic [entryHeader]; children map to c.entries.
func (c *Content) cursorEntry() entry {
	i := c.tree.Cursor()
	if i <= 0 {
		return entry{Kind: entryHeader, Abs: c.current, Name: c.headerLabel()}
	}
	if i-1 >= len(c.entries) {
		return entry{Kind: entryEmpty}
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
	}
	return nil
}

// onSelectCursor resolves the modal using the row currently under the cursor.
// Non-selectable rows (header, empty placeholder) are silent no-ops.
func (c *Content) onSelectCursor() tea.Cmd {
	e := c.cursorEntry()
	if !e.isSelectable() {
		return nil
	}
	c.state = modal.Confirmed
	c.value = Result{Path: e.Abs, IsDir: e.isDir()}
	return nil
}

// onSelectCurrent resolves the modal with the folder currently being browsed
// (irrespective of the cursor row).
func (c *Content) onSelectCurrent() tea.Cmd {
	c.state = modal.Confirmed
	c.value = Result{Path: c.current, IsDir: true}
	return nil
}

// onToggleHidden flips dotfile visibility, rebuilds the listing, and swaps
// the button label between [Show hidden] and [Hide hidden].
func (c *Content) onToggleHidden() tea.Cmd {
	c.showHidden = !c.showHidden
	if cmd := c.refresh(c.current); cmd != nil {
		return cmd
	}
	c.hiddenBtn = mnemonic.New(hiddenButtonLabel(c.showHidden), 'h', func() tea.Cmd { return c.onToggleHidden() })
	c.rebuildButtonSet()
	return nil
}

// onCancel resolves the modal in the Cancelled state.
func (c *Content) onCancel() tea.Cmd {
	c.state = modal.Cancelled
	c.value = nil
	return nil
}

// navigate moves the modal into target. Constraint violations and I/O errors
// leave the current folder untouched and emit an error notification.
func (c *Content) navigate(target string) tea.Cmd {
	resolved := resolveAbs(target, c.opts.followSymlinks)
	if !isUnderRoot(resolved, c.opts.constraint) {
		return emitNotify(errs.SeverityError, "Cannot leave "+c.opts.constraint)
	}
	return c.refresh(resolved)
}

// refresh sets the modal's current folder to target, rebuilds the entry list,
// and refreshes the tree. Returns an error-notification command when the
// os.ReadDir underneath fails.
func (c *Content) refresh(target string) tea.Cmd {
	entries, err := buildEntries(target, c.opts, c.showHidden)
	if err != nil {
		return emitNotify(err.Severity(), err.Error())
	}
	c.current = target
	c.entries = entries
	c.tree.SetRoot(c.buildRootNode())
	return nil
}

// rebuildButtonSet re-registers the screen-level buttons after the hidden-
// files toggle swaps out the [Show hidden] button instance.
func (c *Content) rebuildButtonSet() {
	set := mnemonic.NewSet()
	set.Add(c.selectCurBtn)
	set.Add(c.hiddenBtn)
	set.Add(c.cancelBtn)
	c.buttons = set
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

// emitNotify wraps a NotificationMsg emission in a tea.Cmd so callsites stay
// one-line and the notification's CreatedAt is captured at cmd-execution time
// (matching the notifications.From bridge convention).
func emitNotify(severity errs.Severity, text string) tea.Cmd {
	return func() tea.Msg {
		return notifications.NotificationMsg{
			Notification: notifications.Notification{
				Severity:  severity,
				Text:      text,
				CreatedAt: time.Now(),
			},
		}
	}
}

// verify: Content satisfies modal.Content and modal.Resizable.
var (
	_ modal.Content   = (*Content)(nil)
	_ modal.Resizable = (*Content)(nil)
)
