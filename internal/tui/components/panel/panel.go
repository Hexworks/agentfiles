// Package panel renders a body string wrapped in a rounded frame whose
// top border may embed a caption — the `┌Caption─...─┐` look shared by
// every focus-aware container in the TUI (tables, treetables, huh
// fields). Centralizing the frame here keeps tables, treetables and form
// fields visually identical and removes the duplicated hand-drawn frame
// logic that lived in each screen.
//
// The component is pure presentation: no state, no message handling, no
// bubbletea. Hosts compute focus + caption + body and call [Render].
package panel

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/hexworks/agentfiles/internal/tui/styles"
)

// Styles bundles the frame and caption presentation.
//
// Border colors the frame glyphs (corners, edges, dividers) when the
// hosting widget is blurred; BorderFocused does the same when focused.
// Both should be foreground-only styles — the frame is drawn by hand
// using rounded box-drawing glyphs, so a lipgloss border attribute on
// either style would render a second nested border around every glyph.
//
// Title styles the caption embedded in the top border; when no caption
// is passed to [Render] the field is unused.
type Styles struct {
	Border        lipgloss.Style
	BorderFocused lipgloss.Style
	Title         lipgloss.Style
}

// DefaultStyles returns styles taken from the active [styles.Palette]:
// muted border when blurred, cyan border when focused, bold caption.
// Hosts that want a different palette pass an explicit [Styles] value
// to [Render].
func DefaultStyles() Styles {
	return Styles{
		Border:        styles.BorderStyle,
		BorderFocused: styles.BorderFocusedStyle,
		Title:         styles.PanelTitleStyle,
	}
}

// Render wraps body in a rounded frame. When caption is non-empty it is
// embedded in the top border styled via st.Title; otherwise the top
// border is a plain run of dashes. Frame color picks st.BorderFocused
// when focused is true, st.Border otherwise.
//
// body is measured with lipgloss.Width per line so embedded ANSI
// sequences do not skew the right-edge alignment. Lines shorter than
// the widest line are padded with spaces so the right frame edge stays
// flush.
func Render(focused bool, caption, body string, st Styles) string {
	frame := st.Border
	if focused {
		frame = st.BorderFocused
	}
	bodyW := lipgloss.Width(body)
	pad := bodyW
	var titleSegment string
	if caption != "" {
		titleSegment = st.Title.Render(caption)
		pad = bodyW - lipgloss.Width(titleSegment)
		if pad < 0 {
			pad = 0
		}
	}

	var sb strings.Builder
	sb.WriteString(frame.Render("╭"))
	sb.WriteString(titleSegment)
	sb.WriteString(frame.Render(strings.Repeat("─", pad) + "╮"))
	sb.WriteString("\n")

	left := frame.Render("│")
	right := frame.Render("│")
	for _, line := range strings.Split(body, "\n") {
		gap := bodyW - lipgloss.Width(line)
		if gap < 0 {
			gap = 0
		}
		sb.WriteString(left)
		sb.WriteString(line)
		sb.WriteString(strings.Repeat(" ", gap))
		sb.WriteString(right)
		sb.WriteString("\n")
	}
	sb.WriteString(frame.Render("╰" + strings.Repeat("─", bodyW) + "╯"))
	return sb.String()
}
