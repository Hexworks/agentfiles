package shell

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/hexworks/agentfiles/internal/actions"
	"github.com/hexworks/agentfiles/internal/app"
	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/profile"
	"github.com/hexworks/agentfiles/internal/project"
	llmsync "github.com/hexworks/agentfiles/internal/sync"
	"github.com/hexworks/agentfiles/internal/tui/components/mnemonic"
	"github.com/hexworks/agentfiles/internal/tui/components/treetable"
	"github.com/hexworks/agentfiles/internal/tui/notifications"
)

// Compile-time guard.
var _ Screen = (*planProjectScreen)(nil)

// fakePlanActions records every action invocation and returns the
// configured profile/project/preview or the configured error.
type fakePlanActions struct {
	prof        *profile.Profile
	proj        *project.Manifest
	preview     *llmsync.Preview
	loadProfErr errs.DomainError
	loadProjErr errs.DomainError
	planErr     errs.DomainError
	syncErr     errs.DomainError
	syncInputs  []actions.SyncProjectInput
}

func (f *fakePlanActions) LoadProfile(in actions.LoadProfileInput) (*profile.Profile, errs.DomainError) {
	if f.loadProfErr != nil {
		return nil, f.loadProfErr
	}
	return f.prof, nil
}

func (f *fakePlanActions) LoadProject(in actions.LoadProjectInput) (*project.Manifest, errs.DomainError) {
	if f.loadProjErr != nil {
		return nil, f.loadProjErr
	}
	return f.proj, nil
}

func (f *fakePlanActions) PlanProject(in actions.PlanProjectInput) (*llmsync.Preview, errs.DomainError) {
	if f.planErr != nil {
		return nil, f.planErr
	}
	return f.preview, nil
}

func (f *fakePlanActions) SyncProject(in actions.SyncProjectInput) (*llmsync.Preview, errs.DomainError) {
	f.syncInputs = append(f.syncInputs, in)
	if f.syncErr != nil {
		return nil, f.syncErr
	}
	return f.preview, nil
}

func newPlanActionsFake(projName string, changes []llmsync.FileChange) *fakePlanActions {
	return &fakePlanActions{
		prof:    &profile.Profile{Manifest: profile.Manifest{ID: "alpha", Name: "Alpha"}},
		proj:    &project.Manifest{ID: "proj-1", Name: projName},
		preview: &llmsync.Preview{ProfileID: "alpha", ProjectID: "proj-1", Changes: changes},
	}
}

func planLoadInto(t *testing.T, s *planProjectScreen, f *fakePlanActions) {
	t.Helper()
	_, _ = s.Update(planProjectLoadedMsg{prof: f.prof, proj: f.proj, preview: f.preview})
}

func TestPlanProjectScreen_PanicsOnInvalidConstruction(t *testing.T) {
	f := newPlanActionsFake("Proj", nil)
	cases := []struct {
		name string
		fn   func()
	}{
		{"nil actions", func() { _ = newPlanProjectScreen(nil, "alpha", "proj-1") }},
		{"empty profile id", func() { _ = newPlanProjectScreen(f, "", "proj-1") }},
		{"empty project id", func() { _ = newPlanProjectScreen(f, "alpha", "") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Errorf("expected panic, got none")
				}
			}()
			tc.fn()
		})
	}
}

func TestPlanProjectScreen_Title(t *testing.T) {
	f := newPlanActionsFake("Proj", nil)
	s := newPlanProjectScreen(f, "alpha", "proj-1")
	if got := s.Title(); got != "Plan Project" {
		t.Errorf("Title() = %q, want %q", got, "Plan Project")
	}
}

func TestPlanProjectScreen_InitDispatchesLoad(t *testing.T) {
	changes := []llmsync.FileChange{{Path: "foo.md", Kind: llmsync.ChangeCreate}}
	f := newPlanActionsFake("Proj", changes)
	s := newPlanProjectScreen(f, "alpha", "proj-1")

	msg := s.Init()()
	loaded, ok := msg.(planProjectLoadedMsg)
	if !ok {
		t.Fatalf("Init produced %T, want planProjectLoadedMsg", msg)
	}
	if loaded.err != nil {
		t.Fatalf("loaded err = %v, want nil", loaded.err)
	}
	if loaded.prof == nil || loaded.proj == nil || loaded.preview == nil {
		t.Fatalf("loaded fields missing: %+v", loaded)
	}
	if loaded.preview.Changes[0].Path != "foo.md" {
		t.Errorf("loaded preview = %+v, want path foo.md", loaded.preview.Changes)
	}
}

