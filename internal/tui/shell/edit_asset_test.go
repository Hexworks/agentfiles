package shell

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/hexworks/agentfiles/internal/actions"
	"github.com/hexworks/agentfiles/internal/app"
	"github.com/hexworks/agentfiles/internal/asset"
	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/registry"
	"github.com/hexworks/agentfiles/internal/tui/components/modal"
	"github.com/hexworks/agentfiles/internal/tui/editor"
	"github.com/hexworks/agentfiles/internal/tui/modals"
)

// editAssetFixture wires a real registry-backed Actions handle and
// seeds one profile + one asset so tests can exercise LoadAsset /
// UpdateAsset end-to-end.
type editAssetFixture struct {
	Root    string
	Service *app.Service
	Actions *actions.Actions
	Profile *registry.ProfileRef
	AssetID string
	AssetDir string
}

func newEditAssetFixture(t *testing.T, assetName string, assetType asset.Type) *editAssetFixture {
	t.Helper()
	root := t.TempDir()
	svc := app.New(filepath.Join(root, "registry.json"))
	ref, err := svc.CreateProfile("alpha", filepath.Join(root, "alpha"))
	if err != nil {
		t.Fatalf("seed profile: %v", err)
	}
	dir, err := svc.InitAsset(ref.ID, asset.Manifest{Name: assetName, Type: assetType})
	if err != nil {
		t.Fatalf("seed asset: %v", err)
	}
	// Service derives the asset id by slugging the name; reload the
	// profile to pick up the canonical id rather than assuming the
	// slug rule.
	prof, lerr := svc.LoadProfile(ref.ID)
	if lerr != nil {
		t.Fatalf("LoadProfile: %v", lerr)
	}
	var id string
	for _, a := range prof.Assets {
		if a.Dir == dir {
			id = a.ID
			break
		}
	}
	if id == "" {
		t.Fatalf("seeded asset id not found in profile")
	}
	return &editAssetFixture{
		Root:     root,
		Service:  svc,
		Actions:  actions.New(svc),
		Profile:  ref,
		AssetID:  id,
		AssetDir: dir,
	}
}

func (f *editAssetFixture) seedFile(t *testing.T, rel string, body string) {
	t.Helper()
	abs := filepath.Join(f.AssetDir, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("mkdir for %q: %v", rel, err)
	}
	if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
		t.Fatalf("write %q: %v", rel, err)
	}
}

// loadInto loads the asset via the real action so the screen reaches
// the same state production would.
func (f *editAssetFixture) loadInto(t *testing.T, s *editAssetScreen) {
	t.Helper()
	msg := drainCmd(t, s.Init())
	loaded, ok := msg.(editAssetLoadedMsg)
	if !ok {
		t.Fatalf("Init produced %T, want editAssetLoadedMsg", msg)
	}
	if _, cmd := s.Update(loaded); cmd != nil {
		// drain follow-ups (focus cmd, etc.) so the test ends in a clean state.
		_ = drainCmd(t, cmd)
	}
}

// fakeLoadedAsset materializes an *asset.Asset rooted at dir with the
// given manifest + seeded files (one byte per file). Used by UI-state
// tests that need a directory layout without round-tripping through the
// service.
func fakeLoadedAsset(t *testing.T, dir string, m asset.Manifest, relFiles []string) *asset.Asset {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for _, rel := range relFiles {
		abs := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(abs, []byte{0x20}, 0o644); err != nil {
			t.Fatalf("write %q: %v", rel, err)
		}
	}
	if m.ID == "" {
		m.ID = "test-asset"
	}
	if m.Name == "" {
		m.Name = "test-asset"
	}
	if m.Type == "" {
		m.Type = asset.TypeSkill
	}
	return &asset.Asset{Manifest: m, Dir: dir}
}

// pushFakeLoad drives a fake asset into the screen without spinning the
// service. Useful for UI-state tests.
func pushFakeLoad(t *testing.T, s *editAssetScreen, a *asset.Asset) {
	t.Helper()
	if _, cmd := s.Update(editAssetLoadedMsg{a: a}); cmd != nil {
		_ = drainCmd(t, cmd)
	}
}

