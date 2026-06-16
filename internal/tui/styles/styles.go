// Package styles holds the shared TUI presentation primitives — the
// semantic [Palette], the package-level color vars derived from it, the
// lipgloss styles every component renders through, the severity icon
// switch, and the terminal-safe string sanitizer. Both internal/tui and
// internal/tui/notifications depend on this leaf package so the styles
// vocabulary stays in one place and no import cycle appears when the
// shell mounts notification components.
//
// Theme model: [Palette] is the single source of truth. [Apply]
// rebuilds every exported var from a palette; callers reference the
// vars by name and never construct their own lipgloss.Style at the
// edge. To install a user-supplied theme call Apply once at startup
// (cmd/af/main.go) before tea.NewProgram runs.
package styles

import (
	"image/color"
	"strconv"
	"unicode"

	"charm.land/lipgloss/v2"

	"github.com/hexworks/agentfiles/internal/errs"
)

// ANSI color vars rebuilt by [Apply]. Existing call sites read these
// by name (e.g. styles.ColorCyan); keeping them avoids a flag-day
// rename when a theme is installed.
var (
	ColorMuted   color.Color
	ColorRed     color.Color
	ColorGreen   color.Color
	ColorYellow  color.Color
	ColorMagenta color.Color
	ColorCyan    color.Color
)

// Mnemonic-button palette vars rebuilt by [Apply]. Distinct from the
// severity palette so the activating-key hint reads as a control glyph
// rather than a status. ColorHighlight is the selected-row color used
// by the mnemonic button's "selected-row" render variant and by every
// table's Selected style — keeping the two in sync means a button on
// the cursor row visually merges with the row highlight.
var (
	ColorMnemonic  color.Color
	ColorText      color.Color
	ColorHighlight color.Color
)

// Lipgloss styles rebuilt by [Apply]. Every component renders through
// one of these — a palette swap touches Apply only.
var (
	// TextStyle is the baseline body foreground. Every non-muted
	// caller renders through this so the whole TUI shares one text
	// color independent of the terminal's default foreground.
	TextStyle lipgloss.Style

	HeaderStyle lipgloss.Style
	CleanStyle  lipgloss.Style
	MutedStyle  lipgloss.Style

	CreateStyle lipgloss.Style
	UpdateStyle lipgloss.Style
	DriftStyle  lipgloss.Style
	DeleteStyle lipgloss.Style

	ErrorStyle lipgloss.Style
	WarnStyle  lipgloss.Style
	InfoStyle  lipgloss.Style

	// ModalStyle is the themed border for components/modal callers.
	// Pass via modal.WithStyle so the modal package itself stays
	// palette-free.
	ModalStyle lipgloss.Style

	// BorderStyle / BorderFocusedStyle are the muted / focused frame
	// foregrounds consumed by panel and treetable. Stored as styles
	// (not raw colors) so consumers compose them via Style.Inherit
	// without rebuilding identical NewStyle().Foreground(...) calls
	// at every site.
	BorderStyle        lipgloss.Style
	BorderFocusedStyle lipgloss.Style

	// PanelTitleStyle is the caption embedded in a panel's top
	// border. Bold so it reads as chrome rather than body text.
	PanelTitleStyle lipgloss.Style

	// ConfirmPromptStyle / ConfirmButtonStyle / ConfirmSelectedStyle
	// drive the confirm modal's prompt + Yes/No buttons.
	ConfirmPromptStyle   lipgloss.Style
	ConfirmButtonStyle   lipgloss.Style
	ConfirmSelectedStyle lipgloss.Style

	// HelpHintStyle is the single-line footer below the manual
	// viewport ("↑/k up • ↓/j down • esc close").
	HelpHintStyle lipgloss.Style
	// HelpTabStyle is the rounded tab embedded in the help dialog's
	// top and bottom borders (title + scroll percent).
	HelpTabStyle lipgloss.Style
	// HelpModalStyle is the help dialog's outer frame: rounded
	// border, no padding, so the viewport's own title row sits flush
	// against the top edge.
	HelpModalStyle lipgloss.Style

	// ShellTitleStyle is the rounded title bar at the top of the
	// alt-screen body. Inherits HeaderStyle's Bold attribute.
	ShellTitleStyle lipgloss.Style

	// ScreenDescriptionStyle is the muted italic caption rendered by
	// the shell directly under the title. Each Screen supplies the
	// text via Screen.Description; the shell prefixes the nerd-font
	// info glyph and renders through this style.
	ScreenDescriptionStyle lipgloss.Style

	// TableHeaderStyle / TableCellStyle / TableSelectedStyle compose
	// the canonical [table.Styles] returned by [TableStyles]. Every
	// table — screen-level (profiles, edit_profile, select assets)
	// and the inner table inside treetable — uses these so the
	// selected-row highlight is identical everywhere.
	TableHeaderStyle   lipgloss.Style
	TableCellStyle     lipgloss.Style
	TableSelectedStyle lipgloss.Style
)

