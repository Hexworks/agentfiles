package shell

import (
	"fmt"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/hexworks/agentfiles/internal/actions"
	"github.com/hexworks/agentfiles/internal/asset"
	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/profile"
	"github.com/hexworks/agentfiles/internal/project"
	llmsync "github.com/hexworks/agentfiles/internal/sync"
	"github.com/hexworks/agentfiles/internal/tui/components/focus"
	"github.com/hexworks/agentfiles/internal/tui/components/mnemonic"
)

// selectProjectAssetsActions is the narrow slice of *actions.Actions the
// Select Project Assets screen invokes. Naming the interface here keeps
// the dependency direction tui→app explicit and lets tests substitute a
// fake.
type selectProjectAssetsActions interface {
	LoadProfile(in actions.LoadProfileInput) (*profile.Profile, errs.DomainError)
	LoadProject(in actions.LoadProjectInput) (*project.Manifest, errs.DomainError)
	SelectAsset(in actions.SelectAssetInput) ([]string, errs.DomainError)
	UnselectAsset(in actions.UnselectAssetInput) ([]string, errs.DomainError)
	PlanProject(in actions.PlanProjectInput) (*llmsync.Preview, errs.DomainError)
	SyncProject(in actions.SyncProjectInput) (*llmsync.Preview, errs.DomainError)
}

// selectProjectAssetsLoadedMsg is the result of the Init load command:
// either the resolved profile + project pair or the load error.
type selectProjectAssetsLoadedMsg struct {
	prof *profile.Profile
	proj *project.Manifest
	err  errs.DomainError
}

// selectionChangedMsg is the success envelope row actions emit so Update
// can refresh the local selection mirror atomically with the
// notification. selectedIDs carries the post-persistence selection
// returned by the service so the screen never projects the next state
// itself.
type selectionChangedMsg struct {
	selectedIDs []string
	info        string
}

// selectProjectAssetsScreen is the management screen reached from the
// Edit Profile row-level `[Select Assets]` action on a Project row. It
// hosts two bubbles/table views (Selected on top, Available below), a
// focus.Handler exposing `[1]` / `[2]` ctrl+digit mnemonics, per-row
// Select/Unselect buttons, and screen-level Plan / Back buttons. Every
// row action persists immediately via SelectAsset / UnselectAsset so the
// Plan Project screen can plan on the current state.
type selectProjectAssetsScreen struct {
	actions   selectProjectAssetsActions
	profileID string
	projectID string

	prof        *profile.Profile
	projectName string
	profileName string
	selectedIDs []string
	available   []*asset.Asset
	selected    []*asset.Asset

	handler        *focus.Handler
	selectedTable  *table.Model
	availableTable *table.Model
	selectedMnemo  *mnemonic.Button
	availableMnemo *mnemonic.Button
	unselectBtn    *mnemonic.Button
	selectBtn      *mnemonic.Button
	planBtn        *mnemonic.Button
	backBtn        *mnemonic.Button
	set            *mnemonic.Set
	selectedIdx    int
	availableIdx   int

	loaded bool
}

var selectProjectAssetsColumnTitles = []string{"ID", "Name", "Type", "Exclusive Group", "Actions"}

func newSelectProjectAssetsScreen(a selectProjectAssetsActions, profileID, projectID string) *selectProjectAssetsScreen {
	if a == nil {
		panic("shell.newSelectProjectAssetsScreen: nil actions")
	}
	if profileID == "" {
		panic("shell.newSelectProjectAssetsScreen: empty profileID")
	}
	if projectID == "" {
		panic("shell.newSelectProjectAssetsScreen: empty projectID")
	}
	s := &selectProjectAssetsScreen{
		actions:   a,
		profileID: profileID,
		projectID: projectID,
	}
	s.buildTables()
	s.buildButtons()
	s.handler = focus.New(focus.WithModifier(focus.ModCtrl))
	s.selectedIdx = 0
	s.selectedMnemo = s.handler.AddMnemonic(s.selectedTable, '1')
	s.availableIdx = 1
	s.availableMnemo = s.handler.AddMnemonic(s.availableTable, '2')
	s.rebuildSet()
	return s
}

