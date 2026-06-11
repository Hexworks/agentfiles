package shell

import (
	"fmt"
	"sort"

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
	"github.com/hexworks/agentfiles/internal/tui/components/modal"
	"github.com/hexworks/agentfiles/internal/tui/modals"
)

// editProfileScreen is the Edit Profile management screen reached from
// the Profiles row-level `[Edit]` action. It owns two bubbles/table
// views (Assets and Projects), a focus.Handler that exposes `[1]` /
// `[2]` ctrl+digit mnemonics, row-level action buttons that swap with
// the focused table, and screen-level Create Asset / Register Project /
// Back mnemonic buttons. Confirmation and form modals composite over
// the body; no sub-screen is pushed for them.
type editProfileScreen struct {
	actions   *actions.Actions
	profileID string

	prof         *profile.Profile
	assetsList   []*asset.Asset
	projectsList []*project.Manifest

	handler       *focus.Handler
	assetsTable   *table.Model
	projectsTable *table.Model
	assetsPanel   *mnemonic.Button // [1]
	projectsPanel *mnemonic.Button // [2]

	editAsset   *mnemonic.Button // e (assets)
	deleteAsset *mnemonic.Button // d (assets)

	editProject   *mnemonic.Button // e (projects)
	selectAssets  *mnemonic.Button // a (projects)
	planProject   *mnemonic.Button // p (projects)
	deleteProject *mnemonic.Button // d (projects)

	createAsset *mnemonic.Button // c
	register    *mnemonic.Button // r
	back        *mnemonic.Button // b + esc

	set *mnemonic.Set

	modal              *modal.Modal
	pendingDeleteAsset string
	pendingDeleteProj  string

	width, height int
}

// editProfileLoadedMsg carries the loaded profile (or load error) that
// Init's command produces. Update populates both tables and seeds focus
// on receipt.
type editProfileLoadedMsg struct {
	prof *profile.Profile
	err  errs.DomainError
}

// editProfileMutationDoneMsg envelopes a finished Create / Update /
// Delete action. Update reacts by emitting a notification plus a
// profile reload — the same pattern profilesScreen uses, for the same
// reason: tea.Sequence's wrapper is unexported and not inspectable in
// tests, so a custom envelope is the testable path.
type editProfileMutationDoneMsg struct {
	text     string
	severity errs.Severity
}

func newEditProfileScreen(a *actions.Actions, profileID string) *editProfileScreen {
	if a == nil {
		panic("shell.newEditProfileScreen: nil actions")
	}
	if profileID == "" {
		panic("shell.newEditProfileScreen: empty profileID")
	}
	s := &editProfileScreen{
		actions:   a,
		profileID: profileID,
	}
	s.buildTables()
	s.buildButtons()
	s.handler = focus.New(focus.WithModifier(focus.ModCtrl))
	s.assetsPanel = s.handler.AddMnemonic(s.assetsTable, '1')
	s.projectsPanel = s.handler.AddMnemonic(s.projectsTable, '2')
	s.rebuildSet()
	return s
}

// buildTables seeds both tables with empty column / row sets so the
// widgets are valid before the first WindowSizeMsg + load arrive.
func (s *editProfileScreen) buildTables() {
	at := table.New(
		table.WithColumns(s.assetsColumns(defaultEditProfileWidth)),
		table.WithRows(nil),
		table.WithFocused(true),
		table.WithWidth(defaultEditProfileWidth),
		table.WithHeight(defaultEditProfileTableHeight),
	)
	s.assetsTable = &at
	pt := table.New(
		table.WithColumns(s.projectsColumns(defaultEditProfileWidth)),
		table.WithRows(nil),
		table.WithFocused(true),
		table.WithWidth(defaultEditProfileWidth),
		table.WithHeight(defaultEditProfileTableHeight),
	)
	s.projectsTable = &pt
}