// current holds the active palette. Read with [Current]; replace with
// [Apply].
var current Palette

func init() {
	Apply(DefaultPalette())
}

// Current returns the palette currently installed. Useful for tests
// that want to assert against the active theme without bypassing the
// public surface.
func Current() Palette { return current }

// Apply installs p as the active palette and rebuilds every exported
// color and style var. Call once at startup after loading any
// user-supplied theme, and again on a runtime palette change. The
// function is not safe for concurrent use; callers are expected to
// Apply on the main goroutine before tea.NewProgram runs (or inside an
// Update handler, which is single-threaded by Bubble Tea contract).
func Apply(p Palette) {
	current = p

	ColorMuted = p.Muted
	ColorRed = p.Red
	ColorGreen = p.Green
	ColorYellow = p.Yellow
	ColorMagenta = p.Magenta
	ColorCyan = p.Cyan

	ColorMnemonic = p.MnemonicHL
	ColorText = p.Text
	ColorHighlight = p.Highlight

	TextStyle = lipgloss.NewStyle().Foreground(p.Text)

	// Header pins Foreground(Text) because the header row is never
	// wrapped in [TableSelectedStyle] — its color cannot be
	// overridden by a row-level highlight, so the explicit
	// foreground is safe.
	TableHeaderStyle = lipgloss.NewStyle().Foreground(p.Text).Bold(true).Padding(0, 1)
	// Cell intentionally does NOT pin Foreground. bubbles/table
	// wraps the cursor row in [TableSelectedStyle] AROUND the
	// already-cell-rendered cells; an inner Cell foreground would
	// leak through every SGR reset and override the outer
	// Selected.Foreground(Highlight). Cell text inherits the
	// terminal foreground for non-selected rows; the cursor row
	// picks up the Highlight color from [TableSelectedStyle].
	TableCellStyle = lipgloss.NewStyle().Padding(0, 1)
	TableSelectedStyle = lipgloss.NewStyle().Foreground(p.Highlight).Bold(true)

	HeaderStyle = lipgloss.NewStyle().Foreground(p.Text).Bold(true)
	CleanStyle = lipgloss.NewStyle().Foreground(p.Green)
	MutedStyle = lipgloss.NewStyle().Foreground(p.Muted)

	CreateStyle = lipgloss.NewStyle().Foreground(p.Green)
	UpdateStyle = lipgloss.NewStyle().Foreground(p.Yellow)
	DriftStyle = lipgloss.NewStyle().Foreground(p.Magenta)
	DeleteStyle = lipgloss.NewStyle().Foreground(p.Red)

	ErrorStyle = lipgloss.NewStyle().Foreground(p.Red).Bold(true)
	WarnStyle = lipgloss.NewStyle().Foreground(p.Yellow)
	InfoStyle = lipgloss.NewStyle().Foreground(p.Cyan)

	ModalStyle = lipgloss.NewStyle().
		Foreground(p.Text).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.Cyan).
		Padding(1, 2)

	BorderStyle = lipgloss.NewStyle().Foreground(p.Muted)
	BorderFocusedStyle = lipgloss.NewStyle().Foreground(p.Cyan)
	PanelTitleStyle = lipgloss.NewStyle().Foreground(p.Text).Bold(true).PaddingLeft(1)

	ConfirmPromptStyle = lipgloss.NewStyle().Foreground(p.Text).Padding(0, 0, 1, 0)
	ConfirmButtonStyle = lipgloss.NewStyle().
		Foreground(p.Text).
		Padding(0, 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.Muted)
	ConfirmSelectedStyle = lipgloss.NewStyle().
		Foreground(p.Text).
		Padding(0, 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.Cyan).
		Bold(true).
		Reverse(true)

	HelpHintStyle = lipgloss.NewStyle().Foreground(p.Muted)
	HelpTabStyle = lipgloss.NewStyle().
		Foreground(p.Text).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.Muted).
		Padding(0, 1)
	HelpModalStyle = lipgloss.NewStyle().
		Foreground(p.Text).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.Cyan)

	ShellTitleStyle = lipgloss.NewStyle().
		Foreground(p.Text).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.Muted).
		Padding(0, 1).
		Bold(true)

	ScreenDescriptionStyle = lipgloss.NewStyle().
		Foreground(p.Muted).
		Italic(true).
		PaddingLeft(1)
}