func (s *selectProjectAssetsScreen) buildTables() {
	cols := naturalColumns(selectProjectAssetsColumnTitles, nil)
	st := table.New(
		table.WithColumns(cols),
		table.WithRows(nil),
		table.WithFocused(true),
		table.WithWidth(tableNaturalWidth(cols)),
		table.WithHeight(1),
	)
	s.selectedTable = &st
	at := table.New(
		table.WithColumns(cols),
		table.WithRows(nil),
		table.WithFocused(true),
		table.WithWidth(tableNaturalWidth(cols)),
		table.WithHeight(1),
	)
	s.availableTable = &at
}

func (s *selectProjectAssetsScreen) buildButtons() {
	s.unselectBtn = mnemonic.New("Unselect", 'u', func() tea.Cmd { return s.onUnselect() })
	s.selectBtn = mnemonic.New("Select", 'l', func() tea.Cmd { return s.onSelect() })
	s.planBtn = mnemonic.New("Plan", 'p', func() tea.Cmd { return s.onPlan() })
	s.backBtn = mnemonic.New(
		"Back",
		'b',
		func() tea.Cmd { return popCmd() },
		mnemonic.WithExtraBindingKeys("esc"),
	)
}

// rebuildSet refreshes the mnemonic set so its registration-time
// uniqueness check covers the current focus + data state. Focus mnemonics
// `[1]` / `[2]` are always present; per-row Unselect / Select only when
// the focused table has rows; screen-level Plan / Back are always present.
func (s *selectProjectAssetsScreen) rebuildSet() {
	set := mnemonic.NewSet()
	if s.selectedMnemo != nil {
		set.Add(s.selectedMnemo)
	}
	if s.availableMnemo != nil {
		set.Add(s.availableMnemo)
	}
	switch s.handler.Focused() {
	case s.selectedIdx:
		if len(s.selected) > 0 {
			set.Add(s.unselectBtn)
		}
	case s.availableIdx:
		if len(s.available) > 0 {
			set.Add(s.selectBtn)
		}
	}
	set.Add(s.planBtn)
	set.Add(s.backBtn)
	s.set = set
}

func (s *selectProjectAssetsScreen) ProfileID() string { return s.profileID }
func (s *selectProjectAssetsScreen) ProjectID() string { return s.projectID }

func (s *selectProjectAssetsScreen) Init() tea.Cmd { return s.loadCmd() }

// loadCmd resolves the project first (typed ProjectNotFoundError when
// missing) and then the parent profile for the screen-header metadata.
// Either failure routes through the existing notification path; both
// successes deliver the matched pair so handleLoaded never has to guard
// against a silent miss.
func (s *selectProjectAssetsScreen) loadCmd() tea.Cmd {
	return func() tea.Msg {
		proj, projErr := s.actions.LoadProject(actions.LoadProjectInput{
			ProfileRef: s.profileID,
			ProjectID:  s.projectID,
		})
		if projErr != nil {
			return selectProjectAssetsLoadedMsg{err: projErr}
		}
		prof, profErr := s.actions.LoadProfile(actions.LoadProfileInput{ProfileRef: s.profileID})
		if profErr != nil {
			return selectProjectAssetsLoadedMsg{err: profErr}
		}
		return selectProjectAssetsLoadedMsg{prof: prof, proj: proj}
	}
}

func (s *selectProjectAssetsScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch m := msg.(type) {
	case selectProjectAssetsLoadedMsg:
		return s.handleLoaded(m)
	case selectionChangedMsg:
		return s.handleSelectionChanged(m)
	case mutationDoneMsg:
		return s, notificationCmd(m.severity, m.text)
	case tea.KeyPressMsg:
		return s.handleKey(m)
	}
	return s, nil
}

func (s *selectProjectAssetsScreen) handleLoaded(m selectProjectAssetsLoadedMsg) (Screen, tea.Cmd) {
	if m.err != nil {
		return s, notificationCmd(m.err.Severity(), m.err.Error())
	}
	s.loaded = true
	s.prof = m.prof
	s.profileName = m.prof.Manifest.Name
	s.projectName = m.proj.Name
	s.selectedIDs = append([]string(nil), m.proj.SelectedAssetIDs...)
	s.rebuildPartition()
	s.rebuildTables()
	focusCmd := s.handler.FocusIndex(s.selectedIdx)
	s.rebuildSet()
	return s, focusCmd
}

// rebuildPartition refreshes the selected/available slices using the
// profile's PartitionAssets so the screen never re-implements the
// ordering or membership rule.
func (s *selectProjectAssetsScreen) rebuildPartition() {
	if s.prof == nil {
		s.selected = nil
		s.available = nil
		return
	}
	s.selected, s.available = s.prof.PartitionAssets(s.selectedIDs)
}

