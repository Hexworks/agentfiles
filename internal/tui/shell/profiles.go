package shell

import (
	"fmt"
	"path/filepath"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/hexworks/agentfiles/internal/actions"
	"github.com/hexworks/agentfiles/internal/appapi"
	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/tui/components/help"
	"github.com/hexworks/agentfiles/internal/tui/components/mnemonic"
	"github.com/hexworks/agentfiles/internal/tui/components/modal"
	"github.com/hexworks/agentfiles/internal/tui/components/panel"
	"github.com/hexworks/agentfiles/internal/tui/modals"
	"github.com/hexworks/agentfiles/internal/tui/modals/pathselector"
	"github.com/hexworks/agentfiles/internal/tui/notifications"
	"github.com/hexworks/agentfiles/internal/tui/styles"
)

// profilesScreen is the Profiles management screen reached from Welcome.
// It owns a bubbles/table view of the registered profiles, screen-level
// mnemonic buttons for Create / Register, and row-level mnemonic buttons
// for Edit / Delete on the cursor row. Confirmation and form modals
// composite over the screen body; no sub-screen is pushed for them.
type profilesScreen struct {
	actions  *actions.Actions
	profiles []*appapi.LoadedProfile
	table    table.Model

	modal             *modal.Modal
	pendingDeleteID   string
	pendingDeleteName string
	// pendingCreateName carries the Name the user typed into the Create
	// Profile form across a retry: when CreateProfile fails, the flow re-
	// opens the pathselector, and the follow-on form must be re-seeded with
	// the same Name so the user doesn't type it again (plan for task 0039).
	pendingCreateName string

	edit     *mnemonic.Button
	delete   *mnemonic.Button
	create   *mnemonic.Button
	register *mnemonic.Button
	back     *mnemonic.Button
	// set is rebuilt on every selection-state transition so duplicate
	// mnemonics panic at Set.Add time — the load-bearing safety net
	// required by task 0025.
	set *mnemonic.Set

	width, height int
}

// profilesLoadedMsg is the action-result message Init's command returns.
// It carries every Profile pointer LoadProfiles produced plus any domain
// error so Update can both rebuild the table and emit a notification.
type profilesLoadedMsg struct {
	profiles []*appapi.LoadedProfile
	err      errs.DomainError
}

// createProfileFailedMsg is dispatched when CreateProfile returns a domain
// error inside the two-step pathselector → form flow. It carries the
// user-selected path and the failure severity/text so the Update handler
// can notify the user and re-open the pathselector seeded at the parent
// folder — the follow-on form is re-seeded with pendingCreateName so the
// user does not re-type the Name they already entered.
type createProfileFailedMsg struct {
	path     string
	text     string
	severity errs.Severity
}

// registerProfileFailedMsg is the RegisterProfile analogue of
// createProfileFailedMsg. Register Profile has no editable non-path
// field, so there is no stashed input to re-seed on the retry.
type registerProfileFailedMsg struct {
	path     string
	text     string
	severity errs.Severity
}

func newProfilesScreen(a *actions.Actions) *profilesScreen {
	if a == nil {
		panic("shell.newProfilesScreen: nil actions")
	}
	s := &profilesScreen{actions: a}
	s.buildButtons()
	s.rebuildSet()
	s.rebuildTable()
	return s
}

// buildButtons creates the mnemonic buttons once. Their Actions close
// over s so each button can reach the current state when its keybinding
// fires — the closures are stable, the data they read is not. The Back
// button also fires on `esc` so it follows the same dismissal pattern as
// the rest of the TUI.
func (s *profilesScreen) buildButtons() {
	s.edit = mnemonic.New("Edit", 'e', func() tea.Cmd { return s.onEdit() })
	s.delete = mnemonic.New("Delete", 'd', func() tea.Cmd { return s.onDelete() })
	s.create = mnemonic.New("Create New Profile", 'c', func() tea.Cmd { return s.onCreate() })
	s.register = mnemonic.New("Register Profile", 'r', func() tea.Cmd { return s.onRegister() })
	s.back = mnemonic.New(
		"Back",
		'b',
		func() tea.Cmd { return popCmd() },
		mnemonic.WithExtraBindingKeys("esc"),
	)
}