// SeverityStyle picks the icon and lipgloss style for a domain severity.
// The notifications subsystem and the error renderer share this switch
// so a warning notification and a warning error look identical.
func SeverityStyle(s errs.Severity) (string, lipgloss.Style) {
	switch s {
	case errs.SeverityError:
		return "✗", ErrorStyle
	case errs.SeverityWarning:
		return "⚠", WarnStyle
	}
	return "ℹ", InfoStyle
}

// SeverityLabel returns the short uppercase label for a severity used
// by table-shaped renderers (e.g. the notifications modal). Centralized
// so every severity-aware view picks the same vocabulary.
func SeverityLabel(s errs.Severity) string {
	switch s {
	case errs.SeverityError:
		return "ERROR"
	case errs.SeverityWarning:
		return "WARN"
	}
	return "INFO"
}

// Safe returns s rendered as a terminal-safe string. Without this, a
// manifest field or attacker-controlled filename containing raw escape
// sequences (e.g. \x1b[2J or OSC title changes), C1 controls, bidi
// overrides, or zero-width formatters could hijack terminal state or
// hide path segments behind a flipped glyph.
//
// Strategy: if every rune in s is "obviously safe" (printable; newline
// or tab) the string is returned unchanged. As soon as a hostile rune
// is detected the entire string is returned via strconv.Quote so the
// user sees the literal escaped form (`"foo‮dlrow.txt"`) rather than
// the rendered glyph. Coarser than per-rune stripping, but deterministic
// and impossible to misread.
func Safe(s string) string {
	if s == "" {
		return s
	}
	for _, r := range s {
		if !isTerminalSafeRune(r) {
			return strconv.Quote(s)
		}
	}
	return s
}

// isTerminalSafeRune reports whether r is safe to render directly into
// a lipgloss-styled terminal string. Newline and tab are allowed; every
// other control rune, C1 control, bidi override, or zero-width
// formatter is rejected.
func isTerminalSafeRune(r rune) bool {
	switch r {
	case '\n', '\t':
		return true
	}
	if r < 0x20 || r == 0x7f {
		return false
	}
	// C1 controls (U+0080..U+009F).
	if r >= 0x80 && r <= 0x9f {
		return false
	}
	// Bidi overrides and isolates (U+202A..U+202E, U+2066..U+2069).
	if (r >= 0x202a && r <= 0x202e) || (r >= 0x2066 && r <= 0x2069) {
		return false
	}
	// Zero-width formatters and BOM (U+200B..U+200D, U+FEFF).
	if r == 0xfeff || (r >= 0x200b && r <= 0x200d) {
		return false
	}
	// Any remaining Unicode-class control rune (Cc).
	if unicode.IsControl(r) {
		return false
	}
	return true
}
