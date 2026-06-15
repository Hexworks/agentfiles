package treetable

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/hexworks/agentfiles/internal/tui/components/mnemonic"
)

func statusValue(n *Node) string  { return "S:" + n.Label }
func currentValue(n *Node) string { return "C:" + n.Label }

func TestValueColumnsZeroIsTransparent(t *testing.T) {
	t.Parallel()

	without := New(WithRoot(sampleTree()))
	with := New(WithRoot(sampleTree()), WithValueColumns())

	gotCols := with.tableColumns()
	wantCols := without.tableColumns()
	if len(gotCols) != len(wantCols) {
		t.Fatalf("column count: got %d want %d", len(gotCols), len(wantCols))
	}
	for i := range gotCols {
		if gotCols[i] != wantCols[i] {
			t.Fatalf("column %d differs: got %v want %v", i, gotCols[i], wantCols[i])
		}
	}
}

func TestValueColumnsHeaderHasFourColumnsWhenActionsEnabled(t *testing.T) {
	t.Parallel()

	m := New(
		WithRoot(sampleTree()),
		WithValueColumns(
			ValueColumn{Title: "Status", Width: 10, Value: statusValue},
			ValueColumn{Title: "Current Action", Width: 16, Value: currentValue},
		),
		WithActions(Column{Title: "Actions", Width: 18}, func(*Node) []*mnemonic.Button { return nil }),
	)

	cols := m.tableColumns()
	if len(cols) != 4 {
		t.Fatalf("column count = %d, want 4", len(cols))
	}
	titles := []string{cols[0].Title, cols[1].Title, cols[2].Title, cols[3].Title}
	want := []string{"Name", "Status", "Current Action", "Actions"}
	for i, got := range titles {
		if got != want[i] {
			t.Fatalf("title[%d] = %q, want %q", i, got, want[i])
		}
	}
	widths := []int{cols[1].Width, cols[2].Width}
	wantW := []int{10, 16}
	for i, got := range widths {
		if got != wantW[i] {
			t.Fatalf("value width[%d] = %d, want %d", i, got, wantW[i])
		}
	}
}

func TestValueColumnsCellsRenderFromCallbacks(t *testing.T) {
	t.Parallel()

	m := New(
		WithRoot(sampleTree()),
		WithValueColumns(
			ValueColumn{Title: "Status", Width: 10, Value: statusValue},
			ValueColumn{Title: "Current Action", Width: 16, Value: currentValue},
		),
	)
	m.Focus()
	m.refreshRows()

	rows := m.Rows()
	if len(rows) != len(m.flat) {
		t.Fatalf("rows = %d, want %d", len(rows), len(m.flat))
	}
	for i, n := range m.flat {
		if got, want := rows[i][1], "S:"+n.Label; got != want {
			t.Fatalf("row %d status cell = %q, want %q", i, got, want)
		}
		if got, want := rows[i][2], "C:"+n.Label; got != want {
			t.Fatalf("row %d current cell = %q, want %q", i, got, want)
		}
	}
}

func TestValueCallbackInvokedForEachRow(t *testing.T) {
	t.Parallel()

	calls := 0
	vc := ValueColumn{
		Title: "Status",
		Width: 10,
		Value: func(*Node) string {
			calls++
			return "x"
		},
	}
	m := New(WithRoot(sampleTree()), WithValueColumns(vc))
	rows := len(m.flat)

	calls = 0
	m.refreshRows()
	if calls != rows {
		t.Fatalf("Value calls = %d, want %d (one per row per render pass)", calls, rows)
	}
}

func TestActionsStayCursorOnlyWithValueColumns(t *testing.T) {
	t.Parallel()

	fn := func(*Node) []*mnemonic.Button {
		return []*mnemonic.Button{
			mnemonic.New("Delete", 'D', func() tea.Cmd { return nil }),
		}
	}
	m := New(
		WithRoot(sampleTree()),
		WithValueColumns(
			ValueColumn{Title: "Status", Width: 10, Value: statusValue},
		),
		WithActions(Column{Title: "Actions", Width: 18}, fn),
	)
	m.Focus()
	m.refreshRows()

	// Button rendering wraps the mnemonic rune in ANSI styling, so the raw
	// "Delete" substring never appears contiguous in the view. Search for
	// the post-mnemonic suffix "elete" instead — it stays unwrapped and
	// proves the button row was rendered, while non-cursor rows leave the
	// actions cell empty.
	out := m.View()
	if n := strings.Count(out, "elete"); n != 1 {
		t.Fatalf("button rendered %d times, want 1 (cursor-only):\n%s", n, out)
	}

	m.table.MoveDown(1)
	m.refreshRows()

	out = m.View()
	if n := strings.Count(out, "elete"); n != 1 {
		t.Fatalf("after MoveDown button rendered %d times, want 1:\n%s", n, out)
	}
}

func TestValueColumnsPanicOnNilValueCallback(t *testing.T) {
	t.Parallel()

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on nil Value, got none")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("panic value is %T, want string", r)
		}
		if !strings.Contains(msg, "nil Value") {
			t.Fatalf("panic %q does not identify the nil-Value contract", msg)
		}
	}()
	WithValueColumns(ValueColumn{Title: "Status", Width: 10}) // Value omitted
}

func TestValueCellSanitizationNeutralizesControlCharacters(t *testing.T) {
	t.Parallel()

	payload := "x\ny\x1b]52;c;evil\x07z"
	vc := ValueColumn{Title: "S", Width: 30, Value: func(*Node) string { return payload }}
	m := New(WithRoot(sampleTree()), WithValueColumns(vc))
	m.refreshRows()

	rows := m.Rows()
	for i, row := range rows {
		cell := row[1]
		if strings.ContainsRune(cell, '\x1b') || strings.ContainsRune(cell, '\x07') || strings.ContainsRune(cell, '\n') {
			t.Fatalf("row %d cell %q still contains control characters", i, cell)
		}
		// '\n' should have been replaced by a space, not stripped.
		if !strings.Contains(cell, "x y") {
			t.Fatalf("row %d cell %q did not collapse newline to space", i, cell)
		}
	}
}

func TestValueCellSanitizationTruncatesToWidth(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("ab", 50)
	vc := ValueColumn{Title: "S", Width: 8, Value: func(*Node) string { return long }}
	m := New(WithRoot(sampleTree()), WithValueColumns(vc))
	m.refreshRows()

	for i, row := range m.Rows() {
		if w := lipgloss.Width(row[1]); w > 8 {
			t.Fatalf("row %d cell width = %d, want ≤ 8", i, w)
		}
	}
}

func TestPanelWidthStaysSquareWithValueColumnsAndTitle(t *testing.T) {
	t.Parallel()

	m := New(
		WithRoot(sampleTree()),
		WithValueColumns(
			ValueColumn{Title: "Status", Width: 10, Value: statusValue},
			ValueColumn{Title: "Current Action", Width: 16, Value: currentValue},
		),
		WithActions(Column{Title: "Actions", Width: 18}, func(*Node) []*mnemonic.Button { return nil }),
		WithTitle("Project Plan"),
	)
	m.Focus()
	m.refreshRows()

	out := m.View()
	lines := strings.Split(out, "\n")
	if len(lines) < 3 {
		t.Fatalf("expected panel with at least 3 lines, got %d:\n%s", len(lines), out)
	}
	// Trim a possible trailing empty line.
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	want := lipgloss.Width(lines[0])
	for i, ln := range lines {
		if got := lipgloss.Width(ln); got != want {
			t.Fatalf("line %d width = %d, want %d (frame asymmetric):\n%s", i, got, want, out)
		}
	}
}