// rebuildSet refreshes the mnemonic.Set after a selection-state change.
// With no profiles the row-level buttons drop out so e/d can be reused
// elsewhere later without conflict; with profiles all five are present
// and the Set's duplicate-rune check verifies they remain disjoint.
func (s *profilesScreen) rebuildSet() {
	set := mnemonic.NewSet()
	if len(s.profiles) > 0 {
		set.Add(s.edit)
		set.Add(s.delete)
	}
	set.Add(s.create)
	set.Add(s.register)
	set.Add(s.back)
	s.set = set
}

func (s *profilesScreen) Init() tea.Cmd { return s.loadCmd() }

// loadCmd returns a tea.Cmd that calls LoadProfiles and emits the
// result. Keeping the action behind a Cmd respects tui.md's "no slow I/O
// in Update" rule.
func (s *profilesScreen) loadCmd() tea.Cmd {
	return func() tea.Msg {
		profiles, err := s.actions.LoadProfiles()
		return profilesLoadedMsg{profiles: profiles, err: err}
	}
}

func (s *profilesScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		s.width = m.Width
		s.height = m.Height
		if s.modal != nil {
			mw, mh := modalSize(m.Width, m.Height)
			s.modal.SetSize(mw, mh)
			// Forward the resize through the modal so the hosted huh
			// form (or any other Bubble Tea content) sees its new
			// viewport. formContent does not satisfy Resizable, so
			// SetSize alone would not reach it.
			var cmd tea.Cmd
			s.modal, cmd = s.modal.Update(m)
			return s, cmd
		}
		return s, nil

	case profilesLoadedMsg:
		s.profiles = m.profiles
		s.rebuildSet()
		s.rebuildTable()
		if m.err != nil {
			return s, notificationCmd(m.err.Severity(), m.err.Error())
		}
		return s, nil

	case mutationDoneMsg:
		return s, tea.Batch(notificationCmd(m.severity, m.text), s.loadCmd())

	case createProfileFailedMsg:
		return s, tea.Batch(
			notificationCmd(m.severity, m.text),
			s.openCreateProfilePathselectorCmd(filepath.Dir(m.path)),
		)

	case registerProfileFailedMsg:
		return s, tea.Batch(
			notificationCmd(m.severity, m.text),
			s.openRegisterProfilePathselectorCmd(filepath.Dir(m.path)),
		)

	case modal.ResolvedMsg:
		return s, s.handleResolved(m)

	case tea.KeyPressMsg:
		if s.modal != nil {
			return s.forwardToModal(msg)
		}
		if btn := s.set.Match(m); btn != nil {
			return s, btn.Trigger()
		}
		before := s.table.Cursor()
		var cmd tea.Cmd
		s.table, cmd = s.table.Update(m)
		if s.table.Cursor() != before {
			s.table.SetRows(s.buildRows(s.table.Cursor()))
		}
		return s, cmd
	}

	if s.modal != nil {
		return s.forwardToModal(msg)
	}
	var cmd tea.Cmd
	s.table, cmd = s.table.Update(msg)
	return s, cmd
}

func (s *profilesScreen) forwardToModal(msg tea.Msg) (Screen, tea.Cmd) {
	var cmd tea.Cmd
	s.modal, cmd = s.modal.Update(msg)
	return s, cmd
}

func (s *profilesScreen) Title() string { return "Profiles" }

func (s *profilesScreen) Description() string {
	return fmt.Sprintf("Browsing %d registered profile(s)", len(s.profiles))
}

func (s *profilesScreen) Topic() help.Topic {
	return help.Topic{Label: "Profiles", File: "profiles.md"}
}

