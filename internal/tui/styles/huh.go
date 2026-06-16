package styles

import (
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
)

// HuhTheme returns a huh.Theme derived from the active [Palette]. It
// starts from huh.ThemeBase so untouched fields keep huh's tested
// defaults, then re-paints the focus / accent / error fields with the
// project palette so a form embedded in a modal matches the rest of
// the TUI.
//
// huh forms are theme-frozen at construction time, so callers apply
// the result via form.WithTheme(...) when they build the form. A
// runtime palette swap (Apply + re-render) only takes effect when a
// new form is built; live theme swap on an open form is out of scope.
func HuhTheme() huh.Theme {
	p := current
	return huh.ThemeFunc(func(isDark bool) *huh.Styles {
		s := huh.ThemeBase(isDark)

		s.Focused.Base = s.Focused.Base.BorderForeground(p.Cyan)
		s.Focused.Title = s.Focused.Title.Foreground(p.Text).Bold(true)
		s.Focused.Description = s.Focused.Description.Foreground(p.Muted)
		s.Focused.SelectSelector = s.Focused.SelectSelector.Foreground(p.Cyan)
		s.Focused.MultiSelectSelector = s.Focused.MultiSelectSelector.Foreground(p.Cyan)
		s.Focused.Option = s.Focused.Option.Foreground(p.Text)
		s.Focused.SelectedOption = s.Focused.SelectedOption.Foreground(p.Cyan)
		s.Focused.SelectedPrefix = s.Focused.SelectedPrefix.Foreground(p.Green)
		s.Focused.UnselectedOption = s.Focused.UnselectedOption.Foreground(p.Text)
		s.Focused.FocusedButton = s.Focused.FocusedButton.
			Foreground(lipgloss.Color("0")).
			Background(p.Cyan)
		s.Focused.BlurredButton = s.Focused.BlurredButton.
			Foreground(p.Text)
		s.Focused.TextInput.Cursor = s.Focused.TextInput.Cursor.Foreground(p.Cyan)
		s.Focused.TextInput.Prompt = s.Focused.TextInput.Prompt.Foreground(p.Cyan)
		s.Focused.TextInput.Text = s.Focused.TextInput.Text.Foreground(p.Text)
		s.Focused.ErrorMessage = s.Focused.ErrorMessage.Foreground(p.Red)
		s.Focused.ErrorIndicator = s.Focused.ErrorIndicator.Foreground(p.Red)

		s.Blurred.Base = s.Blurred.Base.BorderForeground(p.Muted)
		s.Blurred.Title = s.Blurred.Title.Foreground(p.Text)
		s.Blurred.Description = s.Blurred.Description.Foreground(p.Muted)
		s.Blurred.SelectSelector = s.Blurred.SelectSelector.Foreground(p.Muted)
		s.Blurred.MultiSelectSelector = s.Blurred.MultiSelectSelector.Foreground(p.Muted)
		s.Blurred.Option = s.Blurred.Option.Foreground(p.Text)
		s.Blurred.SelectedOption = s.Blurred.SelectedOption.Foreground(p.Text)
		s.Blurred.UnselectedOption = s.Blurred.UnselectedOption.Foreground(p.Text)
		s.Blurred.TextInput.Text = s.Blurred.TextInput.Text.Foreground(p.Text)
		s.Blurred.ErrorMessage = s.Blurred.ErrorMessage.Foreground(p.Red)
		s.Blurred.ErrorIndicator = s.Blurred.ErrorIndicator.Foreground(p.Red)

		s.Help.Ellipsis = s.Help.Ellipsis.Foreground(p.Muted)
		s.Help.ShortKey = s.Help.ShortKey.Foreground(p.Muted)
		s.Help.ShortDesc = s.Help.ShortDesc.Foreground(p.Muted)
		s.Help.ShortSeparator = s.Help.ShortSeparator.Foreground(p.Muted)
		s.Help.FullKey = s.Help.FullKey.Foreground(p.Muted)
		s.Help.FullDesc = s.Help.FullDesc.Foreground(p.Muted)
		s.Help.FullSeparator = s.Help.FullSeparator.Foreground(p.Muted)

		return s
	})
}
