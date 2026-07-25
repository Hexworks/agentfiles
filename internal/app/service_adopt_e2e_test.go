package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hexworks/agentfiles/internal/agent"
	"github.com/hexworks/agentfiles/internal/appapi"
	"github.com/hexworks/agentfiles/internal/asset"
	"github.com/hexworks/agentfiles/internal/render"
)

// TestAdopt_SkillDrift_EndToEnd exercises the whole Adopt reverse flow
// through the real stack: a managed skill file is drifted on disk, the
// render plan's ReverseLookup recovers its (assetID, sourceRel), and a
// real Service.Apply with a DriftAdopt resolution writes the local body
// back into the owning profile asset. The proof is the profile asset file
// read off disk after Apply — not a constant the code also produced.
func TestAdopt_SkillDrift_EndToEnd(t *testing.T) {
	root := t.TempDir()
	svc := newSvc(root)
	if _, err := svc.CreateProfile("Personal", filepath.Join(root, "profile")); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	profileID := "personal"
	repoPath := filepath.Join(root, "repo")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, addErrs := svc.AddProject(profileID, "Repo", repoPath, []agent.Agent{agent.ClaudeCode}, nil); len(addErrs) > 0 {
		t.Fatalf("add project: %v", addErrs)
	}
	projectID := "repo"

	// First apply records state so later drift detection runs.
	if _, err := svc.Apply(profileID, projectID, appapi.Resolutions{}); err != nil {
		t.Fatalf("initial apply: %v", err)
	}

	// Register an in-repo skill folder as a managed asset, then apply so
	// on-disk == desired and state carries v3 provenance.
	writeRepoFile(t, repoPath, ".claude/skills/mine/SKILL.md", "orig skill\n")
	if _, err := svc.CreateAssetFromFolder(profileID, projectID, asset.Manifest{
		Name: "mine", Type: asset.TypeSkill,
	}, ".claude/skills/mine"); err != nil {
		t.Fatalf("register folder: %v", err)
	}
	if _, err := svc.Apply(profileID, projectID, appapi.Resolutions{}); err != nil {
		t.Fatalf("apply after register: %v", err)
	}

	// Drift the managed file on disk.
	const driftedBody = "drifted local\n"
	writeRepoFile(t, repoPath, ".claude/skills/mine/SKILL.md", driftedBody)

	// The render plan's ReverseLookup is the single source of the
	// repo→asset mapping the Adopt write relies on.
	loaded, loadErr := svc.LoadProfile(profileID)
	if loadErr != nil {
		t.Fatalf("reload profile: %v", loadErr)
	}
	proj := loaded.Projects[projectID]
	plan, buildErrs := render.Build(loaded.Profile, proj)
	if len(buildErrs) > 0 {
		t.Fatalf("build: %v", buildErrs)
	}
	match, ok := plan.ReverseLookup(".claude/skills/mine/SKILL.md")
	if !ok || match.AssetID != "mine" || match.SourceRel != "SKILL.md" {
		t.Fatalf("ReverseLookup = (%q,%q,%v), want (mine, SKILL.md, true)", match.AssetID, match.SourceRel, ok)
	}

	// Adopt the drift: Service.Apply must write the local body back into
	// the owning profile asset.
	if _, err := svc.Apply(profileID, projectID, appapi.Resolutions{
		Drift: []appapi.DriftResolution{{Path: ".claude/skills/mine/SKILL.md", Decision: appapi.DriftAdopt}},
	}); err != nil {
		t.Fatalf("adopt apply: %v", err)
	}

	assetDir := loaded.Profile.Assets[match.AssetID].Dir
	got, readErr := os.ReadFile(filepath.Join(assetDir, filepath.FromSlash(match.SourceRel)))
	if readErr != nil {
		t.Fatalf("read profile asset: %v", readErr)
	}
	if string(got) != driftedBody {
		t.Fatalf("profile asset SKILL.md = %q, want adopted body %q", got, driftedBody)
	}
}
