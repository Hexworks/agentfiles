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
	s.prof = prof
	s.rebuildLists()
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

func TestEditProfileScreen_AssetsFocusedMnemonicSet_PopulatedIsUnique(t *testing.T) {
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
	want := []string{"1", "2", "Edit", "Delete", "Create Asset", "Register Project", "Back"}
	if !labelsEqual(got, want) {
		t.Errorf("set labels = %v, want %v", got, want)
	}
}

func TestEditProfileScreen_ProjectsFocusedMnemonicSet_PopulatedIsUnique(t *testing.T) {
	f := newEditProfileFixture(t)
	s := newEditProfileScreen(f.Actions, f.Profile.ID)
	withProfile(s, fakeLoadedProfile(t, "/tmp/x",
		[]*asset.Asset{{Manifest: asset.Manifest{ID: "skill-1", Name: "Skill", Type: asset.TypeSkill}}},
		[]*project.Manifest{{ID: "proj-1", Name: "Proj", Path: "/tmp/proj"}},
	))
	_ = s.handler.FocusIndex(1)
	s.rebuildSet()

	got := mnemonicLabels(s.set)
	want := []string{"1", "2", "Edit", "Select Assets", "Plan", "Delete", "Create Asset", "Register Project", "Back"}
	if !labelsEqual(got, want) {
		t.Errorf("set labels = %v, want %v", got, want)
	}
}

