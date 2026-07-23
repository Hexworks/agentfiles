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

// Repo names a git work tree. Dir is the directory the caller handed to
// Detect (a subdirectory may sit deep inside a repo); Root is the
// work-tree top-level returned by `git rev-parse --show-toplevel`.
// Every git subcommand runs at Root so paths interpretable by git line
// up with the repo-relative paths reported by porcelain output.
type Repo struct {
	Dir  string
	Root string
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
	inside, err := runCapture("-C", dir, "rev-parse", "--is-inside-work-tree")
	if err != nil || strings.TrimSpace(inside) != "true" {
		return nil, NotARepoError{Dir: dir}
	}
	top, err := runCapture("-C", dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, NotARepoError{Dir: dir}
	}
	root := strings.TrimSpace(top)
	if root == "" {
		return nil, NotARepoError{Dir: dir}
	}
	return &Repo{Dir: dir, Root: root}, nil
}

// Commit records a scoped commit against pathspec with subject msg.
// Each pathspec entry is an ABSOLUTE filesystem path; a `/**` suffix
// marks a recursive directory match. The wrapper converts every entry
// to a repo-relative path so the coverage check, `git add`, and
// `git status` all agree on shape.
//
// Behavior in order:
//  1. Convert absolute pathspec → repo-relative pathspec via r.Root.
//     An entry outside the work tree returns UnrelatedStagedChangesError.
//  2. If any pre-staged path lies outside pathspec (Covers rule) →
//     UnrelatedStagedChangesError.
//  3. If nothing under pathspec differs from HEAD or the index →
//     ("", nil) — silent skip on empty diff.
//  4. `git add -- <converted>` then `git commit -m msg`. Non-zero
//     exits classify as HookFailedError when a commit-time hook is
//     installed, CommitError otherwise.
//  5. Return the short SHA of the new HEAD.
func (r *Repo) Commit(pathspec []string, msg string) (string, errs.DomainError) {
	relSpec, err := r.toRepoRelative(pathspec)
	if err != nil {
		return "", err
	}
	staged, stagedErr := r.stagedPaths()
	if stagedErr != nil {
		return "", stagedErr
	}
	var unrelated []string
	for _, p := range staged {
		if !Covers(relSpec, p) {
			unrelated = append(unrelated, p)
		}
	}
	if len(unrelated) > 0 {
		return "", UnrelatedStagedChangesError{Paths: unrelated}
	}
	gitSpec := toGitPathspec(relSpec)
	dirty, dirtyErr := r.hasChanges(gitSpec)
	if dirtyErr != nil {
		return "", dirtyErr
	}
	if !dirty {
		return "", nil
	}
	addArgs := append([]string{"-C", r.Root, "add", "--"}, gitSpec...)
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
	if _, err := r.runCommit("-C", r.Root, "commit", "-m", msg); err != nil {
		return "", err
	}
	sha, shaErr := r.run("-C", r.Root, "rev-parse", "--short", "HEAD")
	if shaErr != nil {
		return "", shaErr
	}
	return strings.TrimSpace(sha), nil
}

// toRepoRelative converts absolute pathspec entries into forward-slash
// paths rooted at r.Root. A trailing `/**` recursive marker is
// preserved. An entry outside r.Root returns
// UnrelatedStagedChangesError so the caller sees the same failure
// shape whether the offending path was pre-staged or hand-crafted.
func (r *Repo) toRepoRelative(pathspec []string) ([]string, errs.DomainError) {
	out := make([]string, 0, len(pathspec))
	var outside []string
	for _, p := range pathspec {
		recursive := strings.HasSuffix(p, "/**")
		base := strings.TrimSuffix(p, "/**")
		abs, err := filepath.Abs(base)
		if err != nil {
			outside = append(outside, p)
			continue
		}
		rel, relErr := filepath.Rel(r.Root, abs)
		if relErr != nil {
			outside = append(outside, p)
			continue
		}
		rel = filepath.ToSlash(rel)
		if rel == ".." || strings.HasPrefix(rel, "../") {
			outside = append(outside, p)
			continue
		}
		if recursive {
			if rel == "." {
				rel = "**"
			} else {
				rel = rel + "/**"
			}
		}
		out = append(out, rel)
	}
	if len(outside) > 0 {
		return nil, UnrelatedStagedChangesError{Paths: outside}
	}
	return out, nil
}

// hasChanges reports whether any file matching pathspec differs from
// HEAD or the index. `git status --porcelain -- <pathspec>` covers
// untracked and modified files without failing on empty matches.
func (r *Repo) hasChanges(pathspec []string) (bool, errs.DomainError) {
	args := append([]string{"-C", r.Root, "status", "--porcelain", "--"}, pathspec...)
	out, err := r.run(args...)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

func (r *Repo) stagedPaths() ([]string, errs.DomainError) {
	out, err := r.run("-C", r.Root, "diff", "--cached", "--name-only")
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
	cmd := exec.Command("git", "-C", r.Root, "diff", "--cached", "--quiet")
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
	out, err := runCapture(args...)
	if err != nil {
		return "", CommitError{Stderr: err.Error()}
	}
	return out, nil
}

// runCapture is the shared exec helper used by both package-level
// callers (Detect) and Repo methods; it returns stdout on success and
// a plain error on non-zero exit so the caller can decide whether to
// wrap into a typed domain error or fall through.
func runCapture(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		s := stderr.String()
		if strings.TrimSpace(s) != "" {
			return "", &runErr{msg: s}
		}
		return "", err
	}
	return stdout.String(), nil
}

type runErr struct{ msg string }

func (e *runErr) Error() string { return e.msg }

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
	hooksDir := filepath.Join(r.Root, ".git", "hooks")
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