func TestPlanProjectScreen_HandleLoadedErrorEmitsNotification(t *testing.T) {
	f := newPlanActionsFake("Proj", nil)
	f.planErr = stubDomainErr{msg: "boom", sev: errs.SeverityError}
	s := newPlanProjectScreen(f, "alpha", "proj-1")

	msg := s.Init()()
	loaded := msg.(planProjectLoadedMsg)
	if loaded.err == nil {
		t.Fatalf("loaded err = nil, want boom")
	}
	_, cmd := s.Update(loaded)
	note, ok := drainCmd(t, cmd).(notifications.NotificationMsg)
	if !ok {
		t.Fatalf("cmd produced %T, want NotificationMsg", cmd())
	}
	if note.Notification.Text != "boom" {
		t.Errorf("notification text = %q, want %q", note.Notification.Text, "boom")
	}
}

func TestPlanProjectScreen_StatusValueForEachKind(t *testing.T) {
	f := newPlanActionsFake("Proj", nil)
	s := newPlanProjectScreen(f, "alpha", "proj-1")
	cases := []struct {
		kind llmsync.ChangeKind
		want string
	}{
		{llmsync.ChangeCreate, "+ add"},
		{llmsync.ChangeUpdate, "~ update"},
		{llmsync.ChangeDelete, "- delete"},
		{llmsync.ChangeDrift, "* drift"},
		{llmsync.ChangeUnknown, "? unknown"},
	}
	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			n := &treetable.Node{Data: planNode{kind: planNodeFile, path: "p", change: llmsync.FileChange{Kind: tc.kind}}}
			if got := s.statusValue(n); got != tc.want {
				t.Errorf("statusValue(%s) = %q, want %q", tc.kind, got, tc.want)
			}
		})
	}
	for _, kind := range []planNodeKind{planNodeRoot, planNodeDir} {
		n := &treetable.Node{Data: planNode{kind: kind}}
		if got := s.statusValue(n); got != "" {
			t.Errorf("statusValue(kind=%d) = %q, want empty", kind, got)
		}
	}
}

func TestPlanProjectScreen_ActionValueDefaultsToKeepForDriftAndUnknown(t *testing.T) {
	f := newPlanActionsFake("Proj", nil)
	s := newPlanProjectScreen(f, "alpha", "proj-1")
	cases := []struct {
		kind llmsync.ChangeKind
		want string
	}{
		{llmsync.ChangeCreate, "-"},
		{llmsync.ChangeUpdate, "-"},
		{llmsync.ChangeDelete, "-"},
		{llmsync.ChangeDrift, "Keep"},
		{llmsync.ChangeUnknown, "Keep"},
	}
	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			n := &treetable.Node{Data: planNode{kind: planNodeFile, path: "p", change: llmsync.FileChange{Path: "p", Kind: tc.kind}}}
			if got := s.actionValue(n); got != tc.want {
				t.Errorf("actionValue(%s) = %q, want %q", tc.kind, got, tc.want)
			}
		})
	}
}

func TestPlanProjectScreen_ActionValueReflectsResolutionMap(t *testing.T) {
	f := newPlanActionsFake("Proj", nil)
	s := newPlanProjectScreen(f, "alpha", "proj-1")
	s.resolutions["drift.md"] = planOverwrite
	s.resolutions["unknown.md"] = planDelete

	drift := &treetable.Node{Data: planNode{kind: planNodeFile, path: "drift.md", change: llmsync.FileChange{Path: "drift.md", Kind: llmsync.ChangeDrift}}}
	unknown := &treetable.Node{Data: planNode{kind: planNodeFile, path: "unknown.md", change: llmsync.FileChange{Path: "unknown.md", Kind: llmsync.ChangeUnknown}}}

	if got := s.actionValue(drift); got != "Overwrite" {
		t.Errorf("drift Overwrite = %q, want Overwrite", got)
	}
	if got := s.actionValue(unknown); got != "Delete" {
		t.Errorf("unknown Delete = %q, want Delete", got)
	}
}