func TestEditProfileScreen_EmptyAssetsListDropsRowMnemonics(t *testing.T) {
	f := newEditProfileFixture(t)
	s := newEditProfileScreen(f.Actions, f.Profile.ID)
	withProfile(s, fakeLoadedProfile(t, "/tmp/x", nil, nil))

	got := mnemonicLabels(s.set)
	want := []string{"1", "2", "Create Asset", "Register Project", "Back"}
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
	want := []string{"1", "2", "Create Asset", "Register Project", "Back"}
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

func TestEditProfileScreen_Ctrl1FocusesAssets(t *testing.T) {
	f := newEditProfileFixture(t)
	s := newEditProfileScreen(f.Actions, f.Profile.ID)
	withProfile(s, fakeLoadedProfile(t, "/tmp/x", nil, nil))
	_ = s.handler.FocusIndex(1) // start with projects focused

	_, _ = s.Update(tea.KeyPressMsg{Code: '1', Mod: tea.ModCtrl})

	if got := s.handler.Focused(); got != 0 {
		t.Errorf("after ctrl+1, focus = %d, want 0 (assets)", got)
	}
}

func TestEditProfileScreen_Ctrl2FocusesProjects(t *testing.T) {
	f := newEditProfileFixture(t)
	s := newEditProfileScreen(f.Actions, f.Profile.ID)
	withProfile(s, fakeLoadedProfile(t, "/tmp/x", nil, nil))

	_, _ = s.Update(tea.KeyPressMsg{Code: '2', Mod: tea.ModCtrl})

	if got := s.handler.Focused(); got != 1 {
		t.Errorf("after ctrl+2, focus = %d, want 1 (projects)", got)
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

func TestEditProfileScreen_AssetsEKeyPushesEditAssetStub(t *testing.T) {
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
	stub, ok := push.Screen.(*editAssetStub)
	if !ok {
		t.Fatalf("pushed screen = %T, want *editAssetStub", push.Screen)
	}
	if stub.profileID != f.Profile.ID || stub.assetID != "skill-1" {
		t.Errorf("stub ids = (%q, %q), want (%q, %q)", stub.profileID, stub.assetID, f.Profile.ID, "skill-1")
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
	if got := s.modal.ID(); got != "delete-asset" {
		t.Errorf("modal id = %q, want delete-asset", got)
	}
	if s.pendingDeleteAsset != "skill-1" {
		t.Errorf("pendingDeleteAsset = %q, want skill-1", s.pendingDeleteAsset)
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

	done, ok := drainCmd(t, cmd).(editProfileMutationDoneMsg)
	if !ok {
		t.Fatalf("cmd produced %T, want editProfileMutationDoneMsg", drainCmd(t, cmd))
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
	if s.pendingDeleteAsset != "" {
		t.Errorf("pendingDeleteAsset = %q, want cleared", s.pendingDeleteAsset)
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
	if got := s.modal.ID(); got != "edit-project" {
		t.Errorf("modal id = %q, want edit-project", got)
	}
}

func TestEditProfileScreen_ProjectsAKeyPushesSelectProjectAssetsStub(t *testing.T) {
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
	stub, ok := push.Screen.(*selectProjectAssetsStub)
	if !ok {
		t.Fatalf("pushed screen = %T, want *selectProjectAssetsStub", push.Screen)
	}
	if stub.profileID != f.Profile.ID || stub.projectID != "proj-1" {
		t.Errorf("stub ids = (%q, %q), want (%q, %q)", stub.profileID, stub.projectID, f.Profile.ID, "proj-1")
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
	if got := s.modal.ID(); got != "delete-project" {
		t.Errorf("modal id = %q, want delete-project", got)
	}
	if s.pendingDeleteProj != "proj-1" {
		t.Errorf("pendingDeleteProj = %q, want proj-1", s.pendingDeleteProj)
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

	done, ok := drainCmd(t, cmd).(editProfileMutationDoneMsg)
	if !ok {
		t.Fatalf("cmd produced %T, want editProfileMutationDoneMsg", drainCmd(t, cmd))
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
	if got := s.modal.ID(); got != "create-asset" {
		t.Errorf("modal id = %q, want create-asset", got)
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
	if got := s.modal.ID(); got != "register-project" {
		t.Errorf("modal id = %q, want register-project", got)
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
	body := s.Body(120, 24)
	for _, want := range []string{"Assets", "Projects", "reate Asset", "egister Project"} {
		if !strings.Contains(body, want) {
			t.Errorf("Body missing %q\n%s", want, body)
		}
	}
}

func TestEditProfileScreen_BodyExactlyMatchesRequestedHeight(t *testing.T) {
	f := newEditProfileFixture(t)
	cases := []struct {
		name     string
		assets   []*asset.Asset
		projects []*project.Manifest
		width    int
		height   int
	}{
		{"empty state", nil, nil, 120, 24},
		{"populated", []*asset.Asset{{Manifest: asset.Manifest{ID: "skill-1", Name: "Skill", Type: asset.TypeSkill}}},
			[]*project.Manifest{{ID: "proj-1", Name: "Proj", Path: "/tmp/proj", EnabledAgents: []string{"codex"}}},
			120, 24},
		{"tall window", nil, nil, 120, 40},
		{"short window", nil, nil, 120, 14},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newEditProfileScreen(f.Actions, f.Profile.ID)
			withProfile(s, fakeLoadedProfile(t, "/tmp/x", tc.assets, tc.projects))
			_, _ = s.Update(tea.WindowSizeMsg{Width: tc.width, Height: tc.height + 4})

			body := s.Body(tc.width, tc.height)
			if got := lipgloss.Height(body); got != tc.height {
				t.Errorf("Body height = %d, want %d", got, tc.height)
			}
		})
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

	_, cmd := s.Update(editProfileMutationDoneMsg{text: "Asset created", severity: errs.SeverityInfo})

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
	projectPath := filepath.Join(f.Root, "proj-edit")
	manifest := f.seedProject(t, "Proj", projectPath)
	// Pre-set SelectedAssetIDs so we can assert it's preserved across edit.
	manifest.SelectedAssetIDs = []string{"some-asset"}

	s := newEditProfileScreen(f.Actions, f.Profile.ID)
	loaded := drainCmd(t, s.Init()).(editProfileLoadedMsg)
	_, _ = s.Update(loaded)
	_ = s.handler.FocusIndex(1)
	s.rebuildSet()
	// Mutate the in-memory selected list to simulate the prior state.
	if proj, ok := s.selectedProject(); ok {
		proj.SelectedAssetIDs = []string{"some-asset"}
	}
	_, _ = s.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	if s.modal == nil {
		t.Fatalf("edit-project modal not opened")
	}

	// Simulate confirm with renamed values.
	in := modals.EditProjectInput{
		Name:          "Renamed",
		Path:          projectPath,
		EnabledAgents: []string{"codex"},
	}
	_, cmd := s.Update(modal.ResolvedMsg{ID: "edit-project", Confirmed: true, Value: in})

	if _, ok := drainCmd(t, cmd).(editProfileMutationDoneMsg); !ok {
		t.Fatalf("cmd produced %T, want editProfileMutationDoneMsg", drainCmd(t, cmd))
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

var _ Screen = (*editProfileScreen)(nil)