// advanceTreeUntilRel walks the treetable cursor down until SelectedNode's
// relative path matches target. Fails the test if the walk wraps without
// finding the target — keeps an infinite loop from happening on a typo.
func advanceTreeUntilRel(t *testing.T, s *editAssetScreen, target string) {
	t.Helper()
	const maxSteps = 64
	for i := 0; i < maxSteps; i++ {
		if s.selectedRel() == target {
			s.rebuildSet()
			return
		}
		_, _ = s.tree.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	t.Fatalf("treetable never reached rel=%q within %d steps (current=%q)", target, maxSteps, s.selectedRel())
}

func TestEditAssetScreen_InitLoadsAssetFromActions(t *testing.T) {
	f := newEditAssetFixture(t, "Skill One", asset.TypeSkill)
	s := newEditAssetScreen(f.Actions, f.Profile.ID, f.AssetID)

	msg := drainCmd(t, s.Init())

	loaded, ok := msg.(editAssetLoadedMsg)
	if !ok {
		t.Fatalf("Init produced %T, want editAssetLoadedMsg", msg)
	}
	if loaded.err != nil {
		t.Fatalf("loaded.err = %v, want nil", loaded.err)
	}
	if loaded.a == nil {
		t.Fatalf("loaded.a = nil, want non-nil asset")
	}
	if loaded.a.ID != f.AssetID {
		t.Errorf("loaded asset id = %q, want %q", loaded.a.ID, f.AssetID)
	}
}

func TestEditAssetScreen_TreetableFocusedLeafSet_HasExpectedMnemonics(t *testing.T) {
	f := newEditAssetFixture(t, "skill", asset.TypeSkill)
	f.seedFile(t, "leaf.txt", "x")
	s := newEditAssetScreen(f.Actions, f.Profile.ID, f.AssetID)
	f.loadInto(t, s)

	// Move cursor to the leaf row (root is row 0; leaf is row 1).
	_, _ = s.tree.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	s.rebuildSet()

	got := mnemonicLabels(s.set)
	want := []string{"1", "2", "Open", "Delete", "Add", "Save", "Back"}
	if !labelsEqual(got, want) {
		t.Errorf("set labels (leaf) = %v, want %v", got, want)
	}
}

func TestEditAssetScreen_TreetableFocusedDirectorySet_OmitsOpen(t *testing.T) {
	// TypeRule has no scaffold file, so the tree is exactly root → scripts/ → inner.sh.
	f := newEditAssetFixture(t, "rule", asset.TypeRule)
	f.seedFile(t, "scripts/inner.sh", "x")
	s := newEditAssetScreen(f.Actions, f.Profile.ID, f.AssetID)
	f.loadInto(t, s)

	// Move cursor to the "scripts/" directory row (row 1).
	_, _ = s.tree.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	s.rebuildSet()
	if got := s.selectedKind(); got != kindAssetDir {
		t.Fatalf("selectedKind = %v, want kindAssetDir at rel=%q", got, s.selectedRel())
	}

	got := mnemonicLabels(s.set)
	want := []string{"1", "2", "Delete", "Add", "Save", "Back"}
	if !labelsEqual(got, want) {
		t.Errorf("set labels (dir) = %v, want %v", got, want)
	}
}

func TestEditAssetScreen_TreetableFocusedEmptyFolder_OmitsRowMnemonics(t *testing.T) {
	f := newEditAssetFixture(t, "rule", asset.TypeRule)
	// TypeRule produces no scaffolded file → asset folder starts empty.
	s := newEditAssetScreen(f.Actions, f.Profile.ID, f.AssetID)
	f.loadInto(t, s)

	got := mnemonicLabels(s.set)
	want := []string{"1", "2", "Add", "Save", "Back"}
	if !labelsEqual(got, want) {
		t.Errorf("set labels (empty) = %v, want %v", got, want)
	}
}

func TestEditAssetScreen_RightColumnFocusedSet_IsEmpty(t *testing.T) {
	f := newEditAssetFixture(t, "skill", asset.TypeSkill)
	s := newEditAssetScreen(f.Actions, f.Profile.ID, f.AssetID)
	f.loadInto(t, s)

	_ = s.handler.FocusIndex(1) // description
	s.rebuildSet()

	if got := mnemonicLabels(s.set); len(got) != 0 {
		t.Errorf("set labels (description focused) = %v, want empty", got)
	}

	_ = s.handler.FocusIndex(2) // tags
	s.rebuildSet()
	if got := mnemonicLabels(s.set); len(got) != 0 {
		t.Errorf("set labels (tags focused) = %v, want empty", got)
	}
}

func TestEditAssetScreen_MnemonicUniquenessExhaustive(t *testing.T) {
	f := newEditAssetFixture(t, "skill", asset.TypeSkill)
	f.seedFile(t, "scripts/run.sh", "x")
	f.seedFile(t, "doc.md", "x")
	s := newEditAssetScreen(f.Actions, f.Profile.ID, f.AssetID)
	f.loadInto(t, s)

	for focusIdx := 0; focusIdx < 5; focusIdx++ {
		_ = s.handler.FocusIndex(focusIdx)
		// Walk cursor across every row.
		s.tree.SetRoot(buildAssetTree(s.asset, s.files))
		for row := 0; row < len(s.files)+3; row++ {
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("rebuildSet panic at focus=%d row=%d: %v", focusIdx, row, r)
					}
				}()
				s.rebuildSet()
			}()
			_, _ = s.tree.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		}
	}
}

