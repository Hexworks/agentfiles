package shell

import (
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/settings"
	"github.com/hexworks/agentfiles/internal/tui/components/modal"
	"github.com/hexworks/agentfiles/internal/tui/notifications"
)

// fakeSettingsActions is the test double for settingsActions. Load
// returns Loaded; Update records the input, returns UpdateErr, and swaps
// Loaded when the call succeeds.
type fakeSettingsActions struct {
	Loaded    settings.Settings
	Updates   []settings.Settings
	UpdateErr errs.DomainError
}

func (f *fakeSettingsActions) LoadSettings(struct{}) (settings.Settings, errs.DomainError) {
	return f.Loaded, nil
}

func (f *fakeSettingsActions) UpdateSettings(next settings.Settings) (struct{}, errs.DomainError) {
	f.Updates = append(f.Updates, next)
	if f.UpdateErr != nil {
		return struct{}{}, f.UpdateErr
	}
	f.Loaded = next
	return struct{}{}, nil
}

// missingBinaryErr is the typed error the pre-flight surfaces when the
// git binary is absent; the fake reproduces it so the screen renders
// the corresponding warn toast.
type missingBinaryErr struct{}

func (missingBinaryErr) Error() string           { return "git binary not found on PATH" }
func (missingBinaryErr) Severity() errs.Severity { return errs.SeverityWarning }

func loadFakeSettings(t *testing.T, s *settingsScreen) {
	t.Helper()
	cmd := s.Init()
	if cmd == nil {
		t.Fatalf("Init produced nil cmd")
	}
	msg := cmd()
	loaded, ok := msg.(settingsLoadedMsg)
	if !ok {
		t.Fatalf("Init cmd produced %T, want settingsLoadedMsg", msg)
	}
	if _, cmd := s.Update(loaded); cmd != nil {
		_ = cmd() // drain focus cmd
	}
}

func TestSettingsScreen_LoadHydratesForm(t *testing.T) {
	fake := &fakeSettingsActions{Loaded: settings.Settings{Version: 1, Git: settings.GitSettings{Enabled: true}}}
	s := newSettingsScreen(fake)
	loadFakeSettings(t, s)
	if !s.form.gitEnabled {
		t.Fatalf("form.gitEnabled = false, want true after load")
	}
	if s.snapshot != s.form {
		t.Fatalf("snapshot not aligned with form after load: %+v vs %+v", s.snapshot, s.form)
	}
}

func TestSettingsScreen_SaveWritesSettingsAndEmitsInfoToast(t *testing.T) {
	fake := &fakeSettingsActions{Loaded: settings.Settings{Version: 1}}
	s := newSettingsScreen(fake)
	loadFakeSettings(t, s)

	s.form.gitEnabled = true

	cmd := s.onSave()
	if cmd == nil {
		t.Fatalf("onSave returned nil cmd")
	}
	msg := cmd()
	saved, ok := msg.(settingsSavedMsg)
	if !ok {
		t.Fatalf("save cmd produced %T, want settingsSavedMsg", msg)
	}
	if len(fake.Updates) != 1 || !fake.Updates[0].Git.Enabled {
		t.Fatalf("expected one Update call with Git.Enabled=true, got %+v", fake.Updates)
	}

	_, toastCmd := s.Update(saved)
	if toastCmd == nil {
		t.Fatalf("saved handler produced nil cmd")
	}
	toast, ok := toastCmd().(notifications.NotificationMsg)
	if !ok {
		t.Fatalf("saved cmd produced %T, want NotificationMsg", toastCmd())
	}
	if toast.Notification.Severity != errs.SeverityInfo {
		t.Fatalf("severity = %v, want Info", toast.Notification.Severity)
	}
	if toast.Notification.Text != "Settings saved" {
		t.Fatalf("text = %q, want %q", toast.Notification.Text, "Settings saved")
	}
	if s.dirty() {
		t.Fatalf("dirty() = true after saved handler, want false")
	}
}

