package git

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git binary not available: %v", err)
	}
}

func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "commit.gpgsign", "false")
	// Seed a first commit so HEAD exists.
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("seed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-q", "-m", "seed")
	return dir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func headSubject(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "log", "-1", "--format=%s")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git log: %v\n%s", err, out)
	}
	return strings.TrimSpace(string(out))
}

func headFiles(t *testing.T, dir string) []string {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "show", "--name-only", "--format=", "HEAD")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git show: %v\n%s", err, out)
	}
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			files = append(files, line)
		}
	}
	return files
}

func headCount(t *testing.T, dir string) int {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "rev-list", "--count", "HEAD")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git rev-list: %v\n%s", err, out)
	}
	c := 0
	for _, b := range strings.TrimSpace(string(out)) {
		c = c*10 + int(b-'0')
	}
	return c
}

func TestDetectNotARepo(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	_, err := Detect(dir)
	var nar NotARepoError
	if !errors.As(err, &nar) {
		t.Fatalf("expected NotARepoError, got %T (%v)", err, err)
	}
}

func TestCommit(t *testing.T) {
	requireGit(t)
	dir := initRepo(t)
	assetDir := filepath.Join(dir, "assets", "foo")
	if err := os.MkdirAll(assetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assetDir, "asset.json"), []byte(`{"id":"foo"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	repo, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	sha, err := repo.Commit([]string{"assets/foo/**"}, "chore(agentfiles): update asset foo manifest")
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if sha == "" {
		t.Fatalf("expected non-empty short SHA")
	}
	if got := headSubject(t, dir); got != "chore(agentfiles): update asset foo manifest" {
		t.Fatalf("subject = %q", got)
	}
	files := headFiles(t, dir)
	if len(files) != 1 || files[0] != "assets/foo/asset.json" {
		t.Fatalf("expected only assets/foo/asset.json, got %v", files)
	}
}

func TestCommit_UnrelatedStaged(t *testing.T) {
	requireGit(t)
	dir := initRepo(t)
	// Pre-stage an unrelated path.
	if err := os.WriteFile(filepath.Join(dir, "OTHER.md"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "OTHER.md")

	assetDir := filepath.Join(dir, "assets", "foo")
	if err := os.MkdirAll(assetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assetDir, "asset.json"), []byte(`{"id":"foo"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	repo, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	countBefore := headCount(t, dir)

	_, err = repo.Commit([]string{"assets/foo/**"}, "chore(agentfiles): edit asset foo files")
	var unrelated UnrelatedStagedChangesError
	if !errors.As(err, &unrelated) {
		t.Fatalf("expected UnrelatedStagedChangesError, got %T (%v)", err, err)
	}
	if len(unrelated.Paths) != 1 || unrelated.Paths[0] != "OTHER.md" {
		t.Fatalf("expected OTHER.md, got %v", unrelated.Paths)
	}
	if got := headCount(t, dir); got != countBefore {
		t.Fatalf("expected no new commits, count %d -> %d", countBefore, got)
	}
}

func TestCommit_EmptyDiff(t *testing.T) {
	requireGit(t)
	dir := initRepo(t)
	repo, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	countBefore := headCount(t, dir)
	sha, err := repo.Commit([]string{"assets/foo/**"}, "chore(agentfiles): edit asset foo files")
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if sha != "" {
		t.Fatalf("expected empty SHA on empty diff, got %q", sha)
	}
	if got := headCount(t, dir); got != countBefore {
		t.Fatalf("expected no new commits, count %d -> %d", countBefore, got)
	}
}

func TestCommit_HookFailure(t *testing.T) {
	requireGit(t)
	dir := initRepo(t)
	hookPath := filepath.Join(dir, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(hookPath, []byte("#!/bin/sh\necho 'nope' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	assetDir := filepath.Join(dir, "assets", "foo")
	if err := os.MkdirAll(assetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assetDir, "asset.json"), []byte(`{"id":"foo"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	repo, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	countBefore := headCount(t, dir)
	_, commitErr := repo.Commit([]string{"assets/foo/**"}, "chore(agentfiles): edit asset foo files")
	var hookErr HookFailedError
	if !errors.As(commitErr, &hookErr) {
		t.Fatalf("expected HookFailedError, got %T (%v)", commitErr, commitErr)
	}
	if got := headCount(t, dir); got != countBefore {
		t.Fatalf("expected no new commits, count %d -> %d", countBefore, got)
	}
}
