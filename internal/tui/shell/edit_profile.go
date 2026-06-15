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
	"github.com/hexworks/agentfiles/internal/tui/components/focus"
	"github.com/hexworks/agentfiles/internal/tui/components/mnemonic"
	"github.com/hexworks/agentfiles/internal/tui/components/modal"
	"github.com/hexworks/agentfiles/internal/tui/components/panel"
	"github.com/hexworks/agentfiles/internal/tui/modals"
)

// editProfileOwnActions is the slice the Edit Profile screen invokes
// itself. Kept separate from editAssetActions so a reader can see at a
// glance which interface widening reflects an own dependency versus a
// child-screen pass-through.
type editProfileOwnActions interface {
	LoadProfile(in actions.LoadProfileInput) (*profile.Profile, errs.DomainError)
	CreateAsset(in actions.CreateAssetInput) (string, errs.DomainError)
	DeleteAsset(in actions.DeleteAssetInput) (struct{}, errs.DomainError)
	AddProject(in actions.AddProjectInput) (*project.Manifest, errs.DomainError)
	UpdateProject(in actions.UpdateProjectInput) (struct{}, errs.DomainError)
	DeleteProject(in actions.DeleteProjectInput) (struct{}, errs.DomainError)
}

// editProfileActions composes the screen's own dependencies with the
// child Edit Asset and Select Project Assets screens' dependencies so
// `s.actions` can be forwarded to either child constructor without a
// type assertion. The composition makes the dependency union visible at
// the declaration instead of widening a single flat interface for
// methods the parent screen never calls.
type editProfileActions interface {
	editProfileOwnActions
	editAssetActions
	selectProjectAssetsActions
}

// modalKind identifies which modal flow the Edit Profile screen
// currently hosts. Tests assert intent through this enum rather than the
// raw modal-id string the dialog carries.
type modalKind int

const (
	modalKindNone modalKind = iota
	modalKindDeleteAsset
	modalKindDeleteProject
	modalKindCreateAsset
	modalKindRegisterProject
	modalKindEditProject
)

// editProfileScreen is the Edit Profile management screen reached from
// the Profiles row-level `[Edit]` action. It owns two bubbles/table views
// (Assets and Projects), a focus.Handler driven by Tab / Shift+Tab,
// row-level action buttons that swap with the focused table, and
// screen-level Create Asset / Register Project / Back buttons.
// Confirmation + form modals composite over the body; no sub-screen is
// pushed for them.
type editProfileScreen struct {
	actions   editProfileActions
	profileID string

	assets   []*asset.Asset
	projects []*project.Manifest

	handler       *focus.Handler
	assetsTable   *table.Model
	projectsTable *table.Model

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

	modal                  *modal.Modal
	modalKind              modalKind
	pendingDeleteAssetID   string
	pendingDeleteProjectID string

	width, height int
}

// editProfileLoadedMsg carries the loaded profile (or load error) that
// Init's command produces.
type editProfileLoadedMsg struct {
	prof *profile.Profile
	err  errs.DomainError
}

func newEditProfileScreen(a editProfileActions, profileID string) *editProfileScreen {
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
	s.handler = focus.New()
	s.handler.Add(s.assetsTable)
	s.handler.Add(s.projectsTable)
	s.rebuildSet()
	return s
}

func (s *editProfileScreen) buildTables() {
	assetCols := naturalColumns(assetColumnTitles, nil)
	at := table.New(
		table.WithColumns(assetCols),
		table.WithRows(nil),
		table.WithWidth(tableNaturalWidth(assetCols)),
		table.WithHeight(1),
	)
	s.assetsTable = &at
	projectCols := naturalColumns(projectColumnTitles, nil)
	pt := table.New(
		table.WithColumns(projectCols),
		table.WithRows(nil),
		table.WithWidth(tableNaturalWidth(projectCols)),
		table.WithHeight(1),
	)
	s.projectsTable = &pt
}

