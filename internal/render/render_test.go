package render

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/addamsson/agentfiles/internal/profile"
	"github.com/addamsson/agentfiles/internal/project"
)

func TestBuildSkillAndAgentsDoc(t *testing.T) {
	root := t.TempDir()
	if _, err := profile.Init(root, "Personal"); err != nil {
		t.Fatalf("init profile: %v", err)
	}
	skillDir := filepath.Join(root, "assets", "skill", "review")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "asset.json"), []byte(`{"id":"review","name":"review","type":"skill"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	docDir := filepath.Join(root, "assets", "agents_doc", "base")
	if err := os.MkdirAll(docDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docDir, "asset.json"), []byte(`{"id":"base","name":"base","type":"agents_doc"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docDir, "AGENTS.md"), []byte("agents"), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := profile.Load(root)
	if err != nil {
		t.Fatalf("load profile: %v", err)
	}
	plan, err := Build(loaded, &project.Manifest{
		ID:               "app",
		Name:             "app",
		Path:             "/tmp/app",
		EnabledAgents:    []string{"codex", "cursor"},
		SelectedAssetIDs: []string{"review", "base"},
		CreatedAt:        time.Now(),
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	paths := map[string]bool{}
	for _, file := range plan.Files {
		paths[file.Path] = true
	}
	if !paths["AGENTS.md"] {
		t.Fatal("expected AGENTS.md")
	}
	if !paths[".codex/skills/review/SKILL.md"] {
		t.Fatal("expected codex skill")
	}
	if !paths[".cursor/commands/review.md"] {
		t.Fatal("expected cursor skill projection")
	}
}
