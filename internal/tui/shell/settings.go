package shell

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"

	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/settings"
	"github.com/hexworks/agentfiles/internal/tui/components/focus"
	"github.com/hexworks/agentfiles/internal/tui/components/help"
	"github.com/hexworks/agentfiles/internal/tui/components/mnemonic"
	"github.com/hexworks/agentfiles/internal/tui/components/modal"
	"github.com/hexworks/agentfiles/internal/tui/components/panel"
	"github.com/hexworks/agentfiles/internal/tui/styles"
)

// settingsActions is the narrow slice of *actions.Actions the Settings
// screen exercises. Naming the interface here keeps the dependency
// direction tui→app explicit and lets tests substitute a fake without
// dragging in the full Actions surface.
type settingsActions interface {
	LoadSettings(struct{}) (settings.Settings, errs.DomainError)
	UpdateSettings(settings.Settings) (struct{}, errs.DomainError)
}

// settingsForm mirrors the huh field values the screen binds to. Kept as
// its own struct so dirty() compares live values against a snapshot
// taken on load / save, matching edit_asset.go's pattern.
type settingsForm struct {
	gitEnabled bool
}

// modalKindSettings identifies which modal flow the Settings screen
// currently hosts. Tests assert intent through this enum rather than the
// raw modal-id string the dialog carries.
type modalKindSettings int

const (
	modalKindSettingsNone modalKindSettings = iota
	modalKindSettingsBackUnsaved
)

// settingsScreen is the persistent-settings management screen reached
// from the Welcome menu. It owns a single huh.Select for the git
// integration toggle plus [Save] / [Back] mnemonic buttons; the Save
// button emits `Settings saved` on success and refuses when the git
// binary pre-flight fails at UpdateSettings time.
type settingsScreen struct {
	actions settingsActions

	form     settingsForm
	snapshot settingsForm

	handler *focus.Handler
	toggle  *huh.Select[bool]

	saveBtn *mnemonic.Button
	backBtn *mnemonic.Button

	set *mnemonic.Set

	modal     *modal.Modal
	modalKind modalKindSettings

	width, height int
}

// settingsLoadedMsg carries the initial settings the Init command
// reads.
type settingsLoadedMsg struct {
	settings settings.Settings
	err      errs.DomainError
}

// settingsSavedMsg is dispatched after UpdateSettings succeeds so the
// snapshot can be refreshed atomically with the toast emission.
type settingsSavedMsg struct {
	snapshot settingsForm
}

func newSettingsScreen(a settingsActions) *settingsScreen {
	if a == nil {
		panic("shell.newSettingsScreen: nil actions")
	}
	s := &settingsScreen{actions: a}
	s.buildFields()
	s.buildButtons()
	s.handler = focus.New()
	s.handler.Add(s.toggle)
	s.rebuildSet()
	return s
}

func (s *settingsScreen) buildFields() {
	keymap := huh.NewDefaultKeyMap()
	theme := styles.HuhTheme()
	s.toggle = huh.NewSelect[bool]().
		Key("git_enabled").
		Title("Git integration").
		Description("Auto-commit changes when the folder is a git repo").
		Value(&s.form.gitEnabled).
		Options(
			huh.NewOption("Disabled", false),
			huh.NewOption("Enabled", true),
		)
	s.toggle.WithKeyMap(keymap)
	s.toggle.WithTheme(theme)
}

func (s *settingsScreen) buildButtons() {
	s.saveBtn = mnemonic.New("Save", 'e', func() tea.Cmd { return s.onSave() })
	s.backBtn = mnemonic.New(
		"Back",
		'b',
		func() tea.Cmd { return s.onBack() },
		mnemonic.WithExtraBindingKeys("esc"),
	)
}

func (s *settingsScreen) rebuildSet() {
	set := mnemonic.NewSet()
	set.Add(s.saveBtn)
	set.Add(s.backBtn)
	s.set = set
}

func (s *settingsScreen) Init() tea.Cmd { return s.loadCmd() }

func (s *settingsScreen) loadCmd() tea.Cmd {
	return func() tea.Msg {
		v, err := s.actions.LoadSettings(struct{}{})
		return settingsLoadedMsg{settings: v, err: err}
	}
}

func (s *settingsScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	if s.modal != nil {
		switch msg.(type) {
		case modal.ResolvedMsg,
			tea.WindowSizeMsg,
			settingsLoadedMsg,
			mutationDoneMsg,
			settingsSavedMsg:
			// fall through
		default:
			return s.forwardToModal(msg)
		}
	}
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		return s.handleResize(m)
	case settingsLoadedMsg:
		return s.handleLoaded(m)
	case settingsSavedMsg:
		return s.handleSaved(m)
	case mutationDoneMsg:
		return s, notificationCmd(m.severity, m.text)
	case modal.ResolvedMsg:
		return s, s.handleResolved(m)
	case tea.KeyPressMsg:
		return s.handleKey(m)
	}
	return s, nil
}

