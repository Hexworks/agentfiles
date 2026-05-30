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
// focus.Handler can adopt it directly. The handler returns a mnemonic.Button;
// pass it to SetMnemonicButton (or WithMnemonicButton) so the panel title
// shows the [N] indicator alongside the caption.
//
// Panel: when a title or a mnemonic button is set, View wraps the inner table
// in a rounded panel whose top border embeds `[N]─Caption`. Otherwise View
// returns the bare table body.
package treetable

import (
	"strings"

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

// ActionsFunc returns the mnemonic buttons for n. Returning nil renders an
// empty actions cell. The function is invoked once per render for the cursor
// row only; non-cursor rows always render an empty cell.
type ActionsFunc func(n *Node) []*mnemonic.Button

// Styles centralizes the component's visual configuration.
type Styles struct {
	// Border styles the rounded panel frame drawn around the table when
	// either a title or mnemonic button is set.
	Border lipgloss.Style
	// Title styles the caption text embedded in the top border.
	Title lipgloss.Style
	// Table forwards to the underlying bubbles/table.Model.
	Table table.Styles
}

// DefaultStyles returns palette-neutral defaults; callers pass [WithStyles]
// to integrate with the surrounding theme.
func DefaultStyles() Styles {
	return Styles{
		Border: lipgloss.NewStyle(),
		Title:  lipgloss.NewStyle().Bold(true),
		Table:  table.DefaultStyles(),
	}
}

// Model is the tree-table widget.
type Model struct {
	root        *Node
	nameCol     Column
	actionsCol  Column
	actionsFn   ActionsFunc
	title       string
	mnemonicBtn *mnemonic.Button
	styles      Styles
	height      int

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

// WithMnemonicButton sets the focus mnemonic button shown next to the title
// in the panel's top border. Typically this is the button returned by
// focus.Handler.AddMnemonic for this component.
func WithMnemonicButton(b *mnemonic.Button) Option { return func(m *Model) { m.mnemonicBtn = b } }

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
func (m *Model) refreshRows() {
	cursor := m.table.Cursor()
	m.currentBtns = nil
	rows := make([]table.Row, len(m.flat))
	for i, n := range m.flat {
		name := ""
		if i < len(m.treeLines) {
			name = m.treeLines[i]
		}
		row := table.Row{name}
		if m.actionsFn != nil {
			cell := ""
			if i == cursor {
				btns := m.actionsFn(n)
				m.currentBtns = btns
				cell = renderButtons(btns)
			}
			row = append(row, cell)
		}
		rows[i] = row
	}
	m.table.SetRows(rows)
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

// Buttons returns the buttons currently rendered in the cursor row. Hosts use
// this to include the buttons in a help bar or to forward presses outside of
// Update.
func (m *Model) Buttons() []*mnemonic.Button { return m.currentBtns }

// SetRoot swaps the displayed tree and re-renders.
func (m *Model) SetRoot(n *Node) {
	m.root = n
	m.rebuild()
}

// SetMnemonicButton overrides the focus mnemonic button shown in the title.
// Pass nil to clear it.
func (m *Model) SetMnemonicButton(b *mnemonic.Button) { m.mnemonicBtn = b }

// SetHeight resizes the underlying table viewport.
func (m *Model) SetHeight(h int) {
	m.height = h
	m.table.SetHeight(h)
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

// View renders the table, optionally wrapped in a panel with title and
// mnemonic button embedded in the top border.
func (m *Model) View() string {
	body := m.table.View()
	if m.title == "" && m.mnemonicBtn == nil {
		return body
	}
	return m.renderPanel(body)
}

// renderPanel draws a rounded frame around body. The top border embeds the
// mnemonic button (if any) and the title joined with `─`, matching the
// `┌[N]─Caption─...─┐` look from the design example. The frame is built by
// hand because lipgloss has no native title-in-border helper.
func (m *Model) renderPanel(body string) string {
	bodyW := lipgloss.Width(body)
	var parts []string
	if m.mnemonicBtn != nil {
		parts = append(parts, m.mnemonicBtn.View())
	}
	if m.title != "" {
		parts = append(parts, m.styles.Title.Render(m.title))
	}
	title := strings.Join(parts, "─")
	titleW := lipgloss.Width(title)
	pad := bodyW - titleW
	if pad < 0 {
		pad = 0
	}
	top := m.styles.Border.Render("┌" + title + strings.Repeat("─", pad) + "┐")

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
		sb.WriteString(m.styles.Border.Render("│"))
		sb.WriteString(l)
		sb.WriteString(strings.Repeat(" ", right))
		sb.WriteString(m.styles.Border.Render("│"))
		sb.WriteString("\n")
	}
	sb.WriteString(m.styles.Border.Render("└" + strings.Repeat("─", bodyW) + "┘"))
	return sb.String()
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
