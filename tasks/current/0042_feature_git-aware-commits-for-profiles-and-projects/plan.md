# Plan — Git-aware commits for profiles and projects

Cross-links:
- Task body: [./description.md](./description.md)
- Guidelines: [git.md](../../../docs/guidelines/git.md),
  [external_tools.md](../../../docs/guidelines/external_tools.md),
  [tui.md](../../../docs/guidelines/tui.md),
  [sync_and_safety.md](../../../docs/guidelines/sync_and_safety.md),
  [clean_architecture.md](../../../docs/guidelines/clean_architecture.md),
  [clean_code.md](../../../docs/guidelines/clean_code.md),
  [domain_model.md](../../../docs/guidelines/domain_model.md),
  [solid.md](../../../docs/guidelines/solid.md),
  [testing.md](../../../docs/guidelines/testing.md),
  [errors.md](../../../docs/guidelines/errors.md).
- ADR to add: [`docs/adr/0019-optional-git-aware-commits.md`](../../../docs/adr/0019-optional-git-aware-commits.md) (new).

## Summary

Add an optional git integration that produces scoped Conventional-Commits
commits when `agentfiles` mutates a git-tracked folder. Enabled via a new
`Settings` screen backed by a persistent `~/.agentfiles/settings.json` store.
Three commit triggers wired at the `app.Service` boundary. `internal/git`
wraps the `git` binary per `external_tools.md`. Committer injected as an
interface seam so unit tests use a fake and never touch a real `git`
process.

## Package additions

- `internal/settings` — mirrors `projectstore` shape: `Settings` value,
  `Store{Path}`, `Load()`, `Save(Settings)`, `DefaultPath()`,
  `NewStore(path)`. Schema `{"version":1, "git":{"enabled":false}}`.
  Missing file → `Settings{Version:1}` with `Git.Enabled == false`, no
  error. Owner-only permissions (`0o600`/`0o700`) via
  `utils.WriteJSONAtomic`.
- `internal/git` — wraps the `git` binary via `os/exec`:
  - `Detect(dir string) (*Repo, error)` — runs `git rev-parse --git-dir`
    inside `dir`. Non-zero → `NotARepoError{Dir}`. Binary missing →
    `BinaryMissingError`. Handles worktrees / submodules / `.git`-file
    cases because `rev-parse` does.
  - `BinaryAvailable() error` — thin `exec.LookPath("git")` helper used
    by settings pre-flight, returning `BinaryMissingError` on failure.
  - `Repo{Dir}` value with `Commit(pathspec []string, msg string)
    (shortSHA string, err error)`:
    1. `git diff --cached --name-only` in `r.Dir`. Any staged path not
       covered by `pathspec` → `UnrelatedStagedChangesError{Paths}`.
       Coverage rule: repo-relative forward-slash paths must have a
       matching entry in `pathspec`; wildcard entries (`assets/foo/**`)
       match any descendant.
    2. `git add -- <pathspec…>` (verbatim, not `-A`) so only the
       intended files enter the index.
    3. `git diff --cached --quiet` to detect an empty diff → return
       `"", nil`.
    4. `git commit -m msg`. Non-zero exit → inspect stderr; if the
       first line matches the hook diagnostic shape (`error: hook
       …`, non-zero from `.git/hooks/*`) → `HookFailedError{Stderr}`.
       Any other non-zero → `CommitError{Stderr}`.
    5. `git rev-parse --short HEAD` → return the SHA.
  - Typed errors in `internal/git/errors.go`
    (`BinaryMissingError`, `NotARepoError`,
    `UnrelatedStagedChangesError{Paths}`, `HookFailedError{Stderr}`,
    `CommitError{Stderr}`), each implementing `errs.DomainError`.

## Public seam

- `internal/app` adds:
  ```go
  type GitCommitter interface {
      // Commit runs against a specific directory. Return "", nil for a
      // silent skip (dir not a repo, empty diff). Any other failure
      // surfaces as a typed error.
      Commit(dir string, pathspec []string, msg string) (sha string, err error)
  }
  ```
  Wired in `cmd/af/main.go` with a concrete implementation in
  `internal/app/git_committer.go` (thin wrapper around `git.Detect`
  + `Repo.Commit` that swallows `NotARepoError`).

