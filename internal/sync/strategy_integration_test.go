package sync

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hexworks/agentfiles/internal/agent"
	"github.com/hexworks/agentfiles/internal/config"
	"github.com/hexworks/agentfiles/internal/profile"
	"github.com/hexworks/agentfiles/internal/project"
	"github.com/hexworks/agentfiles/internal/render"
)

// setupAgentsDocProject scaffolds a profile whose single agents_doc asset
// is selected for all four agents, and returns the loaded profile plus a
// project manifest pointing at projectRoot.
func setupAgentsDocProject(t *testing.T, projectRoot string) (*profile.Profile, *project.Manifest) {
	t.Helper()
	profileRoot := t.TempDir()
	if _, err := profile.Init(profileRoot, "Personal"); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(profileRoot, config.AssetsDirName, "agents_doc", "doc")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, config.AssetManifestFileName), []byte(`{"id":"doc","name":"doc","type":"agents_doc"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("shared-doc-body"), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := profile.Load(profileRoot)
	if err != nil {
		t.Fatal(err)
	}
	return loaded, &project.Manifest{
		ID:               "app",
		Name:             "app",
		Path:             projectRoot,
		EnabledAgents:    agent.All(),
		SelectedAssetIDs: []string{"doc"},
	}
}

// TestBuild_AgentsDoc_PerAgentTargets exercises the real render→sync
// stack: an agents_doc asset enabled for all four agents must materialize
// CLAUDE.md (claude-code) and AGENTS.md (codex/cursor/opencode) on disk,
// both carrying the shared source body. Asserted against the actual bytes
// written into t.TempDir(), not a constant the code also produced.
func TestBuild_AgentsDoc_PerAgentTargets(t *testing.T) {
	projectRoot := t.TempDir()
	loaded, proj := setupAgentsDocProject(t, projectRoot)

	preview, planErr := Plan(loaded, proj)
	if planErr != nil {
		t.Fatalf("Plan: %v", planErr)
	}
	if _, applyErr := Apply(preview, Resolutions{}); applyErr != nil {
		t.Fatalf("Apply: %v", applyErr)
	}

	for _, name := range []string{"CLAUDE.md", "AGENTS.md"} {
		body, err := os.ReadFile(filepath.Join(projectRoot, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if string(body) != "shared-doc-body" {
			t.Errorf("%s = %q, want %q", name, body, "shared-doc-body")
		}
	}
}

// TestPlan_ClaudeMd_SurfacesNotFenced pins that CLAUDE.md flows through
// the render→sync stack as a normal managed change rather than being
// rejected by the managed-surface fence. The claude-code agents_doc
// target could not exist before the fence was widened.
func TestPlan_ClaudeMd_SurfacesNotFenced(t *testing.T) {
	projectRoot := t.TempDir()
	profileRoot := t.TempDir()
	if _, err := profile.Init(profileRoot, "Personal"); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(profileRoot, config.AssetsDirName, "agents_doc", "doc")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, config.AssetManifestFileName), []byte(`{"id":"doc","name":"doc","type":"agents_doc"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("claude-doc"), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := profile.Load(profileRoot)
	if err != nil {
		t.Fatal(err)
	}
	proj := &project.Manifest{
		ID: "app", Name: "app", Path: projectRoot,
		EnabledAgents:    []agent.Agent{agent.ClaudeCode},
		SelectedAssetIDs: []string{"doc"},
	}

	preview, planErr := Plan(loaded, proj)
	if planErr != nil {
		t.Fatalf("Plan: %v", planErr)
	}
	change := findChange(t, preview.Changes, "CLAUDE.md")
	if change.Kind != ChangeCreate {
		t.Fatalf("CLAUDE.md kind = %q, want create (fence rejection would drop it)", change.Kind)
	}

	// A real Apply must write it without an OutsideSurfaceError.
	if _, applyErr := Apply(preview, Resolutions{}); applyErr != nil {
		t.Fatalf("Apply: %v", applyErr)
	}
	if _, err := os.Stat(filepath.Join(projectRoot, "CLAUDE.md")); err != nil {
		t.Fatalf("CLAUDE.md not written: %v", err)
	}
}

// TestReverse_CursorSkill_NotInvertible pins that cursor's flat-file
// skill layout stays non-adoptable: (Cursor, Skill).Reverse reports
// ok=false, and a stray file inside .cursor/commands surfaces on the plan
// with no OwningAssetID — so the TUI offers no Adopt row for it.
func TestReverse_CursorSkill_NotInvertible(t *testing.T) {
	projectRoot := t.TempDir()
	profileRoot := t.TempDir()
	if _, err := profile.Init(profileRoot, "Personal"); err != nil {
		t.Fatal(err)
	}
	skillDir := filepath.Join(profileRoot, config.AssetsDirName, "skill", "foo")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, config.AssetManifestFileName), []byte(`{"id":"foo","name":"foo","type":"skill"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("skill body"), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := profile.Load(profileRoot)
	if err != nil {
		t.Fatal(err)
	}
	proj := &project.Manifest{
		ID: "app", Name: "app", Path: projectRoot,
		EnabledAgents:    []agent.Agent{agent.Cursor},
		SelectedAssetIDs: []string{"foo"},
	}

	// First apply the known cursor command so a subsequent plan records it.
	firstPlan, planErr := Plan(loaded, proj)
	if planErr != nil {
		t.Fatalf("Plan: %v", planErr)
	}
	if _, applyErr := Apply(firstPlan, Resolutions{}); applyErr != nil {
		t.Fatalf("Apply: %v", applyErr)
	}

	// The known flat file is itself non-invertible: the strategy is the
	// authority, independent of any stored provenance.
	renderPlan := &render.ProjectPlan{Files: firstPlan.Files}
	if _, _, ok := renderPlan.ReverseLookup(".cursor/commands/foo.md"); ok {
		t.Fatal("ReverseLookup(.cursor/commands/foo.md) ok=true, want false (cursor skill is lossy)")
	}

	// Drop a stray file into .cursor/commands and re-plan: it must show up
	// as unknown with no owning asset, so no Adopt row is offered.
	stray := filepath.Join(projectRoot, ".cursor", "commands", "stray.md")
	if err := os.WriteFile(stray, []byte("stray"), 0o644); err != nil {
		t.Fatal(err)
	}
	preview, planErr := Plan(loaded, proj)
	if planErr != nil {
		t.Fatalf("Plan: %v", planErr)
	}
	unknown := findChange(t, preview.Changes, ".cursor/commands/stray.md")
	if unknown.Kind != ChangeUnknown {
		t.Fatalf("stray kind = %q, want unknown", unknown.Kind)
	}
	if unknown.OwningAssetID != "" {
		t.Fatalf("OwningAssetID = %q, want empty (cursor dir is non-adoptable)", unknown.OwningAssetID)
	}
}
