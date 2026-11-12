package profile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hexworks/agentfiles/internal/asset"
)

func mkdirs(t *testing.T, root string, rels ...string) {
	t.Helper()
	for _, rel := range rels {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(rel)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func touch(t *testing.T, root, rel string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScanUnadopted_ListsFoldersWithoutManifest(t *testing.T) {
	root := t.TempDir()
	mkdirs(t, root,
		"assets/skill/copied",     // unadopted
		"assets/skill/.hidden",    // hidden → skipped
		"assets/agents_doc/notes", // unadopted, other type
		"assets/mcp/server",       // generic type → not scanned
		"assets/skill/adopted",
	)
	touch(t, root, "assets/skill/adopted/asset.json")
	touch(t, root, "assets/skill/loose-file.md") // file, not dir → skipped

	got, err := ScanUnadopted(root)
	if err != nil {
		t.Fatalf("ScanUnadopted: %v", err)
	}
	want := []UnadoptedFolder{
		{Key: "agents_doc/notes", Type: asset.TypeAgentsDoc, Name: "notes", Dir: filepath.Join(root, "assets", "agents_doc", "notes")},
		{Key: "skill/copied", Type: asset.TypeSkill, Name: "copied", Dir: filepath.Join(root, "assets", "skill", "copied")},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d folders %+v, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] got %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestScanUnadopted_MissingAssetsDirIsEmpty(t *testing.T) {
	got, err := ScanUnadopted(t.TempDir())
	if err != nil {
		t.Fatalf("ScanUnadopted: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %+v, want empty", got)
	}
}
