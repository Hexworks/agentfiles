package render

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hexworks/agentfiles/internal/agent"
	"github.com/hexworks/agentfiles/internal/asset"
	"github.com/hexworks/agentfiles/internal/config"
	"github.com/hexworks/agentfiles/internal/profile"
	"github.com/hexworks/agentfiles/internal/project"
)

// writeAsset scaffolds one asset directory (manifest + content files)
// under a profile root. files maps asset-relative paths to bodies.
func writeAsset(t *testing.T, profileRoot, typeDir, manifestJSON string, files map[string]string) {
	t.Helper()
	id := mustID(t, manifestJSON)
	dir := filepath.Join(profileRoot, config.AssetsDirName, typeDir, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, config.AssetManifestFileName), []byte(manifestJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	for rel, body := range files {
		abs := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// mustID reads the "id" field out of a manifest literal so writeAsset can
// place the asset in <type>/<id>/ without the caller repeating it.
func mustID(t *testing.T, manifestJSON string) string {
	t.Helper()
	var m struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(manifestJSON), &m); err != nil {
		t.Fatalf("bad manifest literal: %v", err)
	}
	if m.ID == "" {
		t.Fatalf("manifest literal missing id: %s", manifestJSON)
	}
	return m.ID
}

// TestBuild_UnchangedPairs_Golden pins that a fixture rendered across all
// four agents is byte-/path-identical to the hand-authored golden for
// every pair whose behavior the strategy refactor left untouched (all
// skill layouts, all settings, the generic projection, and (Codex,
// AgentsDoc)). The new (claude-code, AgentsDoc) → CLAUDE.md pair is the
// only entry beyond the pre-refactor set and is asserted here too so the
// golden is a full-set equality, not a subset check.
func TestBuild_UnchangedPairs_Golden(t *testing.T) {
	root := t.TempDir()
	if _, err := profile.Init(root, "Personal"); err != nil {
		t.Fatal(err)
	}
	writeAsset(t, root, "skill", `{"id":"review","name":"review","type":"skill"}`, map[string]string{
		"SKILL.md":     "skill-body",
		"ref/notes.md": "notes",
	})
	writeAsset(t, root, "settings", `{"id":"cfg","name":"cfg","type":"settings"}`, map[string]string{
		"codex.toml":       "codex-cfg",
		"claude-code.json": "claude-cfg",
	})
	writeAsset(t, root, "rule", `{"id":"guard","name":"guard","type":"rule","projections":[{"agent":"codex","source":"body.md","target":".codex/guard.md"}]}`, map[string]string{
		"body.md": "guard-body",
	})
	writeAsset(t, root, "agents_doc", `{"id":"doc","name":"doc","type":"agents_doc"}`, map[string]string{
		"AGENTS.md": "doc-body",
	})
	loaded, err := profile.Load(root)
	if err != nil {
		t.Fatal(err)
	}

	plan, buildErrs := Build(loaded, &project.Manifest{
		ID: "app", Name: "app", Path: "/tmp/app",
		EnabledAgents:    agent.All(),
		SelectedAssetIDs: []string{"review", "cfg", "guard", "doc"},
	})
	if len(buildErrs) > 0 {
		t.Fatalf("build: %v", buildErrs)
	}

	type want struct{ body, sourceRel string }
	golden := map[string]want{
		// skill — folder-per-skill for the three container agents.
		".codex/skills/review/SKILL.md":        {"skill-body", "SKILL.md"},
		".codex/skills/review/ref/notes.md":    {"notes", "ref/notes.md"},
		".claude/skills/review/SKILL.md":       {"skill-body", "SKILL.md"},
		".claude/skills/review/ref/notes.md":   {"notes", "ref/notes.md"},
		".opencode/skills/review/SKILL.md":     {"skill-body", "SKILL.md"},
		".opencode/skills/review/ref/notes.md": {"notes", "ref/notes.md"},
		// skill — cursor flat file.
		".cursor/commands/review.md": {"skill-body", "SKILL.md"},
		// settings — only codex + claude-code sources exist.
		".codex/config.toml":          {"codex-cfg", "codex.toml"},
		".claude/settings.local.json": {"claude-cfg", "claude-code.json"},
		// generic projection.
		".codex/guard.md": {"guard-body", "body.md"},
		// agents_doc — AGENTS.md is the unchanged (Codex/Cursor/OpenCode)
		// pair; CLAUDE.md is the new (claude-code) pair.
		"AGENTS.md": {"doc-body", "AGENTS.md"},
		"CLAUDE.md": {"doc-body", "AGENTS.md"},
	}

	got := map[string]want{}
	for _, f := range plan.Files {
		got[f.Path] = want{string(f.Body), f.SourceRel}
	}
	if len(got) != len(golden) {
		t.Fatalf("rendered %d files, want %d\n got: %v", len(got), len(golden), got)
	}
	for path, w := range golden {
		g, ok := got[path]
		if !ok {
			t.Errorf("missing rendered file %q", path)
			continue
		}
		if g != w {
			t.Errorf("%q = %+v, want %+v", path, g, w)
		}
	}
}

// TestStrategyFor_MissingPair_AggregatesUnsupportedError pins that an
// enabled agent with no strategy for a selected asset's type surfaces a
// single joined UnsupportedRenderingError naming the agent and type,
// rather than short-circuiting or silently rendering nothing.
func TestStrategyFor_MissingPair_AggregatesUnsupportedError(t *testing.T) {
	if _, ok := strategyFor(agent.Agent("bogus"), asset.TypeSkill); ok {
		t.Fatal("strategyFor returned ok for an unregistered pair")
	}

	root := t.TempDir()
	if _, err := profile.Init(root, "Personal"); err != nil {
		t.Fatal(err)
	}
	writeAsset(t, root, "skill", `{"id":"review","name":"review","type":"skill"}`, map[string]string{
		"SKILL.md": "body",
	})
	loaded, err := profile.Load(root)
	if err != nil {
		t.Fatal(err)
	}

	_, buildErrs := Build(loaded, &project.Manifest{
		ID: "app", Name: "app", Path: "/tmp/app",
		EnabledAgents:    []agent.Agent{agent.Agent("bogus")},
		SelectedAssetIDs: []string{"review"},
	})
	if len(buildErrs) != 1 {
		t.Fatalf("expected exactly one accumulated error, got %v", buildErrs)
	}
	var unsupported UnsupportedRenderingError
	if !errors.As(buildErrs[0], &unsupported) {
		t.Fatalf("expected UnsupportedRenderingError, got %T: %v", buildErrs[0], buildErrs[0])
	}
	if unsupported.Agent != agent.Agent("bogus") || unsupported.Type != asset.TypeSkill {
		t.Fatalf("error carried %+v, want {bogus skill}", unsupported)
	}
	if msg := unsupported.Error(); msg != "missing render strategy for bogus skill" {
		t.Fatalf("Error() = %q, want it to name agent and type", msg)
	}
}

// TestReverse_SkillFolderAgents_RoundTrip pins that a multi-file skill
// rendered for each folder-per-skill agent reverses every rendered file
// and any untracked sibling inside the skill dir back to its
// asset-relative source.
func TestReverse_SkillFolderAgents_RoundTrip(t *testing.T) {
	root := t.TempDir()
	if _, err := profile.Init(root, "Personal"); err != nil {
		t.Fatal(err)
	}
	writeAsset(t, root, "skill", `{"id":"foo","name":"foo","type":"skill"}`, map[string]string{
		"SKILL.md":    "body",
		"ref/note.md": "note",
	})
	loaded, err := profile.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	plan, buildErrs := Build(loaded, &project.Manifest{
		ID: "app", Name: "app", Path: "/tmp/app",
		EnabledAgents:    []agent.Agent{agent.Codex, agent.ClaudeCode, agent.OpenCode},
		SelectedAssetIDs: []string{"foo"},
	})
	if len(buildErrs) > 0 {
		t.Fatalf("build: %v", buildErrs)
	}

	for _, prefix := range []string{".codex/skills/foo", ".claude/skills/foo", ".opencode/skills/foo"} {
		assertReverse(t, plan, prefix+"/SKILL.md", "foo", "SKILL.md")
		assertReverse(t, plan, prefix+"/ref/note.md", "foo", "ref/note.md")
		// Untracked sibling not in the plan still reverses via the owning dir.
		assertReverse(t, plan, prefix+"/EXTRA.md", "foo", "EXTRA.md")
	}
}

// TestReverse_Settings_And_AgentsDoc_RoundTrip pins that the two
// filename-changing single-file strategies reverse to the source name the
// forward render read from — settings to its descriptor source, and both
// the claude-code CLAUDE.md and the other agents' AGENTS.md to the shared
// agents_doc source.
func TestReverse_Settings_And_AgentsDoc_RoundTrip(t *testing.T) {
	root := t.TempDir()
	if _, err := profile.Init(root, "Personal"); err != nil {
		t.Fatal(err)
	}
	writeAsset(t, root, "settings", `{"id":"cfg","name":"cfg","type":"settings"}`, map[string]string{
		"codex.toml":       "codex-cfg",
		"claude-code.json": "claude-cfg",
	})
	writeAsset(t, root, "agents_doc", `{"id":"doc","name":"doc","type":"agents_doc"}`, map[string]string{
		"AGENTS.md": "doc-body",
	})
	loaded, err := profile.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	plan, buildErrs := Build(loaded, &project.Manifest{
		ID: "app", Name: "app", Path: "/tmp/app",
		EnabledAgents:    []agent.Agent{agent.Codex, agent.ClaudeCode},
		SelectedAssetIDs: []string{"cfg", "doc"},
	})
	if len(buildErrs) > 0 {
		t.Fatalf("build: %v", buildErrs)
	}

	assertReverse(t, plan, ".codex/config.toml", "cfg", "codex.toml")
	assertReverse(t, plan, ".claude/settings.local.json", "cfg", "claude-code.json")
	assertReverse(t, plan, "CLAUDE.md", "doc", "AGENTS.md")
	assertReverse(t, plan, "AGENTS.md", "doc", "AGENTS.md")
}

// TestReverse_GenericProjection_RoundTrip pins that a single-file
// generic projection (which renames its target) and a walked-directory
// projection both reverse to source — the renamed file via its recorded
// SourceRel, and an untracked sibling inside the walked dir via the
// suffix-preserving inverse.
func TestReverse_GenericProjection_RoundTrip(t *testing.T) {
	root := t.TempDir()
	if _, err := profile.Init(root, "Personal"); err != nil {
		t.Fatal(err)
	}
	writeAsset(t, root, "rule", `{"id":"guard","name":"guard","type":"rule","projections":[{"agent":"codex","source":"body.md","target":".codex/guard.md"}]}`, map[string]string{
		"body.md": "guard-body",
	})
	writeAsset(t, root, "hook", `{"id":"chain","name":"chain","type":"hook","projections":[{"agent":"codex","source":"pre","target":".codex/hooks"}]}`, map[string]string{
		"pre/a.sh": "a",
	})
	loaded, err := profile.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	plan, buildErrs := Build(loaded, &project.Manifest{
		ID: "app", Name: "app", Path: "/tmp/app",
		EnabledAgents:    []agent.Agent{agent.Codex},
		SelectedAssetIDs: []string{"guard", "chain"},
	})
	if len(buildErrs) > 0 {
		t.Fatalf("build: %v", buildErrs)
	}

	// Renamed single-file projection: recorded SourceRel is authoritative.
	assertReverse(t, plan, ".codex/guard.md", "guard", "body.md")
	// Walked directory: rendered file and an untracked sibling.
	assertReverse(t, plan, ".codex/hooks/a.sh", "chain", "pre/a.sh")
	assertReverse(t, plan, ".codex/hooks/b.sh", "chain", "pre/b.sh")
}

func assertReverse(t *testing.T, plan *ProjectPlan, repoPath, wantAsset, wantSource string) {
	t.Helper()
	id, src, ok := plan.ReverseLookup(repoPath)
	if !ok {
		t.Fatalf("ReverseLookup(%q) ok=false, want (%q,%q)", repoPath, wantAsset, wantSource)
	}
	if id != wantAsset || src != wantSource {
		t.Fatalf("ReverseLookup(%q) = (%q,%q), want (%q,%q)", repoPath, id, src, wantAsset, wantSource)
	}
}
