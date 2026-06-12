package shell

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/hexworks/agentfiles/internal/actions"
	"github.com/hexworks/agentfiles/internal/app"
	"github.com/hexworks/agentfiles/internal/asset"
	"github.com/hexworks/agentfiles/internal/config"
	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/profile"
	"github.com/hexworks/agentfiles/internal/project"
	"github.com/hexworks/agentfiles/internal/registry"
	"github.com/hexworks/agentfiles/internal/tui/components/modal"
	"github.com/hexworks/agentfiles/internal/tui/modals"
	"github.com/hexworks/agentfiles/internal/tui/notifications"
)

// editProfileFixture wires a real registry-backed Actions handle and
// seeds one profile so tests can exercise LoadProfile / CreateAsset /
// AddProject / DeleteAsset / DeleteProject end-to-end.
type editProfileFixture struct {
	Root    string
	Service *app.Service
	Actions *actions.Actions
	Profile *registry.ProfileRef
}

func newEditProfileFixture(t *testing.T) *editProfileFixture {
	t.Helper()
	root := t.TempDir()
	svc := app.New(filepath.Join(root, "registry.json"))
	ref, err := svc.CreateProfile("alpha", filepath.Join(root, "alpha"))
	if err != nil {
		t.Fatalf("seed profile: %v", err)
	}
	return &editProfileFixture{
		Root:    root,
		Service: svc,
		Actions: actions.New(svc),
		Profile: ref,
	}
}

// seedAsset adds an asset to the seeded profile via the real service
// so it appears on the next LoadProfile.
func (f *editProfileFixture) seedAsset(t *testing.T, name string, typ asset.Type) {
	t.Helper()
	if _, err := f.Service.InitAsset(f.Profile.ID, asset.Manifest{
		Name: name,
		Type: typ,
	}); err != nil {
		t.Fatalf("seed asset %q: %v", name, err)
	}
}

// seedProject adds a project to the seeded profile and returns the
// manifest the service produced (with normalized path / sorted slices).
func (f *editProfileFixture) seedProject(t *testing.T, name, path string) *project.Manifest {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("seed project dir: %v", err)
	}
	manifest, errs := f.Service.AddProject(f.Profile.ID, name, path, []string{"codex"}, nil)
	if len(errs) > 0 {
		t.Fatalf("seed project %q: %v", name, errs)
	}
	return manifest
}

// withProfile bypasses the load command and pushes a pre-loaded
// profile + sorted asset / project slices into the screen so UI-state
// tests run without spinning the registry.
func withProfile(s *editProfileScreen, prof *profile.Profile) {
	s.rebuildLists(prof)
	s.rebuildAssetsTable()
	s.rebuildProjectsTable()
	_ = s.handler.FocusIndex(0)
	s.rebuildSet()
}

func fakeLoadedProfile(t *testing.T, root string, assets []*asset.Asset, projects []*project.Manifest) *profile.Profile {
	t.Helper()
	prof := &profile.Profile{
		Root:     root,
		Manifest: profile.Manifest{ID: "alpha", Name: "Alpha"},
		Assets:   map[string]*asset.Asset{},
		Projects: map[string]*project.Manifest{},
	}
	for _, a := range assets {
		prof.Assets[a.ID] = a
	}
	for _, p := range projects {
		prof.Projects[p.ID] = p
	}
	return prof
}

func TestEditProfileScreen_InitLoadsProfileFromActions(t *testing.T) {
	f := newEditProfileFixture(t)
	f.seedAsset(t, "Skill One", asset.TypeSkill)
	s := newEditProfileScreen(f.Actions, f.Profile.ID)

	msg := drainCmd(t, s.Init())

	loaded, ok := msg.(editProfileLoadedMsg)
	if !ok {
		t.Fatalf("Init produced %T, want editProfileLoadedMsg", msg)
	}
	if loaded.err != nil {
		t.Fatalf("loaded.err = %v, want nil", loaded.err)
	}
	if loaded.prof == nil {
		t.Fatalf("loaded.prof = nil, want a profile")
	}
	if len(loaded.prof.Assets) != 1 {
		t.Errorf("loaded asset count = %d, want 1", len(loaded.prof.Assets))
	}
}