func (s *selectProjectAssetsScreen) handleSelectionChanged(m selectionChangedMsg) (Screen, tea.Cmd) {
	s.selectedIDs = append([]string(nil), m.selectedIDs...)
	s.rebuildPartition()
	s.rebuildTables()
	s.rebuildSet()
	return s, notificationCmd(errs.SeverityInfo, m.info)
}

func (s *selectProjectAssetsScreen) handleKey(m tea.KeyPressMsg) (Screen, tea.Cmd) {
	if handled, cmd := s.handler.Update(m); handled {
		s.rebuildSet()
		return s, cmd
	}
	if btn := s.set.Match(m); btn != nil {
		return s, btn.Trigger()
	}
	return s, s.routeToFocusedTable(m)
}

// routeToFocusedTable forwards keypresses the handler and mnemonic set
// did not consume to the table the handler considers focused. On a cursor
// change, the actions cell rebuilds for the newly-selected row.
func (s *selectProjectAssetsScreen) routeToFocusedTable(m tea.KeyPressMsg) tea.Cmd {
	c, ok := s.handler.FocusedComponent().(*table.Model)
	if !ok {
		return nil
	}
	before := c.Cursor()
	updated, cmd := c.Update(m)
	*c = updated
	if c.Cursor() != before {
		switch c {
		case s.selectedTable:
			c.SetRows(s.buildSelectedRows(c.Cursor()))
		case s.availableTable:
			c.SetRows(s.buildAvailableRows(c.Cursor()))
		}
	}
	return cmd
}

func (s *selectProjectAssetsScreen) Title() string { return "Select Project Assets" }

// InputFocused is always false — this screen hosts no text inputs.
func (s *selectProjectAssetsScreen) InputFocused() bool { return false }

// StatusKeys returns the focused table's row-context mnemonic plus [Back].
// Per the screen contract, screen-level Plan / Back are not duplicated in
// the bar except for [Back] which mirrors the explicit Edit-Profile rule
// so users always have a way out.
func (s *selectProjectAssetsScreen) StatusKeys() []key.Binding {
	out := make([]key.Binding, 0, 2)
	switch s.handler.Focused() {
	case s.selectedIdx:
		if len(s.selected) > 0 {
			out = append(out, s.unselectBtn.Binding())
		}
	case s.availableIdx:
		if len(s.available) > 0 {
			out = append(out, s.selectBtn.Binding())
		}
	}
	out = append(out, s.backBtn.Binding())
	return out
}

func (s *selectProjectAssetsScreen) Body(width int) string {
	if !s.loaded {
		return " Loading…"
	}
	sanitizeCursor(s.selectedTable, len(s.selected))
	sanitizeCursor(s.availableTable, len(s.available))
	selectedRows := s.buildSelectedRows(s.selectedTable.Cursor())
	availableRows := s.buildAvailableRows(s.availableTable.Cursor())

	selectedCols := naturalColumns(selectProjectAssetsColumnTitles, selectedRows)
	availableCols := naturalColumns(selectProjectAssetsColumnTitles, availableRows)
	const selectedElasticIdx = 1
	const availableElasticIdx = 1
	equalizePanelWidth(selectedCols, availableCols, selectedElasticIdx, availableElasticIdx)

	applyTable(s.selectedTable, selectedCols, selectedRows)
	applyTable(s.availableTable, availableCols, availableRows)

	header := fmt.Sprintf(
		" Selecting assets for project %q (%s)",
		s.projectName, s.profileName,
	)
	focused := s.handler.Focused()
	selectedHeader := lipgloss.NewStyle().Bold(true).Render(
		s.selectedMnemo.View() + " Selected Assets",
	)
	availableHeader := lipgloss.NewStyle().Bold(true).Render(
		s.availableMnemo.View() + " Available Assets",
	)
	buttonRow := lipgloss.PlaceHorizontal(
		width, lipgloss.Right,
		s.planBtn.View()+"  "+s.backBtn.View(),
	)

	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		"",
		selectedHeader,
		panelBorderFor(focused == s.selectedIdx).Render(s.selectedTable.View()),
		"",
		availableHeader,
		panelBorderFor(focused == s.availableIdx).Render(s.availableTable.View()),
		"",
		buttonRow,
	)
}