func TestPlanProjectScreen_TreeActionsFnDriftKeepRendersOverwriteBtn(t *testing.T) {
	f := newPlanActionsFake("Proj", nil)
	s := newPlanProjectScreen(f, "alpha", "proj-1")
	fn := s.treeActionsFn()
	n := &treetable.Node{Data: planNode{kind: planNodeFile, path: "p", change: llmsync.FileChange{Path: "p", Kind: llmsync.ChangeDrift}}}
	got := fn(n)
	if len(got) != 1 {
		t.Fatalf("got %d buttons, want 1", len(got))
	}
	assertBtn(t, got[0], "Overwrite", 'o')
}

func TestPlanProjectScreen_TreeActionsFnDriftOverwriteRendersKeepBtn(t *testing.T) {
	f := newPlanActionsFake("Proj", nil)
	s := newPlanProjectScreen(f, "alpha", "proj-1")
	s.resolutions["p"] = planOverwrite
	fn := s.treeActionsFn()
	n := &treetable.Node{Data: planNode{kind: planNodeFile, path: "p", change: llmsync.FileChange{Path: "p", Kind: llmsync.ChangeDrift}}}
	got := fn(n)
	if len(got) != 1 {
		t.Fatalf("got %d buttons, want 1", len(got))
	}
	assertBtn(t, got[0], "Keep", 'k')
}

func TestPlanProjectScreen_TreeActionsFnUnknownKeepRendersDeleteBtn(t *testing.T) {
	f := newPlanActionsFake("Proj", nil)
	s := newPlanProjectScreen(f, "alpha", "proj-1")
	fn := s.treeActionsFn()
	n := &treetable.Node{Data: planNode{kind: planNodeFile, path: "p", change: llmsync.FileChange{Path: "p", Kind: llmsync.ChangeUnknown}}}
	got := fn(n)
	if len(got) != 1 {
		t.Fatalf("got %d buttons, want 1", len(got))
	}
	assertBtn(t, got[0], "Delete", 'd')
}

func TestPlanProjectScreen_TreeActionsFnUnknownDeleteRendersKeepBtn(t *testing.T) {
	f := newPlanActionsFake("Proj", nil)
	s := newPlanProjectScreen(f, "alpha", "proj-1")
	s.resolutions["p"] = planDelete
	fn := s.treeActionsFn()
	n := &treetable.Node{Data: planNode{kind: planNodeFile, path: "p", change: llmsync.FileChange{Path: "p", Kind: llmsync.ChangeUnknown}}}
	got := fn(n)
	if len(got) != 1 {
		t.Fatalf("got %d buttons, want 1", len(got))
	}
	assertBtn(t, got[0], "Keep", 'k')
}

func TestPlanProjectScreen_TreeActionsFnNoButtonForCreateUpdateDelete(t *testing.T) {
	f := newPlanActionsFake("Proj", nil)
	s := newPlanProjectScreen(f, "alpha", "proj-1")
	fn := s.treeActionsFn()
	for _, kind := range []llmsync.ChangeKind{llmsync.ChangeCreate, llmsync.ChangeUpdate, llmsync.ChangeDelete} {
		t.Run(string(kind), func(t *testing.T) {
			n := &treetable.Node{Data: planNode{kind: planNodeFile, path: "p", change: llmsync.FileChange{Path: "p", Kind: kind}}}
			if got := fn(n); got != nil {
				t.Errorf("fn(%s) = %v, want nil", kind, got)
			}
		})
	}
	root := &treetable.Node{Data: planNode{kind: planNodeRoot}}
	if got := fn(root); got != nil {
		t.Errorf("fn(root) = %v, want nil", got)
	}
	dir := &treetable.Node{Data: planNode{kind: planNodeDir, path: "sub"}}
	if got := fn(dir); got != nil {
		t.Errorf("fn(dir) = %v, want nil", got)
	}
}

func TestPlanProjectScreen_ToggleDriftSwapsState(t *testing.T) {
	changes := []llmsync.FileChange{{Path: "p", Kind: llmsync.ChangeDrift}}
	f := newPlanActionsFake("Proj", changes)
	s := newPlanProjectScreen(f, "alpha", "proj-1")
	planLoadInto(t, s, f)

	fn := s.treeActionsFn()
	n := &treetable.Node{Data: planNode{kind: planNodeFile, path: "p", change: changes[0]}}

	btn := fn(n)[0]
	_ = btn.Trigger()
	if s.resolutions["p"] != planOverwrite {
		t.Fatalf("after first toggle: state = %v, want planOverwrite", s.resolutions["p"])
	}

	btn = fn(n)[0]
	_ = btn.Trigger()
	if _, present := s.resolutions["p"]; present {
		t.Fatalf("after second toggle: state still present (%v), want absent", s.resolutions["p"])
	}
}

