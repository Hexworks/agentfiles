// Package styles Palette is the small set of semantic colors the rest of the TUI
// references through the package-level Color* / *Style vars. Every
// theme is a Palette value; calling [Apply] rebuilds every exported
// style var from it so a single Apply call swaps the entire palette.
//
// Field names map to semantic roles, not concrete shades, so a theme
// can shift a role from red to cyan without touching call sites.
package styles

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// Palette holds every semantic color the rest of the TUI consumes.
// New theme keys go here; never reach for a raw color in a caller.
//
// All fields use image/color.Color (the lipgloss v2 color interface)
// so a theme can supply ANSI indices, hex strings, or true-color
// values interchangeably.
type Palette struct {
	// Text is the foreground for all body text that is not
	// explicitly muted: panel content, table cells, form values,
	// the non-mnemonic chunk of button labels. Sets the baseline
	// readability of the TUI; every other foreground (Muted,
	// MnemonicHL) is defined relative to it.
	Text color.Color
	// Muted is the dim grey used for blurred borders, unfocused
	// chrome, footer hints, and secondary labels.
	Muted color.Color
	// Highlight is the selected-row color: name column, value
	// columns, and the rest-of-label chunk of any mnemonic button
	// rendered inside the cursor row. The button accent + mnemonic
	// letter keep their own colors so the shortcut hint stays
	// recognizable against the row highlight.
	Highlight color.Color
	// Cyan is the canonical "focused" highlight (border, info text,
	// modal border, panel title-on-focus).
	Cyan color.Color
	// Red is the destructive / error color.
	Red color.Color
	// Green is the success / create color.
	Green color.Color
	// Yellow is the warning / update color.
	Yellow color.Color
	// Magenta is the drift color (local divergence from rendered).
	Magenta color.Color

	// MnemonicHL paints the highlighted shortcut letter inside a
	// mnemonic button label. The brackets and the rest of the
	// label use [Text] so the button reads as text-with-one-letter-
	// emphasized rather than its own widget color.
	MnemonicHL color.Color
}

// DefaultPalette returns the project's built-in theme: the same ANSI
// indexed palette the TUI used before themes existed. Override at
// startup with [Apply] to install a user-supplied theme.
func DefaultPalette() Palette {
	return Palette{
		Text:      lipgloss.Color("15"),
		Muted:     lipgloss.Color("8"),
		Highlight: lipgloss.Color("14"),
		Cyan:      lipgloss.Color("14"),
		Red:       lipgloss.Color("9"),
		Green:     lipgloss.Color("10"),
		Yellow:    lipgloss.Color("11"),
		Magenta:   lipgloss.Color("13"),

		MnemonicHL: lipgloss.Color("10"),
	}
}