func (s *editProfileScreen) buildButtons() {
	s.editAsset = mnemonic.New("Edit", 'e', func() tea.Cmd { return s.onEditAsset() })
	s.deleteAsset = mnemonic.New("Delete", 'd', func() tea.Cmd { return s.onDeleteAsset() })

	s.editProject = mnemonic.New("Edit", 'e', func() tea.Cmd { return s.onEditProject() })
	s.selectAssets = mnemonic.New("Select Assets", 'a', func() tea.Cmd { return s.onSelectAssets() })
	s.planProject = mnemonic.New("Plan", 'p', func() tea.Cmd { return s.onPlanProject() })
	s.deleteProject = mnemonic.New("Delete", 'd', func() tea.Cmd { return s.onDeleteProject() })

	s.createAsset = mnemonic.New("Create Asset", 'c', func() tea.Cmd { return s.onCreateAsset() })
	s.register = mnemonic.New("Register Project", 'r', func() tea.Cmd { return s.onRegisterProject() })
	s.back = mnemonic.New(
		"Back",
		'b',
		func() tea.Cmd { return popCmd() },
		mnemonic.WithExtraBindingKeys("esc"),
	)
}

// rebuildSet refreshes the mnemonic set so its registration-time
// uniqueness check covers the current focus + data state. Assets focus
// exposes e/d; Projects focus exposes e/a/p/d. The two focus panels
// `[1]` / `[2]` and the screen-level c/r/b are always present.
func (s *editProfileScreen) rebuildSet() {
	set := mnemonic.NewSet()
	set.Add(s.assetsPanel)
	set.Add(s.projectsPanel)

	switch s.handler.Focused() {
	case 0:
		if len(s.assetsList) > 0 {
			set.Add(s.editAsset)
			set.Add(s.deleteAsset)
		}
	case 1:
		if len(s.projectsList) > 0 {
			set.Add(s.editProject)
			set.Add(s.selectAssets)
			set.Add(s.planProject)
			set.Add(s.deleteProject)
		}
	}

	set.Add(s.createAsset)
	set.Add(s.register)
	set.Add(s.back)
	s.set = set
}

func (s *editProfileScreen) Init() tea.Cmd { return s.loadCmd() }

func (s *editProfileScreen) loadCmd() tea.Cmd {
	return func() tea.Msg {
		prof, err := s.actions.LoadProfile(actions.LoadProfileInput{ProfileRef: s.profileID})
		return editProfileLoadedMsg{prof: prof, err: err}
	}
}

func (s *editProfileScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		s.width = m.Width
		s.height = m.Height
		if s.modal != nil {
			mw, mh := modalSize(m.Width, m.Height)
			s.modal.SetSize(mw, mh)
			var cmd tea.Cmd
			s.modal, cmd = s.modal.Update(m)
			return s, cmd
		}
		return s, nil

	case editProfileLoadedMsg:
		s.prof = m.prof
		s.rebuildLists()
		s.rebuildAssetsTable()
		s.rebuildProjectsTable()
		focusCmd := s.handler.FocusIndex(0)
		s.rebuildSet()
		if m.err != nil {
			return s, tea.Batch(focusCmd, notificationCmd(m.err.Severity(), m.err.Error()))
		}
		return s, focusCmd

	case editProfileMutationDoneMsg:
		return s, tea.Batch(notificationCmd(m.severity, m.text), s.loadCmd())

	case modal.ResolvedMsg:
		return s, s.handleResolved(m)

	case tea.KeyPressMsg:
		if s.modal != nil {
			return s.forwardToModal(msg)
		}
		// Focus handler consumes tab / shift+tab / ctrl+1 / ctrl+2.
		if handled, cmd := s.handler.Update(msg); handled {
			s.rebuildSet()
			return s, cmd
		}
		if btn := s.set.Match(m); btn != nil {
			return s, btn.Trigger()
		}
		return s, s.routeToFocusedTable(m)
	}

	if s.modal != nil {
		return s.forwardToModal(msg)
	}
	return s, nil
}