func TestEditProfileScreen_AssetsFocusedMnemonicSet_HasExpectedMnemonicsInOrder(t *testing.T) {
	f := newEditProfileFixture(t)
	s := newEditProfileScreen(f.Actions, f.Profile.ID)
	withProfile(s, fakeLoadedProfile(t, "/tmp/x",
		[]*asset.Asset{{Manifest: asset.Manifest{ID: "skill-1", Name: "Skill", Type: asset.TypeSkill}}},
		[]*project.Manifest{{ID: "proj-1", Name: "Proj", Path: "/tmp/proj"}},
	))

	// Defaults to assets focused (index 0).
	if got := s.handler.Focused(); got != 0 {
		t.Fatalf("initial focus = %d, want 0 (assets)", got)
	}
	got := mnemonicLabels(s.set)
	want := []string{"Edit", "Delete", "Create Asset", "Register Project", "Back"}
	if !labelsEqual(got, want) {
		t.Errorf("set labels = %v, want %v", got, want)
	}
}

func TestEditProfileScreen_ProjectsFocusedMnemonicSet_HasExpectedMnemonicsInOrder(t *testing.T) {
	f := newEditProfileFixture(t)
	s := newEditProfileScreen(f.Actions, f.Profile.ID)
	withProfile(s, fakeLoadedProfile(t, "/tmp/x",
		[]*asset.Asset{{Manifest: asset.Manifest{ID: "skill-1", Name: "Skill", Type: asset.TypeSkill}}},
		[]*project.Manifest{{ID: "proj-1", Name: "Proj", Path: "/tmp/proj"}},
	))
	_ = s.handler.FocusIndex(1)
	s.rebuildSet()

	got := mnemonicLabels(s.set)
	want := []string{"Edit", "Select Assets", "Plan", "Delete", "Create Asset", "Register Project", "Back"}
	if !labelsEqual(got, want) {
		t.Errorf("set labels = %v, want %v", got, want)
	}
}

func TestEditProfileScreen_EmptyAssetsListDropsRowMnemonics(t *testing.T) {
	f := newEditProfileFixture(t)
	s := newEditProfileScreen(f.Actions, f.Profile.ID)
	withProfile(s, fakeLoadedProfile(t, "/tmp/x", nil, nil))

	got := mnemonicLabels(s.set)
	want := []string{"Create Asset", "Register Project", "Back"}
	if !labelsEqual(got, want) {
		t.Errorf("set labels (assets empty, focused) = %v, want %v", got, want)
	}
}

func TestEditProfileScreen_EmptyProjectsListDropsRowMnemonics(t *testing.T) {
	f := newEditProfileFixture(t)
	s := newEditProfileScreen(f.Actions, f.Profile.ID)
	withProfile(s, fakeLoadedProfile(t, "/tmp/x", nil, nil))
	_ = s.handler.FocusIndex(1)
	s.rebuildSet()

	got := mnemonicLabels(s.set)
	want := []string{"Create Asset", "Register Project", "Back"}
	if !labelsEqual(got, want) {
		t.Errorf("set labels (projects empty, focused) = %v, want %v", got, want)
	}
}

func TestEditProfileScreen_TabCyclesFocus(t *testing.T) {
	f := newEditProfileFixture(t)
	s := newEditProfileScreen(f.Actions, f.Profile.ID)
	withProfile(s, fakeLoadedProfile(t, "/tmp/x", nil, nil))

	if s.handler.Focused() != 0 {
		t.Fatalf("initial focus = %d, want 0", s.handler.Focused())
	}
	_, _ = s.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if s.handler.Focused() != 1 {
		t.Errorf("after Tab, focus = %d, want 1", s.handler.Focused())
	}
	_, _ = s.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if s.handler.Focused() != 0 {
		t.Errorf("after second Tab, focus = %d, want 0", s.handler.Focused())
	}
}

func TestEditProfileScreen_ShiftTabCyclesBackwards(t *testing.T) {
	f := newEditProfileFixture(t)
	s := newEditProfileScreen(f.Actions, f.Profile.ID)
	withProfile(s, fakeLoadedProfile(t, "/tmp/x", nil, nil))

	_, _ = s.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if s.handler.Focused() != 1 {
		t.Errorf("after shift+Tab, focus = %d, want 1", s.handler.Focused())
	}
}

func TestEditProfileScreen_BTriggersPop(t *testing.T) {
	f := newEditProfileFixture(t)
	s := newEditProfileScreen(f.Actions, f.Profile.ID)
	withProfile(s, fakeLoadedProfile(t, "/tmp/x", nil, nil))

	_, cmd := s.Update(tea.KeyPressMsg{Code: 'b', Text: "b"})
	if cmd == nil {
		t.Fatalf("b produced nil cmd")
	}
	if _, ok := cmd().(PopScreenMsg); !ok {
		t.Fatalf("cmd produced %T, want PopScreenMsg", cmd())
	}
}

