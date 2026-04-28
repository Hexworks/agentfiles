package doctor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/addamsson/agentfiles/internal/config"
	"github.com/addamsson/agentfiles/internal/profile"
)

func TestCheckProfile_CleanProjectHasEmptyChanges(t *testing.T) {
	profileRoot := t.TempDir()
	projectRoot := t.TempDir()
	loaded := setupProfileWithProject(t, profileRoot, projectRoot, "matched")
	// project file content matches the asset content, so no changes
	if err := os.WriteFile(filepath.Join(projectRoot, "AGENTS.md"), []byte("matched"), 0o644); err != nil {
		t.Fatal(err)
	}

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

	report, checkErrs := CheckProfile(loaded)
	if len(checkErrs) > 0 {
		t.Fatalf("check: %v", checkErrs)
	}

	status := report.Projects[0]
	if status.IsClean() {
		t.Fatalf("expected dirty status, got clean")
	}
	if len(status.Changes) != 1 || status.Changes[0].Kind != ChangeUpdate {
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
