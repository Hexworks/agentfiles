package shell

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/hexworks/agentfiles/internal/actions"
	"github.com/hexworks/agentfiles/internal/app"
	"github.com/hexworks/agentfiles/internal/config"
	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/profile"
	"github.com/hexworks/agentfiles/internal/registry"
	"github.com/hexworks/agentfiles/internal/tui/components/mnemonic"
	"github.com/hexworks/agentfiles/internal/tui/components/modal"
	"github.com/hexworks/agentfiles/internal/tui/modals"
	"github.com/hexworks/agentfiles/internal/tui/notifications"
)

// profilesFixture wires up a real registry-backed Actions handle the
// same way shell_test.go's newTestShell does. Tests that need actual
// LoadProfiles / DeleteProfile flow through this; tests that only
// exercise UI state seed the slice directly via the helpers below.
type profilesFixture struct {
	Root    string
	Service *app.Service
	Actions *actions.Actions
}

func newProfilesFixture(t *testing.T) *profilesFixture {
	t.Helper()
	root := t.TempDir()
	svc := app.New(filepath.Join(root, "registry.json"))
	return &profilesFixture{Root: root, Service: svc, Actions: actions.New(svc)}
}

func (f *profilesFixture) seed(t *testing.T, name string) *registry.ProfileRef {
	t.Helper()
	ref, err := f.Service.CreateProfile(name, filepath.Join(f.Root, name))
	if err != nil {
		t.Fatalf("seed %q: %v", name, err)
	}
	return ref
}

// withProfiles bypasses LoadProfiles and pushes a hand-built slice into
// the screen so UI-state tests can run without spinning a real registry.
func withProfiles(s *profilesScreen, profiles []*profile.Profile) {
	s.profiles = profiles
	s.rebuildSet()
	s.rebuildTable()
}

func fakeProfile(id, name, root string) *profile.Profile {
	return &profile.Profile{
		Root:     root,
		Manifest: profile.Manifest{ID: id, Name: name},
	}
}

// drainCmd runs cmd to completion. If the cmd is a tea.Sequence /
// tea.Batch the resulting wrapper message is returned; tests destructure
// it via tea.BatchMsg / tea.SequenceMsg as needed.
func drainCmd(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatalf("expected non-nil cmd")
	}
	return cmd()
}

func TestProfilesScreen_InitLoadsProfilesFromActions(t *testing.T) {
	f := newProfilesFixture(t)
	f.seed(t, "alpha")
	s := newProfilesScreen(f.Actions)

	msg := drainCmd(t, s.Init())

	loaded, ok := msg.(profilesLoadedMsg)
	if !ok {
		t.Fatalf("Init produced %T, want profilesLoadedMsg", msg)
	}
	if loaded.err != nil {
		t.Fatalf("loaded err = %v, want nil", loaded.err)
	}
	if len(loaded.profiles) != 1 {
		t.Fatalf("loaded %d profiles, want 1", len(loaded.profiles))
	}
	if loaded.profiles[0].Manifest.Name != "alpha" {
		t.Errorf("loaded name = %q, want alpha", loaded.profiles[0].Manifest.Name)
	}
}

func TestProfilesScreen_NoProfiles_MnemonicSetHasOnlyCRB(t *testing.T) {
	f := newProfilesFixture(t)
	s := newProfilesScreen(f.Actions)
	withProfiles(s, nil)

	got := mnemonicLabels(s.set)
	want := []string{"Create New Profile", "Register Profile", "Back"}
	if !labelsEqual(got, want) {
		t.Errorf("set labels = %v, want %v", got, want)
	}
}

func TestProfilesScreen_WithProfiles_MnemonicSetHasEDCRB_AllUnique(t *testing.T) {
	f := newProfilesFixture(t)
	s := newProfilesScreen(f.Actions)
	withProfiles(s, []*profile.Profile{fakeProfile("alpha", "Alpha", "/tmp/alpha")})

	got := mnemonicLabels(s.set)
	want := []string{"Edit", "Delete", "Create New Profile", "Register Profile", "Back"}
	if !labelsEqual(got, want) {
		t.Errorf("set labels = %v, want %v", got, want)
	}
}

func TestProfilesScreen_BTriggersPop(t *testing.T) {
	f := newProfilesFixture(t)
	s := newProfilesScreen(f.Actions)
	withProfiles(s, nil)

	_, cmd := s.Update(tea.KeyPressMsg{Code: 'b', Text: "b"})

	if cmd == nil {
		t.Fatalf("b produced nil cmd")
	}
	if _, ok := cmd().(PopScreenMsg); !ok {
		t.Fatalf("cmd produced %T, want PopScreenMsg", cmd())
	}
}

