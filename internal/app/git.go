package app

import (
	"errors"

	"github.com/hexworks/agentfiles/internal/appapi"
	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/git"
)

// GitCommitter records a scoped commit inside a specific directory.
// TODO: document that GitCommitter is a git adapter that translates
// between raw errors and domain errors
//
// The port shape returns appapi.CommitOutcome directly so the
// implementation owns the full anti-corruption translation from the
// git wrapper's typed errors (NotARepoError, HookFailedError,
// CommitError, UnrelatedStagedChangesError) into the boundary domain
// values (Committed / Skipped / Failed). Service.runCommit shrinks to
// "if feature is off → Skipped{SkipDisabled}; else committer.Commit".
//
// BinaryAvailable lets the Settings screen's pre-flight run through
// the same seam as commit calls so the entire git dependency stays
// injectable — no `internal/git` import in service.go, no test that
// has to touch $PATH.
// TODO: this name is not good, let's name it GitClient, that's the conventional name
type GitCommitter interface {
	Commit(dir string, pathspec []string, msg string, runHooks bool) appapi.CommitOutcome
	// TODO: document what this does
	BinaryAvailable() errs.DomainError
}

// gitBinaryCommitter is the production GitCommitter wired in
// cmd/af/main.go. It is the sole anti-corruption layer between
// internal/git's typed errors and appapi's discriminated commit
// outcome — every caller downstream sees Committed / Skipped / Failed
// only.
type gitBinaryCommitter struct{}

// NewGitCommitter returns the production committer that wraps
// internal/git. Split out as a constructor so tests can swap it for a
// fake without touching wiring in main.
func NewGitCommitter() GitCommitter { return gitBinaryCommitter{} }

func (gitBinaryCommitter) BinaryAvailable() errs.DomainError {
	return git.BinaryAvailable()
}

func (gitBinaryCommitter) Commit(dir string, pathspec []string, msg string, runHooks bool) appapi.CommitOutcome {
	repo, err := git.Detect(dir)
	if err != nil {
		var notRepo git.NotARepoError
		// FIX: fix according to the lint note
		if errors.As(err, &notRepo) {
			return appapi.Skipped{Reason: appapi.SkipNotARepo}
		}
		return appapi.Failed{Err: err}
	}
	sha, commitErr := repo.Commit(pathspec, msg, runHooks)
	if commitErr != nil {
		return appapi.Failed{Err: commitErr}
	}
	if sha == "" {
		return appapi.Skipped{Reason: appapi.SkipEmptyDiff}
	}
	return appapi.Committed{SHA: sha}
}
