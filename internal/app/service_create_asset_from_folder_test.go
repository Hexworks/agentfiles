package app

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/hexworks/agentfiles/internal/asset"
)

// seedFolderRegisterProject builds a profile with one project whose repo
// holds the given project-relative files. The files land under managed
// surfaces (e.g. ".claude/...") so a plan classifies them as unknown, which
// is what makes their parent folders registerable.
func seedFolderRegisterProject(t *testing.T, files map[string]string) (svc *Service, profileID, projectID, repoPath string) {
	t.Helper()
	root := t.TempDir()
	svc = newSvc(root)
	if _, err := svc.CreateProfile("Personal", filepath.Join(root, "profile")); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	profileID = "personal"
	repoPath = filepath.Join(root, "repo")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, addErrs := svc.AddProject(profileID, "Repo", repoPath, []string{"codex"}, nil); len(addErrs) > 0 {
		t.Fatalf("add project: %v", addErrs)
	}
	projectID = "repo"
	// An initial apply writes .agentfiles/state.json so subsequent plans run
	// the unknown-detection pass (skipped on first apply). Only then will the
	// files below be classified as unknown, which is what makes their parent
	// folders registerable.
	if _, err := svc.Apply(profileID, projectID, Resolutions{}); err != nil {
		t.Fatalf("initial apply: %v", err)
	}
	for rel, body := range files {
		full := filepath.Join(repoPath, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return svc, profileID, projectID, repoPath
}

func TestRegisterableDirs(t *testing.T) {
	changes := []FileChange{
		{Path: "a/x.md", Kind: ChangeUnknown},
		{Path: "a/y.md", Kind: ChangeUnknown},
		{Path: "b/z.md", Kind: ChangeUnknown},
		{Path: "b/w.md", Kind: ChangeCreate},
		{Path: "c/deep/i.md", Kind: ChangeUnknown},
	}
	got := RegisterableDirs(changes)
	for _, dir := range []string{"a", "c", "c/deep"} {
		if !got[dir] {
			t.Errorf("dir %q should be registerable", dir)
		}
	}
	if got["b"] {
		t.Errorf("dir %q has a managed leaf and must not be registerable", "b")
	}
}

func TestCreateAssetFromFolder_CopiesContentAndSelectsForProject(t *testing.T) {
	// given a project repo with an unmanaged skill folder
	svc, profileID, projectID, _ := seedFolderRegisterProject(t, map[string]string{
		".claude/skills/something/SKILL.md": "real skill\n",
	})

	// when the folder is registered as a new asset
	id, err := svc.CreateAssetFromFolder(profileID, projectID, asset.Manifest{
		Name: "something", Type: asset.TypeSkill,
	}, ".claude/skills/something")
	if err != nil {
		t.Fatalf("CreateAssetFromFolder: %v", err)
	}

	// then the asset exists with the copied content and is selected
	if id != "something" {
		t.Fatalf("returned id = %q, want %q", id, "something")
	}
	prof, loadErr := svc.LoadProfile(profileID)
	if loadErr != nil {
		t.Fatalf("reload profile: %v", loadErr)
	}
	created := prof.Profile.Assets["something"]
	if created == nil {
		t.Fatal("expected asset 'something' in profile")
	}
	body, readErr := os.ReadFile(filepath.Join(created.Dir, "SKILL.md"))
	if readErr != nil {
		t.Fatalf("read copied content: %v", readErr)
	}
	if string(body) != "real skill\n" {
		t.Fatalf("copied content = %q, want %q", string(body), "real skill\n")
	}
	p, projErr := svc.LoadProject(profileID, projectID)
	if projErr != nil {
		t.Fatalf("reload project: %v", projErr)
	}
	if !slices.Contains(p.SelectedAssetIDs, "something") {
		t.Fatalf("expected 'something' selected, got %v", p.SelectedAssetIDs)
	}
}

func TestCreateAssetFromFolder_CopiesNestedSubtree(t *testing.T) {
	svc, profileID, projectID, _ := seedFolderRegisterProject(t, map[string]string{
		".claude/skills/deep/SKILL.md":     "top\n",
		".claude/skills/deep/sub/inner.md": "deep\n",
	})

	id, err := svc.CreateAssetFromFolder(profileID, projectID, asset.Manifest{
		Name: "deep", Type: asset.TypeSkill,
	}, ".claude/skills/deep")
	if err != nil {
		t.Fatalf("CreateAssetFromFolder: %v", err)
	}
	prof, _ := svc.LoadProfile(profileID)
	created := prof.Profile.Assets[id]
	body, readErr := os.ReadFile(filepath.Join(created.Dir, "sub", "inner.md"))
	if readErr != nil {
		t.Fatalf("read nested copied content: %v", readErr)
	}
	if string(body) != "deep\n" {
		t.Fatalf("nested content = %q, want %q", string(body), "deep\n")
	}
}

func TestCreateAssetFromFolder_DuplicateIDReturnsAssetExistsError(t *testing.T) {
	// given a profile whose asset id collides with the derived id, and a
	// registerable folder for that id in the repo
	svc, profileID, projectID, _ := seedFolderRegisterProject(t, map[string]string{
		".claude/skills/dup/SKILL.md": "x\n",
	})
	if _, err := svc.InitAsset(profileID, asset.Manifest{
		ID: "dup", Name: "dup", Type: asset.TypeSkill,
	}); err != nil {
		t.Fatalf("seed asset: %v", err)
	}

	// when a folder slugs to the existing id
	_, err := svc.CreateAssetFromFolder(profileID, projectID, asset.Manifest{
		Name: "dup", Type: asset.TypeSkill,
	}, ".claude/skills/dup")

	// then creation is rejected
	var typed AssetExistsError
	if !errors.As(err, &typed) {
		t.Fatalf("expected AssetExistsError, got %T: %v", err, err)
	}
}

func TestCreateAssetFromFolder_MissingProjectReturnsProjectNotFoundError(t *testing.T) {
	svc, profileID, _, _ := seedFolderRegisterProject(t, nil)

	_, err := svc.CreateAssetFromFolder(profileID, "missing-project", asset.Manifest{
		Name: "something", Type: asset.TypeSkill,
	}, ".claude/skills/something")

	var typed ProjectNotFoundError
	if !errors.As(err, &typed) {
		t.Fatalf("expected ProjectNotFoundError, got %T: %v", err, err)
	}
}

func TestCreateAssetFromFolder_NonRegisterableFolderRejected(t *testing.T) {
	svc, profileID, projectID, _ := seedFolderRegisterProject(t, nil)

	// dirKey that is not an unknown folder in the plan
	_, err := svc.CreateAssetFromFolder(profileID, projectID, asset.Manifest{
		Name: "x", Type: asset.TypeSkill,
	}, ".claude/skills/missing")

	var typed FolderNotRegisterableError
	if !errors.As(err, &typed) {
		t.Fatalf("expected FolderNotRegisterableError, got %T: %v", err, err)
	}
}

func TestCreateAssetFromFolder_InvalidSourceLeavesNoPartialState(t *testing.T) {
	// given a registerable folder that lacks the skill's required SKILL.md
	svc, profileID, projectID, _ := seedFolderRegisterProject(t, map[string]string{
		".claude/skills/x/notes.txt": "no skill md\n",
	})

	// when registered as a skill
	_, err := svc.CreateAssetFromFolder(profileID, projectID, asset.Manifest{
		Name: "x", Type: asset.TypeSkill,
	}, ".claude/skills/x")

	// then it fails before creating or selecting anything
	var typed asset.MissingContentFileError
	if !errors.As(err, &typed) {
		t.Fatalf("expected MissingContentFileError, got %T: %v", err, err)
	}
	prof, _ := svc.LoadProfile(profileID)
	if prof.Profile.Assets["x"] != nil {
		t.Fatal("asset created despite invalid source")
	}
	p, _ := svc.LoadProject(profileID, projectID)
	if slices.Contains(p.SelectedAssetIDs, "x") {
		t.Fatal("asset selected despite failed creation")
	}
}