func (s *settingsScreen) forwardToModal(msg tea.Msg) (Screen, tea.Cmd) {
	var cmd tea.Cmd
	s.modal, cmd = s.modal.Update(msg)
	return s, cmd
}

func (s *settingsScreen) handleResize(m tea.WindowSizeMsg) (Screen, tea.Cmd) {
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

func (s *settingsScreen) handleLoaded(m settingsLoadedMsg) (Screen, tea.Cmd) {
	if m.err != nil {
		return s, notificationCmd(m.err.Severity(), m.err.Error())
	}
	s.form = settingsForm{gitEnabled: m.settings.Git.Enabled}
	s.snapshot = s.form
	s.toggle.Value(&s.form.gitEnabled)
	focusCmd := s.handler.FocusIndex(0)
	s.rebuildSet()
	return s, focusCmd
}

func (s *settingsScreen) handleSaved(m settingsSavedMsg) (Screen, tea.Cmd) {
	s.snapshot = m.snapshot
	return s, notificationCmd(errs.SeverityInfo, "Settings saved")
}

func (s *settingsScreen) handleKey(m tea.KeyPressMsg) (Screen, tea.Cmd) {
	if s.modal != nil {
		return s.forwardToModal(m)
	}
	if handled, cmd := s.handler.Update(m); handled {
		s.rebuildSet()
		return s, cmd
	}
	if btn := s.set.Match(m); btn != nil {
		return s, btn.Trigger()
	}
	return s, s.routeToFocusedComponent(m)
}

func (s *settingsScreen) routeToFocusedComponent(m tea.KeyPressMsg) tea.Cmd {
	c := s.handler.FocusedComponent()
	if f, ok := c.(huh.Field); ok {
		_, cmd := f.Update(m)
		return cmd
	}
	return nil
}

func (s *settingsScreen) handleResolved(msg modal.ResolvedMsg) tea.Cmd {
	s.modal = nil
	kind := s.modalKind
	s.modalKind = modalKindSettingsNone
	if kind == modalKindSettingsBackUnsaved {
		if msg.Confirmed {
			return popCmd()
		}
	}
	return nil
}

func (s *settingsScreen) onSave() tea.Cmd {
	composed := settings.Settings{
		Version: settings.Version,
		Git:     settings.GitSettings{Enabled: s.form.gitEnabled},
	}
	snapshot := s.form
	return func() tea.Msg {
		if _, err := s.actions.UpdateSettings(composed); err != nil {
			return mutationDoneMsg{severity: err.Severity(), text: err.Error()}
		}
		return settingsSavedMsg{snapshot: snapshot}
	}
}

func (s *settingsScreen) onBack() tea.Cmd {
	if !s.dirty() {
		return popCmd()
	}
	s.openModal(modal.NewConfirm("back-unsaved", "Discard unsaved changes?", nil), modalKindSettingsBackUnsaved)
	return s.modal.Init()
}

func (s *settingsScreen) openModal(m *modal.Modal, kind modalKindSettings) {
	s.modal = m
	s.modalKind = kind
	if s.width > 0 && s.height > 0 {
		mw, mh := modalSize(s.width, s.height)
		s.modal.SetSize(mw, mh)
	}
}

func (s *settingsScreen) dirty() bool {
	return s.form != s.snapshot
}

func (s *settingsScreen) InputFocused() bool {
	if s.modal != nil && s.modal.Active() {
		return true
	}
	return s.handler != nil && s.handler.Focused() >= 0
}

func (s *settingsScreen) Title() string { return "Settings" }

func (s *settingsScreen) Description() string {
	return "Application preferences and configuration"
}

func (s *settingsScreen) Topic() help.Topic {
	return help.Topic{Label: "Settings", File: "settings.md"}
}

func (s *settingsScreen) StatusKeys() []key.Binding {
	return []key.Binding{s.saveBtn.Binding(), s.backBtn.Binding()}
}

func (s *settingsScreen) Body(width int) string {
	clamp := width
	if clamp <= 0 {
		clamp = 40
	}
	innerWidth := clamp - 2
	if innerWidth < 1 {
		innerWidth = 1
	}
	s.toggle.WithWidth(innerWidth)
	st := focusAwarePanelStyles()
	fieldPanel := panel.Render(s.handler.Focused() == 0, "", s.toggle.View(), st)
	buttons := " " + s.saveBtn.View() + "  " + s.backBtn.View()
	background := lipgloss.JoinVertical(lipgloss.Left, fieldPanel, "", buttons)
	if s.modal == nil {
		return background
	}
	return s.modal.Render(background, width, lipgloss.Height(background))
}
