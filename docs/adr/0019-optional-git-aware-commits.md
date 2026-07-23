# Optional Git-Aware Commits

## Status

accepted

## Context

Users repeatedly reported the same friction: after editing an asset in
`af` (or applying a plan into a target repo), they had to switch to a
terminal, run `git add` and `git commit`, then repeat the message every
time. Some carried a shell alias; most did not, and the history in both
profile repos and target repos ended up either empty or full of squashed
"wip" commits.

Two properties made a project-owned commit path attractive:

- Every mutation `af` performs already touches a well-defined, narrow
  set of paths (an asset directory, an asset manifest, a plan-apply
  file set + `state.json`). The scope of the commit is trivial for the
  tool to describe; it is easy for a human to get wrong.
- Users who already use `af` on git-tracked folders can benefit
  immediately; users who do not (or who do not want automated commits)
  should not have to notice.

Alternatives considered:

- **A [go-git](https://github.com/go-git/go-git) library dependency.**
  Rejected: adds a heavy dep with its own LFS / hook / config story
  that would drift from the user's installed `git`. The external-tools
  guideline already points at `os/exec` for this shape.
- **Always-on commits.** Rejected: removes user choice on a mutation
  the tool cannot undo, and interacts badly with pre-commit hooks that
  interactively prompt.
- **CLI flag to control the behavior per-invocation.** Rejected:
  violates ADR 0006 (TUI-only). The setting belongs behind a Settings
  screen that reads persisted preferences.
- **Per-project overrides.** Rejected as premature — no user has asked
  for it; a global toggle covers the reported cases.

## Decision

Add an opt-in **git-aware commit** path controlled by a new
persistent setting.

- A new `internal/settings` package owns the persisted user setting
  (`~/.agentfiles/settings.json`, schema `{version:1, git:{enabled}}`),
  mirroring the shape of `internal/projectstore` and
  `internal/registry`. Missing file → defaults (`git.enabled = false`).
- A new `internal/git` package wraps the `git` binary via `os/exec`
  per `docs/guidelines/external_tools.md`, exposing `Detect(dir)`,
  `BinaryAvailable()`, and `Repo.Commit(pathspec, msg)`. It is the
  only place in the codebase that talks to `git`.
- `app.Service` gains a `GitCommitter` interface seam whose
  `Commit(dir, pathspec, msg, runHooks) appapi.CommitOutcome` returns a
  discriminated boundary value (`appapi.Committed{SHA}`,
  `appapi.Skipped{Reason SkipReason}`, `appapi.Failed{Err}`) so the
  compiler enforces coverage of the three skip flavors (feature
  disabled, dir not a repo, empty diff). `Service.UpdateAsset`,
  `Service.SaveAssetFilesEdit` (new, for the editor-return flow), and
  `Service.Apply` return `CommitOutcome` alongside their existing
  outputs. `GitCommitter.BinaryAvailable() errs.DomainError` folds the
  pre-flight through the same seam so the pre-flight is testable
  without touching `$PATH`. A production wrapper (`gitBinaryCommitter`)
  implements the interface as the sole anti-corruption layer between
  `internal/git`'s typed errors and the boundary values; unit tests
  inject a fake committer.
- The boundary value types (`CommitOutcome`, `Preview`, `FileChange`,
  `DriftDecision`, `UnknownDecision`, `LoadedProfile`, plus the
  helpers `RegisterableDirs`, `DesiredIgnored`,
  `DriftResolutionsFromMap`, `SanitizeSubject`) live in
  `internal/appapi` so the documented `tui/shell → actions → app` edge
  stays honest — the shell reads value types from the leaf and never
  imports `internal/app` (see arc42 chapter 5).
- Commit-trigger templates live once in `internal/app/triggers.go` as
  three package-level `commitTrigger` values (`triggerSyncProject`,
  `triggerAssetManifest`, `triggerAssetFiles`); every subject
  interpolation flows through `appapi.SanitizeSubject` so a stray
  newline / control char / paste-accident value can never split the
  subject or blow past the 50-char Conventional-Commits norm.
- Commit messages follow **Conventional Commits** with the scope
  `agentfiles`:
  - `chore(agentfiles): update asset <id> manifest`
    (pathspec `assets/<id>/asset.json`, profile repo)
  - `chore(agentfiles): edit asset <id> files`
    (pathspec `assets/<id>/**`, profile repo)
  - `chore(agentfiles): sync project <name> (N files)`
    (pathspec = union of mutated files + `.agentfiles/state.json`,
    target repo)
- `Repo.Commit` refuses when the index already has staged paths
  outside the pathspec (`UnrelatedStagedChangesError`) so a scoped
  auto-commit cannot silently absorb unrelated user work.
- `Repo.Commit` returns `("", nil)` when nothing in the pathspec
  differs from HEAD/index — a silent skip, no `--allow-empty`.
- Pre-commit hooks are **bypassed by default** (`git commit --no-verify`).
  Users who rely on hooks (GPG-signing enforcement, formatters,
  compliance checks) opt back in via `git.run_hooks: true` in
  `settings.json`. Rationale: target-repo hooks come from
  user-checked-out code, so a hostile branch could ship a
  `.git/hooks/pre-commit` that runs the moment `af` records its next
  auto-commit — an execution path the user's mental model ("typed a
  comment into the TUI") does not cover. The opt-in narrows the
  attack surface to the users who explicitly accept it.
- When hooks are enabled, a non-zero commit exit is classified as
  `HookFailedError` when the stderr text carries a well-known marker
  (`hook exited`, `cannot spawn`, `error running .git/hooks`,
  `hook declined`, `failed to exec .git/hooks`) or, as a fallback,
  when a commit-time hook is present on disk. Anything else surfaces
  as `CommitError`. Neither rolls back the file writes that already
  succeeded.
- `internal/git` filters the child process environment through an
  allow-list so an inherited `GIT_DIR`, `GIT_WORK_TREE`,
  `GIT_INDEX_FILE`, `GIT_CONFIG_*`, `GIT_ALTERNATE_OBJECT_DIRECTORIES`,
  `GIT_TEMPLATE_DIR`, or similar cannot silently redirect the child
  away from the wrapper's declared work tree. `GIT_OPTIONAL_LOCKS=0`
  is forced so a background editor session does not race the commit
  sequence.
- `Repo.Root` is `EvalSymlinks`-resolved at `Detect` time, and
  `toRepoRelative` resolves the caller's pathspec entries (or their
  nearest existing ancestor) before the containment check. A
  symlinked profile folder that lexically sits under the repo but
  whose target escapes it is rejected as
  `UnrelatedStagedChangesError` rather than committed.
