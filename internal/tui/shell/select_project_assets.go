package shell

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/hexworks/agentfiles/internal/actions"
	"github.com/hexworks/agentfiles/internal/asset"
	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/profile"
	"github.com/hexworks/agentfiles/internal/project"
	"github.com/hexworks/agentfiles/internal/tui/components/focus"
	"github.com/hexworks/agentfiles/internal/tui/components/mnemonic"
)

// selectProjectAssetsActions is the narrow slice of *actions.Actions the
// Select Project Assets screen invokes. Naming the interface here keeps
// the dependency direction tui→app explicit and lets tests substitute a
// fake.
type selectProjectAssetsActions interface {
	LoadProfile(in actions.LoadProfileInput) (*profile.Profile, errs.DomainError)
	SelectAsset(in actions.SelectAssetInput) (struct{}, errs.DomainError)
	UnselectAsset(in actions.UnselectAssetInput) (struct{}, errs.DomainError)
}

// selectProjectAssetsLoadedMsg is the result of the Init load command:
// either the resolved profile + project pair or the load error.
type selectProjectAssetsLoadedMsg struct {
	prof *profile.Profile
	proj *project.Manifest
	err  errs.DomainError
}

// selectionChangedMsg is the success envelope row actions emit so Update
// can refresh the local selection mirror atomically with the notification.
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

	projectName string
	profileName string
	assetsByID  map[string]*asset.Asset
	available   []*asset.Asset
	selected    []*asset.Asset

	handler        *focus.Handler
	selectedTable  *table.Model
	availableTable *table.Model
	selectedMnemo  *mnemonic.Button // [1]
	availableMnemo *mnemonic.Button // [2]
	unselectBtn    *mnemonic.Button // u
	selectBtn      *mnemonic.Button // l
	planBtn        *mnemonic.Button // p
	backBtn        *mnemonic.Button // b + esc
	set            *mnemonic.Set
	selectedIdx    int // focus.Handler index of selected table
	availableIdx   int // focus.Handler index of available table

	loaded bool

	width, height int
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
		actions:    a,
		profileID:  profileID,
		projectID:  projectID,
		assetsByID: map[string]*asset.Asset{},
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

func (s *selectProjectAssetsScreen) loadCmd() tea.Cmd {
	return func() tea.Msg {
		prof, err := s.actions.LoadProfile(actions.LoadProfileInput{ProfileRef: s.profileID})
		if err != nil {
			return selectProjectAssetsLoadedMsg{err: err}
		}
		proj := prof.Projects[s.projectID]
		return selectProjectAssetsLoadedMsg{prof: prof, proj: proj}
	}
}

func (s *selectProjectAssetsScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		s.width = m.Width
		s.height = m.Height
		return s, nil
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
	if m.prof == nil || m.proj == nil {
		return s, nil
	}
	s.loaded = true
	s.profileName = m.prof.Manifest.Name
	s.projectName = m.proj.Name
	s.assetsByID = map[string]*asset.Asset{}
	for _, a := range m.prof.AssetList() {
		s.assetsByID[a.ID] = a
	}
	s.rebuildPartition(m.proj.SelectedAssetIDs)
	s.rebuildTables()
	focusCmd := s.handler.FocusIndex(s.selectedIdx)
	s.rebuildSet()
	return s, focusCmd
}

// rebuildPartition splits the profile's full asset list into the selected
// and available slices based on selectedIDs. Ordering follows AssetList
// (sorted by display name), so reloads stay stable.
func (s *selectProjectAssetsScreen) rebuildPartition(selectedIDs []string) {
	inSelection := make(map[string]struct{}, len(selectedIDs))
	for _, id := range selectedIDs {
		inSelection[id] = struct{}{}
	}
	all := make([]*asset.Asset, 0, len(s.assetsByID))
	for _, a := range s.assetsByID {
		all = append(all, a)
	}
	// Order matches profile.AssetList — sort by Name ascending.
	sortAssetsByName(all)
	s.selected = s.selected[:0]
	s.available = s.available[:0]
	for _, a := range all {
		if _, ok := inSelection[a.ID]; ok {
			s.selected = append(s.selected, a)
		} else {
			s.available = append(s.available, a)
		}
	}
}

func (s *selectProjectAssetsScreen) handleSelectionChanged(m selectionChangedMsg) (Screen, tea.Cmd) {
	s.rebuildPartition(m.selectedIDs)
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
	// Equalize the elastic Name column across both tables so they share
	// width. Index 1 is Name in selectProjectAssetsColumnTitles.
	const elasticIdx = 1
	equalizePanelWidth(selectedCols, availableCols, elasticIdx, elasticIdx)

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

func (s *selectProjectAssetsScreen) selectedAvailable() (*asset.Asset, bool) {
	if len(s.available) == 0 {
		return nil, false
	}
	cur := s.availableTable.Cursor()
	if cur < 0 || cur >= len(s.available) {
		return nil, false
	}
	return s.available[cur], true
}

func (s *selectProjectAssetsScreen) selectedSelected() (*asset.Asset, bool) {
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
	a, ok := s.selectedAvailable()
	if !ok {
		return nil
	}
	return s.applySelectionChange(
		func() errs.DomainError {
			_, err := s.actions.SelectAsset(actions.SelectAssetInput{
				ProfileRef: s.profileID,
				ProjectID:  s.projectID,
				AssetID:    a.ID,
			})
			return err
		},
		appendUnique(idsOf(s.selected), a.ID),
		fmt.Sprintf("Asset %q selected", a.Name),
	)
}

func (s *selectProjectAssetsScreen) onUnselect() tea.Cmd {
	a, ok := s.selectedSelected()
	if !ok {
		return nil
	}
	return s.applySelectionChange(
		func() errs.DomainError {
			_, err := s.actions.UnselectAsset(actions.UnselectAssetInput{
				ProfileRef: s.profileID,
				ProjectID:  s.projectID,
				AssetID:    a.ID,
			})
			return err
		},
		removeID(idsOf(s.selected), a.ID),
		fmt.Sprintf("Asset %q unselected", a.Name),
	)
}

// applySelectionChange runs the persistence action synchronously and, on
// success, emits a selectionChangedMsg carrying the projected new
// selection set. On failure, a mutationDoneMsg surfaces the error in the
// notification area and the local state stays as-is.
func (s *selectProjectAssetsScreen) applySelectionChange(action func() errs.DomainError, next []string, info string) tea.Cmd {
	return func() tea.Msg {
		if err := action(); err != nil {
			return mutationDoneMsg{text: err.Error(), severity: err.Severity()}
		}
		return selectionChangedMsg{selectedIDs: next, info: info}
	}
}

func (s *selectProjectAssetsScreen) onPlan() tea.Cmd {
	return pushCmd(newPlanProjectStub(s.profileID, s.projectID))
}

func sortAssetsByName(list []*asset.Asset) {
	slices.SortFunc(list, func(a, b *asset.Asset) int {
		return strings.Compare(a.Name, b.Name)
	})
}

func idsOf(list []*asset.Asset) []string {
	out := make([]string, len(list))
	for i, a := range list {
		out[i] = a.ID
	}
	return out
}

func appendUnique(ids []string, id string) []string {
	for _, existing := range ids {
		if existing == id {
			return ids
		}
	}
	return append(ids, id)
}

func removeID(ids []string, id string) []string {
	for i, existing := range ids {
		if existing == id {
			return append(ids[:i], ids[i+1:]...)
		}
	}
	return ids
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
