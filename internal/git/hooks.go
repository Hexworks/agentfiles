package git

import (
	"os"
	"path/filepath"
	"strings"
)

// classifyHookFailure decides whether a `git commit` non-zero exit was
// caused by a repo-supplied hook. Preference order:
//  1. Stderr markers git emits when a hook rejects or fails to spawn.
//     These are the most reliable signal — a marker match means the
//     hook is culpable regardless of what else is installed on disk.
//  2. hookInstalled heuristic as a fallback. Only trusted when the
//     stderr text is silent (no marker) but a commit-time hook is
//     present. Without stderr signal a false-positive is still
//     possible; the marker path avoids it.
func classifyHookFailure(stderr string, hookOnDisk bool) bool {
	if stderrMentionsHook(stderr) {
		return true
	}
	return hookOnDisk
}

// stderrMentionsHook scans a git stderr blob for the diagnostic strings
// git emits when a hook is the reason for the failure. Case-insensitive
// prefix match on the well-known lines so a translated locale message
// still fails the check gracefully (falling through to the heuristic).
func stderrMentionsHook(stderr string) bool {
	if stderr == "" {
		return false
	}
	lower := strings.ToLower(stderr)
	for _, marker := range hookStderrMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// hookStderrMarkers are the stable lower-case substrings git prints when
// a hook is the reason for a commit failure. Keep the list narrow —
// each entry is a compromise between recall and false-positive risk.
var hookStderrMarkers = []string{
	"hook exited",  // "hint: The 'pre-commit' hook exited with status 1"
	"cannot spawn", // "error: cannot spawn .git/hooks/pre-commit: ..."
	"error running .git/hooks",
	"hook declined",
	"failed to exec .git/hooks",
}

// hookInstalled reports whether any commit-time hook is present and
// executable. Used as the fallback signal by classifyHookFailure when
// stderr is silent about a hook.
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