func TestEditAssetScreen_OpenButtonRenderedOnlyOnLeafNodes(t *testing.T) {
	// TypeRule keeps the asset folder empty except for what the test seeds:
	// scripts/ (dir) → run.sh (leaf). Row 0 root, row 1 scripts/, row 2 run.sh.
	f := newEditAssetFixture(t, "rule", asset.TypeRule)
	f.seedFile(t, "scripts/run.sh", "x")
	s := newEditAssetScreen(f.Actions, f.Profile.ID, f.AssetID)
	f.loadInto(t, s)

	// Cursor on root (row 0): no Open.
	if got := stripANSI(s.tree.View()); strings.Contains(got, "[Open]") {
		t.Errorf("root row shows [Open]:\n%s", got)
	}

	// Move to the directory row.
	_, _ = s.tree.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if got := s.selectedKind(); got != kindAssetDir {
		t.Fatalf("after Down: selectedKind = %v, want kindAssetDir; rel=%q", got, s.selectedRel())
	}
	if got := stripANSI(s.tree.View()); strings.Contains(got, "[Open]") {
		t.Errorf("directory row shows [Open]:\n%s", got)
	}

	// Move to the leaf row.
	_, _ = s.tree.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if got := s.selectedKind(); got != kindAssetFile {
		t.Fatalf("after second Down: selectedKind = %v, want kindAssetFile; rel=%q", got, s.selectedRel())
	}
	leafView := stripANSI(s.tree.View())
	if !strings.Contains(leafView, "[Open]") {
		t.Errorf("leaf row missing [Open]:\n%s", leafView)
	}
}

func TestEditAssetScreen_OpenLeafTriggersEditor(t *testing.T) {
	f := newEditAssetFixture(t, "skill", asset.TypeSkill)
	f.seedFile(t, "leaf.txt", "x")
	s := newEditAssetScreen(f.Actions, f.Profile.ID, f.AssetID)
	f.loadInto(t, s)

	// Move to leaf row.
	_, _ = s.tree.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	s.rebuildSet()

	if got := s.onOpen(); got == nil {
		t.Fatalf("onOpen on leaf returned nil cmd")
	}
	if s.pendingOpenFile == "" {
		t.Errorf("pendingOpenFile = empty, want leaf.txt")
	}
}

