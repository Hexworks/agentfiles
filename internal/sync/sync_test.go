package sync

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/addamsson/agentfiles/internal/profile"
	"github.com/addamsson/agentfiles/internal/project"
)

func TestPlanDetectsDriftAndDeleteCandidate(t *testing.T) {
	projectRoot := t.TempDir()
	profileRoot := t.TempDir()
	if _, err := profile.Init(profileRoot, "Personal"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(profileRoot, "assets", "agents_doc", "base"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profileRoot, "assets", "agents_doc", "base", "asset.json"), []byte(`{"id":"base","name":"base","type":"agents_doc"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profileRoot, "assets", "agents_doc", "base", "AGENTS.md"), []byte("wanted"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "AGENTS.md"), []byte("drifted"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectRoot, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, ".codex", "old.txt"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectRoot, ".agentfiles"), 0o755); err != nil {
		t.Fatal(err)
	}
	state := ManagedState{
		ProfileID:        "personal",
		ProjectID:        "app",
		GeneratorVersion: GeneratorVersion,
		LastAppliedAt:    time.Now(),
		ManagedFiles:     map[string]string{"AGENTS.md": "previous"},
	}
	if err := os.WriteFile(filepath.Join(projectRoot, StatePath), mustJSON(t, state), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := profile.Load(profileRoot)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := Plan(loaded, &project.Manifest{
		ID:               "app",
		Name:             "app",
		Path:             projectRoot,
		EnabledAgents:    []string{"codex"},
		SelectedAssetIDs: []string{"base"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var sawDrift, sawDelete bool
	for _, change := range preview.Changes {
		if change.Kind == ChangeDrift && change.Path == "AGENTS.md" {
			sawDrift = true
		}
		if change.Kind == ChangeDelete && change.Path == ".codex/old.txt" {
			sawDelete = true
		}
	}
	if !sawDrift {
		t.Fatal("expected drift")
	}
	if !sawDelete {
		t.Fatal("expected delete candidate")
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
