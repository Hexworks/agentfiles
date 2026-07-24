package shell

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/hexworks/agentfiles/internal/actions"
	"github.com/hexworks/agentfiles/internal/appapi"
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
// syncOutcome + syncErr drive the SyncProject result so tests can
// exercise the git-aware toast paths.
type fakePlanActions struct {
	prof         *appapi.LoadedProfile
	proj         *project.Manifest
	preview      *appapi.Preview
	syncResult   *appapi.Preview
	syncOutcome  appapi.CommitOutcome
	adoptOutcome appapi.CommitOutcome
	syncErr      errs.DomainError
	planErr      errs.DomainError
	syncInputs   []actions.SyncProjectInput

	createInputs []actions.CreateAssetFromFolderInput
	createID     string
	createErr    errs.DomainError

	diffInputs []actions.DiffFileInput
	diffBodies appapi.DiffBodies
	diffErr    errs.DomainError
}

func (f *fakePlanActions) LoadProfile(in actions.LoadProfileInput) (*appapi.LoadedProfile, errs.DomainError) {
	return f.prof, nil
}

func (f *fakePlanActions) LoadProject(in actions.LoadProjectInput) (*project.Manifest, errs.DomainError) {
	return f.proj, nil
}

func (f *fakePlanActions) PlanProject(in actions.PlanProjectInput) (*appapi.Preview, errs.DomainError) {
	if f.planErr != nil {
		return nil, f.planErr
	}
	return f.preview, nil
}

func (f *fakePlanActions) SyncProject(in actions.SyncProjectInput) (appapi.ApplyOutcome, errs.DomainError) {
	f.syncInputs = append(f.syncInputs, in)
	adopt := f.adoptOutcome
	if adopt == nil {
		adopt = appapi.Skipped{Reason: appapi.SkipDisabled}
	}
	return appapi.ApplyOutcome{Preview: f.syncResult, Sync: f.syncOutcome, Adopt: adopt}, f.syncErr
}

func (f *fakePlanActions) CreateAssetFromFolder(in actions.CreateAssetFromFolderInput) (string, errs.DomainError) {
	f.createInputs = append(f.createInputs, in)
	if f.createErr != nil {
		return "", f.createErr
	}
	return f.createID, nil
}

func (f *fakePlanActions) DiffFile(in actions.DiffFileInput) (appapi.DiffBodies, errs.DomainError) {
	f.diffInputs = append(f.diffInputs, in)
	if f.diffErr != nil {
		return appapi.DiffBodies{}, f.diffErr
	}
	return f.diffBodies, nil
}

func newPlanActionsFake(projName string, changes []appapi.FileChange) *fakePlanActions {
	return &fakePlanActions{
		prof: &appapi.LoadedProfile{
			Profile: &profile.Profile{Manifest: profile.Manifest{ID: "alpha", Name: "Alpha"}},
		},
		proj:       &project.Manifest{ID: "proj-1", Name: projName},
		preview:    &appapi.Preview{ProfileID: "alpha", ProjectID: "proj-1", Changes: changes},
		syncResult: &appapi.Preview{ProfileID: "alpha", ProjectID: "proj-1"},
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
	changes := []appapi.FileChange{{Path: "foo.md", Kind: appapi.ChangeCreate}}
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
	changes := []appapi.FileChange{{Path: "foo.md", Kind: appapi.ChangeCreate}}
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
		kind appapi.ChangeKind
		want string
	}{
		{appapi.ChangeCreate, "+ add"},
		{appapi.ChangeUpdate, "~ update"},
		{appapi.ChangeDelete, "- delete"},
		{appapi.ChangeDrift, "* drift"},
		{appapi.ChangeUnknown, "? unknown"},
	}
	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			n := &treetable.Node{Data: planNode{kind: planNodeFile, path: "p", change: appapi.FileChange{Kind: tc.kind}}}
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
		kind appapi.ChangeKind
		want string
	}{
		{appapi.ChangeCreate, "-"},
		{appapi.ChangeUpdate, "-"},
		{appapi.ChangeDelete, "-"},
		{appapi.ChangeDrift, "Keep"},
		{appapi.ChangeUnknown, "Keep"},
	}
	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			n := &treetable.Node{Data: planNode{kind: planNodeFile, path: "p", change: appapi.FileChange{Path: "p", Kind: tc.kind}}}
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

	_ = s.toggleDrift("drift.md", appapi.DriftOverwrite)
	_ = s.toggleUnknown("unknown.md", appapi.UnknownDelete)

	drift := &treetable.Node{Data: planNode{kind: planNodeFile, path: "drift.md", change: appapi.FileChange{Path: "drift.md", Kind: appapi.ChangeDrift}}}
	unknown := &treetable.Node{Data: planNode{kind: planNodeFile, path: "unknown.md", change: appapi.FileChange{Path: "unknown.md", Kind: appapi.ChangeUnknown}}}

	if got := s.actionValue(drift); got != "Overwrite" {
		t.Errorf("drift Overwrite = %q, want Overwrite", got)
	}
	if got := s.actionValue(unknown); got != "Delete" {
		t.Errorf("unknown Delete = %q, want Delete", got)
	}
}

func TestPlanProjectScreen_TreeActionsFnFileRowsAlwaysGetOpen(t *testing.T) {
	f := newPlanActionsFake("Proj", nil)
	s := newPlanProjectScreen(f, "alpha", "proj-1")
	fn := s.treeActionsFn()
	// Create and delete rows carry only [Open]; they lack one side of a diff
	// so no [Diff] is offered (update is exercised separately below).
	for _, kind := range []appapi.ChangeKind{appapi.ChangeCreate, appapi.ChangeDelete} {
		t.Run(string(kind), func(t *testing.T) {
			n := &treetable.Node{Data: planNode{kind: planNodeFile, path: "p", change: appapi.FileChange{Path: "p", Kind: kind}}}
			got := fn(n)
			if len(got) != 1 {
				t.Fatalf("fn(%s) returned %d buttons, want 1", kind, len(got))
			}
			assertBtn(t, got[0], "Open", 'o')
		})
	}
	t.Run("update also gets Diff", func(t *testing.T) {
		n := &treetable.Node{Data: planNode{kind: planNodeFile, path: "p", change: appapi.FileChange{Path: "p", Kind: appapi.ChangeUpdate}}}
		got := fn(n)
		if len(got) != 2 {
			t.Fatalf("fn(update) returned %d buttons, want 2 ([Open]+[Diff])", len(got))
		}
		assertBtn(t, got[0], "Open", 'o')
		assertBtn(t, got[1], "Diff", 'd')
	})
	root := &treetable.Node{Data: planNode{kind: planNodeRoot}}
	if got := fn(root); got != nil {
		t.Errorf("fn(root) = %v, want nil", got)
	}
	dir := &treetable.Node{Data: planNode{kind: planNodeDir, path: "sub"}}
	if got := fn(dir); got != nil {
		t.Errorf("fn(dir) = %v, want nil", got)
	}
}

