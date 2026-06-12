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
	"github.com/hexworks/agentfiles/internal/tui/components/mnemonic"
	"github.com/hexworks/agentfiles/internal/tui/components/modal"
	"github.com/hexworks/agentfiles/internal/tui/components/treetable"
	"github.com/hexworks/agentfiles/internal/tui/editor"
	"github.com/hexworks/agentfiles/internal/tui/modals"
)

// editAssetFixture wires a real registry-backed Actions handle and
// seeds one profile + one asset so tests can exercise LoadAsset /
// UpdateAsset end-to-end.
type editAssetFixture struct {
	Root     string
	Service  *app.Service
	Actions  *actions.Actions
	Profile  *registry.ProfileRef
	AssetID  string
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

func pushFakeLoad(t *testing.T, s *editAssetScreen, a *asset.Asset) {
	t.Helper()
	if _, cmd := s.Update(editAssetLoadedMsg{a: a}); cmd != nil {
		_ = drainCmd(t, cmd)
	}
}

// advanceTreeUntilRel walks the treetable cursor down until the cursor
// node's relative path matches target.
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

	_, _ = s.tree.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	s.rebuildSet()

	got := mnemonicLabels(s.set)
	want := []string{"1", "2", "Open", "Delete", "Add", "Save", "Back"}
	if !labelsEqual(got, want) {
		t.Errorf("set labels (leaf) = %v, want %v", got, want)
	}
}

func TestEditAssetScreen_TreetableFocusedDirectorySet_OmitsOpen(t *testing.T) {
	f := newEditAssetFixture(t, "rule", asset.TypeRule)
	f.seedFile(t, "scripts/inner.sh", "x")
	s := newEditAssetScreen(f.Actions, f.Profile.ID, f.AssetID)
	f.loadInto(t, s)

	_, _ = s.tree.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	s.rebuildSet()
	if got := s.selectedKind(); got != nodeDir {
		t.Fatalf("selectedKind = %v, want nodeDir at rel=%q", got, s.selectedRel())
	}

	got := mnemonicLabels(s.set)
	want := []string{"1", "2", "Delete", "Add", "Save", "Back"}
	if !labelsEqual(got, want) {
		t.Errorf("set labels (dir) = %v, want %v", got, want)
	}
}

func TestEditAssetScreen_TreetableFocusedEmptyFolder_OmitsRowMnemonics(t *testing.T) {
	f := newEditAssetFixture(t, "rule", asset.TypeRule)
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

	_ = s.handler.FocusIndex(s.descIdx)
	s.rebuildSet()
	if got := mnemonicLabels(s.set); len(got) != 0 {
		t.Errorf("set labels (description focused) = %v, want empty", got)
	}

	_ = s.handler.FocusIndex(s.tagsIdx)
	s.rebuildSet()
	if got := mnemonicLabels(s.set); len(got) != 0 {
		t.Errorf("set labels (tags focused) = %v, want empty", got)
	}
}

// TestEditAssetScreen_MnemonicUniquenessExhaustive walks (focus index)
// × (cursor state) and asserts both no-panic AND no-duplicate-mnemonic
// across visible buttons. The cross-product covers the three load-bearing
// cursor states: leaf, directory, empty folder.
func TestEditAssetScreen_MnemonicUniquenessExhaustive(t *testing.T) {
	cases := []struct {
		name  string
		seed  func(*editAssetFixture, *testing.T)
		atype asset.Type
	}{
		{"leaf-and-dir", func(f *editAssetFixture, t *testing.T) {
			f.seedFile(t, "scripts/run.sh", "x")
			f.seedFile(t, "doc.md", "x")
		}, asset.TypeSkill},
		{"dir-only", func(f *editAssetFixture, t *testing.T) {
			f.seedFile(t, "scripts/inner.sh", "x")
		}, asset.TypeRule},
		{"empty", func(f *editAssetFixture, t *testing.T) {}, asset.TypeRule},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newEditAssetFixture(t, "asset", tc.atype)
			tc.seed(f, t)
			s := newEditAssetScreen(f.Actions, f.Profile.ID, f.AssetID)
			f.loadInto(t, s)
			for focusIdx := 0; focusIdx < 5; focusIdx++ {
				_ = s.handler.FocusIndex(focusIdx)
				// Walk cursor across every row plus a buffer.
				rowMax := len(s.files) + 3
				for row := 0; row < rowMax; row++ {
					func() {
						defer func() {
							if r := recover(); r != nil {
								t.Errorf("rebuildSet panic at focus=%d row=%d: %v", focusIdx, row, r)
							}
						}()
						s.rebuildSet()
					}()
					assertUniqueMnemonics(t, s.set, focusIdx, row)
					_, _ = s.tree.Update(tea.KeyPressMsg{Code: tea.KeyDown})
				}
			}
		})
	}
}

