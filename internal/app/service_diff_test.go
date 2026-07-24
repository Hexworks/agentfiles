package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hexworks/agentfiles/internal/agent"
	"github.com/hexworks/agentfiles/internal/appapi"
	"github.com/hexworks/agentfiles/internal/asset"
	llmsync "github.com/hexworks/agentfiles/internal/sync"
)

// seedDiffProject builds a real profile+project whose repo holds one managed
// skill asset (two files) rendered to disk, then diverges the two files so a
// re-plan classifies one as drift and the other as update:
//
//   - .claude/skills/mine/SKILL.md  — drifted: the on-disk copy is edited so
//     local != desired (managed).
//   - .claude/skills/mine/helper.md — updated: the asset *source* is edited so
//     desired (managed) != the untouched on-disk copy.
//
// Everything is real — real profile.Init via CreateProfile, real
// CreateAssetFromFolder copy, real render.Build, real os writes under
// t.TempDir() — so DiffFile is exercised end to end (no fakes).
func seedDiffProject(t *testing.T) (svc *Service, profileID, projectID, repoPath string) {
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
	if _, addErrs := svc.AddProject(profileID, "Repo", repoPath, []agent.Agent{agent.ClaudeCode}, nil); len(addErrs) > 0 {
		t.Fatalf("add project: %v", addErrs)
	}
	projectID = "repo"

	// Initial apply writes .agentfiles/state.json so the later unknown/drift
	// detection pass runs (it is skipped on the first apply).
	if _, err := svc.Apply(profileID, projectID, appapi.Resolutions{}); err != nil {
		t.Fatalf("initial apply: %v", err)
	}

	// Seed an unmanaged skill folder in the repo, then register it as a
	// profile-owned asset so its files become managed.
	writeRepoFile(t, repoPath, ".claude/skills/mine/SKILL.md", "orig skill\n")
	writeRepoFile(t, repoPath, ".claude/skills/mine/helper.md", "orig helper\n")
	if _, err := svc.CreateAssetFromFolder(profileID, projectID, asset.Manifest{
		Name: "mine", Type: asset.TypeSkill,
	}, ".claude/skills/mine"); err != nil {
		t.Fatalf("register folder: %v", err)
	}

	// Apply so the managed files are written and recorded in state; now
	// on-disk == desired for both.
	if _, err := svc.Apply(profileID, projectID, appapi.Resolutions{}); err != nil {
		t.Fatalf("apply after register: %v", err)
	}

	// Drift SKILL.md on disk (local diverges from managed).
	writeRepoFile(t, repoPath, ".claude/skills/mine/SKILL.md", "drifted local\n")

	// Update helper.md via the asset source (managed diverges from local).
	loaded, loadErr := svc.LoadProfile(profileID)
	if loadErr != nil {
		t.Fatalf("reload profile: %v", loadErr)
	}
	assetDir := loaded.Profile.Assets["mine"].Dir
	if err := os.WriteFile(filepath.Join(assetDir, "helper.md"), []byte("updated managed\n"), 0o644); err != nil {
		t.Fatalf("edit asset source: %v", err)
	}
	return svc, profileID, projectID, repoPath
}