- `app.Service` gains two new fields:
  - `settings settings.Settings`
  - `committer GitCommitter`
- `app.CommitOutcome{SHA string, Err error}` — every git-aware service
  method returns one so the TUI composes the success/warning toast
  without touching `internal/git` types.

## Service methods (call sites)

Three triggers, per the description:

| Trigger | Service method | Pathspec | Message |
|---------|----------------|----------|---------|
| Asset files edited (editor returned) | `Service.SaveAssetFilesEdit(profileRef, manifest)` | `assets/<asset-id>/**` (in profile repo) | `chore(agentfiles): edit asset <asset-id> files` |
| Asset manifest saved | `Service.UpdateAsset(profileRef, manifest)` (existing, extended) | `assets/<asset-id>/asset.json` | `chore(agentfiles): update asset <asset-id> manifest` |
| Plan applied to project | `Service.Apply(profileRef, projectID, r)` (existing, extended) | union of mutated `preview.Changes` paths + `.agentfiles/state.json` | `chore(agentfiles): sync project <name> (N files)` |

New service method:
- `Service.SaveAssetFilesEdit(profileRef, manifest) (CommitOutcome,
  errs.DomainError)` — persists the manifest (same as UpdateAsset) so
  form edits made while the editor was open survive, then attempts the
  files-scoped commit. Introduced because the editor-return flow needs
  a different commit shape from the plain Save button.

Existing service methods change signature:
- `UpdateAsset(profileRef, manifest) (CommitOutcome, errs.DomainError)`
- `Apply(profileRef, projectID, r) (*Preview, CommitOutcome, errs.DomainError)`

Rationale: making the outcome an explicit return keeps the seam
type-checked at every call site and lets the TUI render the merged
success toast without new state on `app.Service`.

Every git-aware method follows the same gate:
```go
if !s.settings.Git.Enabled { return CommitOutcome{}, nil }
outcome, err := s.commit(dir, pathspec, msg)
```
`s.commit` calls the injected `GitCommitter` and translates typed
errors into `CommitOutcome{Err: …}` (never returns a domain-error
because the file write already succeeded).

Additional service surface:
- `Service.Settings() settings.Settings` — read-only accessor for the
  Settings screen.
- `Service.UpdateSettings(new settings.Settings) errs.DomainError` —
  pre-flight when `new.Git.Enabled && !s.settings.Git.Enabled`: call
  `git.BinaryAvailable()`; failure surfaces `git.BinaryMissingError`
  and the on-disk file is not touched. On success: write via
  `settings.Store.Save`, then swap in-memory.

## Wiring changes

- `internal/config/paths.go` adds
  `const SettingsStoreFileName = "settings.json"`.
- `cmd/af/main.go`:
  1. Build `settingsStore := settings.NewStore("")`.
  2. Add a `--settings` flag mirroring `--registry` / `--projects`.
  3. Load settings before the TUI opens; log to stderr on load error
     and exit non-zero (matches the projectstore behavior).
  4. Build the concrete committer.
  5. Change `app.New(reg, proj)` → `app.NewWithStores(reg, proj,
     settingsStore, loadedSettings, committer)`. Constructor renamed to
     `NewWithStores` so the current three-argument `New` breaks
     compilers if a callsite is missed; both stores live inside the
     Service.

## TUI changes

### Settings screen (`internal/tui/shell/settings.go`)

Replace the "Coming soon." placeholder with a `huh.NewSelect[bool]()`
git-enabled toggle plus `[Save]` (mnemonic `e`, matching `edit_asset`)
and `[Back]` (mnemonic `b` + `esc`) buttons.

- Pattern the file-and-form structure after `edit_asset.go`: dedicated
  `settingsForm` struct, `hydrateForm(settings)`, `composeSettings()`,
  `dirty()` comparing form vs `snapshot`.
- `handler := focus.New()` with the toggle registered so the focused
  component behavior stays consistent with the rest of the shell.
- Save action:
  1. `svc.UpdateSettings(composed)` (via `actions.UpdateSettings`).
  2. On `git.BinaryMissingError`: refuse — emit warn toast
     `git binary not found on PATH`; snapshot is not refreshed so
     `dirty()` still returns true.
  3. On other errors: standard `mutationDoneMsg` failure path.
  4. On success: refresh snapshot, emit info toast `Settings saved`.
