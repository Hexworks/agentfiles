package shell

import (
	"fmt"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/hexworks/agentfiles/internal/actions"
	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/profile"
	"github.com/hexworks/agentfiles/internal/registry"
	"github.com/hexworks/agentfiles/internal/tui/components/mnemonic"
	"github.com/hexworks/agentfiles/internal/tui/components/modal"
	"github.com/hexworks/agentfiles/internal/tui/modals"
	"github.com/hexworks/agentfiles/internal/tui/notifications"
)

// profilesScreen is the Profiles management screen reached from Welcome.
// It owns a bubbles/table view of the registered profiles, screen-level
// mnemonic buttons for Create / Register, and row-level mnemonic buttons
// for Edit / Delete on the cursor row. Confirmation and form modals
// composite over the screen body; no sub-screen is pushed for them.
type profilesScreen struct {
	actions  *actions.Actions
	profiles []*profile.Profile
	table    table.Model

	modal             *modal.Modal
	pendingDeleteID   string
	pendingDeleteName string

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
	profiles []*profile.Profile
	err      errs.DomainError
}

// profileMutationDoneMsg is dispatched once a Create / Register / Delete
// action has run to completion. The screen reacts by emitting both a
// NotificationMsg (so the user sees the outcome) and a fresh load so the
// table reflects the new registry state. Using a single, internal
// envelope lets us guarantee the action completed before the reload
// starts — tea.Sequence's wrapper message is unexported and not
// inspectable in tests, so a custom message is the testable path.
type profileMutationDoneMsg struct {
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
		s.applyTableSize()
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

	case profileMutationDoneMsg:
		return s, tea.Batch(notificationCmd(m.severity, m.text), s.loadCmd())

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

// StatusKeys exposes only the row-level mnemonics (e/edit, d/delete).
// Screen-level c/r/b are visible on the button row beneath the table
// and must not be duplicated in the bar (parent task 0015 rule).
func (s *profilesScreen) StatusKeys() []key.Binding {
	if len(s.profiles) == 0 {
		return nil
	}
	return []key.Binding{s.edit.Binding(), s.delete.Binding()}
}

func (s *profilesScreen) Body(width, height int) string {
	background := s.bodyContent(width, height)
	if s.modal == nil {
		return background
	}
	return s.modal.Render(background, width, height)
}

// bodyContent renders the table plus the screen-level button row at
// exactly height rows. The shell hands Body the height it has reserved
// for screen content (window minus title, toast, and status bar) — the
// content must match that height exactly or the JoinVertical in
// shell.View will push the status bar off the bottom of the terminal.
func (s *profilesScreen) bodyContent(width, height int) string {
	const reservedBelow = 2 // 1 spacer + 1 button row
	tableH := height - reservedBelow
	if tableH < 1 {
		tableH = 1
	}
	s.table.SetWidth(width)
	s.table.SetColumns(s.columns(width))
	s.table.SetHeight(tableH)

	buttonRow := " " + s.create.View() + "  " + s.register.View() + "  " + s.back.View()
	buttons := lipgloss.PlaceHorizontal(width, lipgloss.Left, buttonRow)
	if len(s.profiles) == 0 {
		empty := " No profiles registered. Press 'c' to create one or 'r' to register an existing folder."
		// Pad the hint block to tableH so the button row stays anchored
		// at the same y-coordinate it occupies when the table is non-empty.
		emptyBlock := lipgloss.NewStyle().Width(width).Height(tableH).Render(empty)
		return lipgloss.JoinVertical(lipgloss.Left, emptyBlock, "", buttons)
	}
	return lipgloss.JoinVertical(lipgloss.Left, s.table.View(), "", buttons)
}

// rebuildTable constructs a fresh bubbles/table model from the current
// profile slice. Re-running on every load keeps row data and column
// widths in sync without juggling partial state mutations. The table is
// seeded with a conservative default size; Body re-sizes it on every
// render using the height the shell has actually reserved.
func (s *profilesScreen) rebuildTable() {
	width := s.tableInnerWidth()
	cols := s.columns(width)
	rows := s.buildRows(0)
	t := table.New(
		table.WithColumns(cols),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithWidth(width),
		table.WithHeight(defaultTableHeight),
	)
	s.table = t
}

// defaultTableHeight is the seed value used when the screen has not
// yet seen a window-size message. Body overrides it on every render
// using the shell-reserved body height.
const defaultTableHeight = 10

// buildRows materializes one table.Row per profile. The Actions cell is
// populated only on the cursor row, matching the task 0015 mockup where
// `[Edit] [Delete]` appears next to the selected profile. The mnemonic
// chars are wrapped in raw SGR underline-on / underline-off codes so the
// surrounding cell style (the table's Selected row highlight) is not
// terminated by an embedded full-reset — a problem mnemonic.Button.View
// has because lipgloss emits `\x1b[0m` at the end of every styled span.
func (s *profilesScreen) buildRows(cursor int) []table.Row {
	rows := make([]table.Row, len(s.profiles))
	for i, p := range s.profiles {
		actions := ""
		if i == cursor {
			actions = actionsCellContent()
		}
		rows[i] = table.Row{p.Manifest.ID, p.Manifest.Name, p.Root, actions}
	}
	return rows
}

// actionsCellContent returns the "[Edit] [Delete]" label with the two
// mnemonic runes underlined via SGR 4 / 24. Manual escape sequences
// avoid lipgloss's full-reset behavior so the cell composes cleanly
// inside the table's Selected style.
func actionsCellContent() string {
	const (
		underlineOn  = "\x1b[4m"
		underlineOff = "\x1b[24m"
	)
	return "[" + underlineOn + "E" + underlineOff + "dit] " +
		"[" + underlineOn + "D" + underlineOff + "elete]"
}

// columns picks fixed widths for ID / Name / Actions and gives the
// remainder to Path so long filesystem paths stay readable on wide
// terminals.
func (s *profilesScreen) columns(innerW int) []table.Column {
	const (
		idW         = 14
		nameW       = 18
		actionsW    = 22
		cellPadding = 8 // 2 cols of padding per column from table's default Cell style
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

const minPathColW = 12

// applyTableSize keeps the table's width and column layout in sync with
// the current window. Height is left to Body, which receives the actual
// shell-reserved body height on every render — sizing the table at
// resize time using full window height was the source of a layout bug
// where the table overflowed the body and pushed status + toast off
// screen.
func (s *profilesScreen) applyTableSize() {
	s.table.SetWidth(s.tableInnerWidth())
	s.table.SetColumns(s.columns(s.tableInnerWidth()))
}

func (s *profilesScreen) tableInnerWidth() int {
	if s.width < 1 {
		return 80
	}
	return s.width
}

// selectedProfile returns the profile under the table cursor. Returns
// ok=false when the table is empty so callers can no-op the row action.
func (s *profilesScreen) selectedProfile() (*profile.Profile, bool) {
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
	return pushCmd(newEditProfileScreen(s.actions, p.Manifest.ID))
}

func (s *profilesScreen) onDelete() tea.Cmd {
	p, ok := s.selectedProfile()
	if !ok {
		return nil
	}
	s.pendingDeleteID = p.Manifest.ID
	s.pendingDeleteName = p.Manifest.Name
	prompt := fmt.Sprintf("Are you sure you want to delete profile %q?", p.Manifest.Name)
	s.openModal(modal.NewConfirm("delete-profile-1", prompt, nil))
	return s.modal.Init()
}

func (s *profilesScreen) onCreate() tea.Cmd {
	s.openModal(modals.NewCreateProfile(modals.CreateProfileInput{}))
	return s.modal.Init()
}

func (s *profilesScreen) onRegister() tea.Cmd {
	s.openModal(modals.NewRegisterProfile(modals.RegisterProfileInput{}))
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

func (s *profilesScreen) afterCreate(msg modal.ResolvedMsg) tea.Cmd {
	if !msg.Confirmed {
		return nil
	}
	in, ok := msg.Value.(modals.CreateProfileInput)
	if !ok {
		return nil
	}
	return mutationCmd(
		func() (*registry.ProfileRef, errs.DomainError) {
			return s.actions.CreateProfile(actions.CreateProfileInput{Name: in.Name, Path: in.Path})
		},
		fmt.Sprintf("Profile %q created", in.Name),
	)
}

func (s *profilesScreen) afterRegister(msg modal.ResolvedMsg) tea.Cmd {
	if !msg.Confirmed {
		return nil
	}
	in, ok := msg.Value.(modals.RegisterProfileInput)
	if !ok {
		return nil
	}
	return mutationCmd(
		func() (*registry.ProfileRef, errs.DomainError) {
			return s.actions.RegisterProfile(actions.RegisterProfileInput{Path: in.Path})
		},
		"Profile registered",
	)
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
			func() (struct{}, errs.DomainError) {
				return s.actions.DeleteProfileWithFolder(actions.DeleteProfileInput{ProfileRef: id})
			},
			fmt.Sprintf("Profile %q and folder deleted", name),
		)
	}
	return mutationCmd(
		func() (struct{}, errs.DomainError) {
			return s.actions.DeleteProfile(actions.DeleteProfileInput{ProfileRef: id})
		},
		fmt.Sprintf("Profile %q deleted", name),
	)
}

// mutationCmd runs action synchronously inside the Cmd closure and
// returns a [profileMutationDoneMsg] regardless of outcome. The screen
// reacts by emitting both a NotificationMsg and a reload — the
// action is guaranteed complete before the reload starts because
// the message itself is only dispatched after action returns.
func mutationCmd[T any](action func() (T, errs.DomainError), successText string) tea.Cmd {
	return func() tea.Msg {
		_, err := action()
		if err != nil {
			return profileMutationDoneMsg{text: err.Error(), severity: err.Severity()}
		}
		return profileMutationDoneMsg{text: successText, severity: errs.SeverityInfo}
	}
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