// StatusKeys exposes only the row-level mnemonics (e/edit, d/delete).
// Screen-level c/r/b are visible on the button row beneath the table
// and must not be duplicated in the bar (parent task 0015 rule).
func (s *profilesScreen) StatusKeys() []key.Binding {
	if len(s.profiles) == 0 {
		return nil
	}
	return []key.Binding{s.edit.Binding(), s.delete.Binding()}
}

func (s *profilesScreen) InputFocused() bool { return s.modal.Active() }

func (s *profilesScreen) Body(width int) string {
	background := s.bodyContent()
	if s.modal == nil {
		return background
	}
	return s.modal.Render(background, width, lipgloss.Height(background))
}

// bodyContent renders the table plus the screen-level button row at
// natural width and height. The empty state collapses to a single hint
// line followed by the button row.
func (s *profilesScreen) bodyContent() string {
	buttonRow := " " + s.create.View() + "  " + s.register.View() + "  " + s.back.View()
	if len(s.profiles) == 0 {
		empty := styles.TextStyle.Render(" No profiles registered. Press 'c' to create one or 'r' to register an existing folder.")
		return lipgloss.JoinVertical(lipgloss.Left, empty, "", buttonRow)
	}
	sanitizeCursor(&s.table, len(s.profiles))
	rows := s.buildRows(s.table.Cursor())
	cols := naturalColumns(profileColumnTitles, rows)
	applyTable(&s.table, cols, rows)
	body := panel.Render(true, "", s.table.View(), focusAwarePanelStyles())
	return lipgloss.JoinVertical(lipgloss.Left, body, "", buttonRow)
}

var profileColumnTitles = []string{"ID", "Name", "Path", "Actions"}

// rebuildTable seeds a fresh bubbles/table model. Columns and width are
// finalized on every render in [bodyContent] from the actual row data.
func (s *profilesScreen) rebuildTable() {
	rows := s.buildRows(0)
	cols := naturalColumns(profileColumnTitles, rows)
	t := table.New(
		table.WithColumns(cols),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithWidth(tableNaturalWidth(cols)),
		table.WithHeight(len(rows)+1),
		table.WithStyles(styles.TableStyles()),
	)
	s.table = t
}

// buildRows materializes one table.Row per profile. The Actions cell is
// populated only on the cursor row, matching the task 0015 mockup where
// `[Edit] [Delete]` appears next to the selected profile. The label is
// produced by the screen's own mnemonic.Button instances so the cursor
// row picks up the themed accent/mnemonic/text palette and survives
// composition with the surrounding table Selected style — Button.View
// emits no intermediate `\x1b[0m` resets.
func (s *profilesScreen) buildRows(cursor int) []table.Row {
	rows := make([]table.Row, len(s.profiles))
	for i, p := range s.profiles {
		actions := ""
		if i == cursor {
			actions = s.actionsCellContent()
		}
		rows[i] = table.Row{p.Profile.Manifest.ID, p.Profile.Manifest.Name, p.Profile.Root, actions}
	}
	return rows
}

// actionsCellContent returns the "[Edit] [Delete]" cell rendered through
// the screen's existing mnemonic.Button instances. Routing the cell
// through the buttons means the cursor-row cell, the bottom-of-screen
// button row, and any future status-bar rendering all share one
// SGR-aware code path.
func (s *profilesScreen) actionsCellContent() string {
	return s.edit.ViewSelected() + " " + s.delete.ViewSelected()
}

// selectedProfile returns the profile under the table cursor. Returns
// ok=false when the table is empty so callers can no-op the row action.
func (s *profilesScreen) selectedProfile() (*appapi.LoadedProfile, bool) {
	if len(s.profiles) == 0 {
		return nil, false
	}
	cursor := s.table.Cursor()
	if cursor < 0 || cursor >= len(s.profiles) {
		return nil, false
	}
	return s.profiles[cursor], true
}

func (s *profilesScreen) onEdit() tea.Cmd {
	p, ok := s.selectedProfile()
	if !ok {
		return nil
	}
	return pushCmd(newEditProfileScreen(s.actions, p.Profile.Manifest.ID))
}

