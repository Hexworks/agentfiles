// Package treetable renders a hierarchical structure as a tabular widget
// with context-aware per-row mnemonic action buttons. The first column carries
// the tree-prefixed node label (rendered through lipgloss/tree); an optional
// trailing actions column shows mnemonic buttons for the cursor row only.
//
// The component is a passive wrapper around bubbles/table.Model: navigation
// keys flow to the underlying table, and the cursor row's mnemonic buttons
// are matched and triggered by Update. Like the existing mnemonic and focus
// components, the hosting screen owns global keys (quit, focus rotation).
//
// Focus: Model satisfies focus.Focusable (Focus/Blur return tea.Cmd) so a
// focus.Handler can adopt it directly.
//
// Panel: when a title is set, View wraps the inner table in a rounded panel
// whose top border embeds the caption. Otherwise View returns the bare table
// body.
package treetable

import (
	"fmt"
	"strings"
	"unicode"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/tree"

	"github.com/hexworks/agentfiles/internal/tui/components/mnemonic"
)

// Node is a single entry in the tree displayed by the component. Children
// drive the visual hierarchy; Label is the text rendered in the first column;
// Data is opaque payload the caller's ActionsFunc consumes to decide which
// buttons a row deserves.
//
// Every node — including the root — must have a non-empty Label. Empty labels
// would desync the line-to-node mapping produced by lipgloss/tree.
type Node struct {
	ID       string
	Label    string
	Data     any
	Children []*Node
}

// Column describes a header title and a fixed render width.
type Column struct {
	Title string
	Width int
}

// ValueColumn is an intermediate column injected between the name column and
// the optional actions column.
//
// Value computes the cell text for a given node. It may be invoked on every
// render and on every [Model.Update] message, so it must be cheap and pure.
// The returned string is sanitized before display: control characters are
// stripped, CR/LF/TAB are collapsed to a single space, and the result is
// truncated to Width using ANSI-aware width measurement.
//
// Title and Width follow the same shape as [Column]; declared as fields here
// (rather than embedded) so call sites can use flat struct literals.
type ValueColumn struct {
	Title string
	Width int
	Value func(*Node) string
}

// ActionsFunc returns the mnemonic buttons for n. Returning nil renders an
// empty actions cell. The function is invoked once per render for the cursor
// row only; non-cursor rows always render an empty cell.
type ActionsFunc func(n *Node) []*mnemonic.Button

// Styles centralizes the component's visual configuration.
//
// Border applies to the rounded panel frame whenever the component is
// blurred; BorderFocused applies when it is focused (see
// [Model.Focused]). Both default to the same neutral style — callers
// that want a focus-aware highlight (typical case) set BorderFocused to
// the accent palette and Border to the muted palette.
type Styles struct {
	// Border styles the rounded panel frame drawn around the table when
	// the component is not focused.
	Border lipgloss.Style
	// BorderFocused styles the rounded panel frame when the component is
	// focused. Defaults to Border on construction; override to highlight.
	BorderFocused lipgloss.Style
	// Title styles the caption text embedded in the top border.
	Title lipgloss.Style
	// Table forwards to the underlying bubbles/table.Model.
	Table table.Styles
}

// DefaultStyles returns palette-neutral defaults; callers pass [WithStyles]
// to integrate with the surrounding theme.
func DefaultStyles() Styles {
	base := lipgloss.NewStyle()
	return Styles{
		Border:        base,
		BorderFocused: base,
		Title:         lipgloss.NewStyle().Bold(true),
		Table:         table.DefaultStyles(),
	}
}

// Model is the tree-table widget.
//
// Exactly three column kinds are supported, always in this fixed order:
//
//  1. Name (always present, always first).
//  2. Value columns (zero or more, via [WithValueColumns]).
//  3. Actions (optional, always last, via [WithActions]).
//
// Adding a new column kind requires editing Model — there is no open
// extension point. The constraint is deliberate: the three kinds cover
// the foreseeable need without paying for a generic column-spec abstraction.
type Model struct {
	root         *Node
	nameCol      Column
	valueColumns []ValueColumn
	actionsCol   Column
	actionsFn    ActionsFunc
	title        string
	styles       Styles
	height       int

	flat        []*Node
	treeLines   []string
	currentBtns []*mnemonic.Button

	table table.Model
}

