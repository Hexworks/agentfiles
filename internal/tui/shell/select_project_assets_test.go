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
	"github.com/hexworks/agentfiles/internal/tui/components/mnemonic"
	"github.com/hexworks/agentfiles/internal/tui/notifications"
)

// Compile-time guard.
var _ Screen = (*selectProjectAssetsScreen)(nil)

// fakeSelectActions records every action invocation and returns the
// preconfigured error (nil by default). Used so screen tests stay
// hermetic — no filesystem, no real registry.
type fakeSelectActions struct {
	prof         *profile.Profile
	loadErr      errs.DomainError
	selectErr    errs.DomainError
	unselectErr  errs.DomainError
	selectCalls  []actions.SelectAssetInput
	unselectCall []actions.UnselectAssetInput
}

func (f *fakeSelectActions) LoadProfile(in actions.LoadProfileInput) (*profile.Profile, errs.DomainError) {
	return f.prof, f.loadErr
}

func (f *fakeSelectActions) SelectAsset(in actions.SelectAssetInput) (struct{}, errs.DomainError) {
	f.selectCalls = append(f.selectCalls, in)
	return struct{}{}, f.selectErr
}

func (f *fakeSelectActions) UnselectAsset(in actions.UnselectAssetInput) (struct{}, errs.DomainError) {
	f.unselectCall = append(f.unselectCall, in)
	return struct{}{}, f.unselectErr
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

func TestSelectProjectAssets_InitLoadsProfile(t *testing.T) {
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

func TestSelectProjectAssets_SelectMovesAssetAndCallsAction(t *testing.T) {
	a1 := newAsset("a1", "A1", asset.TypeSkill, "")
	a2 := newAsset("a2", "A2", asset.TypeSkill, "")
	proj := &project.Manifest{ID: "proj-1", Name: "Proj"}
	f := newSelectActionsFake([]*asset.Asset{a1, a2}, proj)
	s := newSelectProjectAssetsScreen(f, "alpha", "proj-1")
	loadInto(t, s, f, "proj-1")
	_ = s.handler.FocusIndex(s.availableIdx)
	s.rebuildSet()
	s.availableTable.SetCursor(0)

	_, cmd := s.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	if cmd == nil {
		t.Fatal("expected cmd")
	}
	msg := drainCmd(t, cmd)
	changed, ok := msg.(selectionChangedMsg)
	if !ok {
		t.Fatalf("got %T, want selectionChangedMsg", msg)
	}
	if !slices.Contains(changed.selectedIDs, "a1") {
		t.Errorf("new selectedIDs = %v, want to contain a1", changed.selectedIDs)
	}
	if len(f.selectCalls) != 1 || f.selectCalls[0].AssetID != "a1" {
		t.Errorf("SelectAsset calls = %v, want one call for a1", f.selectCalls)
	}

	// Apply the change and verify the partition rebuilt.
	_, _ = s.Update(changed)
	if !slices.Contains(idsOf(s.selected), "a1") {
		t.Errorf("after change, selected = %v, want to contain a1", idsOf(s.selected))
	}
	if slices.Contains(idsOf(s.available), "a1") {
		t.Errorf("after change, available still contains a1: %v", idsOf(s.available))
	}
}

func TestSelectProjectAssets_UnselectMovesAssetAndCallsAction(t *testing.T) {
	a1 := newAsset("a1", "A1", asset.TypeSkill, "")
	proj := &project.Manifest{ID: "proj-1", Name: "Proj", SelectedAssetIDs: []string{"a1"}}
	f := newSelectActionsFake([]*asset.Asset{a1}, proj)
	s := newSelectProjectAssetsScreen(f, "alpha", "proj-1")
	loadInto(t, s, f, "proj-1")
	_ = s.handler.FocusIndex(s.selectedIdx)
	s.rebuildSet()
	s.selectedTable.SetCursor(0)

	_, cmd := s.Update(tea.KeyPressMsg{Code: 'u', Text: "u"})
	if cmd == nil {
		t.Fatal("expected cmd")
	}
	msg := drainCmd(t, cmd)
	changed, ok := msg.(selectionChangedMsg)
	if !ok {
		t.Fatalf("got %T, want selectionChangedMsg", msg)
	}
	if slices.Contains(changed.selectedIDs, "a1") {
		t.Errorf("new selectedIDs = %v, want a1 removed", changed.selectedIDs)
	}
	if len(f.unselectCall) != 1 || f.unselectCall[0].AssetID != "a1" {
		t.Errorf("UnselectAsset calls = %v, want one call for a1", f.unselectCall)
	}

	_, _ = s.Update(changed)
	if slices.Contains(idsOf(s.selected), "a1") {
		t.Errorf("after change, selected still contains a1: %v", idsOf(s.selected))
	}
	if !slices.Contains(idsOf(s.available), "a1") {
		t.Errorf("after change, available = %v, want to contain a1", idsOf(s.available))
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

func TestSelectProjectAssets_PlanPushesPlanProjectStub(t *testing.T) {
	f := newSelectActionsFake(nil, &project.Manifest{ID: "proj-1", Name: "Proj"})
	s := newSelectProjectAssetsScreen(f, "alpha", "proj-1")
	loadInto(t, s, f, "proj-1")

	_, cmd := s.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	push, ok := drainCmd(t, cmd).(PushScreenMsg)
	if !ok {
		t.Fatalf("got %T, want PushScreenMsg", cmd())
	}
	stub, ok := push.Screen.(*planProjectStub)
	if !ok {
		t.Fatalf("pushed %T, want *planProjectStub", push.Screen)
	}
	if stub.profileID != "alpha" || stub.projectID != "proj-1" {
		t.Errorf("stub ids = (%q, %q), want (alpha, proj-1)", stub.profileID, stub.projectID)
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
// exhaustive test.
func TestSelectProjectAssets_MnemonicUniquenessExhaustive(t *testing.T) {
	a1 := newAsset("a1", "A1", asset.TypeSkill, "")
	a2 := newAsset("a2", "A2", asset.TypeSkill, "")

	for selCount := 0; selCount <= 2; selCount++ {
		for avlExtra := 0; avlExtra <= 1; avlExtra++ {
			assets := []*asset.Asset{a1, a2}
			selected := []string{}
			if selCount >= 1 {
				selected = append(selected, "a1")
			}
			if selCount == 2 {
				selected = append(selected, "a2")
			}
			if avlExtra == 0 {
				// shrink available to 0 by selecting everything not already selected.
				// (already covered by selCount=2)
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