func TestEditAssetScreen_EditorFinishedTriggersUpdateAsset(t *testing.T) {
	f := newEditAssetFixture(t, "skill", asset.TypeSkill)
	f.seedFile(t, "leaf.txt", "x")
	s := newEditAssetScreen(f.Actions, f.Profile.ID, f.AssetID)
	f.loadInto(t, s)
	s.pendingOpenFile = "leaf.txt"

	_, cmd := s.Update(editor.FinishedMsg{Err: nil})
	done, ok := drainCmd(t, cmd).(mutationDoneMsg)
	if !ok {
		t.Fatalf("editor.FinishedMsg produced %T, want mutationDoneMsg", drainCmd(t, cmd))
	}
	if done.severity != errs.SeverityInfo {
		t.Errorf("severity = %v, want SeverityInfo", done.severity)
	}
}

func TestEditAssetScreen_DeleteFileConfirmedRemovesFileAndCallsUpdate(t *testing.T) {
	f := newEditAssetFixture(t, "rule", asset.TypeRule)
	f.seedFile(t, "doomed.txt", "x")
	s := newEditAssetScreen(f.Actions, f.Profile.ID, f.AssetID)
	f.loadInto(t, s)

	target := "doomed.txt"
	advanceTreeUntilRel(t, s, target)

	_ = s.onDeleteFile()
	if s.modal == nil || s.modalKind != ModalKindAssetDeleteFile {
		t.Fatalf("modal not opened: modal=%v kind=%v", s.modal, s.modalKind)
	}
	if s.pendingDeleteFile != target {
		t.Errorf("pendingDeleteFile = %q, want %q", s.pendingDeleteFile, target)
	}

	_, cmd := s.Update(modal.ResolvedMsg{ID: "delete-file", Confirmed: true})
	done, ok := drainCmd(t, cmd).(mutationDoneMsg)
	if !ok {
		t.Fatalf("delete cmd produced %T, want mutationDoneMsg", drainCmd(t, cmd))
	}
	if done.severity != errs.SeverityInfo {
		t.Errorf("severity = %v, want SeverityInfo, text=%q", done.severity, done.text)
	}
	if _, err := os.Stat(filepath.Join(f.AssetDir, target)); !os.IsNotExist(err) {
		t.Errorf("file still exists: %v", err)
	}
}

func TestEditAssetScreen_DeleteFileRejectedMakesNoFsChange(t *testing.T) {
	f := newEditAssetFixture(t, "rule", asset.TypeRule)
	f.seedFile(t, "kept.txt", "x")
	s := newEditAssetScreen(f.Actions, f.Profile.ID, f.AssetID)
	f.loadInto(t, s)

	target := "kept.txt"
	advanceTreeUntilRel(t, s, target)
	_ = s.onDeleteFile()

	_, cmd := s.Update(modal.ResolvedMsg{ID: "delete-file", Confirmed: false})
	if cmd != nil {
		t.Errorf("Reject cmd = %v, want nil", cmd)
	}
	if s.modal != nil {
		t.Errorf("modal still open after No")
	}
	if s.pendingDeleteFile != "" {
		t.Errorf("pendingDeleteFile = %q, want empty", s.pendingDeleteFile)
	}
	if _, err := os.Stat(filepath.Join(f.AssetDir, target)); err != nil {
		t.Errorf("file gone: %v", err)
	}
}

func TestEditAssetScreen_AddFileConfirmedCreatesPhysicalFile(t *testing.T) {
	f := newEditAssetFixture(t, "skill", asset.TypeSkill)
	s := newEditAssetScreen(f.Actions, f.Profile.ID, f.AssetID)
	f.loadInto(t, s)

	if cmd := s.onAdd(); cmd == nil {
		t.Fatalf("onAdd returned nil cmd")
	}

	_, cmd := s.Update(modal.ResolvedMsg{
		ID:        "create-file",
		Confirmed: true,
		Value:     modals.CreateFileInput{Path: "scripts/hello.sh"},
	})
	done, ok := drainCmd(t, cmd).(mutationDoneMsg)
	if !ok {
		t.Fatalf("add cmd produced %T, want mutationDoneMsg", drainCmd(t, cmd))
	}
	if done.severity != errs.SeverityInfo {
		t.Errorf("severity = %v, want SeverityInfo", done.severity)
	}
	if _, err := os.Stat(filepath.Join(f.AssetDir, "scripts", "hello.sh")); err != nil {
		t.Errorf("file not created: %v", err)
	}
}

