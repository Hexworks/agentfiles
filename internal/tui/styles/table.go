package styles

import "charm.land/bubbles/v2/table"

// TableStyles returns the bubbles [table.Styles] every table in the
// TUI should use. Header / Cell / Selected come from the active
// [Palette] so the selected-row highlight is identical across the
// profiles screen, edit-profile screen, select-project-assets screen,
// and the inner table inside treetable.
//
// Returned by value: the bubbles widget copies styles internally, so
// a later [Apply] call rebuilds the package-level vars without
// touching tables that have already been seeded — those tables keep
// the palette they were constructed with until the host calls
// [table.Model.SetStyles] again.
func TableStyles() table.Styles {
	return table.Styles{
		Header:   TableHeaderStyle,
		Cell:     TableCellStyle,
		Selected: TableSelectedStyle,
	}
}