func TestSettingsScreen_SavePreflightFailureRefusesWrite(t *testing.T) {
	fake := &fakeSettingsActions{
		Loaded:    settings.Settings{Version: 1},
		UpdateErr: missingBinaryErr{},
	}
	s := newSettingsScreen(fake)
	loadFakeSettings(t, s)
	s.form.gitEnabled = true

	cmd := s.onSave()
	msg := cmd()
	done, ok := msg.(mutationDoneMsg)
	if !ok {
		t.Fatalf("save cmd produced %T, want mutationDoneMsg", msg)
	}
	if !strings.Contains(done.text, "git binary not found") {
		t.Fatalf("expected pre-flight text, got %q", done.text)
	}
	if done.severity != errs.SeverityWarning {
		t.Fatalf("severity = %v, want Warning", done.severity)
	}
	// snapshot must NOT refresh so dirty stays true.
	if !s.dirty() {
		t.Fatalf("dirty() = false after refused save, want true")
	}
}

func TestSettingsScreen_DirtyBackOpensConfirmModal(t *testing.T) {
	fake := &fakeSettingsActions{Loaded: settings.Settings{Version: 1}}
	s := newSettingsScreen(fake)
	loadFakeSettings(t, s)
	s.form.gitEnabled = true

	_ = s.onBack()
	if s.modal == nil {
		t.Fatalf("expected confirm modal to be open")
	}
	if s.modalKind != modalKindSettingsBackUnsaved {
		t.Fatalf("modalKind = %v, want back-unsaved", s.modalKind)
	}
}

func TestSettingsScreen_CleanBackPopsImmediately(t *testing.T) {
	fake := &fakeSettingsActions{Loaded: settings.Settings{Version: 1}}
	s := newSettingsScreen(fake)
	loadFakeSettings(t, s)

	cmd := s.onBack()
	if cmd == nil {
		t.Fatalf("onBack produced nil cmd on clean state")
	}
	if _, ok := cmd().(PopScreenMsg); !ok {
		t.Fatalf("cmd = %T, want PopScreenMsg", cmd())
	}
	if s.modal != nil {
		t.Fatalf("did not expect modal on clean back")
	}
}

func TestSettingsScreen_StatusKeysExposeSaveAndBack(t *testing.T) {
	fake := &fakeSettingsActions{Loaded: settings.Settings{Version: 1}}
	s := newSettingsScreen(fake)
	keys := s.StatusKeys()
	if len(keys) != 2 {
		t.Fatalf("StatusKeys length = %d, want 2", len(keys))
	}
	got := []string{keys[0].Help().Key, keys[1].Help().Key}
	if !reflect.DeepEqual(got, []string{"e", "b"}) {
		t.Fatalf("keys = %v, want [e b]", got)
	}
}

func TestSettingsScreen_TitleAndBodyRenderToggle(t *testing.T) {
	fake := &fakeSettingsActions{Loaded: settings.Settings{Version: 1}}
	s := newSettingsScreen(fake)
	loadFakeSettings(t, s)
	if got := s.Title(); got != "Settings" {
		t.Errorf("Title() = %q, want Settings", got)
	}
	body := s.Body(80)
	for _, want := range []string{"Git integration", "Enabled", "Disabled"} {
		if !strings.Contains(body, want) {
			t.Errorf("Body missing %q\n%s", want, body)
		}
	}
}

func TestSettingsScreen_ResolvedModalDiscardsPops(t *testing.T) {
	fake := &fakeSettingsActions{Loaded: settings.Settings{Version: 1}}
	s := newSettingsScreen(fake)
	loadFakeSettings(t, s)
	s.form.gitEnabled = true

	_ = s.onBack()
	if s.modal == nil {
		t.Fatal("expected modal after dirty back")
	}
	cmd := s.handleResolved(modal.ResolvedMsg{Confirmed: true})
	if cmd == nil {
		t.Fatal("resolved with confirm produced nil cmd")
	}
	if _, ok := cmd().(PopScreenMsg); !ok {
		t.Fatalf("cmd = %T, want PopScreenMsg", cmd())
	}
	if s.modal != nil {
		t.Fatalf("modal should clear after resolution")
	}
}

func TestSettingsScreen_ResolvedModalCancelIsNoop(t *testing.T) {
	fake := &fakeSettingsActions{Loaded: settings.Settings{Version: 1}}
	s := newSettingsScreen(fake)
	loadFakeSettings(t, s)
	s.form.gitEnabled = true

	_ = s.onBack()
	cmd := s.handleResolved(modal.ResolvedMsg{Confirmed: false})
	if cmd != nil {
		t.Fatalf("resolved with cancel = %v, want nil", cmd)
	}
	// Modal cleared; user stays on Settings with unsaved changes.
	_ = tea.KeyPressMsg{}
}
