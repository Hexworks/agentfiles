package git

import (
	"errors"
	"os"
	"os/exec"
	"strings"

	"github.com/hexworks/agentfiles/internal/errs"
)

// Commit records a scoped commit against pathspec with subject msg.
// Each pathspec entry is an ABSOLUTE filesystem path; a `/**` suffix
// marks a recursive directory match. runHooks controls whether the
// repo's commit-time hooks execute; when false the wrapper passes
// --no-verify so a hostile checked-out hook cannot silently run.
//
// Behavior in order:
//  1. Convert absolute pathspec → repo-relative pathspec via r.Root
//     (symlinks resolved). An entry outside the resolved work tree
//     returns UnrelatedStagedChangesError.
//  2. If any pre-staged path lies outside pathspec (Covers rule) →
//     UnrelatedStagedChangesError.
//  3. If nothing under pathspec differs from HEAD or the index →
//     ("", nil) — silent skip on empty diff.
//  4. `git add -- <converted>` then re-verify the index still lies
//     inside pathspec (TOCTOU guard against a concurrent git add), then
//     `git commit -m msg [--no-verify]`. Non-zero exits classify as
//     HookFailedError when the stderr signals a hook (or falls back to
//     the installed-hook heuristic), CommitError otherwise.
//  5. Return the short SHA of the new HEAD.
func (r *Repo) Commit(pathspec []string, msg string, runHooks bool) (string, errs.DomainError) {
	relSpec, err := r.ensureCoveredBy(pathspec)
	if err != nil {
		return "", err
	}
	gitSpec := toGitPathspec(relSpec)
	dirty, dirtyErr := r.hasChanges(gitSpec)
	if dirtyErr != nil {
		return "", dirtyErr
	}
	if !dirty {
		return "", nil
	}
	return r.commitOrSkip(relSpec, gitSpec, msg, runHooks)
}

// ensureCoveredBy converts the absolute pathspec to repo-relative form
// and asserts every pre-staged path already lies under one of the
// entries. Any pathspec entry outside the work tree, or any pre-staged
// path outside the pathspec, returns UnrelatedStagedChangesError so
// unrelated user work never rolls into an automated commit.
func (r *Repo) ensureCoveredBy(pathspec []string) ([]string, errs.DomainError) {
	relSpec, err := r.toRepoRelative(pathspec)
	if err != nil {
		return nil, err
	}
	staged, stagedErr := r.stagedPaths()
	if stagedErr != nil {
		return nil, stagedErr
	}
	if unrelated := unrelatedStaged(staged, relSpec); len(unrelated) > 0 {
		return nil, UnrelatedStagedChangesError{Paths: unrelated}
	}
	return relSpec, nil
}

// commitOrSkip runs `git add` + `git commit` on the already-verified
// pathspec, re-checks the index between the two calls to close the
// TOCTOU window against a concurrent git add, and returns the new
// short SHA on success or ("", nil) when the staged index turns out to
// be empty at commit time.
func (r *Repo) commitOrSkip(relSpec, gitSpec []string, msg string, runHooks bool) (string, errs.DomainError) {
	addArgs := append([]string{"-C", r.Root, "add", "--"}, gitSpec...)
	if _, err := r.run(addArgs...); err != nil {
		return "", err
	}
	// Re-verify: another shell (or a second af invocation) may have
	// staged an unrelated path between our initial check and the add.
	// If the index now contains anything outside pathspec, abort — the
	// docstring guarantees the commit only covers relSpec.
	staged, stagedErr := r.stagedPaths()
	if stagedErr != nil {
		return "", stagedErr
	}
	if unrelated := unrelatedStaged(staged, relSpec); len(unrelated) > 0 {
		return "", UnrelatedStagedChangesError{Paths: unrelated}
	}
	empty, emptyErr := r.stagedIsEmpty()
	if emptyErr != nil {
		return "", emptyErr
	}
	if empty {
		return "", nil
	}
	commitArgs := []string{"-C", r.Root, "commit", "-m", msg}
	if !runHooks {
		commitArgs = append(commitArgs, "--no-verify")
	}
	if _, err := r.runCommit(commitArgs...); err != nil {
		return "", err
	}
	sha, shaErr := r.run("-C", r.Root, "rev-parse", "--short", "HEAD")
	if shaErr != nil {
		return "", shaErr
	}
	return strings.TrimSpace(sha), nil
}

// unrelatedStaged reports staged paths that fall outside pathspec.
// Returns nil when everything is covered.
func unrelatedStaged(staged, pathspec []string) []string {
	var out []string
	for _, p := range staged {
		if !Covers(pathspec, p) {
			out = append(out, p)
		}
	}
	return out
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
	cmd.Env = filteredEnv(os.Environ())
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return false, nil
		}
		return false, CommitError{Detail: err.Error()}
	}
	return true, nil
}
