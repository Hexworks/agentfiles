package shell

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/hexworks/agentfiles/internal/actions"
	"github.com/hexworks/agentfiles/internal/app"
	"github.com/hexworks/agentfiles/internal/asset"
	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/profile"
	"github.com/hexworks/agentfiles/internal/project"
	"github.com/hexworks/agentfiles/internal/tui/components/mnemonic"
	"github.com/hexworks/agentfiles/internal/tui/components/modal"
	"github.com/hexworks/agentfiles/internal/tui/components/treetable"
	"github.com/hexworks/agentfiles/internal/tui/notifications"
)

// Compile-time guard.
var _ Screen = (*planProjectScreen)(nil)

// fakePlanActions records every action invocation and returns the
// configured profile/project/preview. planErr exercises the load-error
// path; syncResult lets tests assert that the screen does not assume
// PlanProject and SyncProject return the same Preview shape.
type fakePlanActions struct {
	prof       *profile.Profile
	proj       *project.Manifest
	preview    *app.Preview
	syncResult *app.Preview
	planErr    errs.DomainError
	syncInputs []actions.SyncProjectInput

	createInputs []actions.CreateAssetFromFolderInput
	createID     string
	createErr    errs.DomainError
}

func (f *fakePlanActions) LoadProfile(in actions.LoadProfileInput) (*profile.Profile, errs.DomainError) {
	return f.prof, nil
}

func (f *fakePlanActions) LoadProject(in actions.LoadProjectInput) (*project.Manifest, errs.DomainError) {
	return f.proj, nil
}

func (f *fakePlanActions) PlanProject(in actions.PlanProjectInput) (*app.Preview, errs.DomainError) {
	if f.planErr != nil {
		return nil, f.planErr
	}
	return f.preview, nil
}

func (f *fakePlanActions) SyncProject(in actions.SyncProjectInput) (*app.Preview, errs.DomainError) {
	f.syncInputs = append(f.syncInputs, in)
	return f.syncResult, nil
}

func (f *fakePlanActions) CreateAssetFromFolder(in actions.CreateAssetFromFolderInput) (string, errs.DomainError) {
	f.createInputs = append(f.createInputs, in)
	if f.createErr != nil {
		return "", f.createErr
	}
	return f.createID, nil
}

func newPlanActionsFake(projName string, changes []app.FileChange) *fakePlanActions {
	return &fakePlanActions{
		prof:       &profile.Profile{Manifest: profile.Manifest{ID: "alpha", Name: "Alpha"}},
		proj:       &project.Manifest{ID: "proj-1", Name: projName},
		preview:    &app.Preview{ProfileID: "alpha", ProjectID: "proj-1", Changes: changes},
		syncResult: &app.Preview{ProfileID: "alpha", ProjectID: "proj-1"},
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
	changes := []app.FileChange{{Path: "foo.md", Kind: app.ChangeCreate}}
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

func TestPlanProjectScreen_BodyPreLoadShowsLoading(t *testing.T) {
	f := newPlanActionsFake("Proj", nil)
	s := newPlanProjectScreen(f, "alpha", "proj-1")
	if !strings.Contains(s.Body(120), "Loading") {
		t.Errorf("Body() pre-load = %q, want to contain Loading", s.Body(120))
	}
}

func TestPlanProjectScreen_BodyLoadedContainsProjectAndButtons(t *testing.T) {
	changes := []app.FileChange{{Path: "foo.md", Kind: app.ChangeCreate}}
	f := newPlanActionsFake("Proj", changes)
	s := newPlanProjectScreen(f, "alpha", "proj-1")
	planLoadInto(t, s, f)

	body := s.Body(120)
	// Button labels render with ANSI escape codes around the mnemonic
	// letter AND around the closing bracket (SGR-aware chunking, see
	// mnemonic.Button.View), so the literal label "Apply" never appears
	// contiguously and "pply]" is also split. Match the post-mnemonic
	// text run instead — stable under styling churn.
	for _, want := range []string{"Proj", "pply", "ack"} {
		if !strings.Contains(body, want) {
			t.Errorf("Body() loaded missing %q", want)
		}
	}
}

func TestPlanProjectScreen_StatusValueForEachKind(t *testing.T) {
	f := newPlanActionsFake("Proj", nil)
	s := newPlanProjectScreen(f, "alpha", "proj-1")
	cases := []struct {
		kind app.ChangeKind
		want string
	}{
		{app.ChangeCreate, "+ add"},
		{app.ChangeUpdate, "~ update"},
		{app.ChangeDelete, "- delete"},
		{app.ChangeDrift, "* drift"},
		{app.ChangeUnknown, "? unknown"},
	}
	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			n := &treetable.Node{Data: planNode{kind: planNodeFile, path: "p", change: app.FileChange{Kind: tc.kind}}}
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
		kind app.ChangeKind
		want string
	}{
		{app.ChangeCreate, "-"},
		{app.ChangeUpdate, "-"},
		{app.ChangeDelete, "-"},
		{app.ChangeDrift, "Keep"},
		{app.ChangeUnknown, "Keep"},
	}
	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			n := &treetable.Node{Data: planNode{kind: planNodeFile, path: "p", change: app.FileChange{Path: "p", Kind: tc.kind}}}
			if got := s.actionValue(n); got != tc.want {
				t.Errorf("actionValue(%s) = %q, want %q", tc.kind, got, tc.want)
			}
		})
	}
}