func (s *profilesScreen) onDelete() tea.Cmd {
	p, ok := s.selectedProfile()
	if !ok {
		return nil
	}
	s.pendingDeleteID = p.Profile.Manifest.ID
	s.pendingDeleteName = p.Profile.Manifest.Name
	prompt := fmt.Sprintf("Are you sure you want to delete profile %q?", p.Profile.Manifest.Name)
	s.openModal(modal.NewConfirm("delete-profile-1", prompt, nil))
	return s.modal.Init()
}

// onCreate starts the two-step Create Profile flow (task 0039): open the
// pathselector first, then open the Create Profile form seeded with the
// chosen path once "create-profile-path" resolves. Any prior stashed name
// is cleared so a fresh flow starts empty.
func (s *profilesScreen) onCreate() tea.Cmd {
	s.pendingCreateName = ""
	return s.openCreateProfilePathselectorCmd("")
}

// onRegister mirrors onCreate for Register Profile. Register Profile has
// no editable field other than the path, so nothing is stashed here.
func (s *profilesScreen) onRegister() tea.Cmd {
	return s.openRegisterProfilePathselectorCmd("")
}

// openCreateProfilePathselectorCmd opens the folder picker for the Create
// Profile flow. startFolder is empty on the first open (defaults to $HOME
// through pathselector.probe) and set to filepath.Dir(previousPath) on
// retry after a failure. A pathselector build failure (e.g. unreadable
// $HOME) is surfaced as a notification so the flow degrades to a visible
// error instead of a silent no-op.
func (s *profilesScreen) openCreateProfilePathselectorCmd(startFolder string) tea.Cmd {
	m, err := modals.NewSelectPath("create-profile-path", pathselector.Options{
		Caption:     "Select profile folder",
		ShowFiles:   false,
		StartFolder: startFolder,
	})
	if err != nil {
		return notificationCmd(err.Severity(), err.Error())
	}
	s.openModal(m)
	return s.modal.Init()
}

func (s *profilesScreen) openRegisterProfilePathselectorCmd(startFolder string) tea.Cmd {
	m, err := modals.NewSelectPath("register-profile-path", pathselector.Options{
		Caption:     "Select existing profile folder",
		ShowFiles:   false,
		StartFolder: startFolder,
	})
	if err != nil {
		return notificationCmd(err.Severity(), err.Error())
	}
	s.openModal(m)
	return s.modal.Init()
}

// openModal mounts m and primes its size from the current viewport so
// the table widget inside doesn't see a zero-dimension rectangle on its
// first render.
func (s *profilesScreen) openModal(m *modal.Modal) {
	s.modal = m
	if s.width > 0 && s.height > 0 {
		mw, mh := modalSize(s.width, s.height)
		s.modal.SetSize(mw, mh)
	}
}

// handleResolved dispatches a modal ResolvedMsg to the right
// post-action handler. The modal field is cleared up-front so the body
// stops compositing the resolved dialog regardless of which branch runs.
func (s *profilesScreen) handleResolved(msg modal.ResolvedMsg) tea.Cmd {
	s.modal = nil
	switch msg.ID {
	case "create-profile-path":
		return s.afterCreatePath(msg)
	case "register-profile-path":
		return s.afterRegisterPath(msg)
	case "create-profile":
		return s.afterCreate(msg)
	case "register-profile":
		return s.afterRegister(msg)
	case "delete-profile-1":
		return s.afterDeleteStep1(msg)
	case "delete-profile-2":
		return s.afterDeleteStep2(msg)
	}
	return nil
}

// afterCreatePath opens the Create Profile form seeded with the picked
// path and the stashed name (empty on the first open, non-empty on retry).
// Cancelling the pathselector aborts the flow.
func (s *profilesScreen) afterCreatePath(msg modal.ResolvedMsg) tea.Cmd {
	result, ok := pathselector.ResultFromMsg(msg)
	if !ok {
		s.pendingCreateName = ""
		return nil
	}
	s.openModal(modals.NewCreateProfile(modals.CreateProfileInput{
		Name: s.pendingCreateName,
		Path: result.Path,
	}))
	return s.modal.Init()
}

