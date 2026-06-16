package mnemonic

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// TestButtonViewEmitsNoInternalResets pins the SGR-aware invariant
// documented on [Button.View]: only the trailing reset may be a bare
// `\x1b[0m` / `\x1b[m`. Any intermediate reset would terminate a parent
// style (table selected-row highlight, modal accent border, …) and
// reproduce the "pink first letter, gray rest" bug.
func TestButtonViewEmitsNoInternalResets(t *testing.T) {
	t.Parallel()

	b := New("Overwrite", 'O', noopAction())
	got := b.View()

	if !strings.HasSuffix(got, ansi.ResetStyle) {
		t.Fatalf("View() must end with the SGR reset; got %q", got)
	}
	body := strings.TrimSuffix(got, ansi.ResetStyle)
	if strings.Contains(body, "\x1b[0m") || strings.Contains(body, "\x1b[m") {
		t.Fatalf("View() must not emit intermediate SGR resets; got %q", got)
	}
}

// TestThemedStylesPinsBoldAndUnderline guards the "fully restate"
// contract of the chunk SGRs. The text chunk (brackets + label) must
// explicitly disable bold + underline so a parent style with either
// attribute enabled does not bleed through; the mnemonic chunk must
// explicitly enable both.
func TestThemedStylesPinsBoldAndUnderline(t *testing.T) {
	t.Parallel()

	s := ThemedStyles(ansi.White, ansi.Green)

	cases := []struct {
		name string
		st   ansi.Style
		want []string
	}{
		{"mnemonic", s.Mnemonic, []string{"1", "4"}}, // bold, underline
		{"text", s.Text, []string{"22", "24"}},       // normal weight, no underline
	}
	for _, tc := range cases {
		seq := tc.st.String()
		for _, attr := range tc.want {
			if !strings.Contains(seq, attr) {
				t.Errorf("%s SGR %q missing attr %q", tc.name, seq, attr)
			}
		}
	}
}

// TestButtonViewChunksUseConfiguredStyles checks that View emits each
// configured style's SGR sequence exactly where expected: text before
// `[`, mnemonic before the highlighted letter, text before the rest,
// text again before `]`. Distinct foreground colors per chunk make
// each SGR sequence identifiable.
func TestButtonViewChunksUseConfiguredStyles(t *testing.T) {
	t.Parallel()

	s := ThemedStyles(ansi.Blue, ansi.Green)
	b := New("Apply", 'A', noopAction(), WithStyles(s))

	got := b.View()
	mn := s.Mnemonic.String()
	text := s.Text.String()

	wantOrder := []string{
		text + "[",
		mn + "A",
		text + "pply]",
		ansi.ResetStyle,
	}
	prev := 0
	for _, w := range wantOrder {
		idx := strings.Index(got[prev:], w)
		if idx < 0 {
			t.Fatalf("View() %q missing %q after offset %d", got, w, prev)
		}
		prev += idx + len(w)
	}
}