// TestPlanProjectScreen_ActionValueReflectsResolutionMap exercises the
// user-visible transition (press the toggle button) rather than reaching
// into the resolution maps, so a future refactor of the storage shape
// stays compatible as long as the public toggle behavior is preserved.
func TestPlanProjectScreen_ActionValueReflectsResolutionMap(t *testing.T) {
	f := newPlanActionsFake("Proj", nil)
	s := newPlanProjectScreen(f, "alpha", "proj-1")

	_ = s.driftToggleBtn("drift.md").Trigger()
	_ = s.unknownToggleBtn("unknown.md").Trigger()

	drift := &treetable.Node{Data: planNode{kind: planNodeFile, path: "drift.md", change: app.FileChange{Path: "drift.md", Kind: app.ChangeDrift}}}
	unknown := &treetable.Node{Data: planNode{kind: planNodeFile, path: "unknown.md", change: app.FileChange{Path: "unknown.md", Kind: app.ChangeUnknown}}}

	if got := s.actionValue(drift); got != "Overwrite" {
		t.Errorf("drift Overwrite = %q, want Overwrite", got)
	}
	if got := s.actionValue(unknown); got != "Delete" {
		t.Errorf("unknown Delete = %q, want Delete", got)
	}
}

func TestPlanProjectScreen_TreeActionsFnDriftKeepRendersOpenAndOverwriteBtn(t *testing.T) {
	f := newPlanActionsFake("Proj", nil)
	s := newPlanProjectScreen(f, "alpha", "proj-1")
	fn := s.treeActionsFn()
	n := &treetable.Node{Data: planNode{kind: planNodeFile, path: "p", change: app.FileChange{Path: "p", Kind: app.ChangeDrift}}}
	got := fn(n)
	if len(got) != 2 {
		t.Fatalf("got %d buttons, want 2", len(got))
	}
	assertBtn(t, got[0], "Open", 'o')
	assertBtn(t, got[1], "Overwrite", 'w')
}

func TestPlanProjectScreen_TreeActionsFnDriftOverwriteRendersOpenAndKeepBtn(t *testing.T) {
	f := newPlanActionsFake("Proj", nil)
	s := newPlanProjectScreen(f, "alpha", "proj-1")
	_ = s.driftToggleBtn("p").Trigger()

	fn := s.treeActionsFn()
	n := &treetable.Node{Data: planNode{kind: planNodeFile, path: "p", change: app.FileChange{Path: "p", Kind: app.ChangeDrift}}}
	got := fn(n)
	if len(got) != 2 {
		t.Fatalf("got %d buttons, want 2", len(got))
	}
	assertBtn(t, got[0], "Open", 'o')
	assertBtn(t, got[1], "Keep", 'p')
}