- Back action:
  - `!dirty()` → `popCmd()`.
  - `dirty()` → open `modal.NewConfirm("back-unsaved", "Discard unsaved
    changes?", nil)` — same pattern as `edit_asset.onBack()`.
- `StatusKeys()` exposes both `[Save]` and `[Back]` bindings.

### Edit Asset screen wiring

- Add `SaveAssetFilesEdit(actions.SaveAssetFilesEditInput) (app.CommitOutcome, errs.DomainError)` to
  `editAssetActions`.
- `handleEditorFinished` calls `s.actions.SaveAssetFilesEdit` instead
  of `saveManifestCmd`, passing the composed manifest. The message
  handler builds the toast text from `CommitOutcome` (see
  "Notification composition" below) instead of a plain success string.
- `onSave` (the Save button path) still calls `UpdateAsset`, but its
  action return is now `(app.CommitOutcome, errs.DomainError)` — the
  toast composer treats both paths identically.

### Plan Project screen wiring

- `SyncProject` action return becomes
  `(*app.Preview, app.CommitOutcome, errs.DomainError)`.
- `onApply` builds the "Project synced" toast using the same
  `notificationText(base, outcome)` helper the asset flow uses. On a
  commit failure the base toast fires as before + a warn toast for the
  git error.

### Notification composition

New shell helper `notificationText(base string, outcome
app.CommitOutcome) (successText string, warn *notifications.Notification)`:
- `outcome.SHA != ""` → `successText = base + " (committed " + sha + ")"`
- `outcome.Err != nil` → warn toast built from the underlying domain
  error's severity/text; `successText = base`
- otherwise → `successText = base`, no warn

Screens dispatch `notificationCmd(errs.SeverityInfo, successText)` and,
if `warn != nil`, `tea.Batch` a second `notificationCmd(warn.Severity,
warn.Text)`.

### Actions layer

- `internal/actions/inputs.go` adds `SaveAssetFilesEditInput{ProfileRef,
  Manifest}`.
- `internal/actions/assets.go` adds a matching action returning
  `(app.CommitOutcome, errs.DomainError)`.
- `UpdateAsset` and `SyncProject` action return types updated.
- New `actions/settings.go` with `LoadSettings() (settings.Settings, nil)`
  and `UpdateSettings(settings.Settings) (struct{}, DomainError)`.

## Domain implementation notes

- Sync's `Apply` still owns file writes. The commit lives in
  `app.Service.Apply` after `llmsync.Apply` returns nil. Path list for
  the pathspec is derived from `syncPreview.Changes` + the resolutions
  the caller passed — helper `mutatedPaths(preview, resolutions)` in
  `internal/app` computes:
  - `ChangeCreate`, `ChangeUpdate`, `ChangeDelete` → always mutated
  - `ChangeDrift` → mutated only when `driftByPath[path] ==
    DriftOverwrite`
  - `ChangeUnknown` → mutated only when `unknownByPath[path] ==
    UnknownDelete`
  - Always appends `.agentfiles/state.json`.
- `N` in the commit subject is the mutated-path count **excluding**
  `state.json`.
- The commit runs against the project's target repo dir (`proj.Path`)
  — that is where sync wrote the files.
- Editor-return flow: after `Repo.Commit` runs on the profile with
  pathspec `assets/<asset-id>/**`, the pattern matches every file
  inside that folder including `asset.json`, so the single commit
  covers both the manifest and the edited file. No double-commit.

## Guideline / doc updates

- New ADR `docs/adr/0019-optional-git-aware-commits.md`:
  - Context: users want history in profile repos and target repos
    without manual `git add` / `git commit` cycles.
  - Decision: opt-in setting, Conventional Commits, `internal/git`
    wraps `os/exec`, `app.GitCommitter` interface seam.
  - Consequences: mismatched pre-existing staged changes surface as
    an abort; hooks honored (may block a commit); binary detection
    on both the settings pre-flight and the commit site.
  - Alternatives rejected: go-git library (adds a heavy dep with a
    different LFS/hook story), always-on commits (removes user
    choice), CLI flag control (violates ADR 0006 TUI-only).