func assertUniqueMnemonics(t *testing.T, set *mnemonic.Set, focusIdx, row int) {
	t.Helper()
	seen := make(map[rune]string)
	for _, b := range set.Buttons() {
		r := b.Mnemonic()
		if prev, dup := seen[r]; dup {
			t.Errorf("duplicate mnemonic %q at focus=%d row=%d: %q vs %q", r, focusIdx, row, prev, b.Label())
			continue
		}
		seen[r] = b.Label()
	}
}

func TestEditAssetScreen_TreeActionsForLeafAndDir(t *testing.T) {
	f := newEditAssetFixture(t, "rule", asset.TypeRule)
	f.seedFile(t, "scripts/run.sh", "x")
	s := newEditAssetScreen(f.Actions, f.Profile.ID, f.AssetID)
	f.loadInto(t, s)
	fn := s.treeActionsFn()

	leafNode := &treetable.Node{Data: assetNode{kind: nodeFile, rel: "scripts/run.sh"}}
	dirNode := &treetable.Node{Data: assetNode{kind: nodeDir, rel: "scripts"}}
	rootNode := &treetable.Node{Data: assetNode{kind: nodeRoot}}

	if got := fn(leafNode); len(got) != 2 {
		t.Errorf("leaf actions = %d buttons, want 2 ([Open] [Delete])", len(got))
	}
	if got := fn(dirNode); len(got) != 1 {
		t.Errorf("dir actions = %d buttons, want 1 ([Delete])", len(got))
	}
	if got := fn(rootNode); got != nil {
		t.Errorf("root actions = %v, want nil", got)
	}
}

func TestEditAssetScreen_OpenButtonRenderedOnlyOnLeafLeafRow(t *testing.T) {
	f := newEditAssetFixture(t, "rule", asset.TypeRule)
	f.seedFile(t, "scripts/run.sh", "x")
	s := newEditAssetScreen(f.Actions, f.Profile.ID, f.AssetID)
	f.loadInto(t, s)

	// Walk to leaf to verify the smoke test: rendered output contains [Open].
	advanceTreeUntilRel(t, s, "scripts/run.sh")
	if got := stripANSI(s.tree.View()); !strings.Contains(got, "[Open]") {
		t.Errorf("leaf row missing [Open]:\n%s", got)
	}
}

func TestEditAssetScreen_OpenLeafTriggersEditor(t *testing.T) {
	f := newEditAssetFixture(t, "skill", asset.TypeSkill)
	f.seedFile(t, "leaf.txt", "x")
	s := newEditAssetScreen(f.Actions, f.Profile.ID, f.AssetID)
	f.loadInto(t, s)

	_, _ = s.tree.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	s.rebuildSet()

	if got := s.onOpen(); got == nil {
		t.Fatalf("onOpen on leaf returned nil cmd")
	}
	if s.editingFile == "" {
		t.Errorf("editingFile = empty, want leaf.txt")
	}
}

func TestEditAssetScreen_EditorFinishedTriggersUpdateAsset(t *testing.T) {
	f := newEditAssetFixture(t, "skill", asset.TypeSkill)
	f.seedFile(t, "leaf.txt", "x")
	s := newEditAssetScreen(f.Actions, f.Profile.ID, f.AssetID)
	f.loadInto(t, s)
	s.editingFile = "leaf.txt"

	_, cmd := s.Update(editor.FinishedMsg{Err: nil})
	got := drainCmd(t, cmd)
	if _, ok := got.(saveSucceededMsg); !ok {
		t.Fatalf("editor.FinishedMsg produced %T, want saveSucceededMsg", got)
	}
}