// afterRegisterPath is the Register Profile analogue. No stashed input to
// re-seed — the form has only the path.
func (s *profilesScreen) afterRegisterPath(msg modal.ResolvedMsg) tea.Cmd {
	result, ok := pathselector.ResultFromMsg(msg)
	if !ok {
		return nil
	}
	s.openModal(modals.NewRegisterProfile(modals.RegisterProfileInput{Path: result.Path}))
	return s.modal.Init()
}

// afterCreate runs the CreateProfile action. On success it returns a
// mutationDoneMsg so the shared refresh path fires; on failure it returns
// the per-flow createProfileFailedMsg so the retry re-opens the
// pathselector seeded at filepath.Dir(path). The stashed name survives the
// retry so the follow-on form is re-seeded with it.
func (s *profilesScreen) afterCreate(msg modal.ResolvedMsg) tea.Cmd {
	if !msg.Confirmed {
		s.pendingCreateName = ""
		return nil
	}
	in, ok := msg.Value.(modals.CreateProfileInput)
	if !ok {
		return nil
	}
	s.pendingCreateName = in.Name
	successText := fmt.Sprintf("Profile %q created", in.Name)
	return func() tea.Msg {
		_, err := s.actions.CreateProfile(actions.CreateProfileInput{Name: in.Name, Path: in.Path})
		if err != nil {
			return createProfileFailedMsg{path: in.Path, text: err.Error(), severity: err.Severity()}
		}
		return mutationDoneMsg{text: successText, severity: errs.SeverityInfo}
	}
}

func (s *profilesScreen) afterRegister(msg modal.ResolvedMsg) tea.Cmd {
	if !msg.Confirmed {
		return nil
	}
	in, ok := msg.Value.(modals.RegisterProfileInput)
	if !ok {
		return nil
	}
	return func() tea.Msg {
		_, err := s.actions.RegisterProfile(actions.RegisterProfileInput{Path: in.Path})
		if err != nil {
			return registerProfileFailedMsg{path: in.Path, text: err.Error(), severity: err.Severity()}
		}
		return mutationDoneMsg{text: "Profile registered", severity: errs.SeverityInfo}
	}
}

func (s *profilesScreen) afterDeleteStep1(msg modal.ResolvedMsg) tea.Cmd {
	if !msg.Confirmed {
		s.pendingDeleteID = ""
		s.pendingDeleteName = ""
		return nil
	}
	s.openModal(modal.NewConfirm(
		"delete-profile-2",
		"Also delete profile folder on disk?",
		nil,
	))
	return s.modal.Init()
}

func (s *profilesScreen) afterDeleteStep2(msg modal.ResolvedMsg) tea.Cmd {
	id := s.pendingDeleteID
	name := s.pendingDeleteName
	s.pendingDeleteID = ""
	s.pendingDeleteName = ""
	if id == "" {
		return nil
	}
	if msg.Confirmed {
		return mutationCmd(
			func() errs.DomainError {
				_, err := s.actions.DeleteProfileWithFolder(actions.DeleteProfileInput{ProfileRef: id})
				return err
			},
			fmt.Sprintf("Profile %q and folder deleted", name),
		)
	}
	return mutationCmd(
		func() errs.DomainError {
			_, err := s.actions.DeleteProfile(actions.DeleteProfileInput{ProfileRef: id})
			return err
		},
		fmt.Sprintf("Profile %q deleted", name),
	)
}

// notificationCmd wraps a one-shot notification dispatch so the screen
// can surface a result text without reaching into the shell's bridge
// directly.
func notificationCmd(severity errs.Severity, text string) tea.Cmd {
	return func() tea.Msg {
		return notifications.NotificationMsg{
			Notification: notifications.Notification{
				Severity:  severity,
				Text:      text,
				CreatedAt: time.Now(),
			},
		}
	}
}