// Option configures a [Model] at construction time.
type Option func(*Model)

// WithRoot installs the initial tree. Use [Model.SetRoot] to swap it later.
func WithRoot(root *Node) Option { return func(m *Model) { m.root = root } }

// WithNameColumn overrides the default name-column title and width. The name
// column is always present and always first.
func WithNameColumn(c Column) Option { return func(m *Model) { m.nameCol = c } }

// WithValueColumns injects intermediate value columns between the name column
// and the optional actions column. Each [ValueColumn.Value] is called per row
// per render-pass to produce the cell text. Render order is name → value
// columns (in order) → actions.
//
// Panics if any column has a nil Value callback — that is a programmer error
// caught at construction so the failure points at the misconfigured call
// site instead of the eventual render loop.
func WithValueColumns(cols ...ValueColumn) Option {
	for i, c := range cols {
		if c.Value == nil {
			panic(fmt.Sprintf("treetable: ValueColumn[%d] %q has nil Value callback", i, c.Title))
		}
	}
	return func(m *Model) { m.valueColumns = cols }
}

// WithActions enables the trailing actions column. fn must be non-nil; passing
// nil disables the column. The width is fixed at construction time and should
// be large enough to fit the widest button set the caller will render.
func WithActions(col Column, fn ActionsFunc) Option {
	return func(m *Model) {
		m.actionsCol = col
		m.actionsFn = fn
	}
}

// WithTitle sets the caption shown in the panel's top border.
func WithTitle(s string) Option { return func(m *Model) { m.title = s } }

// WithStyles replaces the default styles.
func WithStyles(s Styles) Option { return func(m *Model) { m.styles = s } }

// WithHeight sets the height of the inner table viewport (in lines).
func WithHeight(h int) Option { return func(m *Model) { m.height = h } }

// New constructs a Model. Callers must pass at least [WithRoot]; an empty
// model renders as an empty table.
func New(opts ...Option) *Model {
	m := &Model{
		nameCol: Column{Title: "Name", Width: 40},
		styles:  DefaultStyles(),
		height:  8,
	}
	for _, opt := range opts {
		opt(m)
	}
	m.initTable()
	return m
}

func (m *Model) initTable() {
	cols := m.tableColumns()
	width := 0
	for _, c := range cols {
		// bubbles/table pads each cell with one space on each side
		width += c.Width + 2
	}
	t := table.New(
		table.WithColumns(cols),
		table.WithWidth(width),
		table.WithHeight(m.height),
		table.WithStyles(m.styles.Table),
	)
	m.table = t
	m.rebuild()
}

func (m *Model) tableColumns() []table.Column {
	out := []table.Column{{Title: m.nameCol.Title, Width: m.nameCol.Width}}
	for _, vc := range m.valueColumns {
		out = append(out, table.Column{Title: vc.Title, Width: vc.Width})
	}
	if m.actionsFn != nil {
		out = append(out, table.Column{Title: m.actionsCol.Title, Width: m.actionsCol.Width})
	}
	return out
}

// rebuild flattens the tree and re-renders the prefix lines. Call after the
// tree shape changes; refreshRows handles cursor-only changes.
func (m *Model) rebuild() {
	m.flat = flatten(m.root)
	m.treeLines = renderTreeLines(m.root)
	m.refreshRows()
}

// refreshRows rebuilds the bubbles table rows. The actions cell is populated
// only for the cursor row so non-cursor rows stay visually quiet.
//
// Tests in this package may call refreshRows directly to drive a single
// render pass without going through [Model.Update]. Production code never
// needs to call it: [Model.Update] and the option pipeline already do.
func (m *Model) refreshRows() {
	cursor := m.table.Cursor()
	m.currentBtns = nil
	rows := make([]table.Row, len(m.flat))
	for i, n := range m.flat {
		rows[i] = m.buildRow(i, n, cursor)
	}
	m.table.SetRows(rows)
}