func (s *editProfileScreen) forwardToModal(msg tea.Msg) (Screen, tea.Cmd) {
	var cmd tea.Cmd
	s.modal, cmd = s.modal.Update(msg)
	return s, cmd
}

// routeToFocusedTable forwards keypresses the handler and mnemonic set
// did not consume to the table the handler currently considers
// focused. On a cursor change, the actions cell rebuilds for the
// newly-selected row.
func (s *editProfileScreen) routeToFocusedTable(m tea.KeyPressMsg) tea.Cmd {
	switch c := s.handler.FocusedComponent().(type) {
	case *table.Model:
		before := c.Cursor()
		updated, cmd := c.Update(m)
		*c = updated
		if c.Cursor() != before {
			if c == s.assetsTable {
				c.SetRows(s.buildAssetsRows(c.Cursor()))
			} else if c == s.projectsTable {
				c.SetRows(s.buildProjectsRows(c.Cursor()))
			}
		}
		return cmd
	}
	return nil
}

func (s *editProfileScreen) Title() string { return "Edit Profile" }

// StatusKeys returns the row-level mnemonics of the focused table plus
// the [Back] hint. Screen-level c/r and the focus mnemonics `[1]` /
// `[2]` are excluded because they're visible on the body (parent task
// 0015's status-bar rule). Back is the same explicit exception the
// Settings + Profiles screens make so the user can still see the back
// hint.
func (s *editProfileScreen) StatusKeys() []key.Binding {
	out := make([]key.Binding, 0, 5)
	switch s.handler.Focused() {
	case 0:
		if len(s.assetsList) > 0 {
			out = append(out, s.editAsset.Binding(), s.deleteAsset.Binding())
		}
	case 1:
		if len(s.projectsList) > 0 {
			out = append(out,
				s.editProject.Binding(),
				s.selectAssets.Binding(),
				s.planProject.Binding(),
				s.deleteProject.Binding(),
			)
		}
	}
	out = append(out, s.back.Binding())
	return out
}

func (s *editProfileScreen) Body(width, height int) string {
	background := s.bodyContent(width, height)
	if s.modal == nil {
		return background
	}
	return s.modal.Render(background, width, height)
}

// bodyContent renders the two-table layout exactly height rows tall.
// JoinVertical of [assetsHeader (1), assetsTable (N), spacer (1),
// projectsHeader (1), projectsTable (M), spacer (1), buttons (1)]
// sums to 5 + N + M, so the two tables get height - 5 rows split
// evenly, with the remainder going to the projects table.
func (s *editProfileScreen) bodyContent(width, height int) string {
	const fixedLines = 5
	tableTotal := height - fixedLines
	if tableTotal < 2 {
		tableTotal = 2
	}
	assetsH := tableTotal / 2
	projectsH := tableTotal - assetsH

	s.applyTableSize(s.assetsTable, s.assetsColumns(width), width, assetsH)
	s.applyTableSize(s.projectsTable, s.projectsColumns(width), width, projectsH)

	assetsHeader := s.assetsPanel.View() + " " + lipgloss.NewStyle().Bold(true).Render("Assets")
	projectsHeader := s.projectsPanel.View() + " " + lipgloss.NewStyle().Bold(true).Render("Projects")

	buttonRow := " " + s.createAsset.View() + "  " + s.register.View() + "  " + s.back.View()
	buttons := lipgloss.PlaceHorizontal(width, lipgloss.Left, buttonRow)

	return lipgloss.JoinVertical(
		lipgloss.Left,
		assetsHeader,
		s.assetsTable.View(),
		"",
		projectsHeader,
		s.projectsTable.View(),
		"",
		buttons,
	)
}

func (s *editProfileScreen) applyTableSize(t *table.Model, cols []table.Column, width, height int) {
	if height < 1 {
		height = 1
	}
	t.SetWidth(width)
	t.SetColumns(cols)
	t.SetHeight(height)
}

const (
	defaultEditProfileWidth       = 80
	defaultEditProfileTableHeight = 5
)

