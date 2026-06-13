package shell

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/hexworks/agentfiles/internal/actions"
	"github.com/hexworks/agentfiles/internal/asset"
	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/profile"
	"github.com/hexworks/agentfiles/internal/project"
	llmsync "github.com/hexworks/agentfiles/internal/sync"
	"github.com/hexworks/agentfiles/internal/tui/components/mnemonic"
	"github.com/hexworks/agentfiles/internal/tui/notifications"
)

// Compile-time guard.
var _ Screen = (*selectProjectAssetsScreen)(nil)

// fakeSelectActions records every action invocation and returns either
// the preconfigured slice or error. The slice it returns from
// SelectAsset / UnselectAsset is the post-persistence selection — the
// real service returns the same shape so the screen can trust it.
type fakeSelectActions struct {
	prof           *profile.Profile
	loadErr        errs.DomainError
	loadProjectErr errs.DomainError
	selectResult   []string
	selectErr      errs.DomainError
	unselectResult []string
	unselectErr    errs.DomainError
	selectInputs   []actions.SelectAssetInput
	unselectInputs []actions.UnselectAssetInput
	preview        *llmsync.Preview
	planErr        errs.DomainError
	syncErr        errs.DomainError
	syncInputs     []actions.SyncProjectInput
}

func (f *fakeSelectActions) LoadProfile(in actions.LoadProfileInput) (*profile.Profile, errs.DomainError) {
	return f.prof, f.loadErr
}

func (f *fakeSelectActions) LoadProject(in actions.LoadProjectInput) (*project.Manifest, errs.DomainError) {
	if f.loadProjectErr != nil {
		return nil, f.loadProjectErr
	}
	if f.prof == nil {
		return nil, nil
	}
	return f.prof.Projects[in.ProjectID], nil
}

func (f *fakeSelectActions) SelectAsset(in actions.SelectAssetInput) ([]string, errs.DomainError) {
	f.selectInputs = append(f.selectInputs, in)
	if f.selectErr != nil {
		return nil, f.selectErr
	}
	return append([]string(nil), f.selectResult...), nil
}

func (f *fakeSelectActions) UnselectAsset(in actions.UnselectAssetInput) ([]string, errs.DomainError) {
	f.unselectInputs = append(f.unselectInputs, in)
	if f.unselectErr != nil {
		return nil, f.unselectErr
	}
	return append([]string(nil), f.unselectResult...), nil
}

func (f *fakeSelectActions) PlanProject(in actions.PlanProjectInput) (*llmsync.Preview, errs.DomainError) {
	if f.planErr != nil {
		return nil, f.planErr
	}
	return f.preview, nil
}

func (f *fakeSelectActions) SyncProject(in actions.SyncProjectInput) (*llmsync.Preview, errs.DomainError) {
	f.syncInputs = append(f.syncInputs, in)
	if f.syncErr != nil {
		return nil, f.syncErr
	}
	return f.preview, nil
}

func newSelectActionsFake(assets []*asset.Asset, proj *project.Manifest) *fakeSelectActions {
	prof := &profile.Profile{
		Root:     "/tmp/x",
		Manifest: profile.Manifest{ID: "alpha", Name: "Alpha"},
		Assets:   map[string]*asset.Asset{},
		Projects: map[string]*project.Manifest{},
	}
	for _, a := range assets {
		prof.Assets[a.ID] = a
	}
	if proj != nil {
		prof.Projects[proj.ID] = proj
	}
	return &fakeSelectActions{prof: prof}
}

// loadInto bypasses the Init command by injecting the fake's loaded
// profile + project directly into the screen.
func loadInto(t *testing.T, s *selectProjectAssetsScreen, f *fakeSelectActions, projID string) {
	t.Helper()
	proj := f.prof.Projects[projID]
	if proj == nil {
		t.Fatalf("loadInto: project %q missing from fake", projID)
	}
	_, _ = s.Update(selectProjectAssetsLoadedMsg{prof: f.prof, proj: proj})
}