func TestProfilesScreen_EscWithNoModalTriggersPop(t *testing.T) {
	f := newProfilesFixture(t)
	s := newProfilesScreen(f.Actions)
	withProfiles(s, nil)

	_, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEsc})

	if cmd == nil {
		t.Fatalf("esc produced nil cmd")
	}
	if _, ok := cmd().(PopScreenMsg); !ok {
		t.Fatalf("cmd produced %T, want PopScreenMsg", cmd())
	}
}

func TestProfilesScreen_DuplicateMnemonicWouldPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic on duplicate mnemonic, got none")
		}
	}()
	set := mnemonic.NewSet()
	set.Add(mnemonic.New("Edit", 'e', func() tea.Cmd { return nil }))
	set.Add(mnemonic.New("Erase", 'e', func() tea.Cmd { return nil }))
}

func TestProfilesScreen_StatusKeysExcludeScreenLevelButtons(t *testing.T) {
	f := newProfilesFixture(t)
	s := newProfilesScreen(f.Actions)
	withProfiles(s, []*profile.Profile{fakeProfile("alpha", "Alpha", "/tmp/alpha")})

	keys := s.StatusKeys()
	if len(keys) != 2 {
		t.Fatalf("StatusKeys length = %d, want 2", len(keys))
	}
	got := []string{keys[0].Help().Key, keys[1].Help().Key}
	want := []string{"e", "d"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("StatusKeys[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	for _, k := range keys {
		if k.Help().Key == "c" || k.Help().Key == "r" {
			t.Errorf("StatusKeys leaked screen-level mnemonic %q", k.Help().Key)
		}
	}
}

func TestProfilesScreen_StatusKeysEmptyWhenNoProfiles(t *testing.T) {
	f := newProfilesFixture(t)
	s := newProfilesScreen(f.Actions)
	withProfiles(s, nil)

	if keys := s.StatusKeys(); keys != nil {
		t.Errorf("StatusKeys = %v, want nil on empty state", keys)
	}
}

func TestProfilesScreen_EscWithModalForwardsToModal(t *testing.T) {
	f := newProfilesFixture(t)
	s := newProfilesScreen(f.Actions)
	withProfiles(s, nil)

	// Open a form modal first; esc should be forwarded to it and abort
	// the form, which clears the modal field on the next ResolvedMsg.
	_, _ = s.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if s.modal == nil {
		t.Fatalf("setup: create modal not opened")
	}

	_, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEsc})

	// The modal abort path emits a ResolvedMsg via cmd; pump it through
	// Update so the screen clears its modal field.
	msgs := collectMessages(t, cmd)
	for _, m := range msgs {
		if resolved, ok := m.(modal.ResolvedMsg); ok {
			_, _ = s.Update(resolved)
		}
	}

	if s.modal != nil {
		t.Errorf("modal still open after esc on form modal, want cleared")
	}
}

func TestProfilesScreen_CKeyOpensCreateProfileModal(t *testing.T) {
	f := newProfilesFixture(t)
	s := newProfilesScreen(f.Actions)
	withProfiles(s, nil)

	_, _ = s.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})

	if s.modal == nil {
		t.Fatalf("modal nil after 'c'")
	}
	if got := s.modal.ID(); got != "create-profile" {
		t.Errorf("modal id = %q, want create-profile", got)
	}
}

func TestProfilesScreen_RKeyOpensRegisterProfileModal(t *testing.T) {
	f := newProfilesFixture(t)
	s := newProfilesScreen(f.Actions)
	withProfiles(s, nil)

	_, _ = s.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})

	if s.modal == nil {
		t.Fatalf("modal nil after 'r'")
	}
	if got := s.modal.ID(); got != "register-profile" {
		t.Errorf("modal id = %q, want register-profile", got)
	}
}

func TestProfilesScreen_EKeyPushesEditProfileStub(t *testing.T) {
	f := newProfilesFixture(t)
	s := newProfilesScreen(f.Actions)
	withProfiles(s, []*profile.Profile{fakeProfile("alpha", "Alpha", "/tmp/alpha")})

	_, cmd := s.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})

	push, ok := drainCmd(t, cmd).(PushScreenMsg)
	if !ok {
		t.Fatalf("cmd produced %T, want PushScreenMsg", cmd())
	}
	stub, ok := push.Screen.(*editProfileStub)
	if !ok {
		t.Fatalf("pushed screen = %T, want *editProfileStub", push.Screen)
	}
	if stub.profileID != "alpha" {
		t.Errorf("stub profileID = %q, want alpha", stub.profileID)
	}
}