// rebuildLists materializes ordered slices of assets and projects from
// the freshly-loaded profile so the tables (and tests) see a stable
// iteration order.
func (s *editProfileScreen) rebuildLists() {
	if s.prof == nil {
		s.assetsList = nil
		s.projectsList = nil
		return
	}
	assets := make([]*asset.Asset, 0, len(s.prof.Assets))
	for _, a := range s.prof.Assets {
		assets = append(assets, a)
	}
	sort.Slice(assets, func(i, j int) bool { return assets[i].Name < assets[j].Name })
	s.assetsList = assets
	s.projectsList = s.prof.ProjectList()
}

func (s *editProfileScreen) rebuildAssetsTable() {
	width := s.tableInnerWidth()
	s.assetsTable.SetWidth(width)
	s.assetsTable.SetColumns(s.assetsColumns(width))
	s.assetsTable.SetRows(s.buildAssetsRows(0))
}

func (s *editProfileScreen) rebuildProjectsTable() {
	width := s.tableInnerWidth()
	s.projectsTable.SetWidth(width)
	s.projectsTable.SetColumns(s.projectsColumns(width))
	s.projectsTable.SetRows(s.buildProjectsRows(0))
}

func (s *editProfileScreen) tableInnerWidth() int {
	if s.width < 1 {
		return defaultEditProfileWidth
	}
	return s.width
}

func (s *editProfileScreen) assetsColumns(innerW int) []table.Column {
	const (
		idW         = 18
		typeW       = 14
		actionsW    = 22
		cellPadding = 8
	)
	nameW := innerW - idW - typeW - actionsW - cellPadding
	if nameW < minNameColW {
		nameW = minNameColW
	}
	return []table.Column{
		{Title: "ID", Width: idW},
		{Title: "Name", Width: nameW},
		{Title: "Type", Width: typeW},
		{Title: "Actions", Width: actionsW},
	}
}

func (s *editProfileScreen) projectsColumns(innerW int) []table.Column {
	const (
		idW         = 18
		nameW       = 18
		actionsW    = 26
		cellPadding = 8
	)
	pathW := innerW - idW - nameW - actionsW - cellPadding
	if pathW < minPathColW {
		pathW = minPathColW
	}
	return []table.Column{
		{Title: "ID", Width: idW},
		{Title: "Name", Width: nameW},
		{Title: "Path", Width: pathW},
		{Title: "Actions", Width: actionsW},
	}
}

const minNameColW = 12

func (s *editProfileScreen) buildAssetsRows(cursor int) []table.Row {
	rows := make([]table.Row, len(s.assetsList))
	for i, a := range s.assetsList {
		cell := ""
		if i == cursor {
			cell = assetActionsCellContent()
		}
		rows[i] = table.Row{a.ID, a.Name, string(a.Type), cell}
	}
	return rows
}

func (s *editProfileScreen) buildProjectsRows(cursor int) []table.Row {
	rows := make([]table.Row, len(s.projectsList))
	for i, p := range s.projectsList {
		cell := ""
		if i == cursor {
			cell = projectActionsCellContent()
		}
		rows[i] = table.Row{p.ID, p.Name, p.Path, cell}
	}
	return rows
}

// assetActionsCellContent renders "[Edit] [Delete]" with the mnemonic
// chars wrapped in SGR underline-on/off so the surrounding cursor-row
// highlight is not terminated by an embedded full reset.
func assetActionsCellContent() string {
	const (
		underlineOn  = "\x1b[4m"
		underlineOff = "\x1b[24m"
	)
	return "[" + underlineOn + "E" + underlineOff + "dit] " +
		"[" + underlineOn + "D" + underlineOff + "elete]"
}

