---
id: 0042
type: feature
status: done
topics: git, tui, external_tools, sync_and_safety
notes: |
    I'd like af to be git-aware. This means adding the following functionality:

    If the profile we're currently editing is a git repository then I'd like to have additional behavior:
    - When an asset's files are edited (there is a change in git) when we return from the editor on the edit asset screen then I'd like to create a commit out of it
    - when an asset's metadata (asset.json) is edited through "Save" I'd like to create a commit

    When the project we edit is a git repository then:
    - changes to the project metadata (eg: `.agentfiles/state.json`) made on the "Editing Project" modal should be committed
    - changes to the rendered asset files (on the "Planning project ..." screen) should be committed

    Git support should be an option on the "Settings screen". All the values that we change on the "Settings" screen should be loaded when af starts
    and changes made to these values should be saved to a new `settings.json` file (next to the already-existing `profiles.json` and `projects.json`).

    The settings screen should have a Save (mnemonic `e`) button just like the profile editor and we should only save when it is invoked.

    A notification should appear when we do so.

    Git integration should only happen if:
    - the folder we work with *is* a git repository
    - Git integration is enabled in settings

    **important**: we should only include our changes in the commit and the commit message should be succinct and it should include some sort of prefix (eg: `[Agentfiles]` or something similar)
    that signifies that it is an automated commit
---

# Git-aware commits for profiles and projects

Make `af` optionally auto-commit changes it writes. Introduce a persistent
settings store so the user can enable/disable the behavior, and add a
`Settings` screen where the toggle lives. When enabled, each write path
that mutates a git-tracked folder produces a scoped commit using the
user's git identity.

## Design decisions

- **`internal/git` package** wraps the `git` binary via `os/exec` (per
  `docs/guidelines/external_tools.md`). Exposes `Detect(dir) *Repo` and
  `Repo.Commit(paths, msg) (shortSHA, error)`.
- **Repo detection**: `git rev-parse --git-dir`, fresh per operation
  (no cache). Handles worktrees / submodules / `.git`-file cases.
- **Callers**: `app.Service` methods (`UpdateAsset`, editor-return
  handler, `Apply`). TUI stays git-agnostic. Committer wired as an
  interface seam (`app.GitCommitter`) so unit tests inject a fake.
- **`internal/settings` package** mirrors `projectstore` / `registry`
  shape. `~/.agentfiles/settings.json`, versioned. Loaded once in
  `cmd/af/main.go`, injected into `app.Service`. Settings screen calls
  `svc.UpdateSettings(new)` on Save; no live reload from disk.
  Missing file → defaults, no error.
- **Schema**:
    ```json
    { "version": 1, "git": { "enabled": false } }
    ```
- **Commit message shape** (Conventional Commits, scope `agentfiles`):
    - Asset files edited: `chore(agentfiles): edit asset <asset-id> files`
    - Asset manifest saved: `chore(agentfiles): update asset <asset-id> manifest`
    - Plan-apply: `chore(agentfiles): sync project <name> (N files)`
    - Subject only, no body.
- **Pathspec per trigger**:
    - Asset files edited → `assets/<asset-id>/**` in profile repo.
    - Asset manifest → `assets/<asset-id>/asset.json` in profile repo.
    - Plan-apply → all files sync.Apply created/updated/deleted **plus**
      `.agentfiles/state.json`, single commit in target repo.
- **"Edit Project" modal writes to `~/.agentfiles/projects.json`, not
  target repo**. Not a commit trigger (dropped from user's original
  list on review — file is outside any project repo).
- **Isolation from unrelated user changes**: before commit, run
  `git diff --cached --name-only`; if any staged path lies outside
  our pathspec, abort commit + notify. No stash, no plumbing tricks.
- **Author identity**: user's `git config user.name` / `user.email`.
  No `-c` override.
- **Hooks honored** (no `--no-verify`). Hook failure → abort +
  notification with first line of stderr.
- **Empty diff → skip silently.** No `--allow-empty`.
- **Failure UX**: file writes already succeeded; commit failure never
  rolls them back. User sees warn toast; can commit manually.
- **`git` binary missing**: pre-flight check when user saves Settings
  with git enabled → refuse save + toast. At commit time (if binary
  vanished later) → skip + warn.
- **Notifications**: success case merges save + commit into one toast
  (`Asset "x" saved (committed abc1234)`). Skip / failure cases emit
  a second warn toast; base save toast unchanged.
- **Settings screen widgets**: `huh.NewSelect[bool]()` toggle for
  git-enabled; `[Save]` mnemonic `e` (matches edit_asset); `[Back]`
  mnemonic `b`. Dirty-check on Back opens confirm modal, same pattern
  as `edit_asset`.
- **Typed errors** in `internal/git/errors.go`: `BinaryMissingError`,
  `NotARepoError`, `UnrelatedStagedChangesError{Paths}`,
  `HookFailedError{Stderr}`, `CommitError{Stderr}`. All implement
  `errs.DomainError`.

## Acceptance Criteria

- [ ] New `internal/settings` package with `Store.Load()` / `Save()`
      round-trip against `~/.agentfiles/settings.json`; missing file
      returns defaults with no error.