func TestPlanProjectScreen_TreeActionsFnUnknownKeepRendersOpenAndDeleteBtn(t *testing.T) {
	f := newPlanActionsFake("Proj", nil)
	s := newPlanProjectScreen(f, "alpha", "proj-1")
	fn := s.treeActionsFn()
	n := &treetable.Node{Data: planNode{kind: planNodeFile, path: "p", change: app.FileChange{Path: "p", Kind: app.ChangeUnknown}}}
	got := fn(n)
	if len(got) != 2 {
		t.Fatalf("got %d buttons, want 2", len(got))
	}
	assertBtn(t, got[0], "Open", 'o')
	assertBtn(t, got[1], "Delete", 'd')
}

func TestPlanProjectScreen_TreeActionsFnUnknownDeleteRendersOpenAndKeepBtn(t *testing.T) {
	f := newPlanActionsFake("Proj", nil)
	s := newPlanProjectScreen(f, "alpha", "proj-1")
	_ = s.unknownToggleBtn("p").Trigger()

	fn := s.treeActionsFn()
	n := &treetable.Node{Data: planNode{kind: planNodeFile, path: "p", change: app.FileChange{Path: "p", Kind: app.ChangeUnknown}}}
	got := fn(n)
	if len(got) != 2 {
		t.Fatalf("got %d buttons, want 2", len(got))
	}
	assertBtn(t, got[0], "Open", 'o')
	assertBtn(t, got[1], "Keep", 'p')
}

