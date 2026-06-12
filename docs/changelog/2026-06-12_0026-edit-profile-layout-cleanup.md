# 0026 follow-up — layout cleanup

Reworked Edit Profile and Profiles screen rendering so every screen
sizes itself to its content rather than the terminal, and patched a
sticky `bubbles/table` cursor bug that dropped the last row of every
table on first render.

## Decisions

- **`Screen.Body(width int) string` — drop the height parameter.** **Why:** The previous contract handed each screen a body rectangle equal to `terminal - chrome` and required the body to match it exactly so the status bar stayed pinned to the bottom. Bodies filled with padding hid how much content actually existed (a single profile rendered an 18-row tall table on a 20-row body). The shell now stacks `title + body + toast + status` with `lipgloss.JoinVertical` and trusts the body to render at its natural height.
- **Tables render at natural width + height.** **Why:** The shell no longer reserves space, so each `bubbles/table` is sized to `len(rows) + 1` (header) and each column to `max(headerWidth, max(rowContentWidth))`. The two helpers `naturalColumns(titles, rows)` and `tableNaturalWidth(cols)` in `internal/tui/shell/cellrender.go` compute these once per render and stay framework-neutral.
- **Edit Profile equalizes panel widths.** **Why:** Two side-by-side tables that disagree on outer width look broken. `equalizePanelWidth(a, b, elasticA, elasticB)` grows the narrower table's elastic column (Name for Assets at index 1, Path for Projects at index 2) so both bordered panels render to the same outer width without losing per-column content fit.
- **Each panel wears a rounded border whose color signals focus.** **Why:** With the `[1]` / `[2]` focus mnemonic buttons gone (see below), users need a visual cue for which panel keystrokes go to. `panelBorderFor(focused bool)` picks `BorderForeground(ColorCyan)` when focused and `BorderForeground(ColorMuted)` when not. The Profiles screen uses the focused variant unconditionally because it hosts a single table.
- **Removed `[1]` / `[2]` focus mnemonic buttons and the `ctrl+digit` shortcuts that drove them.** **Why:** The buttons duplicated information now carried by the focused-border color and added two extra entries to the mnemonic uniqueness alphabet (`{1, 2, c, r, b, e, d, a, p}` → `{c, r, b, e, d, a, p}`). `Tab` / `Shift+Tab` cycling through `focus.Handler` is preserved and remains the only way to switch panels.
- **`projectActionsCell` renders the full action labels instead of the compact `[E] [A] [P] [D]`.** **Why:** With the table no longer expanding to fill the terminal, the long-form labels fit comfortably and removed the previous compromise the comment had to apologize for. `assetActionsCell` was already long-form.
- **`sanitizeCursor(t, rowCount)` guards every render that consumes `table.Cursor()`.** **Why:** `bubbles/v2/table.SetRows` clamps the cursor down when `len(rows)` drops (cursor=0 with a nil slice becomes cursor=-1) but never raises it back when rows reappear. The first `View()` after a screen push runs before the screen's load command completes — `Body` hands the table empty rows, `SetRows(nil)` parks cursor at -1, and the next render after the load builds rows with no action cell on any row *and* drops one viewport line (the viewport's `end = clamp(cursor + height, cursor, len(rows))` evaluates to `len(rows) - 1`). `sanitizeCursor` snaps an out-of-range cursor back into `[0, rowCount)` so the row data + action cell + viewport count all agree on the next render. `applyTable` additionally skips `SetRows` on an empty rows slice so the corruption never happens in the first place.

## Assumptions

- **`Tab` / `Shift+Tab` discoverability is sufficient without an on-screen hint.** **Why:** The status bar still shows the focused row's mnemonics, and the focused-border color makes the active panel obvious. Adding a "tab to switch" line would crowd the row of `[Create Asset] [Register Project] [Back]` buttons.
- **`bubbles` will not fix the SetRows(nil) cursor=-1 sticky behavior upstream.** **Why:** The behavior matches the comment in `SetRows` ("clamp cursor down to keep it in range") and changing it would break apps that rely on the clamped sentinel. The shell carries the recovery.

## Other Notes

- ADR 0011 (TUI Screen Router) updated to reflect the new `Body(width int)` signature.
- `docs/guidelines/tui.md` gained a "Natural Sizing For Bubble Tea Screens" section that captures the four contracts: natural row count, natural column widths, equal outer widths for side-by-side panels, and cursor sanitation.
- Task `tasks/current/0026_feature_edit-profile-screen/description.md` updated to drop the `[1]` / `[2]` focus model and reflect the natural-sizing contract.
- No ADR added: the changes refine ADR 0011 without altering its core decision (screen-router stack).

