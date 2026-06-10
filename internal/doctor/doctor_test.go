package doctor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hexworks/agentfiles/internal/config"
	"github.com/hexworks/agentfiles/internal/profile"
	llmsync "github.com/hexworks/agentfiles/internal/sync"
)

func TestCheckProfile_CleanProjectHasEmptyChanges(t *testing.T) {
	profileRoot := t.TempDir()
	projectRoot := t.TempDir()
	loaded := setupProfileWithProject(t, profileRoot, projectRoot, "matched")
	// project file content matches the asset content, so no changes
	if err := os.WriteFile(filepath.Join(projectRoot, "AGENTS.md"), []byte("matched"), 0o644); err != nil {
		t.Fatal(err)
	}
	seedManagedState(t, projectRoot)

	report, checkErrs := CheckProfile(loaded)
	if len(checkErrs) > 0 {
		t.Fatalf("check: %v", checkErrs)
	}

	if report.ProfileName != "Personal" {
		t.Fatalf("unexpected profile name: %q", report.ProfileName)
	}
	if len(report.Projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(report.Projects))
	}
	status := report.Projects[0]
	if !status.IsClean() {
		t.Fatalf("expected clean, got %+v", status)
	}
}

func TestCheckProfile_DirtyProjectListsChanges(t *testing.T) {
	profileRoot := t.TempDir()
	projectRoot := t.TempDir()
	loaded := setupProfileWithProject(t, profileRoot, projectRoot, "wanted")
	// project file content differs from the asset content -> change
	if err := os.WriteFile(filepath.Join(projectRoot, "AGENTS.md"), []byte("differs"), 0o644); err != nil {
		t.Fatal(err)
	}
	seedManagedState(t, projectRoot)

	report, checkErrs := CheckProfile(loaded)
	if len(checkErrs) > 0 {
		t.Fatalf("check: %v", checkErrs)
	}

	status := report.Projects[0]
	if status.IsClean() {
		t.Fatalf("expected dirty status, got clean")
	}
	if len(status.Changes) != 1 || status.Changes[0].Kind != llmsync.ChangeUpdate {
		t.Fatalf("expected single update, got %+v", status.Changes)
	}
}

// setupProfileWithProject scaffolds a profile with one agents_doc asset and
// one project at projectRoot, then returns the loaded profile.
func setupProfileWithProject(t *testing.T, profileRoot, projectRoot, assetBody string) *profile.Profile {
	t.Helper()
	if _, err := profile.Init(profileRoot, "Personal"); err != nil {
		t.Fatal(err)
	}
	assetDir := filepath.Join(profileRoot, config.AssetsDirName, "agents_doc", "base")
	if err := os.MkdirAll(assetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assetDir, config.AssetManifestFileName),
		[]byte(`{"id":"base","name":"base","type":"agents_doc"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assetDir, "AGENTS.md"), []byte(assetBody), 0o644); err != nil {
		t.Fatal(err)
	}
	projectFile := filepath.Join(profileRoot, config.ProjectsDirName, "app.json")
	body := `{"id":"app","name":"app","path":"` + projectRoot + `","enabled_agents":["codex"],"selected_asset_ids":["base"]}`
	if err := os.WriteFile(projectFile, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := profile.Load(profileRoot)
	if err != nil {
		t.Fatal(err)
	}
	return loaded
}

// seedManagedState writes a minimal `.agentfiles/state.json` so Plan treats
// the project as having been applied before. The empty ManagedFiles map is
// enough: the file's mere presence flips Plan out of first-apply mode, and
// the empty map means drift detection falls through to ChangeUpdate when
// content differs.
func seedManagedState(t *testing.T, projectRoot string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(projectRoot, config.StateDirName), 0o755); err != nil {
		t.Fatal(err)
	}
	state := llmsync.ManagedState{
		ProfileID:        "personal",
		ProjectID:        "app",
		GeneratorVersion: llmsync.GeneratorVersion,
		LastAppliedAt:    time.Now(),
		ManagedFiles:     map[string]string{},
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, config.StateDirName, config.StateFileName), data, 0o644); err != nil {
		t.Fatal(err)
	}
}
