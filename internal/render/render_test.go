package render

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/addamsson/agentfiles/internal/asset"
	"github.com/addamsson/agentfiles/internal/config"
	"github.com/addamsson/agentfiles/internal/profile"
	"github.com/addamsson/agentfiles/internal/project"
)

func TestBuildSkillAndAgentsDoc(t *testing.T) {
	root := t.TempDir()
	if _, err := profile.Init(root, "Personal"); err != nil {
		t.Fatalf("init profile: %v", err)
	}
	skillDir := filepath.Join(root, config.AssetsDirName, "skill", "review")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, config.AssetManifestFileName), []byte(`{"id":"review","name":"review","type":"skill"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	docDir := filepath.Join(root, config.AssetsDirName, "agents_doc", "base")
	if err := os.MkdirAll(docDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docDir, config.AssetManifestFileName), []byte(`{"id":"base","name":"base","type":"agents_doc"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docDir, "AGENTS.md"), []byte("agents"), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := profile.Load(root)
	if err != nil {
		t.Fatalf("load profile: %v", err)
	}
	plan, buildErrs := Build(loaded, &project.Manifest{
		ID:               "app",
		Name:             "app",
		Path:             "/tmp/app",
		EnabledAgents:    []string{"codex", "cursor"},
		SelectedAssetIDs: []string{"review", "base"},
		CreatedAt:        time.Now(),
	})
	if len(buildErrs) > 0 {
		t.Fatalf("build: %v", buildErrs)
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

func TestBuild_AccumulatesMissingAssets(t *testing.T) {
	root := t.TempDir()
	if _, err := profile.Init(root, "Personal"); err != nil {
		t.Fatal(err)
	}
	loaded, err := profile.Load(root)
	if err != nil {
		t.Fatal(err)
	}

	_, buildErrs := Build(loaded, &project.Manifest{
		ID:               "app",
		Name:             "app",
		Path:             "/tmp/app",
		EnabledAgents:    []string{"codex"},
		SelectedAssetIDs: []string{"missing-1", "missing-2"},
		CreatedAt:        time.Now(),
	})

	if len(buildErrs) == 0 {
		t.Fatal("expected error")
	}
	missingIDs := map[string]bool{}
	for _, e := range buildErrs {
		var typed AssetNotFoundError
		if errors.As(e, &typed) {
			missingIDs[typed.AssetID] = true
		}
	}
	if !missingIDs["missing-1"] || !missingIDs["missing-2"] {
		t.Fatalf("expected both ids accumulated, got %v", missingIDs)
	}
}

func TestResolveAssets_AccumulatesMissingIDs(t *testing.T) {
	root := t.TempDir()
	if _, err := profile.Init(root, "Personal"); err != nil {
		t.Fatal(err)
	}
	loaded, err := profile.Load(root)
	if err != nil {
		t.Fatal(err)
	}

	selected, resolveErrs := resolveAssets(loaded, &project.Manifest{
		SelectedAssetIDs: []string{"a", "b", "c"},
	})

	if len(selected) != 0 {
		t.Fatalf("expected no resolved assets, got %d", len(selected))
	}
	if len(resolveErrs) != 3 {
		t.Fatalf("expected 3 typed errors, got %d", len(resolveErrs))
	}
	for _, e := range resolveErrs {
		var typed AssetNotFoundError
		if !errors.As(e, &typed) {
			t.Fatalf("expected AssetNotFoundError, got %T: %v", e, e)
		}
	}
}

func TestBuild_AccumulatesExclusiveGroupConflicts(t *testing.T) {
	root := t.TempDir()
	if _, err := profile.Init(root, "Personal"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"primary-a", "primary-b"} {
		dir := filepath.Join(root, config.AssetsDirName, "agents_doc", id)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		manifest := `{"id":"` + id + `","name":"` + id + `","type":"agents_doc","exclusive_group":"primary"}`
		if err := os.WriteFile(filepath.Join(dir, config.AssetManifestFileName), []byte(manifest), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(id), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	loaded, err := profile.Load(root)
	if err != nil {
		t.Fatal(err)
	}

	_, buildErrs := Build(loaded, &project.Manifest{
		ID:               "app",
		Name:             "app",
		Path:             "/tmp/app",
		EnabledAgents:    []string{"codex"},
		SelectedAssetIDs: []string{"primary-a", "primary-b"},
		CreatedAt:        time.Now(),
	})

	if len(buildErrs) == 0 {
		t.Fatal("expected error")
	}
	var conflict ExclusiveGroupConflictError
	if !errors.As(buildErrs[0], &conflict) {
		t.Fatalf("expected ExclusiveGroupConflictError first, got %T: %v", buildErrs[0], buildErrs[0])
	}
	if conflict.Group != "primary" {
		t.Fatalf("expected group 'primary', got %q", conflict.Group)
	}
	if len(conflict.AssetIDs) != 2 {
		t.Fatalf("expected 2 asset ids, got %v", conflict.AssetIDs)
	}
}

func TestBuild_WrapsPerAssetFailureWithAssetSourceMissing(t *testing.T) {
	root := t.TempDir()
	if _, err := profile.Init(root, "Personal"); err != nil {
		t.Fatal(err)
	}
	// Skill manifest exists but no SKILL.md body — addSkillOutputs will
	// hit a "source missing" failure that the per-asset wrapper must
	// classify as AssetSourceMissingError.
	skillDir := filepath.Join(root, config.AssetsDirName, "skill", "review")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, config.AssetManifestFileName),
		[]byte(`{"id":"review","name":"review","type":"skill"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := profile.Load(root)
	if err != nil {
		t.Fatal(err)
	}

	_, buildErrs := Build(loaded, &project.Manifest{
		ID:               "app",
		Name:             "app",
		Path:             "/tmp/app",
		EnabledAgents:    []string{"codex"},
		SelectedAssetIDs: []string{"review"},
		CreatedAt:        time.Now(),
	})

	if len(buildErrs) == 0 {
		t.Fatal("expected error")
	}
	var typed AssetSourceMissingError
	if !errors.As(buildErrs[0], &typed) {
		t.Fatalf("expected AssetSourceMissingError, got %T: %v", buildErrs[0], buildErrs[0])
	}
	if typed.AssetID != "review" {
		t.Fatalf("expected asset id 'review', got %q", typed.AssetID)
	}
	if typed.RelPath != config.SkillStarterFileName {
		t.Fatalf("expected relative path %q, got %q", config.SkillStarterFileName, typed.RelPath)
	}
}

func TestBuild_TargetOutsideSurfacesIsTyped(t *testing.T) {
	root := t.TempDir()
	if _, err := profile.Init(root, "Personal"); err != nil {
		t.Fatal(err)
	}
	// A rule asset whose projection target lies outside managed surfaces.
	ruleDir := filepath.Join(root, config.AssetsDirName, "rule", "leak")
	if err := os.MkdirAll(ruleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"id":"leak","name":"leak","type":"rule","projections":[{"agent":"codex","source":"body.md","target":"outside/leak.md"}]}`
	if err := os.WriteFile(filepath.Join(ruleDir, config.AssetManifestFileName), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ruleDir, "body.md"), []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := profile.Load(root)
	if err != nil {
		t.Fatal(err)
	}

	_, buildErrs := Build(loaded, &project.Manifest{
		ID:               "app",
		Name:             "app",
		Path:             "/tmp/app",
		EnabledAgents:    []string{"codex"},
		SelectedAssetIDs: []string{"leak"},
		CreatedAt:        time.Now(),
	})

	if len(buildErrs) == 0 {
		t.Fatal("expected error")
	}
	var typed TargetOutsideSurfacesError
	if !errors.As(buildErrs[0], &typed) {
		t.Fatalf("expected TargetOutsideSurfacesError, got %T: %v", buildErrs[0], buildErrs[0])
	}
	if typed.Target != "outside/leak.md" {
		t.Fatalf("expected target preserved, got %q", typed.Target)
	}
}

// silence unused import when only some tests reference asset package.
var _ = asset.TypeSkill