func TestEditProfileScreen_EscTriggersPop(t *testing.T) {
	f := newEditProfileFixture(t)
	s := newEditProfileScreen(f.Actions, f.Profile.ID)
	withProfile(s, fakeLoadedProfile(t, "/tmp/x", nil, nil))

	_, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd == nil {
		t.Fatalf("esc produced nil cmd")
	}
	if _, ok := cmd().(PopScreenMsg); !ok {
		t.Fatalf("cmd produced %T, want PopScreenMsg", cmd())
	}
}

func TestEditProfileScreen_AssetsEKeyPushesEditAssetScreen(t *testing.T) {
	f := newEditProfileFixture(t)
	s := newEditProfileScreen(f.Actions, f.Profile.ID)
	withProfile(s, fakeLoadedProfile(t, "/tmp/x",
		[]*asset.Asset{{Manifest: asset.Manifest{ID: "skill-1", Name: "Skill", Type: asset.TypeSkill}}},
		nil,
	))

	_, cmd := s.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})

	push, ok := drainCmd(t, cmd).(PushScreenMsg)
	if !ok {
		t.Fatalf("cmd produced %T, want PushScreenMsg", cmd())
	}
	screen, ok := push.Screen.(*editAssetScreen)
	if !ok {
		t.Fatalf("pushed screen = %T, want *editAssetScreen", push.Screen)
	}
	if screen.ProfileID() != f.Profile.ID || screen.AssetID() != "skill-1" {
		t.Errorf("screen ids = (%q, %q), want (%q, %q)", screen.ProfileID(), screen.AssetID(), f.Profile.ID, "skill-1")
	}
}

func TestEditProfileScreen_AssetsDKeyOpensDeleteAssetConfirm(t *testing.T) {
	f := newEditProfileFixture(t)
	s := newEditProfileScreen(f.Actions, f.Profile.ID)
	withProfile(s, fakeLoadedProfile(t, "/tmp/x",
		[]*asset.Asset{{Manifest: asset.Manifest{ID: "skill-1", Name: "Skill", Type: asset.TypeSkill}}},
		nil,
	))

	_, _ = s.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})

	if s.modal == nil {
		t.Fatalf("modal nil after 'd'")
	}
	if got := s.modalKind; got != modalKindDeleteAsset {
		t.Errorf("modalKind = %v, want modalKindDeleteAsset", got)
	}
	if s.pendingDeleteAssetID != "skill-1" {
		t.Errorf("pendingDeleteAssetID = %q, want skill-1", s.pendingDeleteAssetID)
	}
}

func TestEditProfileScreen_AssetDeleteConfirmedRemovesAsset(t *testing.T) {
	f := newEditProfileFixture(t)
	f.seedAsset(t, "Skill One", asset.TypeSkill)

	s := newEditProfileScreen(f.Actions, f.Profile.ID)
	// Load the real profile so selectedAsset() returns the seeded asset.
	loaded := drainCmd(t, s.Init()).(editProfileLoadedMsg)
	_, _ = s.Update(loaded)

	// Press 'd', then confirm.
	_, _ = s.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	_, cmd := s.Update(modal.ResolvedMsg{ID: "delete-asset", Confirmed: true})

	done, ok := drainCmd(t, cmd).(mutationDoneMsg)
	if !ok {
		t.Fatalf("cmd produced %T, want mutationDoneMsg", drainCmd(t, cmd))
	}
	if done.severity != errs.SeverityInfo {
		t.Errorf("severity = %v, want info", done.severity)
	}

	// Reload via the service directly and assert the asset is gone.
	fresh, err := f.Service.LoadProfile(f.Profile.ID)
	if err != nil {
		t.Fatalf("LoadProfile after delete: %v", err)
	}
	if len(fresh.Assets) != 0 {
		t.Errorf("asset count after delete = %d, want 0", len(fresh.Assets))
	}
}