func fileNode(p string, kind appapi.ChangeKind) *treetable.Node {
	return &treetable.Node{Data: planNode{kind: planNodeFile, path: p, change: appapi.FileChange{Path: p, Kind: kind}}}
}

func dirNode(p string, children ...*treetable.Node) *treetable.Node {
	return &treetable.Node{Data: planNode{kind: planNodeDir, path: p}, Children: children}
}

func TestPlanProjectScreen_TreeActionsFnRegisterableDirGetsRegisterAndIgnoreBtns(t *testing.T) {
	f := newPlanActionsFake("Proj", nil)
	s := newPlanProjectScreen(f, "alpha", "proj-1")
	s.registerableDirs = map[string]bool{"sub": true}
	fn := s.treeActionsFn()
	got := fn(dirNode("sub", fileNode("sub/a.md", appapi.ChangeUnknown)))
	if len(got) != 2 {
		t.Fatalf("got %d buttons, want 2", len(got))
	}
	assertBtn(t, got[0], "Register", 'r')
	assertBtn(t, got[1], "Ignore", 'i')
}

func TestPlanProjectScreen_TreeActionsFnIgnoredDirGetsShowBtnOnly(t *testing.T) {
	f := newPlanActionsFake("Proj", nil)
	s := newPlanProjectScreen(f, "alpha", "proj-1")
	s.registerableDirs = map[string]bool{"sub": true}
	s.ignoredPaths = map[string]bool{"sub": true}
	fn := s.treeActionsFn()
	got := fn(dirNode("sub", fileNode("sub/a.md", appapi.ChangeUnknown)))
	if len(got) != 1 {
		t.Fatalf("got %d buttons, want 1", len(got))
	}
	assertBtn(t, got[0], "Show", 'w')
}