func TestPlanProjectScreen_ToggleUnknownSwapsState(t *testing.T) {
	changes := []llmsync.FileChange{{Path: "p", Kind: llmsync.ChangeUnknown}}
	f := newPlanActionsFake("Proj", changes)
	s := newPlanProjectScreen(f, "alpha", "proj-1")
	planLoadInto(t, s, f)

	fn := s.treeActionsFn()
	n := &treetable.Node{Data: planNode{kind: planNodeFile, path: "p", change: changes[0]}}

	btn := fn(n)[0]
	_ = btn.Trigger()
	if s.resolutions["p"] != planDelete {
		t.Fatalf("after first toggle: state = %v, want planDelete", s.resolutions["p"])
	}

	btn = fn(n)[0]
	_ = btn.Trigger()
	if _, present := s.resolutions["p"]; present {
		t.Fatalf("after second toggle: state still present (%v), want absent", s.resolutions["p"])
	}
}

func TestPlanProjectScreen_OnApplyEmptyMapBuildsEmptySlices(t *testing.T) {
	changes := []llmsync.FileChange{
		{Path: "create.md", Kind: llmsync.ChangeCreate},
		{Path: "drift.md", Kind: llmsync.ChangeDrift},
		{Path: "unknown.md", Kind: llmsync.ChangeUnknown},
	}
	f := newPlanActionsFake("Proj", changes)
	s := newPlanProjectScreen(f, "alpha", "proj-1")
	planLoadInto(t, s, f)

	cmd := s.onApply()
	if cmd == nil {
		t.Fatalf("onApply returned nil cmd")
	}
	done, ok := cmd().(mutationDoneMsg)
	if !ok {
		t.Fatalf("onApply produced %T, want mutationDoneMsg", cmd())
	}
	if done.severity != errs.SeverityInfo {
		t.Errorf("severity = %v, want Info", done.severity)
	}
	if len(f.syncInputs) != 1 {
		t.Fatalf("syncInputs len = %d, want 1", len(f.syncInputs))
	}
	in := f.syncInputs[0]
	if in.Drift != nil {
		t.Errorf("Drift = %v, want nil", in.Drift)
	}
	if in.Unknown != nil {
		t.Errorf("Unknown = %v, want nil", in.Unknown)
	}
	if in.ProfileRef != "alpha" || in.ProjectID != "proj-1" {
		t.Errorf("input ids = (%q, %q), want (alpha, proj-1)", in.ProfileRef, in.ProjectID)
	}
}

func TestPlanProjectScreen_OnApplyWithSelectionsBuildsCorrectSlices(t *testing.T) {
	changes := []llmsync.FileChange{
		{Path: "a/drift.md", Kind: llmsync.ChangeDrift},
		{Path: "b/unknown.md", Kind: llmsync.ChangeUnknown},
		{Path: "c/create.md", Kind: llmsync.ChangeCreate},
	}
	f := newPlanActionsFake("Proj", changes)
	s := newPlanProjectScreen(f, "alpha", "proj-1")
	planLoadInto(t, s, f)
	s.resolutions["a/drift.md"] = planOverwrite
	s.resolutions["b/unknown.md"] = planDelete

	_ = s.onApply()()
	if len(f.syncInputs) != 1 {
		t.Fatalf("syncInputs len = %d, want 1", len(f.syncInputs))
	}
	in := f.syncInputs[0]
	if len(in.Drift) != 1 || in.Drift[0].Path != "a/drift.md" || in.Drift[0].Decision != app.DriftOverwrite {
		t.Errorf("Drift = %+v, want [{a/drift.md overwrite}]", in.Drift)
	}
	if len(in.Unknown) != 1 || in.Unknown[0].Path != "b/unknown.md" || in.Unknown[0].Decision != app.UnknownDelete {
		t.Errorf("Unknown = %+v, want [{b/unknown.md delete}]", in.Unknown)
	}
}