func (s *selectProjectAssetsScreen) buildSelectedRows(cursor int) []table.Row {
	rows := make([]table.Row, len(s.selected))
	for i, a := range s.selected {
		cell := ""
		if i == cursor {
			cell = unselectActionsCell()
		}
		rows[i] = table.Row{a.ID, a.Name, string(a.Type), a.ExclusiveGroup, cell}
	}
	return rows
}

func (s *selectProjectAssetsScreen) buildAvailableRows(cursor int) []table.Row {
	rows := make([]table.Row, len(s.available))
	for i, a := range s.available {
		cell := ""
		if i == cursor {
			cell = selectActionsCell()
		}
		rows[i] = table.Row{a.ID, a.Name, string(a.Type), a.ExclusiveGroup, cell}
	}
	return rows
}

// unselectActionsCell renders "[Unselect]" with the mnemonic letter
// underlined via shell.underline so the surrounding cursor-row highlight
// survives.
func unselectActionsCell() string {
	return "[" + underline("U") + "nselect]"
}

// selectActionsCell renders "[Select]" with the mnemonic letter
// underlined via shell.underline so the surrounding cursor-row highlight
// survives.
func selectActionsCell() string {
	return "[Se" + underline("l") + "ect]"
}

func (s *selectProjectAssetsScreen) availableAtCursor() (*asset.Asset, bool) {
	if len(s.available) == 0 {
		return nil, false
	}
	cur := s.availableTable.Cursor()
	if cur < 0 || cur >= len(s.available) {
		return nil, false
	}
	return s.available[cur], true
}

func (s *selectProjectAssetsScreen) selectedAtCursor() (*asset.Asset, bool) {
	if len(s.selected) == 0 {
		return nil, false
	}
	cur := s.selectedTable.Cursor()
	if cur < 0 || cur >= len(s.selected) {
		return nil, false
	}
	return s.selected[cur], true
}

func (s *selectProjectAssetsScreen) onSelect() tea.Cmd {
	a, ok := s.availableAtCursor()
	if !ok {
		return nil
	}
	return s.applySelectionChange(
		func() ([]string, errs.DomainError) {
			return s.actions.SelectAsset(actions.SelectAssetInput{
				ProfileRef: s.profileID,
				ProjectID:  s.projectID,
				AssetID:    a.ID,
			})
		},
		fmt.Sprintf("Asset %q selected", a.Name),
	)
}

func (s *selectProjectAssetsScreen) onUnselect() tea.Cmd {
	a, ok := s.selectedAtCursor()
	if !ok {
		return nil
	}
	return s.applySelectionChange(
		func() ([]string, errs.DomainError) {
			return s.actions.UnselectAsset(actions.UnselectAssetInput{
				ProfileRef: s.profileID,
				ProjectID:  s.projectID,
				AssetID:    a.ID,
			})
		},
		fmt.Sprintf("Asset %q unselected", a.Name),
	)
}

// applySelectionChange runs the persistence action and, on success,
// emits a selectionChangedMsg carrying the server-side selection slice
// the action returned. On failure, a mutationDoneMsg surfaces the error
// in the notification area and the local state stays as-is.
func (s *selectProjectAssetsScreen) applySelectionChange(action func() ([]string, errs.DomainError), info string) tea.Cmd {
	return func() tea.Msg {
		next, err := action()
		if err != nil {
			return mutationDoneMsg{text: err.Error(), severity: err.Severity()}
		}
		return selectionChangedMsg{selectedIDs: next, info: info}
	}
}

func (s *selectProjectAssetsScreen) onPlan() tea.Cmd {
	return pushCmd(newPlanProjectScreen(s.actions, s.profileID, s.projectID))
}

func (s *selectProjectAssetsScreen) rebuildTables() {
	rows := s.buildSelectedRows(0)
	cols := naturalColumns(selectProjectAssetsColumnTitles, rows)
	s.selectedTable.SetColumns(cols)
	s.selectedTable.SetWidth(tableNaturalWidth(cols))
	s.selectedTable.SetRows(rows)
	s.selectedTable.SetHeight(len(rows) + 1)

	arows := s.buildAvailableRows(0)
	acols := naturalColumns(selectProjectAssetsColumnTitles, arows)
	s.availableTable.SetColumns(acols)
	s.availableTable.SetWidth(tableNaturalWidth(acols))
	s.availableTable.SetRows(arows)
	s.availableTable.SetHeight(len(arows) + 1)
}
