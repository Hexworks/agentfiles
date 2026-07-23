package app

import (
	"errors"

	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/git"
)

// GitCommitter records a scoped commit inside a specific directory.
// Implementations return ("", nil) when the operation is a silent skip
// (dir not a git repo, or empty diff for the given pathspec) and a
// typed domain error otherwise. The interface lives here so app.Service
// depends on the seam, not the concrete git package — unit tests inject
// a fake committer that records the (dir, pathspec, msg) tuple.
type GitCommitter interface {
	Commit(dir string, pathspec []string, msg string) (string, errs.DomainError)
}

// CommitOutcome is the value each git-aware Service method returns so
// callers compose the merged save-plus-commit toast without importing
// internal/git types. Zero-value means "no commit was attempted"
// (feature disabled or dir not a repo). SHA carries the short hash on
// success; Err carries a typed domain error when a commit was attempted
// and failed.
type CommitOutcome struct {
	SHA string
	Err errs.DomainError
}

// gitBinaryCommitter is the production GitCommitter wired in
// cmd/af/main.go. It swallows NotARepoError so "the folder is not a git
// repo" stays a silent skip at the boundary; every other typed error
// from internal/git surfaces on CommitOutcome.Err.
type gitBinaryCommitter struct{}

// NewGitCommitter returns the production committer that wraps
// internal/git. Split out as a constructor so tests can swap it for a
// fake without touching wiring in main.
func NewGitCommitter() GitCommitter { return gitBinaryCommitter{} }

func (gitBinaryCommitter) Commit(dir string, pathspec []string, msg string) (string, errs.DomainError) {
	repo, err := git.Detect(dir)
	if err != nil {
		var notRepo git.NotARepoError
		if errors.As(err, &notRepo) {
			return "", nil
		}
		return "", err
	}
	return repo.Commit(pathspec, msg)
}
