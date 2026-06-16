// Palette is the small set of semantic colors the rest of the TUI
// references through the package-level Color* / *Style vars. Every
// theme is a Palette value; calling [Apply] rebuilds every exported
// style var from it so a single Apply call swaps the entire palette.
//
// Field names map to semantic roles, not concrete shades, so a theme
// can shift "Accent" from red to cyan without touching call sites.
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
	// Muted is the dim grey used for blurred borders, unfocused
	// chrome, and secondary labels.
	Muted color.Color
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

	// MnemonicAccent paints the surrounding `[`/`]` of mnemonic
	// buttons. Distinct field so a theme can keep button brackets
	// red while moving Red itself to another role.
	MnemonicAccent color.Color
	// MnemonicHL paints the highlighted shortcut letter.
	MnemonicHL color.Color
	// MnemonicText paints the rest of a button's label.
	MnemonicText color.Color
}

// DefaultPalette returns the project's built-in theme: the same ANSI
// indexed palette the TUI used before themes existed. Override at
// startup with [Apply] to install a user-supplied theme.
func DefaultPalette() Palette {
	return Palette{
		Muted:   lipgloss.Color("8"),
		Cyan:    lipgloss.Color("14"),
		Red:     lipgloss.Color("9"),
		Green:   lipgloss.Color("10"),
		Yellow:  lipgloss.Color("11"),
		Magenta: lipgloss.Color("13"),

		MnemonicAccent: lipgloss.Color("9"),
		MnemonicHL:     lipgloss.Color("10"),
		MnemonicText:   lipgloss.Color("8"),
	}
}