func TestProfilesScreen_DKeyOpensFirstConfirmModal(t *testing.T) {
	f := newProfilesFixture(t)
	s := newProfilesScreen(f.Actions)
	withProfiles(s, []*profile.Profile{fakeProfile("alpha", "Alpha", "/tmp/alpha")})

	_, _ = s.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})

	if s.modal == nil {
		t.Fatalf("modal nil after 'd'")
	}
	if got := s.modal.ID(); got != "delete-profile-1" {
		t.Errorf("modal id = %q, want delete-profile-1", got)
	}
	if s.pendingDeleteID != "alpha" {
		t.Errorf("pendingDeleteID = %q, want alpha", s.pendingDeleteID)
	}
	if s.pendingDeleteName != "Alpha" {
		t.Errorf("pendingDeleteName = %q, want Alpha", s.pendingDeleteName)
	}
}

func TestProfilesScreen_DeleteStep1NoMakesNoServiceCall(t *testing.T) {
	f := newProfilesFixture(t)
	ref := f.seed(t, "alpha")
	s := newProfilesScreen(f.Actions)
	withProfiles(s, []*profile.Profile{fakeProfile(ref.ID, ref.Name, ref.Path)})

	_, _ = s.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	if s.modal == nil {
		t.Fatalf("setup: modal not opened")
	}

	_, cmd := s.Update(modal.ResolvedMsg{ID: "delete-profile-1", Confirmed: false})

	if cmd != nil {
		t.Errorf("step-1 No produced cmd = %v, want nil", cmd)
	}
	if s.modal != nil {
		t.Errorf("modal still open after step-1 No, want cleared")
	}
	if s.pendingDeleteID != "" {
		t.Errorf("pendingDeleteID = %q, want cleared", s.pendingDeleteID)
	}
	profiles, err := f.Actions.LoadProfiles()
	if err != nil {
		t.Fatalf("LoadProfiles: %v", err)
	}
	if len(profiles) != 1 {
		t.Errorf("profile count = %d, want 1 (no service call)", len(profiles))
	}
}

func TestProfilesScreen_DeleteStep2NoCallsKeepFolders(t *testing.T) {
	f := newProfilesFixture(t)
	ref := f.seed(t, "alpha")
	s := newProfilesScreen(f.Actions)
	withProfiles(s, []*profile.Profile{fakeProfile(ref.ID, ref.Name, ref.Path)})

	_, _ = s.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	_, _ = s.Update(modal.ResolvedMsg{ID: "delete-profile-1", Confirmed: true})
	if s.modal == nil || s.modal.ID() != "delete-profile-2" {
		t.Fatalf("setup: step-2 modal not opened")
	}

	_, cmd := s.Update(modal.ResolvedMsg{ID: "delete-profile-2", Confirmed: false})

	// Running cmd executes DeleteProfile synchronously inside its
	// closure; the returned message is the mutation-done envelope.
	done, ok := drainCmd(t, cmd).(profileMutationDoneMsg)
	if !ok {
		t.Fatalf("cmd produced %T, want profileMutationDoneMsg", drainCmd(t, cmd))
	}
	if done.severity != errs.SeverityInfo {
		t.Errorf("severity = %v, want info", done.severity)
	}

	if _, statErr := os.Stat(ref.Path); statErr != nil {
		t.Errorf("expected folder kept on Yes/No, got %v", statErr)
	}
	profiles, err := f.Actions.LoadProfiles()
	if err != nil {
		t.Fatalf("LoadProfiles: %v", err)
	}
	if len(profiles) != 0 {
		t.Errorf("registry profile count = %d, want 0 after delete", len(profiles))
	}
}

func TestProfilesScreen_DeleteStep2YesCallsDeleteFolders(t *testing.T) {
	f := newProfilesFixture(t)
	ref := f.seed(t, "alpha")
	s := newProfilesScreen(f.Actions)
	withProfiles(s, []*profile.Profile{fakeProfile(ref.ID, ref.Name, ref.Path)})

	_, _ = s.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	_, _ = s.Update(modal.ResolvedMsg{ID: "delete-profile-1", Confirmed: true})

	_, cmd := s.Update(modal.ResolvedMsg{ID: "delete-profile-2", Confirmed: true})

	if _, ok := drainCmd(t, cmd).(profileMutationDoneMsg); !ok {
		t.Fatalf("cmd produced %T, want profileMutationDoneMsg", drainCmd(t, cmd))
	}

	if _, statErr := os.Stat(ref.Path); !os.IsNotExist(statErr) {
		t.Errorf("expected folder removed on Yes/Yes, stat err = %v", statErr)
	}
	profiles, err := f.Actions.LoadProfiles()
	if err != nil {
		t.Fatalf("LoadProfiles: %v", err)
	}
	if len(profiles) != 0 {
		t.Errorf("registry profile count = %d, want 0 after delete", len(profiles))
	}
}