## Shell stops reserving a body rectangle

```go
// before — shell.View subtracted chrome and forced body to match
bodyH := m.height - headerH - toastH - lipgloss.Height(status)
if bodyH < 0 {
    bodyH = 0
}
body := top.Body(m.width, bodyH)
```

```go
// after — body sizes itself; shell joins everything top-down
body := top.Body(m.width)
sections := []string{title, body}
if toast != "" {
    sections = append(sections, toast)
}
sections = append(sections, status)
v := tea.NewView(lipgloss.JoinVertical(lipgloss.Left, sections...))
```

## Natural column widths + applyTable

```go
// after — cellrender.go provides framework-neutral helpers every screen reuses
func naturalColumns(titles []string, rows []table.Row) []table.Column {
    widths := make([]int, len(titles))
    for i, t := range titles {
        widths[i] = lipgloss.Width(t)
    }
    for _, r := range rows {
        for i, cell := range r {
            if i >= len(widths) {
                break
            }
            if w := lipgloss.Width(cell); w > widths[i] {
                widths[i] = w
            }
        }
    }
    cols := make([]table.Column, len(titles))
    for i, t := range titles {
        cols[i] = table.Column{Title: t, Width: widths[i]}
    }
    return cols
}

func applyTable(t *table.Model, cols []table.Column, rows []table.Row) {
    t.SetColumns(cols)
    t.SetWidth(tableNaturalWidth(cols))
    if len(rows) > 0 {
        t.SetRows(rows)
    }
    t.SetHeight(len(rows) + 1)
}
```

## Cursor sanitation guards `Body` from the bubbles `SetRows(nil)` bug

```go
// after — sanitizeCursor recovers cursor before the screen reads it for
// the action-cell decision, so a pre-load render that handed the table a
// nil rows slice never silently drops a row on the next render.
func sanitizeCursor(t *table.Model, rowCount int) {
    if rowCount == 0 {
        return
    }
    c := t.Cursor()
    if c < 0 || c >= rowCount {
        t.SetCursor(0)
    }
}

// editProfileScreen.renderBody
sanitizeCursor(s.assetsTable, len(s.assets))
sanitizeCursor(s.projectsTable, len(s.projects))
assetRows := s.buildAssetsRows(s.assetsTable.Cursor())
projectRows := s.buildProjectsRows(s.projectsTable.Cursor())
```

The regression test
`TestEditProfileScreen_BodyRecoversFromPreLoadRender` reproduces the
exact sequence (Body → load → Body) and asserts all rows + the action
cell on the cursor row appear in the rendered body.

## Equalized panel widths + focus border

```go
// after — panel widths agree; border color signals focus
equalizePanelWidth(assetCols, projectCols, 1 /* asset Name */, 2 /* project Path */)
applyTable(s.assetsTable, assetCols, assetRows)
applyTable(s.projectsTable, projectCols, projectRows)

focused := s.handler.Focused()
return lipgloss.JoinVertical(
    lipgloss.Left,
    assetsHeader,
    panelBorderFor(focused == 0).Render(s.assetsTable.View()),
    "",
    projectsHeader,
    panelBorderFor(focused == 1).Render(s.projectsTable.View()),
    "",
    buttonRow,
)

var (
    focusedPanelBorder   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(styles.ColorCyan)
    unfocusedPanelBorder = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(styles.ColorMuted)
)
```

## Focus mnemonic buttons removed

```go
// before — newEditProfileScreen wired [1] / [2] ctrl-digit mnemonics
s.handler = focus.New(focus.WithModifier(focus.ModCtrl))
s.assetsFocusButton = s.handler.AddMnemonic(s.assetsTable, '1')
s.projectsFocusButton = s.handler.AddMnemonic(s.projectsTable, '2')
```

```go
// after — Tab / Shift+Tab cycle focus; border color is the only visual cue
s.handler = focus.New()
s.handler.Add(s.assetsTable)
s.handler.Add(s.projectsTable)
```

Tests `TestEditProfileScreen_Ctrl1FocusesAssets` and
`TestEditProfileScreen_Ctrl2FocusesProjects` were deleted; the mnemonic
uniqueness tests dropped `1` / `2` from their expected label slices.