// buildRow assembles one row in the order Name → value columns → optional
// actions cell. The actions cell is populated only when i == cursor.
func (m *Model) buildRow(i int, n *Node, cursor int) table.Row {
	name := ""
	if i < len(m.treeLines) {
		name = m.treeLines[i]
	}
	row := table.Row{name}
	for _, vc := range m.valueColumns {
		row = append(row, sanitizeCell(vc.Value(n), vc.Width))
	}
	if m.actionsFn != nil {
		cell := ""
		if i == cursor {
			btns := m.actionsFn(n)
			m.currentBtns = btns
			cell = renderButtons(btns)
		}
		row = append(row, cell)
	}
	return row
}

// sanitizeCell neutralizes untrusted text returned from a value callback
// before it reaches the table renderer. CR/LF/TAB collapse to a single
// space so a multi-line payload does not desync the row layout; other
// non-printable runes (NUL, BEL, ESC and the rest of the ANSI control
// range) are stripped so embedded terminal escape sequences cannot leak
// styling, move the cursor, or trigger OSC side channels. If width > 0
// the result is truncated to that visual width using lipgloss's ANSI-aware
// measurement.
func sanitizeCell(s string, width int) string {
	var sb strings.Builder
	sb.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			sb.WriteByte(' ')
		case unicode.IsPrint(r):
			sb.WriteRune(r)
		}
	}
	out := sb.String()
	if width <= 0 || lipgloss.Width(out) <= width {
		return out
	}
	var trunc strings.Builder
	w := 0
	for _, r := range out {
		rw := lipgloss.Width(string(r))
		if w+rw > width {
			break
		}
		trunc.WriteRune(r)
		w += rw
	}
	return trunc.String()
}

func renderButtons(bs []*mnemonic.Button) string {
	if len(bs) == 0 {
		return ""
	}
	parts := make([]string, len(bs))
	for i, b := range bs {
		parts[i] = b.View()
	}
	return strings.Join(parts, " ")
}

// Focus and Blur satisfy focus.Focusable so a focus.Handler can adopt the
// component directly.
func (m *Model) Focus() tea.Cmd { m.table.Focus(); return nil }

// Blur removes keyboard focus from the underlying table.
func (m *Model) Blur() tea.Cmd { m.table.Blur(); return nil }

// Focused reports whether the underlying table currently holds focus.
func (m *Model) Focused() bool { return m.table.Focused() }

// SelectedNode returns the node under the cursor, or nil if the table is
// empty.
func (m *Model) SelectedNode() *Node {
	i := m.table.Cursor()
	if i < 0 || i >= len(m.flat) {
		return nil
	}
	return m.flat[i]
}

// Cursor returns the cursor row index.
func (m *Model) Cursor() int { return m.table.Cursor() }

// Rows returns the currently materialized table rows in display order.
// Useful for tests and hosts that need to inspect rendered cell content
// without parsing [Model.View] output.
func (m *Model) Rows() []table.Row { return m.table.Rows() }

// Buttons returns the buttons currently rendered in the cursor row. Hosts use
// this to include the buttons in a help bar or to forward presses outside of
// Update.
func (m *Model) Buttons() []*mnemonic.Button { return m.currentBtns }

// SetRoot swaps the displayed tree and re-renders.
func (m *Model) SetRoot(n *Node) {
	m.root = n
	m.rebuild()
}

// RefreshActions re-runs the cursor row's ActionsFunc and re-renders the
// table rows. Use this when the row's button factory result has changed
// (e.g. a state flip toggling the rendered button label) but the tree
// shape itself has not. Cheaper than SetRoot and limits the surface
// hosts touch when they only need the actions cell to refresh.
func (m *Model) RefreshActions() {
	m.refreshRows()
}