- After `git add` and before `git commit`, `Repo.Commit` re-verifies
  the index against the pathspec. A concurrent `git add` (another
  shell, a second `af` instance) between the initial check and the
  add is caught here and aborts the commit with
  `UnrelatedStagedChangesError` instead of silently absorbing the
  extra path.
- Enabling the setting runs a pre-flight `exec.LookPath("git")`.
  Failure refuses the save with the setting untouched so the toggle
  can never enter an unusable state.
- The Settings TUI screen hosts a `huh.NewSelect[bool]()` git toggle
  plus `[Save]` (mnemonic `e`) and `[Back]` (`b` + `esc`) buttons.
  Save success emits a `Settings saved` toast; a commit success
  toast merges save + commit into one line
  (`Asset "x" saved (committed abc1234)`).
- A hard commit failure surfaces as a second warn toast so the base
  save toast stays unchanged and the user sees the git error verbatim.

## Consequences

Positive:

- Users who opt in get clean, Conventional-Commits history in both
  profile repos and target repos without leaving the TUI.
- `internal/git` isolates the shell-out surface; every other package
  keeps its testable, in-memory shape.
- The typed errors (`UnrelatedStagedChangesError`, `HookFailedError`,
  `CommitError`, `BinaryMissingError`, `NotARepoError`) let the TUI
  render specific messages without introspecting on strings. The
  `Detail` field on `HookFailedError` / `CommitError` (renamed from
  `Stderr`) is honest about carrying either the wrapped stderr or the
  exec fallback summary.
- Settings persistence is symmetric with the existing registry /
  projects stores so wiring stays uniform.

Negative:

- Users with pre-existing staged changes in a profile or target repo
  see an abort with the offending paths listed. This is the price of
  the pathspec guarantee; the message names the paths so recovery is
  a single `git reset HEAD -- <path>` away.
- Hooks with interactive prompts block the commit because they own
  the tty; we surface `HookFailedError` with the first stderr line so
  the user knows to complete the hook manually.
- The setting is process-local at read time. Changing
  `settings.json` on disk while `af` runs has no effect until the next
  launch (out of scope for this ADR).

Neutral:

- The `git` binary must be on PATH to enable the feature; the
  pre-flight makes this discoverable rather than mysterious.

## References

- Guidelines: `docs/guidelines/git.md`,
  `docs/guidelines/external_tools.md`.
- Related ADRs: 0006 (TUI-only), 0008 (Domain errors everywhere),
  0009 (Editor via `os/exec`), 0017 (Centralized user config stores).
