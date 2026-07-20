package pathselector

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListingSortOrder(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "Bar"))
	mustMkdir(t, filepath.Join(dir, "alpha"))
	mustWriteFile(t, filepath.Join(dir, "README.md"))
	mustWriteFile(t, filepath.Join(dir, "apple.txt"))

	opts := resolvedOptions{
		showFiles:  true,
		constraint: dir, // suppress the `..` row so ordering matches the AC verbatim
	}
	got, err := buildEntries(dir, opts, nil, false)
	if err != nil {
		t.Fatalf("buildEntries: %v", err)
	}

	want := []string{"alpha/", "Bar/", "apple.txt", "README.md"}
	names := entryNames(got)
	if len(names) != len(want) {
		t.Fatalf("names = %v, want %v", names, want)
	}
	for i, w := range want {
		if names[i] != w {
			t.Fatalf("names[%d] = %q, want %q (all=%v)", i, names[i], w, names)
		}
	}
}

func TestListingSortIncludesParentAtTopWhenNotAtConstraintRoot(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "alpha"))

	opts := resolvedOptions{showFiles: true} // no constraint → `..` present
	got, err := buildEntries(dir, opts, nil, false)
	if err != nil {
		t.Fatalf("buildEntries: %v", err)
	}
	if got[0].Kind != entryParent {
		t.Fatalf("first entry kind = %v, want entryParent", got[0].Kind)
	}
	if got[0].Name != ".." {
		t.Fatalf("first entry name = %q, want %q", got[0].Name, "..")
	}
}

func TestFoldersOnlyMode(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "alpha"))
	mustWriteFile(t, filepath.Join(dir, "a.txt"))
	mustWriteFile(t, filepath.Join(dir, "b.md"))

	opts := resolvedOptions{
		showFiles:  false,
		allowedExt: map[string]struct{}{".md": {}}, // must be ignored
		constraint: dir,
	}
	got, err := buildEntries(dir, opts, nil, false)
	if err != nil {
		t.Fatalf("buildEntries: %v", err)
	}
	names := entryNames(got)
	if len(names) != 1 || names[0] != "alpha/" {
		t.Fatalf("names = %v, want just [alpha/]", names)
	}
}

func TestExtensionFilterCaseInsensitive(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "NOTES.MD"))
	mustWriteFile(t, filepath.Join(dir, "config.JSON"))
	mustWriteFile(t, filepath.Join(dir, "skip.txt"))

	opts := resolvedOptions{
		showFiles:  true,
		allowedExt: map[string]struct{}{".md": {}, ".json": {}},
		constraint: dir,
	}
	got, err := buildEntries(dir, opts, nil, false)
	if err != nil {
		t.Fatalf("buildEntries: %v", err)
	}
	names := entryNames(got)
	if len(names) != 2 {
		t.Fatalf("names = %v, want 2 entries", names)
	}
	// Both allowed names must be present; order is case-insensitive alpha:
	// "config.JSON" (c < n) then "NOTES.MD".
	if names[0] != "config.JSON" || names[1] != "NOTES.MD" {
		t.Fatalf("names = %v, want [config.JSON NOTES.MD]", names)
	}
}

func TestBuildEntriesEmptyFolderYieldsPlaceholder(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	opts := resolvedOptions{showFiles: true, constraint: dir}
	got, err := buildEntries(dir, opts, nil, false)
	if err != nil {
		t.Fatalf("buildEntries: %v", err)
	}
	if len(got) != 1 || got[0].Kind != entryEmpty {
		t.Fatalf("got %v, want single entryEmpty", got)
	}
}

func TestBuildEntriesHidesDotfilesWhenShowHiddenFalse(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, ".secret"))
	mustWriteFile(t, filepath.Join(dir, "visible.md"))

	opts := resolvedOptions{showFiles: true, constraint: dir}
	got, err := buildEntries(dir, opts, nil, false)
	if err != nil {
		t.Fatalf("buildEntries: %v", err)
	}
	names := entryNames(got)
	for _, n := range names {
		if n == ".secret" {
			t.Fatalf(".secret leaked into listing (showHidden=false): %v", names)
		}
	}
}

func TestBuildEntriesShowHiddenSurfacesDotfiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, ".secret"))
	mustWriteFile(t, filepath.Join(dir, "visible.md"))

	opts := resolvedOptions{showFiles: true, constraint: dir}
	got, err := buildEntries(dir, opts, nil, true)
	if err != nil {
		t.Fatalf("buildEntries: %v", err)
	}
	names := entryNames(got)
	seen := false
	for _, n := range names {
		if n == ".secret" {
			seen = true
			break
		}
	}
	if !seen {
		t.Fatalf(".secret missing with showHidden=true: %v", names)
	}
}

func entryNames(es []entry) []string {
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = e.Name
	}
	return out
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWriteFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
}