// projectActionsCellContent renders "[Edit] [select Assets] [Plan]
// [Delete]" with each mnemonic char wrapped in manual SGR codes. The
// cell is intentionally compact — the long form is shown in the status
// bar.
func projectActionsCellContent() string {
	const (
		on  = "\x1b[4m"
		off = "\x1b[24m"
	)
	return "[" + on + "E" + off + "] " +
		"[" + on + "A" + off + "] " +
		"[" + on + "P" + off + "] " +
		"[" + on + "D" + off + "]"
}

func (s *editProfileScreen) selectedAsset() (*asset.Asset, bool) {
	if len(s.assetsList) == 0 {
		return nil, false
	}
	cur := s.assetsTable.Cursor()
	if cur < 0 || cur >= len(s.assetsList) {
		return nil, false
	}
	return s.assetsList[cur], true
}

func (s *editProfileScreen) selectedProject() (*project.Manifest, bool) {
	if len(s.projectsList) == 0 {
		return nil, false
	}
	cur := s.projectsTable.Cursor()
	if cur < 0 || cur >= len(s.projectsList) {
		return nil, false
	}
	return s.projectsList[cur], true
}

func (s *editProfileScreen) onEditAsset() tea.Cmd {
	a, ok := s.selectedAsset()
	if !ok {
		return nil
	}
	return pushCmd(newEditAssetStub(s.profileID, a.ID))
}

func (s *editProfileScreen) onDeleteAsset() tea.Cmd {
	a, ok := s.selectedAsset()
	if !ok {
		return nil
	}
	s.pendingDeleteAsset = a.ID
	prompt := fmt.Sprintf("Are you sure you want to delete asset %q?", a.Name)
	s.openModal(modal.NewConfirm("delete-asset", prompt, nil))
	return s.modal.Init()
}

func (s *editProfileScreen) onEditProject() tea.Cmd {
	p, ok := s.selectedProject()
	if !ok {
		return nil
	}
	s.openModal(modals.NewEditProject(modals.EditProjectInput{
		Name:          p.Name,
		Path:          p.Path,
		EnabledAgents: append([]string(nil), p.EnabledAgents...),
	}))
	return s.modal.Init()
}

func (s *editProfileScreen) onSelectAssets() tea.Cmd {
	p, ok := s.selectedProject()
	if !ok {
		return nil
	}
	return pushCmd(newSelectProjectAssetsStub(s.profileID, p.ID))
}

func (s *editProfileScreen) onPlanProject() tea.Cmd {
	p, ok := s.selectedProject()
	if !ok {
		return nil
	}
	return pushCmd(newPlanProjectStub(s.profileID, p.ID))
}

func (s *editProfileScreen) onDeleteProject() tea.Cmd {
	p, ok := s.selectedProject()
	if !ok {
		return nil
	}
	s.pendingDeleteProj = p.ID
	prompt := fmt.Sprintf(
		"Are you sure you want to delete project %q? (only metadata — repo files are kept)",
		p.Name,
	)
	s.openModal(modal.NewConfirm("delete-project", prompt, nil))
	return s.modal.Init()
}

func (s *editProfileScreen) onCreateAsset() tea.Cmd {
	s.openModal(modals.NewCreateAsset(asset.Manifest{}))
	return s.modal.Init()
}

func (s *editProfileScreen) onRegisterProject() tea.Cmd {
	s.openModal(modals.NewRegisterProject(modals.RegisterProjectInput{}))
	return s.modal.Init()
}

func (s *editProfileScreen) openModal(m *modal.Modal) {
	s.modal = m
	if s.width > 0 && s.height > 0 {
		mw, mh := modalSize(s.width, s.height)
		s.modal.SetSize(mw, mh)
	}
}

func (s *editProfileScreen) handleResolved(msg modal.ResolvedMsg) tea.Cmd {
	s.modal = nil
	switch msg.ID {
	case "delete-asset":
		return s.afterDeleteAsset(msg)
	case "delete-project":
		return s.afterDeleteProject(msg)
	case "create-asset":
		return s.afterCreateAsset(msg)
	case "register-project":
		return s.afterRegisterProject(msg)
	case "edit-project":
		return s.afterEditProject(msg)
	}
	return nil
}