- `docs/architecture/05-building-block-view.md` — add package blocks
  for `internal/settings`, `internal/git`, and note the seam on
  `app.Service`.
- `docs/architecture/06-runtime-view.md` — add the commit flow to
  the Apply and Edit-asset sequence diagrams.
- `docs/architecture/08-concepts.md` — cross-reference the new
  external tool alongside the editor.
- `docs/architecture/09-architecture-decisions.md` — list ADR 0019.
- `docs/glossary.md` — add "settings store", "commit trigger",
  "git-aware".
- `CLAUDE.md` (package layout section) — add one-line entries for the
  new `settings` and `git` packages plus a note about the
  `GitCommitter` seam on `app.Service`.
- No new files under `docs/guidelines/` — `git.md` and
  `external_tools.md` already cover the shape.

## Execution plan

Order chosen so each phase compiles + tests pass before moving on.

1. **Config constant.** Add `SettingsStoreFileName` in
   `internal/config/paths.go`. No new imports.
2. **`internal/settings` package.**
   - `settings.go`: `Settings`, `GitSettings`, `Version`.
   - `store.go`: `Store`, `Path`, `NewStore`, `DefaultPath`, `Load`,
     `Save`. Uses `utils.WriteJSONAtomic` + `utils.ReadJSON`.
   - `errors.go`: `HomeDirUnavailableError` (mirroring
     `projectstore` for consistency).
   - `store_test.go`: `TestLoadSave`
     (missing file → defaults; write → readback matches).
3. **`internal/git` package.**
   - `git.go`: `Repo`, `Detect`, `BinaryAvailable`,
     `Repo.Commit`, `shellSafeArg` helper. Every subcommand runs via
     `exec.Command("git", ...)` inside `r.Dir`, with `cmd.Env`
     inherited so the user's `HOME`, `GIT_*`, and hook environment
     stay live. No `sh -c`.
   - `pathspec.go`: `covers(pathspec []string, path string) bool` —
     implements the `foo/**` wildcard rule.
   - `errors.go`: typed errors listed above.
   - `git_test.go`: `TestCommit`, `TestCommit_UnrelatedStaged`,
     `TestCommit_EmptyDiff`, `TestCommit_NotARepo`,
     `TestCommit_HookFailure`. Each `t.TempDir()` + `git init`,
     `git config user.email test@example` + `.name`; skipped if
     `exec.LookPath("git")` fails. Pathspec-coverage matcher gets its
     own unit table.
4. **`app.GitCommitter` seam.**
   - Add the interface + `CommitOutcome` in `internal/app/git.go`.
   - Add the production `internal/app/git_committer.go` wrapping
     `git.Detect` + `Repo.Commit`, so `internal/app` is the boundary
     that decides "not-a-repo → silent skip".
5. **`app.Service` wiring.**
   - Add `settings settings.Settings` + `committer GitCommitter` fields.
   - Rename existing constructor from `New` → `NewWithStores(reg,
     proj, sset, s, committer)`. Update every caller (`cmd/af/main.go`,
     `internal/app/*_test.go`).
   - Add helpers `mutatedPaths`, `commitEnabled`, `runCommit(dir,
     pathspec, msg) CommitOutcome`.
   - Change `UpdateAsset` signature → `(CommitOutcome, DomainError)`.
   - Add `SaveAssetFilesEdit(profileRef, manifest)` (persists manifest
     via `asset.SaveManifest`, then commits files-scoped).
   - Change `Apply` signature → `(*Preview, CommitOutcome, DomainError)`.
   - Add `Settings()` + `UpdateSettings(new)` (pre-flight via
     `git.BinaryAvailable`).
   - Unit tests in `internal/app/git_committer_test.go` covering:
     - fake committer records `(dir, pathspec, msg)` for each of the
       three triggers
     - `!Enabled` → committer never called
     - path-not-a-repo returns `CommitOutcome{}`
     - fake committer returning an error surfaces on `CommitOutcome.Err`
       while the underlying save still succeeds
   - Add `internal/app/service_settings_test.go` covering
     `UpdateSettings` pre-flight (fake `BinaryAvailable` seam) and
     write-then-swap semantics.