func TestEditAssetScreen_TagsRoundTrip(t *testing.T) {
	f := newEditAssetFixture(t, "skill", asset.TypeSkill)
	dir := filepath.Join(t.TempDir(), "asset")
	a := fakeLoadedAsset(t, dir, asset.Manifest{
		ID:   "test",
		Name: "test",
		Type: asset.TypeSkill,
		Tags: []string{"foo", "bar"},
	}, nil)

	s := newEditAssetScreen(f.Actions, f.Profile.ID, f.AssetID)
	pushFakeLoad(t, s, a)

	if got := s.state.tagsCSV; got != "foo, bar" {
		t.Errorf("tagsCSV = %q, want %q", got, "foo, bar")
	}

	s.state.tagsCSV = "baz, qux"
	s.syncFieldsToAsset()
	if got := s.asset.Tags; !slices.Equal(got, []string{"baz", "qux"}) {
		t.Errorf("Tags = %v, want [baz qux]", got)
	}
}

func TestEditAssetScreen_SyncCopiesFieldsToAsset(t *testing.T) {
	f := newEditAssetFixture(t, "skill", asset.TypeSkill)
	dir := filepath.Join(t.TempDir(), "asset")
	a := fakeLoadedAsset(t, dir, asset.Manifest{
		ID:   "test",
		Name: "test",
		Type: asset.TypeSkill,
	}, nil)

	s := newEditAssetScreen(f.Actions, f.Profile.ID, f.AssetID)
	pushFakeLoad(t, s, a)

	s.state.description = "hello"
	s.state.tagsCSV = "a, b"
	s.state.compatible = []string{"codex"}
	s.state.exclusive = "main"

	s.syncFieldsToAsset()

	if s.asset.Description != "hello" {
		t.Errorf("Description = %q, want hello", s.asset.Description)
	}
	if !slices.Equal(s.asset.Tags, []string{"a", "b"}) {
		t.Errorf("Tags = %v, want [a b]", s.asset.Tags)
	}
	if !slices.Equal(s.asset.CompatibleAgents, []string{"codex"}) {
		t.Errorf("CompatibleAgents = %v, want [codex]", s.asset.CompatibleAgents)
	}
	if s.asset.ExclusiveGroup != "main" {
		t.Errorf("ExclusiveGroup = %q, want main", s.asset.ExclusiveGroup)
	}
}

func TestEditAssetScreen_BackWithCleanChangesPopsDirectly(t *testing.T) {
	f := newEditAssetFixture(t, "skill", asset.TypeSkill)
	s := newEditAssetScreen(f.Actions, f.Profile.ID, f.AssetID)
	f.loadInto(t, s)

	cmd := s.onBack()
	if cmd == nil {
		t.Fatalf("onBack returned nil cmd")
	}
	if _, ok := cmd().(PopScreenMsg); !ok {
		t.Fatalf("cmd produced %T, want PopScreenMsg", cmd())
	}
}

func TestEditAssetScreen_BackWithUnsavedChangesOpensConfirmModal(t *testing.T) {
	f := newEditAssetFixture(t, "skill", asset.TypeSkill)
	s := newEditAssetScreen(f.Actions, f.Profile.ID, f.AssetID)
	f.loadInto(t, s)

	s.state.tagsCSV = "newtag"
	_ = s.onBack()
	if s.modal == nil {
		t.Fatalf("modal not opened")
	}
	if s.modalKind != ModalKindAssetBackUnsaved {
		t.Errorf("modalKind = %v, want ModalKindAssetBackUnsaved", s.modalKind)
	}
}