func writeRepoFile(t *testing.T, repoPath, rel, body string) {
	t.Helper()
	full := filepath.Join(repoPath, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDiffFile(t *testing.T) {
	svc, profileID, projectID, _ := seedDiffProject(t)

	// Sanity: the plan classifies the two paths as drift and update, so the
	// fixture exercises exactly the two kinds that offer [Diff].
	preview, planErr := svc.Plan(profileID, projectID)
	if planErr != nil {
		t.Fatalf("plan: %v", planErr)
	}
	assertKind(t, preview, ".claude/skills/mine/SKILL.md", appapi.ChangeDrift)
	assertKind(t, preview, ".claude/skills/mine/helper.md", appapi.ChangeUpdate)

	cases := []struct {
		name        string
		path        string
		wantLocal   string
		wantDesired string
	}{
		{"drifted file", ".claude/skills/mine/SKILL.md", "drifted local\n", "orig skill\n"},
		{"updated file", ".claude/skills/mine/helper.md", "orig helper\n", "updated managed\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bodies, err := svc.DiffFile(profileID, projectID, tc.path)
			if err != nil {
				t.Fatalf("DiffFile: %v", err)
			}
			if string(bodies.Local) != tc.wantLocal {
				t.Errorf("Local = %q, want %q", bodies.Local, tc.wantLocal)
			}
			if string(bodies.Desired) != tc.wantDesired {
				t.Errorf("Desired = %q, want %q", bodies.Desired, tc.wantDesired)
			}
		})
	}
}

func TestDiffFile_LocalReadFailureReturnsTypedError(t *testing.T) {
	svc, profileID, projectID, repoPath := seedDiffProject(t)

	// The file was here at plan time; remove it before the diff so the local
	// read fails.
	if err := os.Remove(filepath.Join(repoPath, ".claude", "skills", "mine", "SKILL.md")); err != nil {
		t.Fatalf("remove local file: %v", err)
	}

	_, err := svc.DiffFile(profileID, projectID, ".claude/skills/mine/SKILL.md")
	if err == nil {
		t.Fatal("DiffFile err = nil, want DiffLocalReadError")
	}
	var typed DiffLocalReadError
	if !errors.As(err, &typed) {
		t.Fatalf("err = %T (%v), want DiffLocalReadError", err, err)
	}
	if typed.Path != ".claude/skills/mine/SKILL.md" {
		t.Errorf("DiffLocalReadError.Path = %q, want the requested path", typed.Path)
	}
}

// TestDiffFile_LocalSymlinkRefused pins the disclosure guard: a managed file
// swapped for a symlink is refused with the same sync.UnsafeSymlinkError the
// Adopt and sync write paths raise — no intra-repo exception, so read and write
// treat a symlinked managed file identically. The target is placed *inside* the
// repo to prove even a non-escaping link is refused, not just an escaping one.
func TestDiffFile_LocalSymlinkRefused(t *testing.T) {
	svc, profileID, projectID, repoPath := seedDiffProject(t)

	inside := filepath.Join(repoPath, "secret")
	if err := os.WriteFile(inside, []byte("secret contents\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(repoPath, ".claude", "skills", "mine", "SKILL.md")
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(inside, link); err != nil {
		t.Fatal(err)
	}

	bodies, err := svc.DiffFile(profileID, projectID, ".claude/skills/mine/SKILL.md")
	var typed llmsync.UnsafeSymlinkError
	if !errors.As(err, &typed) {
		t.Fatalf("err = %T (%v), want sync.UnsafeSymlinkError", err, err)
	}
	if string(bodies.Local) != "" {
		t.Errorf("Local = %q, want empty (no bytes read through the symlink)", bodies.Local)
	}
}

// TestDiffFile_RejectsPathEscape pins the boundary path-key guard: a crafted
// "../.." key is rejected before any join/read, independent of render's clean
// output shape.
func TestDiffFile_RejectsPathEscape(t *testing.T) {
	svc, profileID, projectID, _ := seedDiffProject(t)

	_, err := svc.DiffFile(profileID, projectID, "../../etc/passwd")
	var typed llmsync.InvalidPathError
	if !errors.As(err, &typed) {
		t.Fatalf("err = %T (%v), want sync.InvalidPathError", err, err)
	}
}

func assertKind(t *testing.T, preview *appapi.Preview, path string, want appapi.ChangeKind) {
	t.Helper()
	for _, ch := range preview.Changes {
		if ch.Path == path {
			if ch.Kind != want {
				t.Fatalf("kind for %s = %q, want %q", path, ch.Kind, want)
			}
			return
		}
	}
	t.Fatalf("path %s not present in plan changes", path)
}