func newAsset(id, name string, typ asset.Type, group string) *asset.Asset {
	return &asset.Asset{
		Manifest: asset.Manifest{
			ID: id, Name: name, Type: typ, ExclusiveGroup: group,
		},
	}
}

// idsOf is a tiny test helper to project an asset list to ids for slice
// equality assertions.
func idsOf(list []*asset.Asset) []string {
	out := make([]string, len(list))
	for i, a := range list {
		out[i] = a.ID
	}
	return out
}

func TestSelectProjectAssets_PanicsOnInvalidConstruction(t *testing.T) {
	f := newSelectActionsFake(nil, nil)
	cases := []struct {
		name string
		fn   func()
	}{
		{"nil actions", func() { _ = newSelectProjectAssetsScreen(nil, "a", "b") }},
		{"empty profile id", func() { _ = newSelectProjectAssetsScreen(f, "", "b") }},
		{"empty project id", func() { _ = newSelectProjectAssetsScreen(f, "a", "") }},
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

func TestSelectProjectAssets_Title(t *testing.T) {
	f := newSelectActionsFake(nil, nil)
	s := newSelectProjectAssetsScreen(f, "alpha", "proj-1")
	if got := s.Title(); got != "Select Project Assets" {
		t.Errorf("Title() = %q, want %q", got, "Select Project Assets")
	}
}

func TestSelectProjectAssets_InitLoadsProfileAndProject(t *testing.T) {
	proj := &project.Manifest{ID: "proj-1", Name: "Proj"}
	f := newSelectActionsFake(nil, proj)
	s := newSelectProjectAssetsScreen(f, "alpha", "proj-1")

	msg := s.Init()()
	loaded, ok := msg.(selectProjectAssetsLoadedMsg)
	if !ok {
		t.Fatalf("Init returned %T, want selectProjectAssetsLoadedMsg", msg)
	}
	if loaded.err != nil {
		t.Fatalf("unexpected err: %v", loaded.err)
	}
	if loaded.prof == nil || loaded.proj == nil {
		t.Fatalf("missing prof or proj in loaded msg: %+v", loaded)
	}
}

func TestSelectProjectAssets_LoadProjectErrorEmitsNotification(t *testing.T) {
	f := newSelectActionsFake(nil, nil)
	f.loadProjectErr = stubDomainErr{msg: "no such project", sev: errs.SeverityError}
	s := newSelectProjectAssetsScreen(f, "alpha", "proj-1")

	msg := s.Init()()
	loaded, ok := msg.(selectProjectAssetsLoadedMsg)
	if !ok {
		t.Fatalf("got %T, want selectProjectAssetsLoadedMsg", msg)
	}
	if loaded.err == nil || !strings.Contains(loaded.err.Error(), "no such project") {
		t.Fatalf("err = %v, want to contain 'no such project'", loaded.err)
	}

	_, cmd := s.Update(loaded)
	n, ok := drainCmd(t, cmd).(notifications.NotificationMsg)
	if !ok {
		t.Fatalf("expected notification cmd, got %T", cmd)
	}
	if !strings.Contains(n.Notification.Text, "no such project") {
		t.Errorf("notification = %q, want to contain 'no such project'", n.Notification.Text)
	}
}

func TestSelectProjectAssets_LoadedSplitsSelectedAndAvailable(t *testing.T) {
	a1 := newAsset("agents", "agents", asset.TypeAgentsDoc, "agents_doc")
	a2 := newAsset("create-task", "create-task", asset.TypeSkill, "")
	a3 := newAsset("review-code", "review-code", asset.TypeSkill, "")
	proj := &project.Manifest{ID: "proj-1", Name: "Proj", SelectedAssetIDs: []string{"agents"}}
	f := newSelectActionsFake([]*asset.Asset{a1, a2, a3}, proj)
	s := newSelectProjectAssetsScreen(f, "alpha", "proj-1")
	loadInto(t, s, f, "proj-1")

	if got := idsOf(s.selected); !slices.Equal(got, []string{"agents"}) {
		t.Errorf("selected = %v, want [agents]", got)
	}
	if got := idsOf(s.available); !slices.Equal(got, []string{"create-task", "review-code"}) {
		t.Errorf("available = %v, want [create-task, review-code]", got)
	}
	if !s.loaded {
		t.Errorf("loaded flag = false")
	}
}

// TestSelectProjectAssets_AvailableOrderingMatchesAssetList pins the
// ordering invariant in one named place: a change to
// profile.AssetList's sort key fails exactly this test, with a self-
// explanatory name, so partition / movement tests never have to encode
// the order rule.
func TestSelectProjectAssets_AvailableOrderingMatchesAssetList(t *testing.T) {
	a1 := newAsset("z-asset", "Aardvark", asset.TypeSkill, "")
	a2 := newAsset("a-asset", "Zebra", asset.TypeSkill, "")
	proj := &project.Manifest{ID: "proj-1", Name: "Proj"}
	f := newSelectActionsFake([]*asset.Asset{a1, a2}, proj)
	s := newSelectProjectAssetsScreen(f, "alpha", "proj-1")
	loadInto(t, s, f, "proj-1")

	want := idsOf(f.prof.AssetList())
	if got := idsOf(s.available); !slices.Equal(got, want) {
		t.Errorf("available order = %v, want AssetList order %v", got, want)
	}
}

func TestSelectProjectAssets_LoadErrorEmitsNotification(t *testing.T) {
	f := newSelectActionsFake(nil, nil)
	f.loadErr = stubDomainErr{msg: "boom", sev: errs.SeverityError}
	s := newSelectProjectAssetsScreen(f, "alpha", "proj-1")

	_, cmd := s.Update(selectProjectAssetsLoadedMsg{err: f.loadErr})
	if cmd == nil {
		t.Fatal("expected notification cmd")
	}
	msg := drainCmd(t, cmd)
	n, ok := msg.(notifications.NotificationMsg)
	if !ok {
		t.Fatalf("got %T, want notifications.NotificationMsg", msg)
	}
	if !strings.Contains(n.Notification.Text, "boom") {
		t.Errorf("notification text = %q, want to contain %q", n.Notification.Text, "boom")
	}
	if n.Notification.Severity != errs.SeverityError {
		t.Errorf("notification severity = %v, want SeverityError", n.Notification.Severity)
	}
}

func TestSelectProjectAssets_SelectInvokesActionWithCursorAssetID(t *testing.T) {
	a1 := newAsset("a1", "A1", asset.TypeSkill, "")
	a2 := newAsset("a2", "A2", asset.TypeSkill, "")
	proj := &project.Manifest{ID: "proj-1", Name: "Proj"}
	f := newSelectActionsFake([]*asset.Asset{a1, a2}, proj)
	f.selectResult = []string{"a1"}
	s := newSelectProjectAssetsScreen(f, "alpha", "proj-1")
	loadInto(t, s, f, "proj-1")
	_ = s.handler.FocusIndex(s.availableIdx)
	s.rebuildSet()
	s.availableTable.SetCursor(0)

	_, cmd := s.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	if cmd == nil {
		t.Fatal("expected cmd")
	}
	_ = drainCmd(t, cmd)

	if len(f.selectInputs) != 1 || f.selectInputs[0].AssetID != "a1" {
		t.Fatalf("SelectAsset calls = %v, want one call for a1", f.selectInputs)
	}
	if f.selectInputs[0].ProfileRef != "alpha" || f.selectInputs[0].ProjectID != "proj-1" {
		t.Errorf("SelectAsset input ids = (%q,%q), want (alpha, proj-1)",
			f.selectInputs[0].ProfileRef, f.selectInputs[0].ProjectID)
	}
}

func TestSelectProjectAssets_SelectEmitsSelectionChangedMsg(t *testing.T) {
	a1 := newAsset("a1", "A1", asset.TypeSkill, "")
	a2 := newAsset("a2", "A2", asset.TypeSkill, "")
	proj := &project.Manifest{ID: "proj-1", Name: "Proj"}
	f := newSelectActionsFake([]*asset.Asset{a1, a2}, proj)
	f.selectResult = []string{"a1"}
	s := newSelectProjectAssetsScreen(f, "alpha", "proj-1")
	loadInto(t, s, f, "proj-1")
	_ = s.handler.FocusIndex(s.availableIdx)
	s.rebuildSet()
	s.availableTable.SetCursor(0)

	_, cmd := s.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	msg := drainCmd(t, cmd)
	changed, ok := msg.(selectionChangedMsg)
	if !ok {
		t.Fatalf("got %T, want selectionChangedMsg", msg)
	}
	if !slices.Equal(changed.selectedIDs, []string{"a1"}) {
		t.Errorf("selectedIDs = %v, want [a1] (server-side slice)", changed.selectedIDs)
	}
}

func TestSelectProjectAssets_SelectionChangedMsgRebuildsPartition(t *testing.T) {
	a1 := newAsset("a1", "A1", asset.TypeSkill, "")
	a2 := newAsset("a2", "A2", asset.TypeSkill, "")
	proj := &project.Manifest{ID: "proj-1", Name: "Proj"}
	f := newSelectActionsFake([]*asset.Asset{a1, a2}, proj)
	s := newSelectProjectAssetsScreen(f, "alpha", "proj-1")
	loadInto(t, s, f, "proj-1")

	_, _ = s.Update(selectionChangedMsg{selectedIDs: []string{"a1"}, info: "added a1"})
	if !slices.Contains(idsOf(s.selected), "a1") {
		t.Errorf("after change, selected = %v, want to contain a1", idsOf(s.selected))
	}
	if slices.Contains(idsOf(s.available), "a1") {
		t.Errorf("after change, available still contains a1: %v", idsOf(s.available))
	}
}

func TestSelectProjectAssets_UnselectInvokesActionWithCursorAssetID(t *testing.T) {
	a1 := newAsset("a1", "A1", asset.TypeSkill, "")
	proj := &project.Manifest{ID: "proj-1", Name: "Proj", SelectedAssetIDs: []string{"a1"}}
	f := newSelectActionsFake([]*asset.Asset{a1}, proj)
	f.unselectResult = []string{}
	s := newSelectProjectAssetsScreen(f, "alpha", "proj-1")
	loadInto(t, s, f, "proj-1")
	_ = s.handler.FocusIndex(s.selectedIdx)
	s.rebuildSet()
	s.selectedTable.SetCursor(0)

	_, cmd := s.Update(tea.KeyPressMsg{Code: 'u', Text: "u"})
	if cmd == nil {
		t.Fatal("expected cmd")
	}
	_ = drainCmd(t, cmd)

	if len(f.unselectInputs) != 1 || f.unselectInputs[0].AssetID != "a1" {
		t.Fatalf("UnselectAsset calls = %v, want one call for a1", f.unselectInputs)
	}
}

func TestSelectProjectAssets_UnselectEmitsSelectionChangedMsg(t *testing.T) {
	a1 := newAsset("a1", "A1", asset.TypeSkill, "")
	proj := &project.Manifest{ID: "proj-1", Name: "Proj", SelectedAssetIDs: []string{"a1"}}
	f := newSelectActionsFake([]*asset.Asset{a1}, proj)
	f.unselectResult = []string{}
	s := newSelectProjectAssetsScreen(f, "alpha", "proj-1")
	loadInto(t, s, f, "proj-1")
	_ = s.handler.FocusIndex(s.selectedIdx)
	s.rebuildSet()
	s.selectedTable.SetCursor(0)

	_, cmd := s.Update(tea.KeyPressMsg{Code: 'u', Text: "u"})
	msg := drainCmd(t, cmd)
	changed, ok := msg.(selectionChangedMsg)
	if !ok {
		t.Fatalf("got %T, want selectionChangedMsg", msg)
	}
	if slices.Contains(changed.selectedIDs, "a1") {
		t.Errorf("selectedIDs = %v, want a1 removed (server-side slice)", changed.selectedIDs)
	}
}

func TestSelectProjectAssets_UnselectionChangedMsgRebuildsPartition(t *testing.T) {
	a1 := newAsset("a1", "A1", asset.TypeSkill, "")
	proj := &project.Manifest{ID: "proj-1", Name: "Proj", SelectedAssetIDs: []string{"a1"}}
	f := newSelectActionsFake([]*asset.Asset{a1}, proj)
	s := newSelectProjectAssetsScreen(f, "alpha", "proj-1")
	loadInto(t, s, f, "proj-1")

	_, _ = s.Update(selectionChangedMsg{selectedIDs: []string{}, info: "removed a1"})
	if slices.Contains(idsOf(s.selected), "a1") {
		t.Errorf("after change, selected still contains a1: %v", idsOf(s.selected))
	}
	if !slices.Contains(idsOf(s.available), "a1") {
		t.Errorf("after change, available = %v, want to contain a1", idsOf(s.available))
	}
}

// TestSelectProjectAssets_SelectOnAlreadySelectedIsIdempotent verifies
// that triggering select on an asset id the server already considers
// selected does not crash the screen and leaves a single occurrence. The
// fake echoes back the existing selection slice (mirrors the real
// Service.SelectAsset idempotency contract).
func TestSelectProjectAssets_SelectOnAlreadySelectedIsIdempotent(t *testing.T) {
	a1 := newAsset("a1", "A1", asset.TypeSkill, "")
	proj := &project.Manifest{ID: "proj-1", Name: "Proj", SelectedAssetIDs: []string{"a1"}}
	f := newSelectActionsFake([]*asset.Asset{a1}, proj)
	f.selectResult = []string{"a1"}
	s := newSelectProjectAssetsScreen(f, "alpha", "proj-1")
	loadInto(t, s, f, "proj-1")

	// Replay the selectionChangedMsg path with the same selection — the
	// partition must still place a1 in selected exactly once.
	_, _ = s.Update(selectionChangedMsg{selectedIDs: []string{"a1"}, info: "idempotent"})
	if got := idsOf(s.selected); !slices.Equal(got, []string{"a1"}) {
		t.Errorf("selected = %v, want [a1] (no duplicates after idempotent replay)", got)
	}
}

// TestSelectProjectAssets_UnselectOnEmptySelectionIsNoOp verifies the
// screen survives an unselect path on an empty selection — no panic,
// row mnemonic gated by `availableAtCursor` / `selectedAtCursor`, and
// the selection stays empty.
func TestSelectProjectAssets_UnselectOnEmptySelectionIsNoOp(t *testing.T) {
	a1 := newAsset("a1", "A1", asset.TypeSkill, "")
	proj := &project.Manifest{ID: "proj-1", Name: "Proj"}
	f := newSelectActionsFake([]*asset.Asset{a1}, proj)
	s := newSelectProjectAssetsScreen(f, "alpha", "proj-1")
	loadInto(t, s, f, "proj-1")
	_ = s.handler.FocusIndex(s.selectedIdx)
	s.rebuildSet()

	cmd := s.onUnselect()
	if cmd != nil {
		t.Errorf("onUnselect with empty selection produced a cmd; want nil no-op")
	}
	if len(f.unselectInputs) != 0 {
		t.Errorf("UnselectAsset called on empty selection: %v", f.unselectInputs)
	}
	if len(s.selected) != 0 {
		t.Errorf("selected = %v, want empty", idsOf(s.selected))
	}
}

func TestSelectProjectAssets_SelectErrorSurfacesAsMutationDoneMsg(t *testing.T) {
	a1 := newAsset("a1", "A1", asset.TypeSkill, "")
	proj := &project.Manifest{ID: "proj-1", Name: "Proj"}
	f := newSelectActionsFake([]*asset.Asset{a1}, proj)
	f.selectErr = stubDomainErr{msg: "save failed", sev: errs.SeverityError}
	s := newSelectProjectAssetsScreen(f, "alpha", "proj-1")
	loadInto(t, s, f, "proj-1")
	_ = s.handler.FocusIndex(s.availableIdx)
	s.rebuildSet()

	_, cmd := s.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	msg := drainCmd(t, cmd)
	done, ok := msg.(mutationDoneMsg)
	if !ok {
		t.Fatalf("got %T, want mutationDoneMsg", msg)
	}
	if !strings.Contains(done.text, "save failed") {
		t.Errorf("mutationDoneMsg.text = %q, want to contain %q", done.text, "save failed")
	}
	// Partition unchanged.
	if slices.Contains(idsOf(s.selected), "a1") {
		t.Errorf("selected partition mutated despite save error: %v", idsOf(s.selected))
	}
}

func TestSelectProjectAssets_PlanPushesPlanProjectScreen(t *testing.T) {
	f := newSelectActionsFake(nil, &project.Manifest{ID: "proj-1", Name: "Proj"})
	s := newSelectProjectAssetsScreen(f, "alpha", "proj-1")
	loadInto(t, s, f, "proj-1")

	_, cmd := s.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	push, ok := drainCmd(t, cmd).(PushScreenMsg)
	if !ok {
		t.Fatalf("got %T, want PushScreenMsg", cmd())
	}
	plan, ok := push.Screen.(*planProjectScreen)
	if !ok {
		t.Fatalf("pushed %T, want *planProjectScreen", push.Screen)
	}
	if plan.profileID != "alpha" || plan.projectID != "proj-1" {
		t.Errorf("screen ids = (%q, %q), want (alpha, proj-1)", plan.profileID, plan.projectID)
	}
}

func TestSelectProjectAssets_BackKeysPop(t *testing.T) {
	f := newSelectActionsFake(nil, &project.Manifest{ID: "proj-1", Name: "Proj"})
	s := newSelectProjectAssetsScreen(f, "alpha", "proj-1")
	loadInto(t, s, f, "proj-1")

	for _, kp := range []tea.KeyPressMsg{
		{Code: 'b', Text: "b"},
		{Code: tea.KeyEsc},
	} {
		_, cmd := s.Update(kp)
		if cmd == nil {
			t.Fatalf("%v produced nil cmd", kp)
		}
		if _, ok := cmd().(PopScreenMsg); !ok {
			t.Fatalf("%v cmd produced %T, want PopScreenMsg", kp, cmd())
		}
	}
}

func TestSelectProjectAssets_FocusJumpsAndCycles(t *testing.T) {
	a1 := newAsset("a1", "A1", asset.TypeSkill, "")
	a2 := newAsset("a2", "A2", asset.TypeSkill, "")
	proj := &project.Manifest{ID: "proj-1", Name: "Proj", SelectedAssetIDs: []string{"a1"}}
	f := newSelectActionsFake([]*asset.Asset{a1, a2}, proj)
	s := newSelectProjectAssetsScreen(f, "alpha", "proj-1")
	loadInto(t, s, f, "proj-1")

	// ctrl+2 jumps to Available. Note: KeyPressMsg.String() omits the
	// modifier prefix when Text is set, so leave Text empty for ctrl+digit.
	_, _ = s.Update(tea.KeyPressMsg{Code: '2', Mod: tea.ModCtrl})
	if s.handler.Focused() != s.availableIdx {
		t.Errorf("after ctrl+2 focus = %d, want %d", s.handler.Focused(), s.availableIdx)
	}
	// ctrl+1 jumps back to Selected.
	_, _ = s.Update(tea.KeyPressMsg{Code: '1', Mod: tea.ModCtrl})
	if s.handler.Focused() != s.selectedIdx {
		t.Errorf("after ctrl+1 focus = %d, want %d", s.handler.Focused(), s.selectedIdx)
	}
	// tab cycles forward.
	_, _ = s.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if s.handler.Focused() != s.availableIdx {
		t.Errorf("after tab focus = %d, want %d", s.handler.Focused(), s.availableIdx)
	}
}

func TestSelectProjectAssets_StatusBarContract(t *testing.T) {
	a1 := newAsset("a1", "A1", asset.TypeSkill, "")
	a2 := newAsset("a2", "A2", asset.TypeSkill, "")
	proj := &project.Manifest{ID: "proj-1", Name: "Proj", SelectedAssetIDs: []string{"a1"}}
	f := newSelectActionsFake([]*asset.Asset{a1, a2}, proj)
	s := newSelectProjectAssetsScreen(f, "alpha", "proj-1")
	loadInto(t, s, f, "proj-1")

	cases := []struct {
		name      string
		focusIdx  int
		wantHas   []string
		wantNoHas []string
	}{
		{"selected focused", s.selectedIdx, []string{"u", "b"}, []string{"l", "p", "1", "2"}},
		{"available focused", s.availableIdx, []string{"l", "b"}, []string{"u", "p", "1", "2"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_ = s.handler.FocusIndex(tc.focusIdx)
			s.rebuildSet()
			keys := s.StatusKeys()
			seen := map[string]bool{}
			for _, k := range keys {
				seen[k.Help().Key] = true
			}
			for _, want := range tc.wantHas {
				if !seen[want] {
					t.Errorf("StatusKeys missing %q (have %v)", want, keysOf(seen))
				}
			}
			for _, no := range tc.wantNoHas {
				if seen[no] {
					t.Errorf("StatusKeys must not contain %q (have %v)", no, keysOf(seen))
				}
			}
		})
	}
}

func TestSelectProjectAssets_EmptyFocusedTableDropsRowMnemonicFromStatusBar(t *testing.T) {
	a1 := newAsset("a1", "A1", asset.TypeSkill, "")
	proj := &project.Manifest{ID: "proj-1", Name: "Proj", SelectedAssetIDs: []string{"a1"}}
	f := newSelectActionsFake([]*asset.Asset{a1}, proj) // available is empty
	s := newSelectProjectAssetsScreen(f, "alpha", "proj-1")
	loadInto(t, s, f, "proj-1")
	_ = s.handler.FocusIndex(s.availableIdx)
	s.rebuildSet()

	keys := s.StatusKeys()
	for _, k := range keys {
		if k.Help().Key == "l" {
			t.Errorf("empty available focused: StatusKeys must drop [l] (have %v)", keys)
		}
	}
}

// TestSelectProjectAssets_MnemonicUniquenessExhaustive walks every focus
// + selection-state combination and asserts every registered button has
// a unique mnemonic rune. Reused safety pattern from the Edit Asset
// exhaustive test. selCount covers empty/half/full selected which also
// covers empty/half/full available implicitly (the two are complements
// of the two-asset universe).
func TestSelectProjectAssets_MnemonicUniquenessExhaustive(t *testing.T) {
	a1 := newAsset("a1", "A1", asset.TypeSkill, "")
	a2 := newAsset("a2", "A2", asset.TypeSkill, "")

	for selCount := 0; selCount <= 2; selCount++ {
		assets := []*asset.Asset{a1, a2}
		selected := []string{}
		if selCount >= 1 {
			selected = append(selected, "a1")
		}
		if selCount == 2 {
			selected = append(selected, "a2")
		}
		proj := &project.Manifest{ID: "proj-1", Name: "Proj", SelectedAssetIDs: selected}
		f := newSelectActionsFake(assets, proj)
		s := newSelectProjectAssetsScreen(f, "alpha", "proj-1")
		loadInto(t, s, f, "proj-1")

		for _, focusIdx := range []int{s.selectedIdx, s.availableIdx} {
			_ = s.handler.FocusIndex(focusIdx)
			s.rebuildSet()
			assertUniqueMnemonicsSPA(t, s.set, focusIdx, selCount)
		}
	}
}

func assertUniqueMnemonicsSPA(t *testing.T, set *mnemonic.Set, focus, selCount int) {
	t.Helper()
	seen := map[rune]string{}
	for _, b := range set.Buttons() {
		r := b.Mnemonic()
		if prev, ok := seen[r]; ok {
			t.Errorf("duplicate mnemonic %q for buttons %q + %q (focus=%d selCount=%d)",
				r, prev, b.Label(), focus, selCount)
		}
		seen[r] = b.Label()
	}
}

func keysOf(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// stubDomainErr is a minimal errs.DomainError implementation for test
// scaffolding. Real errors live in domain packages; tests just need a
// typed shape that carries a message + severity.
type stubDomainErr struct {
	msg string
	sev errs.Severity
}

func (e stubDomainErr) Error() string           { return e.msg }
func (e stubDomainErr) Severity() errs.Severity { return e.sev }