6. **`internal/actions` layer.**
   - New input `SaveAssetFilesEditInput` + action.
   - `UpdateAsset`, `SyncProject`, `Apply` return types updated.
   - New `LoadSettings` / `UpdateSettings` actions.
7. **`internal/tui/shell` Settings screen.**
   - Rewrite `settings.go`: form + snapshot + buttons + focus handler +
     modal for dirty-back confirm.
   - Extend `settings_test.go`:
     - Save writes settings + info toast (`Settings saved`).
     - Back on dirty opens confirm modal.
     - Enabling git while `BinaryAvailable` is stubbed to fail
       surfaces the warn toast and skips the write.
     - Toggle round-trip test asserts the composed value.
8. **Edit Asset screen wiring.**
   - Extend `editAssetActions` interface with
     `SaveAssetFilesEdit`.
   - Rewrite `handleEditorFinished` to call the new action and
     surface `CommitOutcome`.
   - Update `saveManifestCmd` / `onSave` paths for the new
     `CommitOutcome` return.
   - Update stubs + tests in `edit_asset_test.go` to cover the
     merged success text on commit success + warn toast on commit
     failure. Explicit case: profile is not a git repo — success text
     stays base string.
9. **Plan Project screen wiring.**
   - Update `SyncProject` interface + `onApply` to surface the
     `CommitOutcome`. New stub + tests in `plan_project_test.go`
     covering all four outcomes (disabled, not-a-repo, success, failure).
10. **`cmd/af/main.go` wiring.**
    - Build `settings.Store`, load settings, build committer, call new
      `app.NewWithStores`.
    - Add `--settings` flag mirroring existing ones.
11. **`make build && make test && make lint`** clean.
12. **Docs.**
    - Write ADR 0019.
    - Update arc42 chapters 05, 06, 08, 09.
    - Update `docs/glossary.md`.
    - Update `CLAUDE.md` package layout.
13. **Changelog entry** under `docs/changelog/` (per af.task.implement
    convention) — record the new packages, the ADR, and the enabled
    flag.

## Testing plan (mirrors ## Verification)

- Unit: `internal/settings` load/save round-trip and default fallback.
- Unit: `internal/git` `Commit`, `Commit_UnrelatedStaged`,
  `Commit_EmptyDiff`, `Commit_NotARepo`, `Commit_HookFailure`
  (skipped when git binary absent).
- Unit: `internal/app` fake-committer tuple assertions for each of
  the three triggers, disabled-flag no-op, not-a-repo no-op, and
  `UpdateSettings` pre-flight.
- Unit: `internal/tui/shell` settings screen Save / Back / toggle /
  pre-flight refusal; Edit Asset merged toast + warn toast; Plan
  Project merged toast + warn toast.
- Manual smoke: end-to-end runs listed in the task's Verification
  section.

## ADRs / docs / guidelines summary

- ADR added: `docs/adr/0019-optional-git-aware-commits.md`.
- Docs updated: `docs/architecture/{05,06,08,09}.md`,
  `docs/glossary.md`, `CLAUDE.md`.
- Guidelines added / updated: none — `git.md` and `external_tools.md`
  already cover the shape.

## Risks + notes

- `git rev-parse --git-dir` treats a bare repo as a repo but `git
  commit` requires a working tree — narrow the detection to
  `rev-parse --is-inside-work-tree` truthy so we do not try to commit
  in a bare repo.
- Hooks with interactive prompts will block the commit (they own the
  tty). Not our problem; hook failures surface as
  `HookFailedError{Stderr}`.
- The `assets/<asset-id>/**` pattern is a semantic pathspec used by
  our coverage checker; `git add` sees literal path arguments, so the
  service passes the full list of currently-staged descendant paths
  rather than the `**` string to `git add`. The `**` string is only
  used by the coverage rule.
- `git commit` respects `commit.gpgsign`; per the description we do
  not override with `-c commit.gpgsign=false`. A gpg failure surfaces
  as `CommitError{Stderr}`.
- `Repo.Commit` never runs `git stash`; unrelated staged changes
  abort the operation and leave the index alone.