func TestEditProfileScreen_AssetDeleteRejectedMakesNoServiceCall(t *testing.T) {
	f := newEditProfileFixture(t)
	f.seedAsset(t, "Skill One", asset.TypeSkill)

	s := newEditProfileScreen(f.Actions, f.Profile.ID)
	loaded := drainCmd(t, s.Init()).(editProfileLoadedMsg)
	_, _ = s.Update(loaded)
	_, _ = s.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})

	_, cmd := s.Update(modal.ResolvedMsg{ID: "delete-asset", Confirmed: false})

	if cmd != nil {
		t.Errorf("No produced cmd = %v, want nil", cmd)
	}
	if s.modal != nil {
		t.Errorf("modal still open after No, want cleared")
	}
	if s.pendingDeleteAssetID != "" {
		t.Errorf("pendingDeleteAssetID = %q, want cleared", s.pendingDeleteAssetID)
	}
	fresh, err := f.Service.LoadProfile(f.Profile.ID)
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}
	if len(fresh.Assets) != 1 {
		t.Errorf("asset count = %d, want 1 (no service call)", len(fresh.Assets))
	}
}

func TestEditProfileScreen_ProjectsEKeyOpensEditProjectModal(t *testing.T) {
	f := newEditProfileFixture(t)
	s := newEditProfileScreen(f.Actions, f.Profile.ID)
	withProfile(s, fakeLoadedProfile(t, "/tmp/x", nil,
		[]*project.Manifest{{ID: "proj-1", Name: "Proj", Path: "/tmp/proj", EnabledAgents: []string{"codex"}}},
	))
	_ = s.handler.FocusIndex(1)
	s.rebuildSet()

	_, _ = s.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})

	if s.modal == nil {
		t.Fatalf("modal nil after 'e' on projects")
	}
	if got := s.modalKind; got != modalKindEditProject {
		t.Errorf("modalKind = %v, want modalKindEditProject", got)
	}
}

func TestEditProfileScreen_ProjectsAKeyPushesSelectProjectAssetsScreen(t *testing.T) {
	f := newEditProfileFixture(t)
	s := newEditProfileScreen(f.Actions, f.Profile.ID)
	withProfile(s, fakeLoadedProfile(t, "/tmp/x", nil,
		[]*project.Manifest{{ID: "proj-1", Name: "Proj", Path: "/tmp/proj", EnabledAgents: []string{"codex"}}},
	))
	_ = s.handler.FocusIndex(1)
	s.rebuildSet()

	_, cmd := s.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})

	push, ok := drainCmd(t, cmd).(PushScreenMsg)
	if !ok {
		t.Fatalf("cmd produced %T, want PushScreenMsg", cmd())
	}
	child, ok := push.Screen.(*selectProjectAssetsScreen)
	if !ok {
		t.Fatalf("pushed screen = %T, want *selectProjectAssetsScreen", push.Screen)
	}
	if child.profileID != f.Profile.ID || child.projectID != "proj-1" {
		t.Errorf("screen ids = (%q, %q), want (%q, %q)", child.profileID, child.projectID, f.Profile.ID, "proj-1")
	}
}

func TestEditProfileScreen_ProjectsPKeyPushesPlanProjectStub(t *testing.T) {
	f := newEditProfileFixture(t)
	s := newEditProfileScreen(f.Actions, f.Profile.ID)
	withProfile(s, fakeLoadedProfile(t, "/tmp/x", nil,
		[]*project.Manifest{{ID: "proj-1", Name: "Proj", Path: "/tmp/proj", EnabledAgents: []string{"codex"}}},
	))
	_ = s.handler.FocusIndex(1)
	s.rebuildSet()

	_, cmd := s.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})

	push, ok := drainCmd(t, cmd).(PushScreenMsg)
	if !ok {
		t.Fatalf("cmd produced %T, want PushScreenMsg", cmd())
	}
	stub, ok := push.Screen.(*planProjectStub)
	if !ok {
		t.Fatalf("pushed screen = %T, want *planProjectStub", push.Screen)
	}
	if stub.profileID != f.Profile.ID || stub.projectID != "proj-1" {
		t.Errorf("stub ids = (%q, %q), want (%q, %q)", stub.profileID, stub.projectID, f.Profile.ID, "proj-1")
	}
}