func TestPlanProjectScreen_OnApplySuccessEmitsNotificationAndPop(t *testing.T) {
	f := newPlanActionsFake("Proj", nil)
	s := newPlanProjectScreen(f, "alpha", "proj-1")
	planLoadInto(t, s, f)

	done := mutationDoneMsg{text: "Project synced", severity: errs.SeverityInfo}
	_, cmd := s.Update(done)
	if cmd == nil {
		t.Fatalf("cmd nil after mutationDoneMsg")
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatalf("cmd produced %T, want tea.BatchMsg", cmd())
	}
	sawNotification := false
	sawPop := false
	for _, c := range batch {
		if c == nil {
			continue
		}
		switch m := c().(type) {
		case notifications.NotificationMsg:
			sawNotification = true
			if m.Notification.Text != "Project synced" {
				t.Errorf("notification text = %q, want %q", m.Notification.Text, "Project synced")
			}
		case PopScreenMsg:
			sawPop = true
		}
	}
	if !sawNotification {
		t.Error("batch missing NotificationMsg")
	}
	if !sawPop {
		t.Error("batch missing PopScreenMsg")
	}
}

func TestPlanProjectScreen_OnApplyFailureEmitsNotificationOnly(t *testing.T) {
	f := newPlanActionsFake("Proj", nil)
	s := newPlanProjectScreen(f, "alpha", "proj-1")
	planLoadInto(t, s, f)

	done := mutationDoneMsg{text: "boom", severity: errs.SeverityError}
	_, cmd := s.Update(done)
	msg := drainCmd(t, cmd)
	note, ok := msg.(notifications.NotificationMsg)
	if !ok {
		t.Fatalf("cmd produced %T, want NotificationMsg", msg)
	}
	if note.Notification.Severity != errs.SeverityError {
		t.Errorf("severity = %v, want Error", note.Notification.Severity)
	}
	if note.Notification.Text != "boom" {
		t.Errorf("text = %q, want boom", note.Notification.Text)
	}
}

// TestPlanProjectScreen_MnemonicUniquenessExhaustive walks every cursor
// row and every state-override combination and asserts every registered
// button has a unique mnemonic rune. The candidate alphabet across all
// states is {o, k, d, a, b}.
func TestPlanProjectScreen_MnemonicUniquenessExhaustive(t *testing.T) {
	changes := []llmsync.FileChange{
		{Path: "a/add.md", Kind: llmsync.ChangeCreate},
		{Path: "b/upd.md", Kind: llmsync.ChangeUpdate},
		{Path: "c/del.md", Kind: llmsync.ChangeDelete},
		{Path: "d/drift.md", Kind: llmsync.ChangeDrift},
		{Path: "e/unknown.md", Kind: llmsync.ChangeUnknown},
	}
	overrides := []map[string]planActionState{
		{},
		{"d/drift.md": planOverwrite},
		{"e/unknown.md": planDelete},
		{"d/drift.md": planOverwrite, "e/unknown.md": planDelete},
	}
	for oi, overrideSet := range overrides {
		f := newPlanActionsFake("Proj", changes)
		s := newPlanProjectScreen(f, "alpha", "proj-1")
		planLoadInto(t, s, f)
		for k, v := range overrideSet {
			s.resolutions[k] = v
		}
		s.tree.SetRoot(buildPlanTree(s.projectName, s.preview.Changes))
		// Walk cursor across every row + a small buffer.
		rowMax := len(changes) + 5
		for row := 0; row < rowMax; row++ {
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("rebuildSet panic at override=%d row=%d: %v", oi, row, r)
					}
				}()
				s.rebuildSet()
			}()
			assertUniquePlanMnemonics(t, s.set, oi, row)
			_, _ = s.tree.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		}
	}
}

func assertUniquePlanMnemonics(t *testing.T, set *mnemonic.Set, override, row int) {
	t.Helper()
	seen := make(map[rune]string)
	for _, b := range set.Buttons() {
		r := b.Mnemonic()
		if prev, dup := seen[r]; dup {
			t.Errorf("duplicate mnemonic %q at override=%d row=%d: %q vs %q", r, override, row, prev, b.Label())
			continue
		}
		seen[r] = b.Label()
	}
}

// assertBtn checks a button's label + mnemonic. Reused across the
// per-state action button tests.
func assertBtn(t *testing.T, b *mnemonic.Button, label string, m rune) {
	t.Helper()
	if b.Label() != label {
		t.Errorf("label = %q, want %q", b.Label(), label)
	}
	if b.Mnemonic() != m {
		t.Errorf("mnemonic = %q, want %q", b.Mnemonic(), m)
	}
}
