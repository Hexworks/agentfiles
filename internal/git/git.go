// Package git is the narrow wrapper around the `git` binary used by
// agentfiles to record scoped commits when a mutated folder is a git
// repository. It follows the external-tools guideline: exec.Command
// lives here, callers see typed errors and never touch os/exec
// themselves.
package git

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/hexworks/agentfiles/internal/errs"
)

// Repo names a git work tree by its absolute directory. Values are
// obtained through Detect and never constructed by callers.
type Repo struct {
	Dir string
}

// BinaryAvailable succeeds when the `git` binary is on PATH. Wired into
// the settings pre-flight so enabling the toggle without a git install
// surfaces immediately.
func BinaryAvailable() errs.DomainError {
	if _, err := exec.LookPath("git"); err != nil {
		return BinaryMissingError{Err: err}
	}
	return nil
}

// Detect returns a Repo for dir when dir is inside a git work tree.
// Missing binary → BinaryMissingError so the caller can decide between
// "silent skip" (commit path) and "hard refuse" (settings save). Any
// other non-zero exit or an explicit "not a work tree" answer maps to
// NotARepoError so the caller treats it as a silent skip.
func Detect(dir string) (*Repo, errs.DomainError) {
	if err := BinaryAvailable(); err != nil {
		return nil, err
	}
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--is-inside-work-tree")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, NotARepoError{Dir: dir}
	}
	if strings.TrimSpace(stdout.String()) != "true" {
		return nil, NotARepoError{Dir: dir}
	}
	return &Repo{Dir: dir}, nil
}

// Commit records a scoped commit against pathspec with subject msg.
// Behavior in order:
//  1. If any pre-staged path lies outside pathspec (Covers rule), return
//     UnrelatedStagedChangesError.
//  2. Compute a git-native pathspec (semantic `foo/**` → `foo`). When
//     `git status --porcelain -- <converted>` reports no matching change
//     return ("", nil) — silent skip on empty diff.
//  3. `git add -- <converted>` then `git commit -m msg`. When commit
//     fails and any commit-time hook is installed, surface
//     HookFailedError. Any other non-zero exit surfaces as CommitError.
//  4. Return the short SHA of the new HEAD.
func (r *Repo) Commit(pathspec []string, msg string) (string, errs.DomainError) {
	staged, err := r.stagedPaths()
	if err != nil {
		return "", err
	}
	var unrelated []string
	for _, p := range staged {
		if !Covers(pathspec, p) {
			unrelated = append(unrelated, p)
		}
	}
	if len(unrelated) > 0 {
		return "", UnrelatedStagedChangesError{Paths: unrelated}
	}
	gitSpec := toGitPathspec(pathspec)
	dirty, dirtyErr := r.hasChanges(gitSpec)
	if dirtyErr != nil {
		return "", dirtyErr
	}
	if !dirty {
		return "", nil
	}
	addArgs := append([]string{"-C", r.Dir, "add", "--"}, gitSpec...)
	if _, err := r.run(addArgs...); err != nil {
		return "", err
	}
	empty, emptyErr := r.stagedIsEmpty()
	if emptyErr != nil {
		return "", emptyErr
	}
	if empty {
		return "", nil
	}
	if _, err := r.runCommit("-C", r.Dir, "commit", "-m", msg); err != nil {
		return "", err
	}
	sha, shaErr := r.run("-C", r.Dir, "rev-parse", "--short", "HEAD")
	if shaErr != nil {
		return "", shaErr
	}
	return strings.TrimSpace(sha), nil
}

// hasChanges reports whether any file matching pathspec differs from
// HEAD or the index. `git status --porcelain -- <pathspec>` covers
// untracked and modified files without failing on empty matches.
func (r *Repo) hasChanges(pathspec []string) (bool, errs.DomainError) {
	args := append([]string{"-C", r.Dir, "status", "--porcelain", "--"}, pathspec...)
	out, err := r.run(args...)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

func (r *Repo) stagedPaths() ([]string, errs.DomainError) {
	out, err := r.run("-C", r.Dir, "diff", "--cached", "--name-only")
	if err != nil {
		return nil, err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

func (r *Repo) stagedIsEmpty() (bool, errs.DomainError) {
	cmd := exec.Command("git", "-C", r.Dir, "diff", "--cached", "--quiet")
	if err := cmd.Run(); err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return false, nil
		}
		return false, CommitError{Stderr: err.Error()}
	}
	return true, nil
}

// run executes a plain git subcommand and returns stdout on success or a
// CommitError on non-zero exit.
func (r *Repo) run(args ...string) (string, errs.DomainError) {
	cmd := exec.Command("git", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", CommitError{Stderr: firstNonEmpty(stderr.String(), err.Error())}
	}
	return stdout.String(), nil
}

// runCommit is run() with the extra hook-detection rule that applies to
// `git commit` non-zero exits.
func (r *Repo) runCommit(args ...string) (string, errs.DomainError) {
	cmd := exec.Command("git", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		stderrStr := stderr.String()
		if r.hookInstalled() {
			return "", HookFailedError{Stderr: firstNonEmpty(stderrStr, err.Error())}
		}
		return "", CommitError{Stderr: firstNonEmpty(stderrStr, err.Error())}
	}
	return stdout.String(), nil
}

// hookInstalled reports whether any commit-time hook is present and
// executable. Git does not include a stable "hook failed" line in its
// stderr, so callers fall back to this heuristic: if a hook is on disk
// and the commit failed, it is very likely the hook that rejected it.
func (r *Repo) hookInstalled() bool {
	hooksDir := filepath.Join(r.Dir, ".git", "hooks")
	for _, name := range []string{"pre-commit", "prepare-commit-msg", "commit-msg"} {
		info, err := os.Stat(filepath.Join(hooksDir, name))
		if err != nil {
			continue
		}
		if info.Mode()&0o111 != 0 && info.Size() > 0 {
			return true
		}
	}
	return false
}

// toGitPathspec converts the semantic `foo/**` wildcard used by Covers
// into the corresponding literal directory that git's own pathspec
// grammar understands. Literal entries pass through unchanged.
func toGitPathspec(pathspec []string) []string {
	out := make([]string, 0, len(pathspec))
	for _, p := range pathspec {
		if strings.HasSuffix(p, "/**") {
			out = append(out, strings.TrimSuffix(p, "/**"))
			continue
		}
		out = append(out, p)
	}
	return out
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