func TestEditAssetScreen_EditorFinishedErrorEmitsNotificationAndSkipsUpdate(t *testing.T) {
	f := newEditAssetFixture(t, "skill", asset.TypeSkill)
	f.seedFile(t, "leaf.txt", "x")
	s := newEditAssetScreen(f.Actions, f.Profile.ID, f.AssetID)
	f.loadInto(t, s)
	s.editingFile = "leaf.txt"

	_, cmd := s.Update(editor.FinishedMsg{Err: os.ErrPermission})
	if cmd == nil {
		t.Fatalf("editor failure produced nil cmd, want notification")
	}
	msg := drainCmd(t, cmd)
	if _, ok := msg.(saveSucceededMsg); ok {
		t.Errorf("editor failure produced saveSucceededMsg, want notification only")
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
	if s.modal == nil || s.modalKind != modalKindAssetDeleteFile {
		t.Fatalf("modal not opened: modal=%v kind=%v", s.modal, s.modalKind)
	}
	if s.deletingFile != target {
		t.Errorf("deletingFile = %q, want %q", s.deletingFile, target)
	}

	_, cmd := s.Update(modal.ResolvedMsg{ID: "delete-file", Confirmed: true})
	msg := drainCmd(t, cmd)
	changed, ok := msg.(filesChangedMsg)
	if !ok {
		t.Fatalf("delete cmd produced %T, want filesChangedMsg", msg)
	}
	if changed.info == "" {
		t.Errorf("filesChangedMsg.info empty, want success text")
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
	if s.deletingFile != "" {
		t.Errorf("deletingFile = %q, want empty", s.deletingFile)
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
	msg := drainCmd(t, cmd)
	if _, ok := msg.(filesChangedMsg); !ok {
		t.Fatalf("add cmd produced %T, want filesChangedMsg", msg)
	}
	if _, err := os.Stat(filepath.Join(f.AssetDir, "scripts", "hello.sh")); err != nil {
		t.Errorf("file not created: %v", err)
	}
}

// TestEditAssetScreen_AddFileRejectsPathTraversal covers the safety rail
// at the integration level: the user pastes "../escape.txt" into the
// create modal and the create cmd produces an error notification (not a
// filesChangedMsg) and writes nothing outside the asset folder.
func TestEditAssetScreen_AddFileRejectsPathTraversal(t *testing.T) {
	f := newEditAssetFixture(t, "skill", asset.TypeSkill)
	s := newEditAssetScreen(f.Actions, f.Profile.ID, f.AssetID)
	f.loadInto(t, s)
	_ = s.onAdd()

	_, cmd := s.Update(modal.ResolvedMsg{
		ID:        "create-file",
		Confirmed: true,
		Value:     modals.CreateFileInput{Path: "../escape.txt"},
	})
	msg := drainCmd(t, cmd)
	done, ok := msg.(mutationDoneMsg)
	if !ok {
		t.Fatalf("traversal cmd produced %T, want mutationDoneMsg (error)", msg)
	}
	if done.severity != errs.SeverityError {
		t.Errorf("severity = %v, want SeverityError", done.severity)
	}
	// File must not have escaped the asset folder.
	parentDir := filepath.Dir(f.AssetDir)
	if _, err := os.Stat(filepath.Join(parentDir, "escape.txt")); !os.IsNotExist(err) {
		t.Errorf("traversal wrote outside asset folder: %v", err)
	}
}

// TestAssetResolveRelative_TraversalRejected unit-tests the safety rail
// directly so the domain rule has a focused test even when the screen
// flow is refactored away.
func TestAssetResolveRelative_TraversalRejected(t *testing.T) {
	dir := t.TempDir()
	for _, rel := range []string{"../escape.txt", "../../etc/passwd"} {
		if _, err := asset.ResolveRelative(dir, rel); err == nil {
			t.Errorf("ResolveRelative(%q) = nil, want error", rel)
		}
	}
}

func TestAssetResolveRelative_ReservedNamesRejected(t *testing.T) {
	dir := t.TempDir()
	for _, rel := range []string{"asset.json", ".secret", "sub/.hidden"} {
		if _, err := asset.ResolveRelative(dir, rel); err == nil {
			t.Errorf("ResolveRelative(%q) = nil, want error", rel)
		}
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

	if got := s.form.tagsCSV; got != "foo, bar" {
		t.Errorf("tagsCSV = %q, want %q", got, "foo, bar")
	}

	s.form.tagsCSV = "baz, qux"
	got := s.composeManifest().Tags
	if !slices.Equal(got, []string{"baz", "qux"}) {
		t.Errorf("Tags = %v, want [baz qux]", got)
	}
}

func TestEditAssetScreen_ComposeManifestReflectsForm(t *testing.T) {
	f := newEditAssetFixture(t, "skill", asset.TypeSkill)
	dir := filepath.Join(t.TempDir(), "asset")
	a := fakeLoadedAsset(t, dir, asset.Manifest{
		ID:   "test",
		Name: "test",
		Type: asset.TypeSkill,
	}, nil)

	s := newEditAssetScreen(f.Actions, f.Profile.ID, f.AssetID)
	pushFakeLoad(t, s, a)

	s.form.descriptionText = "hello"
	s.form.tagsCSV = "a, b"
	s.form.compatibleAgents = []string{"codex"}
	s.form.exclusiveGroup = "main"

	got := s.composeManifest()
	if got.Description != "hello" {
		t.Errorf("Description = %q, want hello", got.Description)
	}
	if !slices.Equal(got.Tags, []string{"a", "b"}) {
		t.Errorf("Tags = %v, want [a b]", got.Tags)
	}
	if !slices.Equal(got.CompatibleAgents, []string{"codex"}) {
		t.Errorf("CompatibleAgents = %v, want [codex]", got.CompatibleAgents)
	}
	if got.ExclusiveGroup != "main" {
		t.Errorf("ExclusiveGroup = %q, want main", got.ExclusiveGroup)
	}
}

func TestEditAssetScreen_BackWhenClean_Pops(t *testing.T) {
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

// TestEditAssetScreen_BackWhenDirty_OpensConfirm drives a keystroke through
// Update with the tags field focused so the dirty signal flows through
// the same path a real user does: huh field binding → form mirror →
// dirty().
func TestEditAssetScreen_BackWhenDirty_OpensConfirm(t *testing.T) {
	f := newEditAssetFixture(t, "skill", asset.TypeSkill)
	s := newEditAssetScreen(f.Actions, f.Profile.ID, f.AssetID)
	f.loadInto(t, s)

	if cmd := s.handler.FocusIndex(s.tagsIdx); cmd != nil {
		_ = drainCmd(t, cmd)
	}
	_, _ = s.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})

	if !s.dirty() {
		t.Fatalf("dirty() = false after typing into tags input")
	}

	_ = s.onBack()
	if s.modal == nil {
		t.Fatalf("modal not opened")
	}
	if s.modalKind != modalKindAssetBackUnsaved {
		t.Errorf("modalKind = %v, want modalKindAssetBackUnsaved", s.modalKind)
	}
}

func TestEditAssetScreen_BackWhenDirty_Confirmed_Pops(t *testing.T) {
	f := newEditAssetFixture(t, "skill", asset.TypeSkill)
	s := newEditAssetScreen(f.Actions, f.Profile.ID, f.AssetID)
	f.loadInto(t, s)
	s.form.tagsCSV = "newtag"
	_ = s.onBack()

	_, cmd := s.Update(modal.ResolvedMsg{ID: "back-unsaved", Confirmed: true})
	if cmd == nil {
		t.Fatalf("confirmed cmd = nil, want PopScreenMsg")
	}
	if _, ok := cmd().(PopScreenMsg); !ok {
		t.Fatalf("cmd produced %T, want PopScreenMsg", cmd())
	}
}

func TestEditAssetScreen_BackWhenDirty_Rejected_KeepsScreen(t *testing.T) {
	f := newEditAssetFixture(t, "skill", asset.TypeSkill)
	s := newEditAssetScreen(f.Actions, f.Profile.ID, f.AssetID)
	f.loadInto(t, s)
	s.form.tagsCSV = "newtag"
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

	s.form.descriptionText = "updated body"

	_, cmd := s.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	msg := drainCmd(t, cmd)
	if _, ok := msg.(saveSucceededMsg); !ok {
		t.Fatalf("save cmd produced %T, want saveSucceededMsg", msg)
	}

	fresh, lerr := f.Service.LoadAsset(f.Profile.ID, f.AssetID)
	if lerr != nil {
		t.Fatalf("LoadAsset after save: %v", lerr)
	}
	if fresh.Description != "updated body" {
		t.Errorf("description after save = %q, want %q", fresh.Description, "updated body")
	}
}

// TestEditAssetScreen_SaveFailureKeepsManifestDirty: the save cmd
// returns mutationDoneMsg on error, and the snapshot is NOT refreshed —
// so dirty() still reports true.
func TestEditAssetScreen_SaveFailureKeepsManifestDirty(t *testing.T) {
	f := newEditAssetFixture(t, "skill", asset.TypeSkill)
	// Use a bogus asset id so the service returns AssetNotFoundError
	// from the save chokepoint (UpdateAsset's resolveAsset).
	s := newEditAssetScreen(f.Actions, f.Profile.ID, f.AssetID)
	f.loadInto(t, s)

	// Make form dirty.
	s.form.descriptionText = "dirty change"
	if !s.dirty() {
		t.Fatalf("dirty() = false after edit")
	}

	// Force a save failure by mangling the captured asset id on the
	// screen. The save cmd reads s.profileID + the composed manifest's
	// ID; flipping the id makes UpdateAsset's resolveAsset fail.
	s.asset.ID = "no-such-asset"

	_, cmd := s.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	msg := drainCmd(t, cmd)
	if _, ok := msg.(mutationDoneMsg); !ok {
		t.Fatalf("failure cmd produced %T, want mutationDoneMsg", msg)
	}
	if !s.dirty() {
		t.Errorf("dirty() = false after save failure, want true")
	}
}

// TestEditAssetScreen_DirtyClearsWhenStateRevertsToOriginal covers the
// edit-then-revert round trip: typing then deleting back to the original
// must zero out dirty().
func TestEditAssetScreen_DirtyClearsWhenStateRevertsToOriginal(t *testing.T) {
	f := newEditAssetFixture(t, "skill", asset.TypeSkill)
	s := newEditAssetScreen(f.Actions, f.Profile.ID, f.AssetID)
	f.loadInto(t, s)

	original := s.form.descriptionText
	s.form.descriptionText = original + "edit"
	if !s.dirty() {
		t.Fatalf("dirty() = false after edit")
	}
	s.form.descriptionText = original
	if s.dirty() {
		t.Errorf("dirty() = true after reverting to original, want false")
	}
}

// TestEditAssetScreen_KeyPressForwardedToModalWhenOpen verifies the
// modal-open guard: an `e` keystroke while the back-unsaved confirm is
// open must not trigger onSave.
func TestEditAssetScreen_KeyPressForwardedToModalWhenOpen(t *testing.T) {
	f := newEditAssetFixture(t, "skill", asset.TypeSkill)
	s := newEditAssetScreen(f.Actions, f.Profile.ID, f.AssetID)
	f.loadInto(t, s)
	s.form.tagsCSV = "newtag" // make dirty
	_ = s.onBack()
	if s.modal == nil {
		t.Fatalf("setup failure: back-unsaved modal not open")
	}

	// `e` would otherwise invoke onSave; while modal is open it must be
	// forwarded into the modal (which does not interpret `e`).
	_, cmd := s.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	if cmd != nil {
		// The modal may emit a cmd of its own; what we forbid is the
		// save cmd, which we identify by its returned msg type.
		msg := drainCmd(t, cmd)
		if _, ok := msg.(saveSucceededMsg); ok {
			t.Errorf("e key while back-unsaved modal open triggered save")
		}
	}
}

func TestEditAssetScreen_StatusKeysTreetableFocused_IncludesSaveAndBack(t *testing.T) {
	f := newEditAssetFixture(t, "skill", asset.TypeSkill)
	f.seedFile(t, "leaf.txt", "x")
	s := newEditAssetScreen(f.Actions, f.Profile.ID, f.AssetID)
	f.loadInto(t, s)

	advanceTreeUntilRel(t, s, "leaf.txt")

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
	_ = s.handler.FocusIndex(s.descIdx)
	s.rebuildSet()

	keys := s.StatusKeys()
	for _, k := range keys {
		switch k.Help().Key {
		case "o", "d":
			t.Errorf("StatusKeys (description focus) leaks row key %q", k.Help().Key)
		}
	}
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
	_ = s.handler.FocusIndex(s.descIdx)
	if !s.InputFocused() {
		t.Errorf("InputFocused() = false with description focused, want true")
	}
	_ = s.handler.FocusIndex(s.treeIdx)
	if s.InputFocused() {
		t.Errorf("InputFocused() = true after returning to treetable, want false")
	}
}

func TestEditAssetScreen_InputFocusedSuppressesGlobalKeys(t *testing.T) {
	f := newEditAssetFixture(t, "skill", asset.TypeSkill)
	s := newEditAssetScreen(f.Actions, f.Profile.ID, f.AssetID)

	if screenWantsRawKey(s, tea.KeyPressMsg{Code: 's', Text: "s"}) {
		t.Errorf("treetable-focused screen wants raw 's', should not")
	}
	f.loadInto(t, s)
	_ = s.handler.FocusIndex(s.descIdx)

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
	ctrlC := tea.KeyPressMsg{Code: 'c', Text: "", Mod: tea.ModCtrl}
	if screenWantsRawKey(s, ctrlC) {
		t.Errorf("screenWantsRawKey(ctrl+c) = true, want false (safety abort path)")
	}
}

// stripANSI strips every CSI SGR escape sequence so substring assertions
// like "[Open]" survive lipgloss's per-rune style switching.
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