func TestPlanProjectScreen_TreeActionsFnNonRegisterableDirGetsNoBtn(t *testing.T) {
	f := newPlanActionsFake("Proj", nil)
	s := newPlanProjectScreen(f, "alpha", "proj-1")
	s.registerableDirs = map[string]bool{"other": true}
	fn := s.treeActionsFn()
	if got := fn(dirNode("sub", fileNode("sub/a.md", appapi.ChangeUnknown))); got != nil {
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

// TestPlanProjectScreen_ApplyEmitsAdoptResolutions pins that onApply
// forwards DriftAdopt and UnknownAdopt selections to the SyncProject
// action as-is, mirroring the existing Overwrite/Delete emission.
func TestPlanProjectScreen_ApplyEmitsAdoptResolutions(t *testing.T) {
	changes := []appapi.FileChange{
		{Path: "d/drift.md", Kind: appapi.ChangeDrift},
		{Path: "u/unknown.md", Kind: appapi.ChangeUnknown, OwningAssetID: "foo"},
	}
	f := newPlanActionsFake("Proj", changes)
	s := newPlanProjectScreen(f, "alpha", "proj-1")
	planLoadInto(t, s, f)
	s.driftResolutions["d/drift.md"] = appapi.DriftAdopt
	s.unknownResolutions["u/unknown.md"] = appapi.UnknownAdopt

	_ = s.onApply()()
	if len(f.syncInputs) != 1 {
		t.Fatalf("syncInputs len = %d, want 1", len(f.syncInputs))
	}
	in := f.syncInputs[0]
	if len(in.Drift) != 1 || in.Drift[0].Decision != appapi.DriftAdopt || in.Drift[0].Path != "d/drift.md" {
		t.Errorf("Drift = %+v, want [{d/drift.md adopt}]", in.Drift)
	}
	adopts := 0
	for _, r := range in.Unknown {
		if r.Decision == appapi.UnknownAdopt && r.Path == "u/unknown.md" {
			adopts++
		}
	}
	if adopts != 1 {
		t.Errorf("Unknown = %+v, want one adopt entry for u/unknown.md", in.Unknown)
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

// TestPlanProjectScreen_OnApplyEmptyMapOmitsDriftKeepAndKeepsUnknown pins
// the post-ADR-0015 contract: an untouched drift row emits no resolution
// (absence == DriftKeep, so sync preserves the prior baseline instead of
// silently rebaselining — bug 0033). Unknown rows still emit an explicit
// UnknownKeep for every row because that default is a state no-op.
func TestPlanProjectScreen_OnApplyEmptyMapOmitsDriftKeepAndKeepsUnknown(t *testing.T) {
	changes := []appapi.FileChange{
		{Path: "create.md", Kind: appapi.ChangeCreate},
		{Path: "drift.md", Kind: appapi.ChangeDrift},
		{Path: "unknown.md", Kind: appapi.ChangeUnknown},
	}
	f := newPlanActionsFake("Proj", changes)
	s := newPlanProjectScreen(f, "alpha", "proj-1")
	planLoadInto(t, s, f)

	cmd := s.onApply()
	if cmd == nil {
		t.Fatalf("onApply returned nil cmd")
	}
	done, ok := cmd().(syncDoneMsg)
	if !ok {
		t.Fatalf("onApply produced %T, want syncDoneMsg", cmd())
	}
	if done.err != nil {
		t.Errorf("err = %v, want nil", done.err)
	}
	if done.info != "Project synced" {
		t.Errorf("info = %q, want %q", done.info, "Project synced")
	}
	if len(f.syncInputs) != 1 {
		t.Fatalf("syncInputs len = %d, want 1", len(f.syncInputs))
	}
	in := f.syncInputs[0]
	if len(in.Drift) != 0 {
		t.Errorf("Drift = %+v, want [] (untouched drift must not emit a resolution)", in.Drift)
	}
	if len(in.Unknown) != 1 || in.Unknown[0].Path != "unknown.md" || in.Unknown[0].Decision != appapi.UnknownKeep {
		t.Errorf("Unknown = %+v, want [{unknown.md keep}]", in.Unknown)
	}
}

func TestPlanProjectScreen_OnApplyWithSelectionsBuildsCorrectSlices(t *testing.T) {
	changes := []appapi.FileChange{
		{Path: "a/drift.md", Kind: appapi.ChangeDrift},
		{Path: "b/unknown.md", Kind: appapi.ChangeUnknown},
		{Path: "c/create.md", Kind: appapi.ChangeCreate},
	}
	f := newPlanActionsFake("Proj", changes)
	s := newPlanProjectScreen(f, "alpha", "proj-1")
	planLoadInto(t, s, f)
	s.driftResolutions["a/drift.md"] = appapi.DriftOverwrite
	s.unknownResolutions["b/unknown.md"] = appapi.UnknownDelete

	_ = s.onApply()()
	if len(f.syncInputs) != 1 {
		t.Fatalf("syncInputs len = %d, want 1", len(f.syncInputs))
	}
	in := f.syncInputs[0]
	if len(in.Drift) != 1 || in.Drift[0].Path != "a/drift.md" || in.Drift[0].Decision != appapi.DriftOverwrite {
		t.Errorf("Drift = %+v, want [{a/drift.md overwrite}]", in.Drift)
	}
	if len(in.Unknown) != 1 || in.Unknown[0].Path != "b/unknown.md" || in.Unknown[0].Decision != appapi.UnknownDelete {
		t.Errorf("Unknown = %+v, want [{b/unknown.md delete}]", in.Unknown)
	}
}

// findPlanNode walks the tree for the dir/file node whose payload path
// matches, so collapse tests can assert structure without parsing rendered
// rows.
func findPlanNode(n *treetable.Node, path string) *treetable.Node {
	if d, ok := n.Data.(planNode); ok && d.path == path {
		return n
	}
	for _, c := range n.Children {
		if got := findPlanNode(c, path); got != nil {
			return got
		}
	}
	return nil
}

func TestBuildPlanTree_IgnoredFolderCollapsesAndDropsSlash(t *testing.T) {
	changes := []appapi.FileChange{
		{Path: "sub/a.md", Kind: appapi.ChangeUnknown},
		{Path: "sub/b.md", Kind: appapi.ChangeUnknown},
	}

	open := buildPlanTree("Proj", changes, map[string]bool{}, nil)
	sub := findPlanNode(open, "sub")
	if sub == nil {
		t.Fatal("expected a node for sub")
	}
	if sub.Label != "sub/" {
		t.Errorf("open folder label = %q, want %q", sub.Label, "sub/")
	}
	if len(sub.Children) != 2 {
		t.Errorf("open folder children = %d, want 2", len(sub.Children))
	}

	collapsed := buildPlanTree("Proj", changes, map[string]bool{"sub": true}, nil)
	sub2 := findPlanNode(collapsed, "sub")
	if sub2 == nil {
		t.Fatal("expected a node for ignored sub")
	}
	if sub2.Label != "sub" {
		t.Errorf("ignored folder label = %q, want %q (no trailing slash)", sub2.Label, "sub")
	}
	if len(sub2.Children) != 0 {
		t.Errorf("ignored folder children = %d, want 0", len(sub2.Children))
	}
}

func TestPlanProjectScreen_ToggleIgnoreCollapsesAndRestores(t *testing.T) {
	changes := []appapi.FileChange{{Path: "sub/a.md", Kind: appapi.ChangeUnknown}}
	f := newPlanActionsFake("Proj", changes)
	s := newPlanProjectScreen(f, "alpha", "proj-1")
	planLoadInto(t, s, f)

	before := len(s.tree.Rows())
	s.setIgnored("sub", true, false)
	if !s.ignoredPaths["sub"] {
		t.Fatal("ignoredPaths[sub] = false, want true after ignore")
	}
	if got := len(s.tree.Rows()); got >= before {
		t.Errorf("rows after ignore = %d, want fewer than %d", got, before)
	}

	s.setIgnored("sub", false, false)
	if s.ignoredPaths["sub"] {
		t.Fatal("ignoredPaths[sub] = true, want false after show")
	}
	if got := len(s.tree.Rows()); got != before {
		t.Errorf("rows after show = %d, want %d (restored)", got, before)
	}
}

func TestPlanProjectScreen_OnApplyForwardsIgnoredDirsSorted(t *testing.T) {
	changes := []appapi.FileChange{
		{Path: "b/x.md", Kind: appapi.ChangeUnknown},
		{Path: "a/y.md", Kind: appapi.ChangeUnknown},
	}
	f := newPlanActionsFake("Proj", changes)
	s := newPlanProjectScreen(f, "alpha", "proj-1")
	planLoadInto(t, s, f)
	s.ignoredPaths = map[string]bool{"b": true, "a": true}

	_ = s.onApply()()
	if len(f.syncInputs) != 1 {
		t.Fatalf("syncInputs len = %d, want 1", len(f.syncInputs))
	}
	want := []string{"a", "b"}
	got := f.syncInputs[0].IgnoredPaths
	if len(got) != len(want) {
		t.Fatalf("Ignored = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Fatalf("Ignored = %v, want %v (sorted)", got, want)
		}
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

func TestBuildPlanTree_InjectsPersistedIgnoredLeafAtSortedPosition(t *testing.T) {
	changes := []appapi.FileChange{{Path: "a/file.md", Kind: appapi.ChangeCreate}}

	root := buildPlanTree("Proj", changes, map[string]bool{}, []string{"a/ignored"})

	leaf := findPlanNode(root, "a/ignored")
	if leaf == nil {
		t.Fatal("expected injected node for a/ignored")
	}
	d, ok := leaf.Data.(planNode)
	if !ok || !d.persistedIgnored || d.kind != planNodeDir {
		t.Fatalf("injected node data = %+v, want persistedIgnored dir", leaf.Data)
	}
	if leaf.Label != "ignored" {
		t.Errorf("label = %q, want %q (no trailing slash)", leaf.Label, "ignored")
	}
	if len(leaf.Children) != 0 {
		t.Errorf("children = %d, want 0 (collapsed leaf)", len(leaf.Children))
	}
	// The injected leaf shares parent "a/" with the change row rather than
	// creating a duplicate parent dir.
	parent := findPlanNode(root, "a")
	if parent == nil || len(parent.Children) != 2 {
		t.Fatalf("parent a children = %+v, want file + injected leaf under one parent", parent)
	}
}

func TestPlanProjectScreen_ShowIgnoredTogglesVisibility(t *testing.T) {
	f := newPlanActionsFake("Proj", []appapi.FileChange{{Path: "x.md", Kind: appapi.ChangeCreate}})
	f.preview.IgnoredPaths = []string{"sub"}
	s := newPlanProjectScreen(f, "alpha", "proj-1")
	planLoadInto(t, s, f)

	if got := s.visiblePersistedIgnored(); len(got) != 0 {
		t.Fatalf("default visible = %v, want none (hidden)", got)
	}
	_ = s.toggleShowIgnored()
	if got := s.visiblePersistedIgnored(); len(got) != 1 || got[0] != "sub" {
		t.Fatalf("after Show Ignored visible = %v, want [sub]", got)
	}
	_ = s.toggleShowIgnored()
	if got := s.visiblePersistedIgnored(); len(got) != 0 {
		t.Fatalf("after Hide Ignored visible = %v, want none", got)
	}
}

func TestPlanProjectScreen_ShownRowStaysPinnedAfterHide(t *testing.T) {
	f := newPlanActionsFake("Proj", []appapi.FileChange{{Path: "x.md", Kind: appapi.ChangeCreate}})
	f.preview.IgnoredPaths = []string{"sub"}
	s := newPlanProjectScreen(f, "alpha", "proj-1")
	planLoadInto(t, s, f)

	_ = s.toggleShowIgnored()            // reveal
	_ = s.setIgnored("sub", false, true) // press [Show] → pin
	_ = s.toggleShowIgnored()            // hide

	got := s.visiblePersistedIgnored()
	if len(got) != 1 || got[0] != "sub" {
		t.Fatalf("pinned row visible = %v, want [sub] after Hide Ignored", got)
	}
}

func TestPlanProjectScreen_PersistedIgnoredRowButtonFlips(t *testing.T) {
	f := newPlanActionsFake("Proj", nil)
	f.preview.IgnoredPaths = []string{"sub"}
	s := newPlanProjectScreen(f, "alpha", "proj-1")
	planLoadInto(t, s, f)

	fn := s.treeActionsFn()
	n := &treetable.Node{Data: planNode{kind: planNodeDir, path: "sub", persistedIgnored: true}}

	got := fn(n)
	if len(got) != 1 {
		t.Fatalf("got %d buttons, want 1", len(got))
	}
	assertBtn(t, got[0], "Show", 'w')

	_ = s.setIgnored("sub", false, true)
	if got := fn(n); len(got) != 1 {
		t.Fatalf("after Show got %d buttons, want 1", len(got))
	} else {
		assertBtn(t, got[0], "Ignore", 'i')
	}

	_ = s.setIgnored("sub", true, true)
	if got := fn(n); len(got) != 1 {
		t.Fatalf("after re-Ignore got %d buttons, want 1", len(got))
	} else {
		assertBtn(t, got[0], "Show", 'w')
	}
}

func TestPlanProjectScreen_StatusForPersistedIgnoredDir(t *testing.T) {
	f := newPlanActionsFake("Proj", nil)
	s := newPlanProjectScreen(f, "alpha", "proj-1")

	ignored := &treetable.Node{Data: planNode{kind: planNodeDir, path: "sub", persistedIgnored: true}}
	if got := s.statusValue(ignored); got != "! ignored" {
		t.Errorf("statusValue(persisted) = %q, want %q", got, "! ignored")
	}
	plain := &treetable.Node{Data: planNode{kind: planNodeDir, path: "sub"}}
	if got := s.statusValue(plain); got != "" {
		t.Errorf("statusValue(plain dir) = %q, want empty", got)
	}
}

// TestPlanProjectScreen_ResolutionBlankForPersistedIgnoredDir pins that a
// persisted-ignored row leaves the Resolution column empty (the dir node has
// no per-file decision), distinct from its "! ignored" Status.
func TestPlanProjectScreen_ResolutionBlankForPersistedIgnoredDir(t *testing.T) {
	f := newPlanActionsFake("Proj", nil)
	s := newPlanProjectScreen(f, "alpha", "proj-1")

	ignored := &treetable.Node{Data: planNode{kind: planNodeDir, path: "sub", persistedIgnored: true}}
	if got := s.actionValue(ignored); got != "" {
		t.Errorf("actionValue(persisted-ignored dir) = %q, want empty", got)
	}
}

// TestPlanProjectScreen_ShowIgnoredButtonFlipsLabelAndMnemonic pins the
// screen-level toggle's label/mnemonic across states: Show Ignored/'g' when
// hidden (default), Hide Ignored/'h' when shown.
func TestPlanProjectScreen_ShowIgnoredButtonFlipsLabelAndMnemonic(t *testing.T) {
	f := newPlanActionsFake("Proj", nil)
	s := newPlanProjectScreen(f, "alpha", "proj-1")
	planLoadInto(t, s, f)

	assertBtn(t, s.showIgnoredBtn, "Show Ignored", 'g') // default hidden
	_ = s.toggleShowIgnored()
	assertBtn(t, s.showIgnoredBtn, "Hide Ignored", 'h') // shown
	_ = s.toggleShowIgnored()
	assertBtn(t, s.showIgnoredBtn, "Show Ignored", 'g') // hidden again
}

func TestPlanProjectScreen_OnApplyBuildsDesiredIgnoredSet(t *testing.T) {
	changes := []appapi.FileChange{{Path: "live/u.md", Kind: appapi.ChangeUnknown}}
	f := newPlanActionsFake("Proj", changes)
	f.preview.IgnoredPaths = []string{"keep", "drop"}
	s := newPlanProjectScreen(f, "alpha", "proj-1")
	planLoadInto(t, s, f)

	_ = s.setIgnored("drop", false, true)          // remove persisted "drop"
	s.ignoredPaths = map[string]bool{"live": true} // newly ignore a live folder

	_ = s.onApply()()
	if len(f.syncInputs) != 1 {
		t.Fatalf("syncInputs len = %d, want 1", len(f.syncInputs))
	}
	want := []string{"keep", "live"}
	got := f.syncInputs[0].IgnoredPaths
	if len(got) != len(want) {
		t.Fatalf("IgnoredPaths = %v, want %v ((persisted−unignored)∪live)", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Fatalf("IgnoredPaths = %v, want %v (sorted)", got, want)
		}
	}
}

// TestPlanProjectScreen_OnApplyPureIgnoreChangeApplies pins that un-ignoring a
// persisted folder is a valid Apply even when there are no file changes — it
// rewrites ignored_paths without touching files.
func TestPlanProjectScreen_OnApplyPureIgnoreChangeApplies(t *testing.T) {
	f := newPlanActionsFake("Proj", nil)
	f.preview.IgnoredPaths = []string{"sub"}
	s := newPlanProjectScreen(f, "alpha", "proj-1")
	planLoadInto(t, s, f)

	_ = s.setIgnored("sub", false, true)

	cmd := s.onApply()
	if cmd == nil {
		t.Fatal("onApply nil for pure ignore-set change, want apply")
	}
	_ = cmd()
	if len(f.syncInputs) != 1 {
		t.Fatalf("syncInputs len = %d, want 1", len(f.syncInputs))
	}
	if got := f.syncInputs[0].IgnoredPaths; len(got) != 0 {
		t.Fatalf("IgnoredPaths = %v, want empty (sub un-ignored)", got)
	}
}

// TestPlanProjectScreen_MnemonicUniquenessOnPersistedIgnoredRow lands the
// cursor on a shown persisted-ignored row in both its [Show] and [Ignore]
// states and asserts the registered mnemonics stay unique, covering the new
// 'g'/'h' (screen toggle) and 'w'/'i' (row) candidates.
func TestPlanProjectScreen_MnemonicUniquenessOnPersistedIgnoredRow(t *testing.T) {
	changes := []appapi.FileChange{
		{Path: "d/drift.md", Kind: appapi.ChangeDrift},
		{Path: "e/unknown.md", Kind: appapi.ChangeUnknown},
	}
	for _, unignore := range []bool{false, true} {
		f := newPlanActionsFake("Proj", changes)
		f.preview.IgnoredPaths = []string{"zsub"}
		s := newPlanProjectScreen(f, "alpha", "proj-1")
		planLoadInto(t, s, f)
		_ = s.toggleShowIgnored() // reveal zsub
		if unignore {
			_ = s.setIgnored("zsub", false, true)
		}

		found := false
		rowMax := len(s.tree.Rows())
		for row := 0; row < rowMax; row++ {
			if row > 0 {
				_, _ = s.tree.Update(tea.KeyPressMsg{Code: tea.KeyDown})
			}
			// rebuildSet calls Set.Add, which panics on any case-insensitive
			// duplicate mnemonic — the walk itself is the uniqueness guard.
			s.rebuildSet()
			if n := s.tree.SelectedNode(); n != nil {
				if d, ok := n.Data.(planNode); ok && d.persistedIgnored {
					found = true
				}
			}
		}
		if !found {
			t.Fatalf("never landed cursor on persisted-ignored row (unignore=%v)", unignore)
		}
	}
}

// TestPlanProjectScreen_MnemonicUniquenessOnPinnedRowWhileHidden walks the
// state the sibling test above never reaches: a persisted-ignored row pinned
// visible while the screen toggle is back in its Show Ignored ('g') state.
// Production reaches it by pressing [Show] (pins the row) then [Hide Ignored]
// (flips the toggle to 'g' while the pinned row stays). The walk lands the
// cursor on the pinned row in both its [Show] ('w') and [Ignore] ('i') button
// states and asserts 'g' coexists uniquely with the row rune plus 'a'/'b'.
func TestPlanProjectScreen_MnemonicUniquenessOnPinnedRowWhileHidden(t *testing.T) {
	changes := []appapi.FileChange{
		{Path: "d/drift.md", Kind: appapi.ChangeDrift},
		{Path: "e/unknown.md", Kind: appapi.ChangeUnknown},
	}
	for _, reignore := range []bool{false, true} {
		f := newPlanActionsFake("Proj", changes)
		f.preview.IgnoredPaths = []string{"zsub"}
		s := newPlanProjectScreen(f, "alpha", "proj-1")
		planLoadInto(t, s, f)
		_ = s.toggleShowIgnored()             // reveal zsub
		_ = s.setIgnored("zsub", false, true) // press [Show] → pin, button is now [Ignore]/'i'
		if reignore {
			_ = s.setIgnored("zsub", true, true) // press [Ignore] → pin stays, button back to [Show]/'w'
		}
		_ = s.toggleShowIgnored() // [Hide Ignored] → screen toggle flips to [Show Ignored]/'g'

		if s.showIgnoredBtn.Mnemonic() != 'g' {
			t.Fatalf("screen toggle mnemonic = %q, want 'g' (hidden)", s.showIgnoredBtn.Mnemonic())
		}
		found := false
		rowMax := len(s.tree.Rows())
		for row := 0; row < rowMax; row++ {
			if row > 0 {
				_, _ = s.tree.Update(tea.KeyPressMsg{Code: tea.KeyDown})
			}
			// rebuildSet calls Set.Add, which panics on any case-insensitive
			// duplicate mnemonic — the walk itself is the uniqueness guard.
			s.rebuildSet()
			if n := s.tree.SelectedNode(); n != nil {
				if d, ok := n.Data.(planNode); ok && d.persistedIgnored {
					found = true
				}
			}
		}
		if !found {
			t.Fatalf("never landed cursor on pinned persisted-ignored row (reignore=%v)", reignore)
		}
	}
}

// TestPlanProjectRowButtonsMatchMatrix walks every combination in the
// four trilean/bilean matrices in task 0044's description.md and asserts
// that the rendered button set matches the spec (label + mnemonic, in
// order) and that pressing each button sets the drift / unknown
// resolution map to its target value.
func TestPlanProjectRowButtonsMatchMatrix(t *testing.T) {
	type matrixRow struct {
		name          string
		kind          appapi.ChangeKind
		adoptEligible bool
		current       appapi.DriftDecision   // for drift rows
		currentUnk    appapi.UnknownDecision // for unknown rows
		wantLabels    []string               // labels of the non-Open buttons in render order
		wantMnemonics []rune
		// wantResolutions maps each button index (in wantLabels) to the
		// drift resolution the press should produce; used for drift rows.
		wantDriftResolutions []appapi.DriftDecision
		// wantUnknownResolutions same for unknown rows.
		wantUnknownResolutions []appapi.UnknownDecision
	}
	cases := []matrixRow{
		// Drift trilean (adoptEligible == true) — description.md:53-59.
		{
			name: "drift trilean Keep",
			kind: appapi.ChangeDrift, adoptEligible: true, current: appapi.DriftKeep,
			wantLabels: []string{"Overwrite", "Adopt"}, wantMnemonics: []rune{'w', 't'},
			wantDriftResolutions: []appapi.DriftDecision{appapi.DriftOverwrite, appapi.DriftAdopt},
		},
		{
			name: "drift trilean Overwrite",
			kind: appapi.ChangeDrift, adoptEligible: true, current: appapi.DriftOverwrite,
			wantLabels: []string{"Keep", "Adopt"}, wantMnemonics: []rune{'p', 't'},
			wantDriftResolutions: []appapi.DriftDecision{appapi.DriftKeep, appapi.DriftAdopt},
		},
		{
			name: "drift trilean Adopt",
			kind: appapi.ChangeDrift, adoptEligible: true, current: appapi.DriftAdopt,
			wantLabels: []string{"Keep", "Overwrite"}, wantMnemonics: []rune{'p', 'w'},
			wantDriftResolutions: []appapi.DriftDecision{appapi.DriftKeep, appapi.DriftOverwrite},
		},
		// Drift bilean (adoptEligible == false) — description.md:61-66.
		{
			name: "drift bilean Keep",
			kind: appapi.ChangeDrift, adoptEligible: false, current: appapi.DriftKeep,
			wantLabels: []string{"Overwrite"}, wantMnemonics: []rune{'w'},
			wantDriftResolutions: []appapi.DriftDecision{appapi.DriftOverwrite},
		},
		{
			name: "drift bilean Overwrite",
			kind: appapi.ChangeDrift, adoptEligible: false, current: appapi.DriftOverwrite,
			wantLabels: []string{"Keep"}, wantMnemonics: []rune{'p'},
			wantDriftResolutions: []appapi.DriftDecision{appapi.DriftKeep},
		},
		// Unknown trilean (owned) — description.md:71-77.
		{
			name: "unknown trilean Keep",
			kind: appapi.ChangeUnknown, adoptEligible: true, currentUnk: appapi.UnknownKeep,
			wantLabels: []string{"Delete", "Adopt"}, wantMnemonics: []rune{'d', 't'},
			wantUnknownResolutions: []appapi.UnknownDecision{appapi.UnknownDelete, appapi.UnknownAdopt},
		},
		{
			name: "unknown trilean Delete",
			kind: appapi.ChangeUnknown, adoptEligible: true, currentUnk: appapi.UnknownDelete,
			wantLabels: []string{"Keep", "Adopt"}, wantMnemonics: []rune{'p', 't'},
			wantUnknownResolutions: []appapi.UnknownDecision{appapi.UnknownKeep, appapi.UnknownAdopt},
		},
		{
			name: "unknown trilean Adopt",
			kind: appapi.ChangeUnknown, adoptEligible: true, currentUnk: appapi.UnknownAdopt,
			wantLabels: []string{"Keep", "Delete"}, wantMnemonics: []rune{'p', 'd'},
			wantUnknownResolutions: []appapi.UnknownDecision{appapi.UnknownKeep, appapi.UnknownDelete},
		},
		// Unknown bilean (orphan) — description.md:79-84.
		{
			name: "unknown bilean Keep",
			kind: appapi.ChangeUnknown, adoptEligible: false, currentUnk: appapi.UnknownKeep,
			wantLabels: []string{"Delete"}, wantMnemonics: []rune{'d'},
			wantUnknownResolutions: []appapi.UnknownDecision{appapi.UnknownDelete},
		},
		{
			name: "unknown bilean Delete",
			kind: appapi.ChangeUnknown, adoptEligible: false, currentUnk: appapi.UnknownDelete,
			wantLabels: []string{"Keep"}, wantMnemonics: []rune{'p'},
			wantUnknownResolutions: []appapi.UnknownDecision{appapi.UnknownKeep},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := "p"
			ch := appapi.FileChange{Path: path, Kind: tc.kind}
			f := newPlanActionsFake("Proj", []appapi.FileChange{ch})
			s := newPlanProjectScreen(f, "alpha", "proj-1")
			planLoadInto(t, s, f)
			// Seed the current selection so the factory returns the
			// non-selected options for that state.
			if tc.kind == appapi.ChangeDrift {
				if tc.adoptEligible {
					ch.AdoptProvenance = appapi.AdoptProvenance{AssetID: "base", SourceRel: path}
				}
				if tc.current != appapi.DriftKeep {
					s.driftResolutions[path] = tc.current
				}
			} else {
				if tc.adoptEligible {
					ch.OwningAssetID = "foo"
				}
				if tc.currentUnk != appapi.UnknownKeep {
					s.unknownResolutions[path] = tc.currentUnk
				}
			}

			fn := s.treeActionsFn()
			n := &treetable.Node{Data: planNode{kind: planNodeFile, path: path, change: ch}}
			got := fn(n)
			// Drift rows carry a trailing [Diff] after the toggles; unknown
			// rows do not (no managed baseline to diff against). See 0045.
			diffBtns := 0
			if tc.kind == appapi.ChangeDrift {
				diffBtns = 1
			}
			wantLen := 1 + len(tc.wantLabels) + diffBtns
			if len(got) != wantLen {
				t.Fatalf("button count = %d, want %d ([Open]+%d+%d diff)", len(got), wantLen, len(tc.wantLabels), diffBtns)
			}
			assertBtn(t, got[0], "Open", 'o')
			for i, wantLabel := range tc.wantLabels {
				assertBtn(t, got[i+1], wantLabel, tc.wantMnemonics[i])
			}
			if diffBtns == 1 {
				assertBtn(t, got[len(got)-1], "Diff", 'd')
			}

			// Press each button and assert the resulting resolution map
			// snaps to the button's target value. Reseed the current
			// selection between presses so state does not leak.
			for i := range tc.wantLabels {
				if tc.kind == appapi.ChangeDrift {
					if tc.current == appapi.DriftKeep {
						delete(s.driftResolutions, path)
					} else {
						s.driftResolutions[path] = tc.current
					}
				} else {
					if tc.currentUnk == appapi.UnknownKeep {
						delete(s.unknownResolutions, path)
					} else {
						s.unknownResolutions[path] = tc.currentUnk
					}
				}
				pressed := fn(n)[i+1]
				_ = pressed.Trigger()
				if tc.kind == appapi.ChangeDrift {
					want := tc.wantDriftResolutions[i]
					if want == appapi.DriftKeep {
						if _, present := s.driftResolutions[path]; present {
							t.Errorf("press %q: drift map still present, want absent (Keep)", tc.wantLabels[i])
						}
					} else if s.driftResolutions[path] != want {
						t.Errorf("press %q: driftResolutions[%s] = %v, want %v", tc.wantLabels[i], path, s.driftResolutions[path], want)
					}
				} else {
					want := tc.wantUnknownResolutions[i]
					if want == appapi.UnknownKeep {
						if _, present := s.unknownResolutions[path]; present {
							t.Errorf("press %q: unknown map still present, want absent (Keep)", tc.wantLabels[i])
						}
					} else if s.unknownResolutions[path] != want {
						t.Errorf("press %q: unknownResolutions[%s] = %v, want %v", tc.wantLabels[i], path, s.unknownResolutions[path], want)
					}
				}
			}
		})
	}
}

// TestPlanProjectMnemonicUniqueness walks the cursor across every row
// kind × every current-state variant listed in task 0044 Step 8 and
// asserts that no two active buttons (row-level + screen-level combined)
// share a case-insensitive mnemonic rune. Also asserts the walk actually
// visited each row kind so a future navigation regression that silently
// skips rows surfaces as a test failure.
func TestPlanProjectMnemonicUniqueness(t *testing.T) {
	// Live changes cover every FileChange-backed row kind:
	// create/update/delete + drift-bilean, drift-trilean, unknown-owned,
	// unknown-orphan. A persisted-ignored dir + a registerable dir are
	// injected via preview.IgnoredPaths and the registerableDirs map so
	// [Show]/'w' and [Register]/'r' + [Ignore]/'i' buttons appear.
	changes := []appapi.FileChange{
		{Path: "add/f.md", Kind: appapi.ChangeCreate},
		{Path: "upd/f.md", Kind: appapi.ChangeUpdate},
		{Path: "del/f.md", Kind: appapi.ChangeDelete},
		{Path: "drift-b/f.md", Kind: appapi.ChangeDrift},
		{Path: "drift-t/f.md", Kind: appapi.ChangeDrift, AdoptProvenance: appapi.AdoptProvenance{AssetID: "base", SourceRel: "drift-t/f.md"}},
		{Path: "owned/f.md", Kind: appapi.ChangeUnknown, OwningAssetID: "foo"},
		{Path: "orphan/f.md", Kind: appapi.ChangeUnknown},
		// A dir that becomes registerable (all-unknown subtree) so its
		// [Register] and [Ignore] buttons enter the mnemonic set.
		{Path: "reg/only-unknown.md", Kind: appapi.ChangeUnknown},
	}
	// Every (drift-b, drift-t, owned, orphan) row is visited in every
	// current-state per matrix, so the sweep hits both bilean states and
	// all three trilean states. Non-toggle rows repeat across overrides
	// but their button set does not depend on the resolution maps, so
	// the extra visits are harmless.
	type override struct {
		drift   map[string]appapi.DriftDecision
		unknown map[string]appapi.UnknownDecision
	}
	overrides := []override{
		{}, // Keep everywhere.
		{drift: map[string]appapi.DriftDecision{"drift-b/f.md": appapi.DriftOverwrite, "drift-t/f.md": appapi.DriftOverwrite}},
		{drift: map[string]appapi.DriftDecision{"drift-t/f.md": appapi.DriftAdopt}},
		{unknown: map[string]appapi.UnknownDecision{"owned/f.md": appapi.UnknownDelete, "orphan/f.md": appapi.UnknownDelete}},
		{unknown: map[string]appapi.UnknownDecision{"owned/f.md": appapi.UnknownAdopt}},
	}
	// Row-kind sentinels the walk must visit at least once.
	type rowKind int
	const (
		kindCreate rowKind = iota
		kindUpdate
		kindDelete
		kindDriftBilean
		kindDriftTrilean
		kindUnknownOwned
		kindUnknownOrphan
		kindPersistedIgnored
		kindRegisterableDir
	)
	kindOf := func(n *treetable.Node) (rowKind, bool) {
		if d, ok := planDirNode(n); ok {
			if d.persistedIgnored {
				return kindPersistedIgnored, true
			}
			return kindRegisterableDir, d.path == "reg" || d.path == "reg/only-unknown"
		}
		d, ok := planFileNode(n)
		if !ok {
			return 0, false
		}
		switch d.change.Kind {
		case appapi.ChangeCreate:
			return kindCreate, true
		case appapi.ChangeUpdate:
			return kindUpdate, true
		case appapi.ChangeDelete:
			return kindDelete, true
		case appapi.ChangeDrift:
			if d.change.AdoptProvenance.Available() {
				return kindDriftTrilean, true
			}
			return kindDriftBilean, true
		case appapi.ChangeUnknown:
			if d.change.OwningAssetID != "" {
				return kindUnknownOwned, true
			}
			return kindUnknownOrphan, true
		}
		return 0, false
	}
	saw := map[rowKind]bool{}
	for _, ov := range overrides {
		f := newPlanActionsFake("Proj", changes)
		f.preview.IgnoredPaths = []string{"persisted"}
		s := newPlanProjectScreen(f, "alpha", "proj-1")
		planLoadInto(t, s, f)
		// Force the reg/ dir onto the registerable list so its row-level
		// [Register]+[Ignore] buttons enter the mnemonic set even though
		// this test bypasses the real all-unknown eligibility check.
		s.registerableDirs["reg"] = true
		// Reveal the persisted-ignored row so the cursor can land on it.
		_ = s.toggleShowIgnored()
		for k, v := range ov.drift {
			s.driftResolutions[k] = v
		}
		for k, v := range ov.unknown {
			s.unknownResolutions[k] = v
		}
		s.tree.SetRoot(buildPlanTree(s.projectName, s.preview.Changes, s.ignoredPaths, s.visiblePersistedIgnored()))
		rowMax := len(s.tree.Rows())
		for row := 0; row < rowMax; row++ {
			if row > 0 {
				_, _ = s.tree.Update(tea.KeyPressMsg{Code: tea.KeyDown})
			}
			// rebuildSet calls Set.Add, which panics on any case-insensitive
			// duplicate mnemonic — this walk asserts the panic never fires.
			s.rebuildSet()
			if n := s.tree.SelectedNode(); n != nil {
				if k, ok := kindOf(n); ok {
					saw[k] = true
				}
			}
		}
	}
	want := []rowKind{
		kindCreate, kindUpdate, kindDelete,
		kindDriftBilean, kindDriftTrilean,
		kindUnknownOwned, kindUnknownOrphan,
		kindPersistedIgnored, kindRegisterableDir,
	}
	for _, k := range want {
		if !saw[k] {
			t.Errorf("walk never visited row kind %d — cursor navigation regression", k)
		}
	}
}

// diffButtonIn returns the [Diff] button among a row's action buttons, or nil
// when the row offers none.
func diffButtonIn(btns []*mnemonic.Button) *mnemonic.Button {
	for _, b := range btns {
		if b.Label() == "Diff" {
			return b
		}
	}
	return nil
}

// TestDiffButton pins Acceptance Criterion 1: the [Diff] (`d`) action renders
// exactly on ChangeUpdate and ChangeDrift file rows, and never on create,
// delete, or unknown rows (each lacks one side of the diff).
func TestDiffButton(t *testing.T) {
	f := newPlanActionsFake("Proj", nil)
	s := newPlanProjectScreen(f, "alpha", "proj-1")
	fn := s.treeActionsFn()
	cases := []struct {
		kind appapi.ChangeKind
		want bool
	}{
		{appapi.ChangeCreate, false},
		{appapi.ChangeUpdate, true},
		{appapi.ChangeDelete, false},
		{appapi.ChangeDrift, true},
		{appapi.ChangeUnknown, false},
	}
	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			n := &treetable.Node{Data: planNode{kind: planNodeFile, path: "p", change: appapi.FileChange{Path: "p", Kind: tc.kind}}}
			btn := diffButtonIn(fn(n))
			if tc.want {
				if btn == nil {
					t.Fatalf("kind %s: [Diff] button absent, want present", tc.kind)
				}
				if btn.Mnemonic() != 'd' {
					t.Errorf("kind %s: Diff mnemonic = %q, want 'd'", tc.kind, btn.Mnemonic())
				}
			} else if btn != nil {
				t.Errorf("kind %s: [Diff] button present, want absent", tc.kind)
			}
		})
	}
}

// TestPlanProjectScreen_DiffButtonDispatchesDiffFile presses [Diff] on a
// drift row and asserts the command forwards the row path through the actions
// seam and the returned message opens the diff modal.
func TestPlanProjectScreen_DiffButtonDispatchesDiffFile(t *testing.T) {
	changes := []appapi.FileChange{{Path: "a/drift.md", Kind: appapi.ChangeDrift}}
	f := newPlanActionsFake("Proj", changes)
	f.diffBodies = appapi.DiffBodies{Local: []byte("local\n"), Desired: []byte("desired\n")}
	s := newPlanProjectScreen(f, "alpha", "proj-1")
	planLoadInto(t, s, f)

	fn := s.treeActionsFn()
	n := &treetable.Node{Data: planNode{kind: planNodeFile, path: "a/drift.md", change: changes[0]}}
	btn := diffButtonIn(fn(n))
	if btn == nil {
		t.Fatal("drift row missing [Diff] button")
	}

	msg := btn.Trigger()()
	ready, ok := msg.(diffReadyMsg)
	if !ok {
		t.Fatalf("Diff press produced %T, want diffReadyMsg", msg)
	}
	if len(f.diffInputs) != 1 || f.diffInputs[0].Path != "a/drift.md" {
		t.Fatalf("DiffFile inputs = %+v, want one call for a/drift.md", f.diffInputs)
	}
	if ready.err != nil {
		t.Fatalf("diffReadyMsg.err = %v, want nil", ready.err)
	}

	if _, _ = s.Update(ready); s.modal == nil {
		t.Fatal("diff modal not opened after diffReadyMsg")
	}
	if !s.InputFocused() {
		t.Error("InputFocused = false with diff modal open, want true")
	}
}

// TestPlanProjectScreen_DiffLocalReadErrorRendersInModal pins Acceptance
// Criterion 7: a typed local-read failure opens the modal showing the error
// (no panic, no blank pane) rather than dropping a toast.
func TestPlanProjectScreen_DiffLocalReadErrorRendersInModal(t *testing.T) {
	changes := []appapi.FileChange{{Path: "u/upd.md", Kind: appapi.ChangeUpdate}}
	f := newPlanActionsFake("Proj", changes)
	f.diffErr = stubDomainErr{msg: "read local file u/upd.md: no such file", sev: errs.SeverityError}
	s := newPlanProjectScreen(f, "alpha", "proj-1")
	planLoadInto(t, s, f)

	fn := s.treeActionsFn()
	n := &treetable.Node{Data: planNode{kind: planNodeFile, path: "u/upd.md", change: changes[0]}}
	btn := diffButtonIn(fn(n))
	if btn == nil {
		t.Fatal("update row missing [Diff] button")
	}

	msg := btn.Trigger()().(diffReadyMsg)
	if msg.err == nil {
		t.Fatal("diffReadyMsg.err = nil, want the typed read error")
	}
	if _, _ = s.Update(msg); s.modal == nil {
		t.Fatal("diff modal not opened for error case, want error rendered in modal")
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