func TestEditProfileScreen_ProjectsDKeyOpensDeleteProjectConfirm(t *testing.T) {
	f := newEditProfileFixture(t)
	s := newEditProfileScreen(f.Actions, f.Profile.ID)
	withProfile(s, fakeLoadedProfile(t, "/tmp/x", nil,
		[]*project.Manifest{{ID: "proj-1", Name: "Proj", Path: "/tmp/proj", EnabledAgents: []string{"codex"}}},
	))
	_ = s.handler.FocusIndex(1)
	s.rebuildSet()

	_, _ = s.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})

	if s.modal == nil {
		t.Fatalf("modal nil after 'd' on projects")
	}
	if got := s.modalKind; got != modalKindDeleteProject {
		t.Errorf("modalKind = %v, want modalKindDeleteProject", got)
	}
	if s.pendingDeleteProjectID != "proj-1" {
		t.Errorf("pendingDeleteProjectID = %q, want proj-1", s.pendingDeleteProjectID)
	}
}

func TestEditProfileScreen_ConfirmedDeleteProjectDoesNotDeleteRepoFiles(t *testing.T) {
	f := newEditProfileFixture(t)
	projectPath := filepath.Join(f.Root, "proj-1-repo")
	manifest := f.seedProject(t, "Proj", projectPath)

	// Write a sentinel file inside the project repo that must survive.
	sentinel := filepath.Join(projectPath, "keep-me.txt")
	if err := os.WriteFile(sentinel, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}

	s := newEditProfileScreen(f.Actions, f.Profile.ID)
	loaded := drainCmd(t, s.Init()).(editProfileLoadedMsg)
	_, _ = s.Update(loaded)
	_ = s.handler.FocusIndex(1)
	s.rebuildSet()
	_, _ = s.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})

	_, cmd := s.Update(modal.ResolvedMsg{ID: "delete-project", Confirmed: true})

	done, ok := drainCmd(t, cmd).(mutationDoneMsg)
	if !ok {
		t.Fatalf("cmd produced %T, want mutationDoneMsg", drainCmd(t, cmd))
	}
	if done.severity != errs.SeverityInfo {
		t.Errorf("severity = %v, want info", done.severity)
	}

	// Project manifest must be gone from the profile.
	fresh, err := f.Service.LoadProfile(f.Profile.ID)
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}
	if _, ok := fresh.Projects[manifest.ID]; ok {
		t.Errorf("project %q still in profile after delete", manifest.ID)
	}
	// Repo files must remain untouched (task 0026 safety contract).
	if _, statErr := os.Stat(sentinel); statErr != nil {
		t.Errorf("expected sentinel file kept after Delete Project, got %v", statErr)
	}
	if _, statErr := os.Stat(projectPath); statErr != nil {
		t.Errorf("expected project dir kept after Delete Project, got %v", statErr)
	}
}

func TestEditProfileScreen_CKeyOpensCreateAssetModal(t *testing.T) {
	f := newEditProfileFixture(t)
	s := newEditProfileScreen(f.Actions, f.Profile.ID)
	withProfile(s, fakeLoadedProfile(t, "/tmp/x", nil, nil))

	_, _ = s.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})

	if s.modal == nil {
		t.Fatalf("modal nil after 'c'")
	}
	if got := s.modalKind; got != modalKindCreateAsset {
		t.Errorf("modalKind = %v, want modalKindCreateAsset", got)
	}
}

func TestEditProfileScreen_RKeyOpensRegisterProjectModal(t *testing.T) {
	f := newEditProfileFixture(t)
	s := newEditProfileScreen(f.Actions, f.Profile.ID)
	withProfile(s, fakeLoadedProfile(t, "/tmp/x", nil, nil))

	_, _ = s.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})

	if s.modal == nil {
		t.Fatalf("modal nil after 'r'")
	}
	if got := s.modalKind; got != modalKindRegisterProject {
		t.Errorf("modalKind = %v, want modalKindRegisterProject", got)
	}
}

func TestEditProfileScreen_StatusKeysAssetsFocusedNonEmpty(t *testing.T) {
	f := newEditProfileFixture(t)
	s := newEditProfileScreen(f.Actions, f.Profile.ID)
	withProfile(s, fakeLoadedProfile(t, "/tmp/x",
		[]*asset.Asset{{Manifest: asset.Manifest{ID: "skill-1", Name: "Skill", Type: asset.TypeSkill}}},
		nil,
	))

	keys := s.StatusKeys()
	got := []string{}
	for _, k := range keys {
		got = append(got, k.Help().Key)
	}
	want := []string{"e", "d", "b"}
	if !slices.Equal(got, want) {
		t.Errorf("StatusKeys = %v, want %v", got, want)
	}
}