func TestProfilesScreen_MutationRefreshesProfiles(t *testing.T) {
	f := newProfilesFixture(t)
	s := newProfilesScreen(f.Actions)
	withProfiles(s, nil)

	in := modals.CreateProfileInput{Name: "Alpha", Path: filepath.Join(f.Root, "alpha")}
	_, cmd := s.Update(modal.ResolvedMsg{ID: "create-profile", Confirmed: true, Value: in})

	// 1. Run mutationCmd → action executes, returns the done envelope.
	done, ok := drainCmd(t, cmd).(profileMutationDoneMsg)
	if !ok {
		t.Fatalf("cmd produced %T, want profileMutationDoneMsg", drainCmd(t, cmd))
	}

	// 2. Pump the done envelope through Update; this should emit a
	// batch with both the notification cmd and the reload.
	_, batch := s.Update(done)
	collected := collectMessages(t, batch)

	// Find the profilesLoadedMsg in the batch results; it carries the
	// freshly-fetched profiles slice.
	var loaded *profilesLoadedMsg
	for _, m := range collected {
		if pm, ok := m.(profilesLoadedMsg); ok {
			loaded = &pm
			break
		}
	}
	if loaded == nil {
		t.Fatalf("batch missing profilesLoadedMsg; got %v", collected)
	}
	if len(loaded.profiles) != 1 {
		t.Errorf("loaded profile count = %d, want 1", len(loaded.profiles))
	}

	// 3. Pump the loaded msg through Update so the screen state reflects
	// the reload — the same path the real runtime would take.
	_, _ = s.Update(*loaded)
	if len(s.profiles) != 1 {
		t.Errorf("s.profiles after reload = %d, want 1", len(s.profiles))
	}
}

func TestProfilesScreen_MutationDoneEmitsNotificationAndReload(t *testing.T) {
	f := newProfilesFixture(t)
	s := newProfilesScreen(f.Actions)
	withProfiles(s, nil)

	_, cmd := s.Update(profileMutationDoneMsg{text: "Profile created", severity: errs.SeverityInfo})

	collected := collectMessages(t, cmd)
	var sawNotification, sawLoad bool
	for _, m := range collected {
		if note, ok := m.(notifications.NotificationMsg); ok {
			sawNotification = true
			if note.Notification.Text != "Profile created" {
				t.Errorf("notification text = %q, want 'Profile created'", note.Notification.Text)
			}
		}
		if _, ok := m.(profilesLoadedMsg); ok {
			sawLoad = true
		}
	}
	if !sawNotification {
		t.Errorf("batch missing NotificationMsg; got %v", collected)
	}
	if !sawLoad {
		t.Errorf("batch missing profilesLoadedMsg; got %v", collected)
	}
}

func TestProfilesScreen_LoadErrorEmitsNotification(t *testing.T) {
	f := newProfilesFixture(t)
	ref := f.seed(t, "alpha")
	// Corrupt the manifest so a subsequent Load surfaces a domain error.
	if err := os.WriteFile(filepath.Join(ref.Path, config.ProfileManifestFileName), []byte("nope"), 0o644); err != nil {
		t.Fatalf("corrupt: %v", err)
	}
	s := newProfilesScreen(f.Actions)

	msg := drainCmd(t, s.Init())
	loaded, ok := msg.(profilesLoadedMsg)
	if !ok {
		t.Fatalf("Init produced %T, want profilesLoadedMsg", msg)
	}
	if loaded.err == nil {
		t.Fatalf("loaded.err nil, want domain error")
	}

	_, cmd := s.Update(loaded)
	out := drainCmd(t, cmd)
	note, ok := out.(notifications.NotificationMsg)
	if !ok {
		t.Fatalf("err branch produced %T, want NotificationMsg", out)
	}
	if note.Notification.Severity != errs.SeverityError {
		t.Errorf("severity = %v, want error", note.Notification.Severity)
	}
}