func (s *editProfileScreen) afterDeleteAsset(msg modal.ResolvedMsg) tea.Cmd {
	id := s.pendingDeleteAsset
	s.pendingDeleteAsset = ""
	if !msg.Confirmed || id == "" {
		return nil
	}
	return editProfileMutationCmd(
		func() (struct{}, errs.DomainError) {
			return s.actions.DeleteAsset(actions.DeleteAssetInput{
				ProfileRef: s.profileID,
				AssetID:    id,
			})
		},
		fmt.Sprintf("Asset %q deleted", id),
	)
}

func (s *editProfileScreen) afterDeleteProject(msg modal.ResolvedMsg) tea.Cmd {
	id := s.pendingDeleteProj
	s.pendingDeleteProj = ""
	if !msg.Confirmed || id == "" {
		return nil
	}
	return editProfileMutationCmd(
		func() (struct{}, errs.DomainError) {
			return s.actions.DeleteProject(actions.DeleteProjectInput{
				ProfileRef: s.profileID,
				ProjectID:  id,
			})
		},
		fmt.Sprintf("Project %q deleted", id),
	)
}

func (s *editProfileScreen) afterCreateAsset(msg modal.ResolvedMsg) tea.Cmd {
	if !msg.Confirmed {
		return nil
	}
	manifest, ok := msg.Value.(asset.Manifest)
	if !ok {
		return nil
	}
	return editProfileMutationCmd(
		func() (string, errs.DomainError) {
			return s.actions.CreateAsset(actions.CreateAssetInput{
				ProfileRef: s.profileID,
				Manifest:   manifest,
			})
		},
		fmt.Sprintf("Asset %q created", manifest.Name),
	)
}

func (s *editProfileScreen) afterRegisterProject(msg modal.ResolvedMsg) tea.Cmd {
	if !msg.Confirmed {
		return nil
	}
	draft, ok := msg.Value.(*project.Manifest)
	if !ok || draft == nil {
		return nil
	}
	name := draft.Name
	return editProfileMutationCmd(
		func() (*project.Manifest, errs.DomainError) {
			return s.actions.AddProject(actions.AddProjectInput{
				ProfileRef:    s.profileID,
				Name:          draft.Name,
				Path:          draft.Path,
				EnabledAgents: draft.EnabledAgents,
			})
		},
		fmt.Sprintf("Project %q registered", name),
	)
}

func (s *editProfileScreen) afterEditProject(msg modal.ResolvedMsg) tea.Cmd {
	if !msg.Confirmed {
		return nil
	}
	in, ok := msg.Value.(modals.EditProjectInput)
	if !ok {
		return nil
	}
	target, ok := s.selectedProject()
	if !ok {
		return nil
	}
	// Merge editable fields back into the loaded manifest so ID,
	// SelectedAssetIDs, and CreatedAt are preserved.
	target.Name = in.Name
	target.Path = in.Path
	target.EnabledAgents = append([]string(nil), in.EnabledAgents...)
	return editProfileMutationCmd(
		func() (struct{}, errs.DomainError) {
			return s.actions.UpdateProject(actions.UpdateProjectInput{
				ProfileRef: s.profileID,
				Project:    target,
			})
		},
		fmt.Sprintf("Project %q updated", target.Name),
	)
}

// editProfileMutationCmd runs the action synchronously inside a Cmd
// closure and returns the editProfileMutationDoneMsg envelope. Update
// then emits a tea.Batch(notification, reload) — same testability
// rationale as profilesScreen.mutationCmd.
func editProfileMutationCmd[T any](action func() (T, errs.DomainError), successText string) tea.Cmd {
	return func() tea.Msg {
		_, err := action()
		if err != nil {
			return editProfileMutationDoneMsg{text: err.Error(), severity: err.Severity()}
		}
		return editProfileMutationDoneMsg{text: successText, severity: errs.SeverityInfo}
	}
}