func TestEditProfileScreen_StatusKeysProjectsFocusedNonEmpty(t *testing.T) {
	f := newEditProfileFixture(t)
	s := newEditProfileScreen(f.Actions, f.Profile.ID)
	withProfile(s, fakeLoadedProfile(t, "/tmp/x", nil,
		[]*project.Manifest{{ID: "proj-1", Name: "Proj", Path: "/tmp/proj", EnabledAgents: []string{"codex"}}},
	))
	_ = s.handler.FocusIndex(1)

	keys := s.StatusKeys()
	got := []string{}
	for _, k := range keys {
		got = append(got, k.Help().Key)
	}
	want := []string{"e", "a", "p", "d", "b"}
	if !slices.Equal(got, want) {
		t.Errorf("StatusKeys = %v, want %v", got, want)
	}
}

func TestEditProfileScreen_StatusKeysExcludeScreenLevel(t *testing.T) {
	f := newEditProfileFixture(t)
	s := newEditProfileScreen(f.Actions, f.Profile.ID)
	withProfile(s, fakeLoadedProfile(t, "/tmp/x",
		[]*asset.Asset{{Manifest: asset.Manifest{ID: "skill-1", Name: "Skill", Type: asset.TypeSkill}}},
		[]*project.Manifest{{ID: "proj-1", Name: "Proj", Path: "/tmp/proj", EnabledAgents: []string{"codex"}}},
	))

	for _, focusIdx := range []int{0, 1} {
		_ = s.handler.FocusIndex(focusIdx)
		for _, k := range s.StatusKeys() {
			if k.Help().Key == "c" || k.Help().Key == "r" {
				t.Errorf("StatusKeys (focus %d) leaked screen-level mnemonic %q", focusIdx, k.Help().Key)
			}
			if k.Help().Key == "ctrl+1" || k.Help().Key == "ctrl+2" {
				t.Errorf("StatusKeys (focus %d) leaked panel mnemonic %q", focusIdx, k.Help().Key)
			}
		}
	}
}

func TestEditProfileScreen_StatusKeysFocusedEmpty(t *testing.T) {
	f := newEditProfileFixture(t)
	s := newEditProfileScreen(f.Actions, f.Profile.ID)
	withProfile(s, fakeLoadedProfile(t, "/tmp/x", nil, nil))

	keys := s.StatusKeys()
	if len(keys) != 1 {
		t.Fatalf("StatusKeys len = %d, want 1 (back only)", len(keys))
	}
	if keys[0].Help().Key != "b" {
		t.Errorf("StatusKeys[0].Help().Key = %q, want b", keys[0].Help().Key)
	}
}

func TestEditProfileScreen_TitleAndBodyContainRequiredText(t *testing.T) {
	f := newEditProfileFixture(t)
	s := newEditProfileScreen(f.Actions, f.Profile.ID)
	withProfile(s, fakeLoadedProfile(t, "/tmp/x", nil, nil))
	_, _ = s.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	if got := s.Title(); got != "Edit Profile" {
		t.Errorf("Title() = %q, want Edit Profile", got)
	}
	body := s.Body(120)
	for _, want := range []string{"Assets", "Projects", "reate Asset", "egister Project"} {
		if !strings.Contains(body, want) {
			t.Errorf("Body missing %q\n%s", want, body)
		}
	}
}

// BodyDoesNotFillViewport asserts the body renders at a natural height
// that ignores the terminal size: adding rows grows it, but it never
// expands to match the window height.
func TestEditProfileScreen_BodyDoesNotFillViewport(t *testing.T) {
	f := newEditProfileFixture(t)

	tall := func(window int, assets []*asset.Asset, projects []*project.Manifest) int {
		s := newEditProfileScreen(f.Actions, f.Profile.ID)
		withProfile(s, fakeLoadedProfile(t, "/tmp/x", assets, projects))
		_, _ = s.Update(tea.WindowSizeMsg{Width: 120, Height: window})
		return lipgloss.Height(s.Body(120))
	}

	emptyShort := tall(20, nil, nil)
	emptyTall := tall(80, nil, nil)
	if emptyShort != emptyTall {
		t.Errorf("Body height changed with window size: short=%d tall=%d", emptyShort, emptyTall)
	}
	if emptyTall >= 80 {
		t.Errorf("Body filled the viewport: height=%d on 80-row window", emptyTall)
	}

	populated := tall(80,
		[]*asset.Asset{{Manifest: asset.Manifest{ID: "skill-1", Name: "Skill", Type: asset.TypeSkill}}},
		[]*project.Manifest{{ID: "proj-1", Name: "Proj", Path: "/tmp/proj", EnabledAgents: []string{"codex"}}},
	)
	if populated < emptyTall {
		t.Errorf("Populated body shorter than empty: populated=%d empty=%d", populated, emptyTall)
	}
}