func TestProfilesScreen_TitleAndBodyContainRequiredText(t *testing.T) {
	f := newProfilesFixture(t)
	s := newProfilesScreen(f.Actions)
	withProfiles(s, nil)
	_, _ = s.Update(tea.WindowSizeMsg{Width: 100, Height: 24})

	if got := s.Title(); got != "Profiles" {
		t.Errorf("Title() = %q, want Profiles", got)
	}
	body := s.Body(100, 20)
	// mnemonic.Button styles the mnemonic rune, splitting it from the
	// rest of the label with ANSI escapes — assert on substrings that
	// survive the styling instead of the full label.
	for _, want := range []string{"reate New Profile", "egister Profile"} {
		if !strings.Contains(body, want) {
			t.Errorf("Body missing %q\n%s", want, body)
		}
	}
}

func TestProfilesScreen_BodyShowsProfileRowsAndActions(t *testing.T) {
	f := newProfilesFixture(t)
	s := newProfilesScreen(f.Actions)
	withProfiles(s, []*profile.Profile{fakeProfile("alpha", "Alpha", "/tmp/alpha")})
	_, _ = s.Update(tea.WindowSizeMsg{Width: 100, Height: 24})

	body := s.Body(100, 20)
	// Selected-row actions cell holds "[Edit] [Delete]" with manual SGR
	// underline markers around the mnemonic chars so the cursor-row
	// highlight is not broken by an embedded lipgloss reset.
	for _, want := range []string{"alpha", "Alpha", "/tmp/alpha", "dit]", "elete]"} {
		if !strings.Contains(body, want) {
			t.Errorf("Body missing %q\n%s", want, body)
		}
	}
}

func TestProfilesScreen_CursorMoveRebuildsActionsColumn(t *testing.T) {
	f := newProfilesFixture(t)
	s := newProfilesScreen(f.Actions)
	withProfiles(s, []*profile.Profile{
		fakeProfile("alpha", "Alpha", "/tmp/alpha"),
		fakeProfile("beta", "Beta", "/tmp/beta"),
	})
	_, _ = s.Update(tea.WindowSizeMsg{Width: 120, Height: 24})

	// Move cursor down; the new row should carry the action buttons,
	// and the old row should drop them.
	_, _ = s.Update(tea.KeyPressMsg{Code: tea.KeyDown})

	if got := s.table.Cursor(); got != 1 {
		t.Fatalf("cursor = %d, want 1", got)
	}
	rows := s.table.Rows()
	if len(rows) != 2 {
		t.Fatalf("row count = %d, want 2", len(rows))
	}
	if rows[0][3] != "" {
		t.Errorf("row 0 actions = %q, want empty after cursor moved off it", rows[0][3])
	}
	if rows[1][3] != actionsCellContent() {
		t.Errorf("row 1 actions = %q, want actionsCellContent on cursor row", rows[1][3])
	}
}

func TestProfilesScreen_ActionsCellUsesSGRUnderlineWithoutReset(t *testing.T) {
	got := actionsCellContent()
	// Underline-on (SGR 4) and underline-off (SGR 24) wrap each
	// mnemonic char — kept manual so the cell composes inside a
	// styled cursor row without a full ANSI reset killing the wrapper.
	for _, want := range []string{"\x1b[4mE\x1b[24m", "\x1b[4mD\x1b[24m"} {
		if !strings.Contains(got, want) {
			t.Errorf("actionsCellContent missing SGR sequence %q\n%q", want, got)
		}
	}
	if strings.Contains(got, "\x1b[0m") {
		t.Errorf("actionsCellContent contains full reset \\x1b[0m which would break cursor-row highlight\n%q", got)
	}
}

// collectMessages flushes cmd and recursively expands any tea.BatchMsg
// the cmd produces, returning every concrete message dispatched. The
// screen's mutation flow leans on tea.Batch (notification + reload), so
// tests need to inspect both branches.
func collectMessages(t *testing.T, cmd tea.Cmd) []tea.Msg {
	t.Helper()
	if cmd == nil {
		return nil
	}
	var out []tea.Msg
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			out = append(out, collectMessages(t, c)...)
		}
		return out
	}
	return append(out, msg)
}

func mnemonicLabels(set *mnemonic.Set) []string {
	bs := set.Buttons()
	out := make([]string, len(bs))
	for i, b := range bs {
		out[i] = b.Label()
	}
	return out
}

func labelsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Compile-time guard ensuring profilesScreen satisfies the Screen
// interface. Mirrors the convention used elsewhere when a Screen is
// constructed by the shell package.
var _ Screen = (*profilesScreen)(nil)
