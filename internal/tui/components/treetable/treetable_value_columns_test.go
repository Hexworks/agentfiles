package treetable

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

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
			ValueColumn{Column: Column{Title: "Status", Width: 10}, Value: statusValue},
			ValueColumn{Column: Column{Title: "Current Action", Width: 16}, Value: currentValue},
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
			ValueColumn{Column: Column{Title: "Status", Width: 10}, Value: statusValue},
			ValueColumn{Column: Column{Title: "Current Action", Width: 16}, Value: currentValue},
		),
	)
	m.Focus()
	m.refreshRows()

	out := m.table.View()
	for _, n := range m.flat {
		if !strings.Contains(out, "S:"+n.Label) {
			t.Fatalf("status cell missing for %q in:\n%s", n.Label, out)
		}
		if !strings.Contains(out, "C:"+n.Label) {
			t.Fatalf("current cell missing for %q in:\n%s", n.Label, out)
		}
	}
}

func TestValueCallbackInvokedOncePerRowPerRender(t *testing.T) {
	t.Parallel()

	calls := 0
	vc := ValueColumn{
		Column: Column{Title: "Status", Width: 10},
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
		t.Fatalf("Value calls = %d, want %d (one per row)", calls, rows)
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
			ValueColumn{Column: Column{Title: "Status", Width: 10}, Value: statusValue},
		),
		WithActions(Column{Title: "Actions", Width: 18}, fn),
	)
	m.Focus()
	m.refreshRows()

	if len(m.Buttons()) != 1 {
		t.Fatalf("cursor row buttons = %d, want 1", len(m.Buttons()))
	}

	m.table.MoveDown(1)
	m.refreshRows()

	if len(m.Buttons()) != 1 {
		t.Fatalf("after MoveDown cursor row buttons = %d, want 1", len(m.Buttons()))
	}
}