- [ ] `internal/config/paths.go` gains `SettingsStoreFileName = "settings.json"`.
- [ ] `cmd/af/main.go` loads settings before TUI opens and injects them
      into `app.Service`; `svc.UpdateSettings(new)` swaps live on Save.
- [ ] Settings screen renders the git-enabled toggle, a `[Save]`
      button (`e`), a `[Back]` button (`b`); Save writes settings.json
      and emits an info toast (`Settings saved`).
- [ ] Back on dirty settings opens a `Discard unsaved changes?`
      confirm modal.
- [ ] Pre-flight: if user enables git and saves while `exec.LookPath("git")`
      fails, Save is refused with `git binary not found on PATH` toast;
      settings.json is not written.
- [ ] New `internal/git` package exports `Detect(dir) (*Repo, error)`
      using `git rev-parse --git-dir`, and `Repo.Commit(paths, msg)
    (shortSHA, error)`.
- [ ] `Repo.Commit` refuses when `git diff --cached --name-only`
      contains any path outside the supplied pathspec — returns
      `UnrelatedStagedChangesError{Paths}` with the offending paths.
- [ ] `Repo.Commit` returns `"", nil` when nothing in the pathspec
      differs from HEAD or the index (silent skip).
- [ ] `Repo.Commit` returns `HookFailedError{Stderr}` on non-zero exit
      from git when stderr begins with a hook diagnostic.
- [ ] `app.Service` holds a `GitCommitter` interface seam; production
      wiring uses `internal/git`; unit tests inject a fake that
      records `(dir, paths, msg)` tuples.
- [ ] Commit trigger — asset files: after editor returns with a real
      diff in a profile that is a git repo (and git enabled), a commit
      is created with subject `chore(agentfiles): edit asset <asset-id> files`
      and pathspec `assets/<asset-id>/**`.
- [ ] Commit trigger — asset manifest: after the `Save` button on the
      edit-asset screen persists a manifest change (profile is a git
      repo, git enabled), a commit is created with subject
      `chore(agentfiles): update asset <asset-id> manifest` and
      pathspec `assets/<asset-id>/asset.json`.
- [ ] Commit trigger — plan-apply: after `sync.Apply` writes to a
      target repo (git enabled), a **single** commit is created with
      subject `chore(agentfiles): sync project <name> (N files)` and
      pathspec = union of the applied files and `.agentfiles/state.json`.
- [ ] No commit is attempted when git is disabled in settings or when
      the mutated folder is not a git repository — file writes succeed
      as today with no `git`-related toast.
- [ ] Success toast merges save + commit: `Asset "x" saved (committed <sha>)`
      when a commit happens; base save text when it does not.
- [ ] Failure/skip cases surface a warn toast with the reasons listed
      under **Notifications** in Design decisions (silent skip for
      empty diff, silent no-op for not-a-repo / git-disabled).

## Out of scope

- Live reload of settings.json from disk while af is running.
- Migration for existing users (lazy-defaults on missing file suffices).
- Commit signing overrides, custom author identity, alternative commit
  message shapes, or per-project settings overrides.
- Any commit triggered by the "Edit Project" modal (writes only to
  `~/.agentfiles/projects.json`, which is outside project repos).
- Push / pull / branch management — commits stay local.
- CLI flags for git behavior; all control flows through the Settings
  screen.

## Plan

[plan.md](./plan.md)

## Verification

- Baseline: `make build && make test && make lint` pass.
- `go test ./internal/settings -run TestLoadSave` — round-trip
  settings.json (missing file → defaults; write → readback matches).
- `go test ./internal/git -run TestCommit` — real-git integration test
  in `t.TempDir()` (`git init`, seeded commit, `Repo.Commit` on
  changed file) asserts `git log -1 --format=%s` == expected subject
  and `git show --name-only HEAD` matches pathspec. Skips if
  `exec.LookPath("git")` fails.
- `go test ./internal/git -run TestCommit_UnrelatedStaged` — pre-stage
  an unrelated path, call `Repo.Commit`, expect
  `UnrelatedStagedChangesError` and no new commit in `git log`.
- `go test ./internal/git -run TestCommit_EmptyDiff` — call
  `Repo.Commit` with no diff, expect `("", nil)` and no new commit.
- `go test ./internal/app -run TestGitCommitter` — fake committer
  asserts (dir, paths, msg) tuple for each of the three triggers
  (asset files edited, asset manifest saved, plan-apply).
- `go test ./internal/tui/shell -run TestSettingsScreen` — Save
  writes settings.json + emits info toast; Back on dirty opens
  confirm modal.
- Smoke: `./bin/af` → Settings → toggle git on → Save → toast
  `Settings saved` appears → open a profile that is a git repo →
  edit an asset file → return from editor → `git log -1` in profile
  repo shows `chore(agentfiles): edit asset <asset-id> files`.
- Smoke: same session → Plan Project → Apply in a target repo that
  is a git repo → `git log -1` in target repo shows single commit
  `chore(agentfiles): sync project <name> (N files)` and
  `git show --name-only HEAD` includes `.agentfiles/state.json`.
- Smoke: with git disabled in settings, repeat both flows → no new
  commits in either repo, save/apply toasts unchanged.