var (
	assetColumnTitles   = []string{"ID", "Name", "Type", "Actions"}
	projectColumnTitles = []string{"ID", "Name", "Path", "Actions"}
)

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
// exposes e/d; Projects focus exposes e/a/p/d. Screen-level c/r/b are
// always present.
func (s *editProfileScreen) rebuildSet() {
	set := mnemonic.NewSet()

	switch s.handler.Focused() {
	case 0:
		if len(s.assets) > 0 {
			set.Add(s.editAsset)
			set.Add(s.deleteAsset)
		}
	case 1:
		if len(s.projects) > 0 {
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
	// Single modal-guard for messages the modal owns. WindowSizeMsg still
	// needs to update the screen's own width/height before being forwarded,
	// so it has its own branch below.
	if s.modal != nil {
		switch msg.(type) {
		case modal.ResolvedMsg, tea.WindowSizeMsg, editProfileLoadedMsg, mutationDoneMsg:
			// fall through to type-specific handling
		default:
			return s.forwardToModal(msg)
		}
	}

	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		return s.handleResize(m)
	case editProfileLoadedMsg:
		return s.handleLoaded(m)
	case mutationDoneMsg:
		return s, tea.Batch(notificationCmd(m.severity, m.text), s.loadCmd())
	case modal.ResolvedMsg:
		return s, s.handleResolved(m)
	case tea.KeyPressMsg:
		return s.handleKey(m)
	}
	return s, nil
}

func (s *editProfileScreen) handleResize(m tea.WindowSizeMsg) (Screen, tea.Cmd) {
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
}

func (s *editProfileScreen) handleLoaded(m editProfileLoadedMsg) (Screen, tea.Cmd) {
	s.rebuildLists(m.prof)
	s.rebuildAssetsTable()
	s.rebuildProjectsTable()
	focusCmd := s.handler.FocusIndex(0)
	s.rebuildSet()
	if m.err != nil {
		return s, tea.Batch(focusCmd, notificationCmd(m.err.Severity(), m.err.Error()))
	}
	return s, focusCmd
}

func (s *editProfileScreen) handleKey(m tea.KeyPressMsg) (Screen, tea.Cmd) {
	if s.modal != nil {
		return s.forwardToModal(m)
	}
	// Focus handler consumes tab / shift+tab.
	if handled, cmd := s.handler.Update(m); handled {
		s.rebuildSet()
		return s, cmd
	}
	if btn := s.set.Match(m); btn != nil {
		return s, btn.Trigger()
	}
	return s, s.routeToFocusedTable(m)
}

func (s *editProfileScreen) forwardToModal(msg tea.Msg) (Screen, tea.Cmd) {
	var cmd tea.Cmd
	s.modal, cmd = s.modal.Update(msg)
	return s, cmd
}

// routeToFocusedTable forwards keypresses the handler and mnemonic set
// did not consume to the table the handler currently considers focused.
// On a cursor change, the actions cell rebuilds for the newly-selected
// row. The focused table is, by definition, the one whose actions cell
// should render — so showActions is true on the rebuild path.
func (s *editProfileScreen) routeToFocusedTable(m tea.KeyPressMsg) tea.Cmd {
	c, ok := s.handler.FocusedComponent().(*table.Model)
	if !ok {
		return nil
	}
	before := c.Cursor()
	updated, cmd := c.Update(m)
	*c = updated
	if c.Cursor() != before {
		switch c {
		case s.assetsTable:
			c.SetRows(s.buildAssetsRows(c.Cursor(), true))
		case s.projectsTable:
			c.SetRows(s.buildProjectsRows(c.Cursor(), true))
		}
	}
	return cmd
}

func (s *editProfileScreen) Title() string { return "Edit Profile" }

// StatusKeys returns the row-level mnemonics of the focused table plus
// the [Back] hint. Screen-level c/r are excluded because they're visible
// on the body. Back is the same explicit exception the Settings +
// Profiles screens make so the user can still see the back hint.
func (s *editProfileScreen) InputFocused() bool { return false }

func (s *editProfileScreen) StatusKeys() []key.Binding {
	out := make([]key.Binding, 0, 5)
	switch s.handler.Focused() {
	case 0:
		if len(s.assets) > 0 {
			out = append(out, s.editAsset.Binding(), s.deleteAsset.Binding())
		}
	case 1:
		if len(s.projects) > 0 {
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

func (s *editProfileScreen) Body(width int) string {
	background := s.renderBody(width)
	if s.modal == nil {
		return background
	}
	return s.modal.Render(background, width, lipgloss.Height(background))
}

// renderBody composes the two-table layout at its natural width and
// height. Both tables share a width: the wider panel's natural width
// wins, and the other panel grows its first content column (Name for
// Assets, Path for Projects) to match. Each table is wrapped in a
// rounded border, sized to its current row count plus a header row.
func (s *editProfileScreen) renderBody(_ int) string {
	sanitizeCursor(s.assetsTable, len(s.assets))
	sanitizeCursor(s.projectsTable, len(s.projects))
	focused := s.handler.Focused()
	assetRows := s.buildAssetsRows(s.assetsTable.Cursor(), focused == 0)
	projectRows := s.buildProjectsRows(s.projectsTable.Cursor(), focused == 1)

	assetCols := naturalColumns(assetColumnTitles, assetRows)
	projectCols := naturalColumns(projectColumnTitles, projectRows)

	// Assets row width is dominated by Name; Projects by Path. Pad the
	// shorter table's elastic column so both render to the same outer
	// width.
	const assetElasticIdx, projectElasticIdx = 1, 2
	equalizePanelWidth(assetCols, projectCols, assetElasticIdx, projectElasticIdx)

	applyTable(s.assetsTable, assetCols, assetRows)
	applyTable(s.projectsTable, projectCols, projectRows)

	buttonRow := " " + s.createAsset.View() + "  " + s.register.View() + "  " + s.back.View()
	st := focusAwarePanelStyles()

	return lipgloss.JoinVertical(
		lipgloss.Left,
		panel.Render(focused == 0, "Assets", s.assetsTable.View(), st),
		"",
		panel.Render(focused == 1, "Projects", s.projectsTable.View(), st),
		"",
		buttonRow,
	)
}

// equalizePanelWidth grows the elastic column on the narrower table so
// both panels render at the same outer width. Mutates cols slices in
// place.
func equalizePanelWidth(a, b []table.Column, elasticA, elasticB int) {
	wA := tableNaturalWidth(a)
	wB := tableNaturalWidth(b)
	if wA == wB {
		return
	}
	if wA < wB {
		a[elasticA].Width += wB - wA
	} else {
		b[elasticB].Width += wA - wB
	}
}

// applyTable pushes columns/rows/height/width onto a bubbles table. The
// order matches the contract that SetHeight runs after SetRows so the
// viewport sizes to the row count handed in.
//
// SetRows is skipped on an empty rows slice because bubbles drops the
// cursor to -1 in that case and never raises it back when rows reappear
// (see [sanitizeCursor] for the recovery path on the next render).
func applyTable(t *table.Model, cols []table.Column, rows []table.Row) {
	t.SetColumns(cols)
	t.SetWidth(tableNaturalWidth(cols))
	if len(rows) > 0 {
		t.SetRows(rows)
	}
	t.SetHeight(len(rows) + 1)
}

// rebuildLists materializes ordered slices of assets and projects from
// the freshly-loaded profile so the tables (and tests) see a stable
// iteration order. Domain ordering is owned by profile.AssetList /
// ProjectList — the screen does not re-implement sort rules.
func (s *editProfileScreen) rebuildLists(prof *profile.Profile) {
	if prof == nil {
		s.assets = nil
		s.projects = nil
		return
	}
	s.assets = prof.AssetList()
	s.projects = prof.ProjectList()
}

// rebuildAssetsTable seeds rows + naturally-sized columns from the
// freshly-loaded profile. Rows are seeded without an actions cell — the
// first render pass through [renderBody] re-emits them with the actions
// cell gated on the live focus state.
func (s *editProfileScreen) rebuildAssetsTable() {
	rows := s.buildAssetsRows(0, false)
	cols := naturalColumns(assetColumnTitles, rows)
	s.assetsTable.SetColumns(cols)
	s.assetsTable.SetWidth(tableNaturalWidth(cols))
	s.assetsTable.SetRows(rows)
}

func (s *editProfileScreen) rebuildProjectsTable() {
	rows := s.buildProjectsRows(0, false)
	cols := naturalColumns(projectColumnTitles, rows)
	s.projectsTable.SetColumns(cols)
	s.projectsTable.SetWidth(tableNaturalWidth(cols))
	s.projectsTable.SetRows(rows)
}

// buildAssetsRows materializes the table rows for the assets list. The
// actions cell on the cursor row is rendered visible when showActions
// is true, and as a whitespace placeholder of the same visible width
// when false — that reserves the column width so the panel does not
// resize on focus changes.
func (s *editProfileScreen) buildAssetsRows(cursor int, showActions bool) []table.Row {
	actions := assetActionsCell()
	rows := make([]table.Row, len(s.assets))
	for i, a := range s.assets {
		cell := ""
		if i == cursor {
			if showActions {
				cell = actions
			} else {
				cell = hiddenActionsCell(actions)
			}
		}
		rows[i] = table.Row{a.ID, a.Name, string(a.Type), cell}
	}
	return rows
}

func (s *editProfileScreen) buildProjectsRows(cursor int, showActions bool) []table.Row {
	actions := projectActionsCell()
	rows := make([]table.Row, len(s.projects))
	for i, p := range s.projects {
		cell := ""
		if i == cursor {
			if showActions {
				cell = actions
			} else {
				cell = hiddenActionsCell(actions)
			}
		}
		rows[i] = table.Row{p.ID, p.Name, p.Path, cell}
	}
	return rows
}

// assetActionsCell renders "[Edit] [Delete]" with the mnemonic letters
// underlined via shell.underline so the surrounding cursor-row highlight
// survives.
func assetActionsCell() string {
	return "[" + underline("E") + "dit] [" + underline("D") + "elete]"
}

// projectActionsCell renders the full action labels with mnemonic
// runes underlined via shell.underline so the surrounding cursor-row
// highlight survives.
func projectActionsCell() string {
	return "[" + underline("E") + "dit] [Select " + underline("A") + "ssets] [" +
		underline("P") + "lan] [" + underline("D") + "elete]"
}

func (s *editProfileScreen) selectedAsset() (*asset.Asset, bool) {
	if len(s.assets) == 0 {
		return nil, false
	}
	cur := s.assetsTable.Cursor()
	if cur < 0 || cur >= len(s.assets) {
		return nil, false
	}
	return s.assets[cur], true
}

func (s *editProfileScreen) selectedProject() (*project.Manifest, bool) {
	if len(s.projects) == 0 {
		return nil, false
	}
	cur := s.projectsTable.Cursor()
	if cur < 0 || cur >= len(s.projects) {
		return nil, false
	}
	return s.projects[cur], true
}

func (s *editProfileScreen) onEditAsset() tea.Cmd {
	a, ok := s.selectedAsset()
	if !ok {
		return nil
	}
	return pushCmd(newEditAssetScreen(s.actions, s.profileID, a.ID))
}

func (s *editProfileScreen) onDeleteAsset() tea.Cmd {
	a, ok := s.selectedAsset()
	if !ok {
		return nil
	}
	s.pendingDeleteAssetID = a.ID
	prompt := fmt.Sprintf(
		"Delete asset %q? It will also be removed from every project's selection.",
		a.Name,
	)
	s.openModal(modal.NewConfirm("delete-asset", prompt, nil), modalKindDeleteAsset)
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
	}), modalKindEditProject)
	return s.modal.Init()
}

func (s *editProfileScreen) onSelectAssets() tea.Cmd {
	p, ok := s.selectedProject()
	if !ok {
		return nil
	}
	return pushCmd(newSelectProjectAssetsScreen(s.actions, s.profileID, p.ID))
}

func (s *editProfileScreen) onPlanProject() tea.Cmd {
	p, ok := s.selectedProject()
	if !ok {
		return nil
	}
	return pushCmd(newPlanProjectScreen(s.actions, s.profileID, p.ID))
}

func (s *editProfileScreen) onDeleteProject() tea.Cmd {
	p, ok := s.selectedProject()
	if !ok {
		return nil
	}
	s.pendingDeleteProjectID = p.ID
	prompt := fmt.Sprintf(
		"Delete project manifest %q? Files in the target repo become orphaned files and are kept on disk.",
		p.Name,
	)
	s.openModal(modal.NewConfirm("delete-project", prompt, nil), modalKindDeleteProject)
	return s.modal.Init()
}

func (s *editProfileScreen) onCreateAsset() tea.Cmd {
	s.openModal(modals.NewCreateAsset(asset.Manifest{}), modalKindCreateAsset)
	return s.modal.Init()
}

func (s *editProfileScreen) onRegisterProject() tea.Cmd {
	s.openModal(modals.NewRegisterProject(modals.RegisterProjectInput{}), modalKindRegisterProject)
	return s.modal.Init()
}

func (s *editProfileScreen) openModal(m *modal.Modal, kind modalKind) {
	s.modal = m
	s.modalKind = kind
	if s.width > 0 && s.height > 0 {
		mw, mh := modalSize(s.width, s.height)
		s.modal.SetSize(mw, mh)
	}
}

// handleResolved dispatches a modal ResolvedMsg to the right post-action
// handler. The modal field + pending ids are cleared up-front so unrelated
// modal lifecycles cannot leave a stale id behind regardless of which
// arm fires.
func (s *editProfileScreen) handleResolved(msg modal.ResolvedMsg) tea.Cmd {
	s.modal = nil
	kind := s.modalKind
	s.modalKind = modalKindNone
	pendingAsset := s.pendingDeleteAssetID
	pendingProject := s.pendingDeleteProjectID
	s.pendingDeleteAssetID = ""
	s.pendingDeleteProjectID = ""
	switch kind {
	case modalKindDeleteAsset:
		return s.afterDeleteAsset(msg, pendingAsset)
	case modalKindDeleteProject:
		return s.afterDeleteProject(msg, pendingProject)
	case modalKindCreateAsset:
		return s.afterCreateAsset(msg)
	case modalKindRegisterProject:
		return s.afterRegisterProject(msg)
	case modalKindEditProject:
		return s.afterEditProject(msg)
	}
	return nil
}

func (s *editProfileScreen) afterDeleteAsset(msg modal.ResolvedMsg, id string) tea.Cmd {
	if !msg.Confirmed || id == "" {
		return nil
	}
	return mutationCmd(
		func() errs.DomainError {
			_, err := s.actions.DeleteAsset(actions.DeleteAssetInput{
				ProfileRef: s.profileID,
				AssetID:    id,
			})
			return err
		},
		fmt.Sprintf("Asset %q deleted", id),
	)
}

func (s *editProfileScreen) afterDeleteProject(msg modal.ResolvedMsg, id string) tea.Cmd {
	if !msg.Confirmed || id == "" {
		return nil
	}
	return mutationCmd(
		func() errs.DomainError {
			_, err := s.actions.DeleteProject(actions.DeleteProjectInput{
				ProfileRef: s.profileID,
				ProjectID:  id,
			})
			return err
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
	return mutationCmd(
		func() errs.DomainError {
			_, err := s.actions.CreateAsset(actions.CreateAssetInput{
				ProfileRef: s.profileID,
				Manifest:   manifest,
			})
			return err
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
	return mutationCmd(
		func() errs.DomainError {
			_, err := s.actions.AddProject(actions.AddProjectInput{
				ProfileRef:    s.profileID,
				Name:          draft.Name,
				Path:          draft.Path,
				EnabledAgents: draft.EnabledAgents,
			})
			return err
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
	// The service owns the merge contract (ID / SelectedAssetIDs /
	// CreatedAt are preserved on the loaded manifest before save) — the
	// TUI only ships the form values.
	return mutationCmd(
		func() errs.DomainError {
			_, err := s.actions.UpdateProject(actions.UpdateProjectInput{
				ProfileRef:    s.profileID,
				ProjectID:     target.ID,
				Name:          in.Name,
				Path:          in.Path,
				EnabledAgents: in.EnabledAgents,
			})
			return err
		},
		fmt.Sprintf("Project %q updated", in.Name),
	)
}