// BodyRecoversFromPreLoadRender mirrors the runtime sequence in which
// bubbletea calls View() on a freshly pushed screen before the screen's
// load command has produced its profile data. The first render hands
// empty rows to the bubbles table, which drops its internal cursor to
// -1; without recovery the next render after the load builds rows
// without an action cell on the cursor row and the viewport renders one
// fewer data line than the table actually holds.
func TestEditProfileScreen_BodyRecoversFromPreLoadRender(t *testing.T) {
	f := newEditProfileFixture(t)
	s := newEditProfileScreen(f.Actions, f.Profile.ID)
	_, _ = s.Update(tea.WindowSizeMsg{Width: 160, Height: 50})

	// Pre-load render: s.assets / s.projects empty, table cursor goes to -1.
	_ = s.Body(160)
	if c := s.assetsTable.Cursor(); c != -1 {
		t.Logf("note: pre-load assets cursor = %d (expected -1 from bubbles SetRows(nil))", c)
	}

	// Now load and render again.
	assets := []*asset.Asset{
		{Manifest: asset.Manifest{ID: "a1", Name: "Alpha", Type: asset.TypeSkill}},
		{Manifest: asset.Manifest{ID: "a2", Name: "Bravo", Type: asset.TypeSkill}},
		{Manifest: asset.Manifest{ID: "a3", Name: "Charlie", Type: asset.TypeSkill}},
	}
	projects := []*project.Manifest{
		{ID: "p1", Name: "One", Path: "/tmp/p1", EnabledAgents: []string{"codex"}},
		{ID: "p2", Name: "Two", Path: "/tmp/p2", EnabledAgents: []string{"codex"}},
		{ID: "p3", Name: "Three", Path: "/tmp/p3", EnabledAgents: []string{"codex"}},
	}
	withProfile(s, fakeLoadedProfile(t, "/tmp/x", assets, projects))
	body := s.Body(160)

	if got := s.assetsTable.Cursor(); got != 0 {
		t.Errorf("assets cursor after recovery = %d, want 0", got)
	}
	if got := s.projectsTable.Cursor(); got != 0 {
		t.Errorf("projects cursor after recovery = %d, want 0", got)
	}
	// All three asset/project rows must be present in the rendered body.
	for _, want := range []string{"a1", "a2", "a3", "p1", "p2", "p3"} {
		if !strings.Contains(body, want) {
			t.Errorf("Body missing %q after pre-load render recovery\n%s", want, body)
		}
	}
	// The cursor row must carry the action cell labels.
	for _, want := range []string{"dit]", "elete]"} {
		if !strings.Contains(body, want) {
			t.Errorf("Body missing action cell label %q\n%s", want, body)
		}
	}
}

func TestEditProfileScreen_LoadErrorEmitsNotification(t *testing.T) {
	f := newEditProfileFixture(t)
	// Corrupt the profile manifest so a subsequent Load surfaces a domain error.
	if err := os.WriteFile(filepath.Join(f.Profile.Path, config.ProfileManifestFileName), []byte("nope"), 0o644); err != nil {
		t.Fatalf("corrupt: %v", err)
	}
	s := newEditProfileScreen(f.Actions, f.Profile.ID)

	msg := drainCmd(t, s.Init())
	loaded, ok := msg.(editProfileLoadedMsg)
	if !ok {
		t.Fatalf("Init produced %T, want editProfileLoadedMsg", msg)
	}
	if loaded.err == nil {
		t.Fatalf("loaded.err nil, want domain error")
	}

	_, cmd := s.Update(loaded)
	out := collectMessages(t, cmd)
	var noteSeen bool
	for _, m := range out {
		if note, ok := m.(notifications.NotificationMsg); ok {
			noteSeen = true
			if note.Notification.Severity != errs.SeverityError {
				t.Errorf("severity = %v, want error", note.Notification.Severity)
			}
		}
	}
	if !noteSeen {
		t.Errorf("err branch missing NotificationMsg; got %v", out)
	}
}