func TestPlanProjectScreen_TreeActionsFnFileRowsAlwaysGetOpen(t *testing.T) {
	f := newPlanActionsFake("Proj", nil)
	s := newPlanProjectScreen(f, "alpha", "proj-1")
	fn := s.treeActionsFn()
	for _, kind := range []app.ChangeKind{app.ChangeCreate, app.ChangeUpdate, app.ChangeDelete} {
		t.Run(string(kind), func(t *testing.T) {
			n := &treetable.Node{Data: planNode{kind: planNodeFile, path: "p", change: app.FileChange{Path: "p", Kind: kind}}}
			got := fn(n)
			if len(got) != 1 {
				t.Fatalf("fn(%s) returned %d buttons, want 1", kind, len(got))
			}
			assertBtn(t, got[0], "Open", 'o')
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

func fileNode(p string, kind app.ChangeKind) *treetable.Node {
	return &treetable.Node{Data: planNode{kind: planNodeFile, path: p, change: app.FileChange{Path: p, Kind: kind}}}
}

func dirNode(p string, children ...*treetable.Node) *treetable.Node {
	return &treetable.Node{Data: planNode{kind: planNodeDir, path: p}, Children: children}
}

func TestPlanProjectScreen_TreeActionsFnRegisterableDirGetsRegisterBtn(t *testing.T) {
	f := newPlanActionsFake("Proj", nil)
	s := newPlanProjectScreen(f, "alpha", "proj-1")
	s.registerableDirs = map[string]bool{"sub": true}
	fn := s.treeActionsFn()
	got := fn(dirNode("sub", fileNode("sub/a.md", app.ChangeUnknown)))
	if len(got) != 1 {
		t.Fatalf("got %d buttons, want 1", len(got))
	}
	assertBtn(t, got[0], "Register", 'r')
}

func TestPlanProjectScreen_TreeActionsFnNonRegisterableDirGetsNoBtn(t *testing.T) {
	f := newPlanActionsFake("Proj", nil)
	s := newPlanProjectScreen(f, "alpha", "proj-1")
	s.registerableDirs = map[string]bool{"other": true}
	fn := s.treeActionsFn()
	if got := fn(dirNode("sub", fileNode("sub/a.md", app.ChangeUnknown))); got != nil {
		t.Errorf("fn(non-registerable dir) = %v, want nil", got)
	}
}

func TestPlanProjectScreen_AfterRegisterAssetForwardsInputAndReloads(t *testing.T) {
	f := newPlanActionsFake("Proj", nil)
	f.createID = "something"
	s := newPlanProjectScreen(f, "alpha", "proj-1")

	cmd := s.afterRegisterAsset(
		modal.ResolvedMsg{Confirmed: true, Value: asset.Manifest{Name: "Something", Type: asset.TypeSkill}},
		"sub/something",
	)
	if cmd == nil {
		t.Fatal("afterRegisterAsset returned nil cmd")
	}
	msg, ok := cmd().(registerAssetDoneMsg)
	if !ok {
		t.Fatalf("cmd produced %T, want registerAssetDoneMsg", cmd())
	}
	if msg.err != nil {
		t.Fatalf("unexpected err: %v", msg.err)
	}
	if len(f.createInputs) != 1 {
		t.Fatalf("CreateAssetFromFolder called %d times, want 1", len(f.createInputs))
	}
	in := f.createInputs[0]
	if in.ProfileRef != "alpha" || in.ProjectID != "proj-1" || in.DirKey != "sub/something" || in.Manifest.Name != "Something" {
		t.Fatalf("unexpected input: %+v", in)
	}

	if _, reload := s.handleRegisterAssetDone(msg); reload == nil {
		t.Error("handleRegisterAssetDone success returned nil cmd, want reload batch")
	}
}

func TestPlanProjectScreen_AfterRegisterAssetCancelDoesNothing(t *testing.T) {
	f := newPlanActionsFake("Proj", nil)
	s := newPlanProjectScreen(f, "alpha", "proj-1")
	if cmd := s.afterRegisterAsset(modal.ResolvedMsg{Confirmed: false}, "sub/something"); cmd != nil {
		t.Error("cancelled register returned non-nil cmd")
	}
	if len(f.createInputs) != 0 {
		t.Error("cancelled register still invoked the action")
	}
}

func TestPlanProjectScreen_ToggleDriftSwapsState(t *testing.T) {
	changes := []app.FileChange{{Path: "p", Kind: app.ChangeDrift}}
	f := newPlanActionsFake("Proj", changes)
	s := newPlanProjectScreen(f, "alpha", "proj-1")
	planLoadInto(t, s, f)

	fn := s.treeActionsFn()
	n := &treetable.Node{Data: planNode{kind: planNodeFile, path: "p", change: changes[0]}}

	// Index 0 is the always-present [Open] button; index 1 is the drift
	// resolution toggle which flips between Overwrite and Keep.
	btn := fn(n)[1]
	_ = btn.Trigger()
	if s.driftResolutions["p"] != app.DriftOverwrite {
		t.Fatalf("after first toggle: state = %v, want DriftOverwrite", s.driftResolutions["p"])
	}

	btn = fn(n)[1]
	_ = btn.Trigger()
	if _, present := s.driftResolutions["p"]; present {
		t.Fatalf("after second toggle: state still present (%v), want absent", s.driftResolutions["p"])
	}
}

func TestPlanProjectScreen_ToggleUnknownSwapsState(t *testing.T) {
	changes := []app.FileChange{{Path: "p", Kind: app.ChangeUnknown}}
	f := newPlanActionsFake("Proj", changes)
	s := newPlanProjectScreen(f, "alpha", "proj-1")
	planLoadInto(t, s, f)

	fn := s.treeActionsFn()
	n := &treetable.Node{Data: planNode{kind: planNodeFile, path: "p", change: changes[0]}}

	// Index 0 is the always-present [Open] button; index 1 is the
	// unknown resolution toggle which flips between Delete and Keep.
	btn := fn(n)[1]
	_ = btn.Trigger()
	if s.unknownResolutions["p"] != app.UnknownDelete {
		t.Fatalf("after first toggle: state = %v, want UnknownDelete", s.unknownResolutions["p"])
	}

	btn = fn(n)[1]
	_ = btn.Trigger()
	if _, present := s.unknownResolutions["p"]; present {
		t.Fatalf("after second toggle: state still present (%v), want absent", s.unknownResolutions["p"])
	}
}

func TestPlanProjectScreen_OnApplyEmptyPreviewDoesNotCallSyncProject(t *testing.T) {
	f := newPlanActionsFake("Proj", nil)
	s := newPlanProjectScreen(f, "alpha", "proj-1")
	planLoadInto(t, s, f)

	if cmd := s.onApply(); cmd != nil {
		t.Fatalf("onApply with empty preview returned non-nil cmd")
	}
	if len(f.syncInputs) != 0 {
		t.Errorf("syncInputs len = %d, want 0", len(f.syncInputs))
	}
}

// TestPlanProjectScreen_OnApplyEmptyMapEmitsExplicitKeepResolutions pins
// the rule that drift/unknown rows always produce an explicit decision
// in the sync input. Without it, a future change to the domain's default
// (DriftKeep / UnknownKeep) would silently change the TUI's behavior.
func TestPlanProjectScreen_OnApplyEmptyMapEmitsExplicitKeepResolutions(t *testing.T) {
	changes := []app.FileChange{
		{Path: "create.md", Kind: app.ChangeCreate},
		{Path: "drift.md", Kind: app.ChangeDrift},
		{Path: "unknown.md", Kind: app.ChangeUnknown},
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
	if len(in.Drift) != 1 || in.Drift[0].Path != "drift.md" || in.Drift[0].Decision != app.DriftKeep {
		t.Errorf("Drift = %+v, want [{drift.md keep}]", in.Drift)
	}
	if len(in.Unknown) != 1 || in.Unknown[0].Path != "unknown.md" || in.Unknown[0].Decision != app.UnknownKeep {
		t.Errorf("Unknown = %+v, want [{unknown.md keep}]", in.Unknown)
	}
	if in.ProfileRef != "alpha" || in.ProjectID != "proj-1" {
		t.Errorf("input ids = (%q, %q), want (alpha, proj-1)", in.ProfileRef, in.ProjectID)
	}
}

func TestPlanProjectScreen_OnApplyWithSelectionsBuildsCorrectSlices(t *testing.T) {
	changes := []app.FileChange{
		{Path: "a/drift.md", Kind: app.ChangeDrift},
		{Path: "b/unknown.md", Kind: app.ChangeUnknown},
		{Path: "c/create.md", Kind: app.ChangeCreate},
	}
	f := newPlanActionsFake("Proj", changes)
	s := newPlanProjectScreen(f, "alpha", "proj-1")
	planLoadInto(t, s, f)
	s.driftResolutions["a/drift.md"] = app.DriftOverwrite
	s.unknownResolutions["b/unknown.md"] = app.UnknownDelete

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
// states is {o, k, d, a, b}. An outer assertion verifies the walk
// actually reached a drift/unknown row (otherwise the inner uniqueness
// would be trivial — only [Apply] and [Back] registered).
func TestPlanProjectScreen_MnemonicUniquenessExhaustive(t *testing.T) {
	changes := []app.FileChange{
		{Path: "a/add.md", Kind: app.ChangeCreate},
		{Path: "b/upd.md", Kind: app.ChangeUpdate},
		{Path: "c/del.md", Kind: app.ChangeDelete},
		{Path: "d/drift.md", Kind: app.ChangeDrift},
		{Path: "e/unknown.md", Kind: app.ChangeUnknown},
	}
	type override struct {
		drift   map[string]app.DriftDecision
		unknown map[string]app.UnknownDecision
	}
	overrides := []override{
		{},
		{drift: map[string]app.DriftDecision{"d/drift.md": app.DriftOverwrite}},
		{unknown: map[string]app.UnknownDecision{"e/unknown.md": app.UnknownDelete}},
		{
			drift:   map[string]app.DriftDecision{"d/drift.md": app.DriftOverwrite},
			unknown: map[string]app.UnknownDecision{"e/unknown.md": app.UnknownDelete},
		},
	}
	sawToggleLabel := false
	for oi, ov := range overrides {
		f := newPlanActionsFake("Proj", changes)
		s := newPlanProjectScreen(f, "alpha", "proj-1")
		planLoadInto(t, s, f)
		for k, v := range ov.drift {
			s.driftResolutions[k] = v
		}
		for k, v := range ov.unknown {
			s.unknownResolutions[k] = v
		}
		s.tree.SetRoot(buildPlanTree(s.projectName, s.preview.Changes))
		rowMax := len(changes) + 5
		for row := 0; row < rowMax; row++ {
			if row > 0 {
				_, _ = s.tree.Update(tea.KeyPressMsg{Code: tea.KeyDown})
			}
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("rebuildSet panic at override=%d row=%d: %v", oi, row, r)
					}
				}()
				s.rebuildSet()
			}()
			assertUniquePlanMnemonics(t, s.set, oi, row)
			for _, b := range s.set.Buttons() {
				switch b.Label() {
				case "Overwrite", "Keep", "Delete":
					sawToggleLabel = true
				}
			}
		}
	}
	if !sawToggleLabel {
		t.Error("mnemonic walk never landed on a drift/unknown row — cursor stuck on header/non-toggle rows")
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
