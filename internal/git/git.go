// Package git is the narrow wrapper around the `git` binary used by
// agentfiles to record scoped commits when a mutated folder is a git
// repository. It follows the external-tools guideline: exec.Command
// lives here, callers see typed errors and never touch os/exec
// themselves.
package git

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/hexworks/agentfiles/internal/errs"
)

// Repo names a git work tree. Dir is the directory the caller handed to
// Detect (a subdirectory may sit deep inside a repo); Root is the
// symlink-resolved work-tree top-level returned by
// `git rev-parse --show-toplevel`. Every git subcommand runs at Root so
// paths interpretable by git line up with the repo-relative paths
// reported by porcelain output.
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
// NotARepoError so the caller treats it as a silent skip. The returned
// Root is symlink-resolved so callers comparing paths against Root see
// canonical values (see toRepoRelative).
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
	if resolved, resolveErr := filepath.EvalSymlinks(root); resolveErr == nil {
		root = resolved
	}
	return &Repo{Dir: dir, Root: root}, nil
}

// run executes a plain git subcommand and returns stdout on success or a
// CommitError on non-zero exit.
func (r *Repo) run(args ...string) (string, errs.DomainError) {
	out, err := runCapture(args...)
	if err != nil {
		return "", CommitError{Detail: err.Error()}
	}
	return out, nil
}

// runCapture is the shared exec helper used by both package-level
// callers (Detect) and Repo methods; it returns stdout on success and
// a plain error on non-zero exit so the caller can decide whether to
// wrap into a typed domain error or fall through. The environment is
// filtered so inherited GIT_* variables cannot silently redirect the
// child (see filteredEnv).
func runCapture(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Env = filteredEnv(os.Environ())
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		s := stderr.String()
		if strings.TrimSpace(s) != "" {
			return "", errors.New(s)
		}
		return "", err
	}
	return stdout.String(), nil
}

// runCommit is run() with the extra hook-detection rule that applies to
// `git commit` non-zero exits. Hook classification prefers markers in
// the stderr text over the installed-hook heuristic so an unrelated
// commit failure (dirty tree, gpg failure, config error) is not
// mislabelled just because a hook happens to be present.
func (r *Repo) runCommit(args ...string) (string, errs.DomainError) {
	cmd := exec.Command("git", args...)
	cmd.Env = filteredEnv(os.Environ())
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		stderrStr := stderr.String()
		detail := firstNonEmpty(stderrStr, err.Error())
		if classifyHookFailure(stderrStr, r.hookInstalled()) {
			return "", HookFailedError{Detail: detail}
		}
		return "", CommitError{Detail: detail}
	}
	return stdout.String(), nil
}

// filteredEnv returns a copy of env stripped of GIT_* variables that
// could redirect the child (GIT_DIR, GIT_WORK_TREE, GIT_INDEX_FILE,
// GIT_CONFIG_*, GIT_ALTERNATE_OBJECT_DIRECTORIES, GIT_TEMPLATE_DIR),
// plus GIT_OPTIONAL_LOCKS forced to 0 so a background editor session
// does not race against our commit sequence.
func filteredEnv(env []string) []string {
	out := make([]string, 0, len(env)+1)
	for _, kv := range env {
		if isFilteredEnv(kv) {
			continue
		}
		out = append(out, kv)
	}
	out = append(out, "GIT_OPTIONAL_LOCKS=0")
	return out
}

func isFilteredEnv(kv string) bool {
	eq := strings.IndexByte(kv, '=')
	if eq <= 0 {
		return false
	}
	key := kv[:eq]
	if !strings.HasPrefix(key, "GIT_") {
		return false
	}
	switch key {
	case "GIT_DIR",
		"GIT_WORK_TREE",
		"GIT_INDEX_FILE",
		"GIT_ALTERNATE_OBJECT_DIRECTORIES",
		"GIT_TEMPLATE_DIR",
		"GIT_OPTIONAL_LOCKS",
		"GIT_NAMESPACE",
		"GIT_OBJECT_DIRECTORY",
		"GIT_COMMON_DIR",
		"GIT_CEILING_DIRECTORIES",
		"GIT_DISCOVERY_ACROSS_FILESYSTEM":
		return true
	}
	if strings.HasPrefix(key, "GIT_CONFIG_") {
		return true
	}
	return false
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