func TestEditProfileScreen_MutationDoneEmitsNotificationAndReload(t *testing.T) {
	f := newEditProfileFixture(t)
	s := newEditProfileScreen(f.Actions, f.Profile.ID)
	withProfile(s, fakeLoadedProfile(t, "/tmp/x", nil, nil))

	_, cmd := s.Update(mutationDoneMsg{text: "Asset created", severity: errs.SeverityInfo})

	collected := collectMessages(t, cmd)
	var sawNotification, sawLoad bool
	for _, m := range collected {
		if note, ok := m.(notifications.NotificationMsg); ok {
			sawNotification = true
			if note.Notification.Text != "Asset created" {
				t.Errorf("notification text = %q, want 'Asset created'", note.Notification.Text)
			}
		}
		if _, ok := m.(editProfileLoadedMsg); ok {
			sawLoad = true
		}
	}
	if !sawNotification {
		t.Errorf("batch missing NotificationMsg; got %v", collected)
	}
	if !sawLoad {
		t.Errorf("batch missing editProfileLoadedMsg; got %v", collected)
	}
}

func TestEditProfileScreen_EditProjectModalPreservesNonEditableFields(t *testing.T) {
	f := newEditProfileFixture(t)
	// Seed the precondition through real domain entry points: an asset
	// exists, a project exists, and the project has the asset selected.
	// Service.SelectAsset is what production code uses to record a
	// project ←→ asset binding, so the test exercises the same persisted
	// shape EditProject must preserve.
	f.seedAsset(t, "Some Asset", asset.TypeSkill)
	projectPath := filepath.Join(f.Root, "proj-edit")
	manifest := f.seedProject(t, "Proj", projectPath)
	if _, err := f.Service.SelectAsset(f.Profile.ID, manifest.ID, "some-asset"); err != nil {
		t.Fatalf("seed SelectAsset: %v", err)
	}

	s := newEditProfileScreen(f.Actions, f.Profile.ID)
	loaded := drainCmd(t, s.Init()).(editProfileLoadedMsg)
	_, _ = s.Update(loaded)
	_ = s.handler.FocusIndex(1)
	s.rebuildSet()
	_, _ = s.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	if s.modal == nil {
		t.Fatalf("edit-project modal not opened")
	}

	in := modals.EditProjectInput{
		Name:          "Renamed",
		Path:          projectPath,
		EnabledAgents: []string{"codex"},
	}
	_, cmd := s.Update(modal.ResolvedMsg{ID: "edit-project", Confirmed: true, Value: in})

	if _, ok := drainCmd(t, cmd).(mutationDoneMsg); !ok {
		t.Fatalf("cmd produced %T, want mutationDoneMsg", drainCmd(t, cmd))
	}

	fresh, err := f.Service.LoadProfile(f.Profile.ID)
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}
	updated := fresh.Projects[manifest.ID]
	if updated == nil {
		t.Fatalf("project %q missing after edit", manifest.ID)
	}
	if updated.Name != "Renamed" {
		t.Errorf("updated name = %q, want Renamed", updated.Name)
	}
	if !slices.Equal(updated.SelectedAssetIDs, []string{"some-asset"}) {
		t.Errorf("SelectedAssetIDs after edit = %v, want preserved [some-asset]", updated.SelectedAssetIDs)
	}
}

// MutationErrorEmitsErrorNotification covers the symmetric branch to
// LoadErrorEmitsNotification: when an action fails, the screen surfaces a
// SeverityError notification so the user sees the failure.
func TestEditProfileScreen_MutationErrorEmitsErrorNotification(t *testing.T) {
	f := newEditProfileFixture(t)
	s := newEditProfileScreen(f.Actions, f.Profile.ID)
	loaded := drainCmd(t, s.Init()).(editProfileLoadedMsg)
	_, _ = s.Update(loaded)
	// Stage a delete-asset for an id that does not exist. afterDeleteAsset
	// runs the service call which returns an AssetNotFoundError; the screen
	// must surface that as SeverityError, not a SeverityInfo "deleted" toast.
	s.pendingDeleteAssetID = "does-not-exist"
	s.modalKind = modalKindDeleteAsset

	_, cmd := s.Update(modal.ResolvedMsg{ID: "delete-asset", Confirmed: true})
	done, ok := drainCmd(t, cmd).(mutationDoneMsg)
	if !ok {
		t.Fatalf("cmd produced %T, want mutationDoneMsg", drainCmd(t, cmd))
	}
	if done.severity != errs.SeverityError {
		t.Errorf("severity = %v, want SeverityError", done.severity)
	}
}

var _ Screen = (*editProfileScreen)(nil)