func TestEditAssetScreen_BackUnsavedConfirmedPops(t *testing.T) {
	f := newEditAssetFixture(t, "skill", asset.TypeSkill)
	s := newEditAssetScreen(f.Actions, f.Profile.ID, f.AssetID)
	f.loadInto(t, s)
	s.state.tagsCSV = "newtag"
	_ = s.onBack()

	_, cmd := s.Update(modal.ResolvedMsg{ID: "back-unsaved", Confirmed: true})
	if cmd == nil {
		t.Fatalf("confirmed cmd = nil, want PopScreenMsg")
	}
	if _, ok := cmd().(PopScreenMsg); !ok {
		t.Fatalf("cmd produced %T, want PopScreenMsg", cmd())
	}
}

func TestEditAssetScreen_BackUnsavedRejectedKeepsScreen(t *testing.T) {
	f := newEditAssetFixture(t, "skill", asset.TypeSkill)
	s := newEditAssetScreen(f.Actions, f.Profile.ID, f.AssetID)
	f.loadInto(t, s)
	s.state.tagsCSV = "newtag"
	_ = s.onBack()

	_, cmd := s.Update(modal.ResolvedMsg{ID: "back-unsaved", Confirmed: false})
	if cmd != nil {
		t.Errorf("rejected cmd = %v, want nil", cmd)
	}
	if s.modal != nil {
		t.Errorf("modal not cleared after No")
	}
}

func TestEditAssetScreen_SavePersistsManifest(t *testing.T) {
	f := newEditAssetFixture(t, "skill", asset.TypeSkill)
	s := newEditAssetScreen(f.Actions, f.Profile.ID, f.AssetID)
	f.loadInto(t, s)

	s.state.description = "updated body"

	_, cmd := s.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	done, ok := drainCmd(t, cmd).(mutationDoneMsg)
	if !ok {
		t.Fatalf("save cmd produced %T, want mutationDoneMsg", drainCmd(t, cmd))
	}
	if done.severity != errs.SeverityInfo {
		t.Errorf("severity = %v, want SeverityInfo", done.severity)
	}

	fresh, lerr := f.Service.LoadAsset(f.Profile.ID, f.AssetID)
	if lerr != nil {
		t.Fatalf("LoadAsset after save: %v", lerr)
	}
	if fresh.Description != "updated body" {
		t.Errorf("description after save = %q, want %q", fresh.Description, "updated body")
	}
}