// SetHeight resizes the underlying table viewport.
func (m *Model) SetHeight(h int) {
	m.height = h
	m.table.SetHeight(h)
}

// SetStyles swaps the component's styles. The underlying bubbles table
// is updated to use the new Table styles too so a focus-state change
// (e.g. cyan border when focused, muted when blurred) is reflected on
// the next render.
func (m *Model) SetStyles(s Styles) {
	m.styles = s
	m.table.SetStyles(s.Table)
}

// Update forwards messages to the underlying table when focused. Key presses
// are first matched against the cursor row's mnemonic buttons; a match
// triggers the button and short-circuits navigation, mirroring the pattern
// from the mnemonic playground.
func (m *Model) Update(msg tea.Msg) (*Model, tea.Cmd) {
	if !m.table.Focused() {
		return m, nil
	}
	if kp, ok := msg.(tea.KeyPressMsg); ok {
		for _, b := range m.currentBtns {
			if b.Matches(kp) {
				return m, b.Trigger()
			}
		}
	}
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	m.refreshRows()
	return m, cmd
}

// View renders the table, optionally wrapped in a panel with the title
// embedded in the top border.
func (m *Model) View() string {
	body := m.table.View()
	if m.title == "" {
		return body
	}
	return m.renderPanel(body)
}

// renderPanel draws a rounded frame around body. The top border embeds the
// title, matching the `┌Caption─...─┐` look from the design example. The
// frame is built by hand because lipgloss has no native title-in-border
// helper.
//
// Border color tracks focus state: Styles.BorderFocused when
// [Model.Focused] reports true, Styles.Border otherwise. Callers that
// don't want a focus-aware highlight leave both styles identical.
func (m *Model) renderPanel(body string) string {
	border := m.borderStyle()
	bodyW := lipgloss.Width(body)
	title := m.styles.Title.Render(m.title)
	titleW := lipgloss.Width(title)
	pad := bodyW - titleW
	if pad < 0 {
		pad = 0
	}
	top := border.Render("┌" + title + strings.Repeat("─", pad) + "┐")

	lines := strings.Split(body, "\n")
	var sb strings.Builder
	sb.WriteString(top)
	sb.WriteString("\n")
	for _, l := range lines {
		w := lipgloss.Width(l)
		right := bodyW - w
		if right < 0 {
			right = 0
		}
		sb.WriteString(border.Render("│"))
		sb.WriteString(l)
		sb.WriteString(strings.Repeat(" ", right))
		sb.WriteString(border.Render("│"))
		sb.WriteString("\n")
	}
	sb.WriteString(border.Render("└" + strings.Repeat("─", bodyW) + "┘"))
	return sb.String()
}

// borderStyle returns the focus-aware border style: BorderFocused when
// the component holds focus, Border otherwise.
func (m *Model) borderStyle() lipgloss.Style {
	if m.table.Focused() {
		return m.styles.BorderFocused
	}
	return m.styles.Border
}

// flatten produces a DFS list of visible nodes. The order matches the line
// order produced by [renderTreeLines] exactly so callers can pair them by
// index. Nil roots produce a nil slice.
func flatten(n *Node) []*Node {
	if n == nil {
		return nil
	}
	out := []*Node{n}
	for _, c := range n.Children {
		out = append(out, flatten(c)...)
	}
	return out
}

// renderTreeLines builds a lipgloss tree mirroring the node structure, renders
// it, and splits the output into one line per node. The order matches
// [flatten]; treetable pairs the two by index to map node → prefixed label.
func renderTreeLines(root *Node) []string {
	if root == nil {
		return nil
	}
	t := buildLipglossTree(root)
	return strings.Split(t.String(), "\n")
}

func buildLipglossTree(n *Node) *tree.Tree {
	t := tree.Root(n.Label)
	for _, c := range n.Children {
		if len(c.Children) == 0 {
			t.Child(c.Label)
			continue
		}
		t.Child(buildLipglossTree(c))
	}
	return t
}