func TestEditAssetScreen_StatusKeysTreetableFocused_IncludesSaveAndBack(t *testing.T) {
	f := newEditAssetFixture(t, "skill", asset.TypeSkill)
	f.seedFile(t, "leaf.txt", "x")
	s := newEditAssetScreen(f.Actions, f.Profile.ID, f.AssetID)
	f.loadInto(t, s)

	// cursor on leaf row.
	for s.selectedRel() != "leaf.txt" {
		_, _ = s.tree.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	s.rebuildSet()

	keys := s.StatusKeys()
	helps := make([]string, len(keys))
	for i, k := range keys {
		helps[i] = k.Help().Key
	}
	want := []string{"o", "d", "e", "b"}
	if !labelsEqual(helps, want) {
		t.Errorf("StatusKeys (leaf focus) = %v, want %v", helps, want)
	}
}

func TestEditAssetScreen_StatusKeysExcludesAdd(t *testing.T) {
	f := newEditAssetFixture(t, "skill", asset.TypeSkill)
	s := newEditAssetScreen(f.Actions, f.Profile.ID, f.AssetID)
	f.loadInto(t, s)

	for _, k := range s.StatusKeys() {
		if k.Help().Key == "a" {
			t.Errorf("StatusKeys includes screen-level [Add]")
		}
	}
}

func TestEditAssetScreen_RightColumnFocusedStatusBarSkipsRowKeys(t *testing.T) {
	f := newEditAssetFixture(t, "skill", asset.TypeSkill)
	f.seedFile(t, "leaf.txt", "x")
	s := newEditAssetScreen(f.Actions, f.Profile.ID, f.AssetID)
	f.loadInto(t, s)
	_ = s.handler.FocusIndex(1)
	s.rebuildSet()

	keys := s.StatusKeys()
	for _, k := range keys {
		switch k.Help().Key {
		case "o", "d":
			t.Errorf("StatusKeys (description focus) leaks row key %q", k.Help().Key)
		}
	}
	// Save + Back must still be advertised.
	helps := make(map[string]bool)
	for _, k := range keys {
		helps[k.Help().Key] = true
	}
	if !helps["e"] || !helps["b"] {
		t.Errorf("StatusKeys missing e/b: %v", keys)
	}
}

func TestEditAssetScreen_LoadErrorEmitsNotification(t *testing.T) {
	f := newEditAssetFixture(t, "skill", asset.TypeSkill)
	s := newEditAssetScreen(f.Actions, f.Profile.ID, "no-such-asset")

	loaded, ok := drainCmd(t, s.Init()).(editAssetLoadedMsg)
	if !ok {
		t.Fatalf("Init produced wrong msg")
	}
	_, cmd := s.Update(loaded)
	if cmd == nil {
		t.Fatalf("no notification cmd after load error")
	}
}

func TestEditAssetScreen_InputFocused_FollowsHandler(t *testing.T) {
	f := newEditAssetFixture(t, "skill", asset.TypeSkill)
	s := newEditAssetScreen(f.Actions, f.Profile.ID, f.AssetID)
	f.loadInto(t, s)

	if s.InputFocused() {
		t.Errorf("InputFocused() = true with treetable focus, want false")
	}
	_ = s.handler.FocusIndex(1)
	if !s.InputFocused() {
		t.Errorf("InputFocused() = false with description focused, want true")
	}
	_ = s.handler.FocusIndex(0)
	if s.InputFocused() {
		t.Errorf("InputFocused() = true after returning to treetable, want false")
	}
}

func TestShell_GlobalKeysSkippedWhenInputFocused(t *testing.T) {
	f := newEditAssetFixture(t, "skill", asset.TypeSkill)
	s := newEditAssetScreen(f.Actions, f.Profile.ID, f.AssetID)

	if screenWantsRawKey(s, tea.KeyPressMsg{Code: 's', Text: "s"}) {
		t.Errorf("treetable-focused screen wants raw 's', should not")
	}
	f.loadInto(t, s)
	_ = s.handler.FocusIndex(1)

	for _, k := range []tea.KeyPressMsg{
		{Code: 's', Text: "s"},
		{Code: 'n', Text: "n"},
		{Code: '?', Text: "?"},
		{Code: 'q', Text: "q"},
	} {
		if !screenWantsRawKey(s, k) {
			t.Errorf("screenWantsRawKey(%q) = false when input focused, want true", k.Text)
		}
	}
	// ctrl+c always escapes — safety rail.
	ctrlC := tea.KeyPressMsg{Code: 'c', Text: "", Mod: tea.ModCtrl}
	if screenWantsRawKey(s, ctrlC) {
		t.Errorf("screenWantsRawKey(ctrl+c) = true, want false (safety abort path)")
	}
}

// stripANSI strips every CSI SGR escape sequence so substring assertions
// like "[Open]" survive lipgloss's per-rune style switching. The codes
// match the form `\x1b[...m`; we accept any digits / semicolons in
// between.
func stripANSI(s string) string {
	out := make([]byte, 0, len(s))
	i := 0
	for i < len(s) {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && s[j] != 'm' {
				j++
			}
			if j < len(s) {
				i = j + 1
				continue
			}
		}
		out = append(out, s[i])
		i++
	}
	return string(out)
}

// Compile-time guard: editAssetScreen satisfies Screen.
var _ Screen = (*editAssetScreen)(nil)
