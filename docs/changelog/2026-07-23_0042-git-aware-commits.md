# 0042 changes

Adds optional git-aware auto-commits so `af` can record scoped
Conventional-Commits commits when it mutates a git-tracked folder. The
behavior is opt-in through a new persistent Settings screen backed by
`~/.agentfiles/settings.json`; three commit triggers wire the paths
`af` already mutates (asset manifest save, asset-files edit,
plan-apply) into typed commit messages.

## Decisions

- **New `internal/git` package wraps the `git` binary via `os/exec`
  rather than depending on go-git.** — **Why:** the external-tools
  guideline already picks this shape for the editor, and go-git would
  add a heavy dependency with its own LFS / hook / config divergence
  story.
- **`app.GitCommitter` interface seam.** — **Why:** keeps `app.Service`
  independent of the `git` package for unit tests and matches
  `docs/guidelines/clean_architecture.md` on side-effect boundaries.
- **Pathspec `assets/<id>/**` for the editor-return flow.** — **Why:**
  the same commit covers both the manifest and every edited file
  inside the asset folder, so the user never sees a double-commit.
- **`Repo.Commit` refuses when the index already carries paths outside
  the pathspec (`UnrelatedStagedChangesError`).** — **Why:** an
  automated commit must never silently absorb the user's unrelated
  work; the abort names the paths so recovery is a single `git reset
  HEAD -- <path>` away.
- **Hook detection is heuristic (any executable hook file → classify
  the failure as `HookFailedError`).** — **Why:** git does not emit a
  stable "hook failed" line; the classification lets the TUI show a
  hook-specific message when a hook is likely to blame.

## Assumptions

- **The user's git identity (`git config user.name / user.email`) is
  set.** — **Why:** every existing profile / target repo the user
  already commits into has one; `git commit` surfaces the misconfig
  clearly if not.
- **The user does not want push / pull automation.** — **Why:** the
  description scopes the feature to local commits; push semantics are
  a much larger surface (auth, remotes, retries) explicitly out of
  scope.

## Other Notes

- New ADR: `docs/adr/0019-optional-git-aware-commits.md`.
- Updated arc42 chapters: 05 (building blocks — `settings`, `git`,
  the `GitCommitter` seam), 06 (runtime view — plan-apply commit),
  08 (concepts — external-tool handoff), 09 (ADR index).
- Glossary additions: **Settings Store**, **Commit Trigger**,
  **Git-Aware Commit**; **User Config Dir** now mentions
  `settings.json`.
- `CLAUDE.md` package layout gains one-liners for `internal/settings`
  and `internal/git` plus the `GitCommitter` seam note.
- No new `docs/guidelines/*.md`: `git.md` and `external_tools.md`
  already cover the shape.

## `settings` package + `~/.agentfiles/settings.json`

New leaf-shaped store mirroring `projectstore` / `registry`.

```go
// after
type Settings struct {
    Version int         `json:"version"`
    Git     GitSettings `json:"git"`
}

type Store struct{ Path string }

func (s *Store) Load() (Settings, errs.DomainError) { ... }
func (s *Store) Save(v Settings) errs.DomainError    { ... }
```

Missing file → `Default()` (git-disabled) with no error so first-time
users see a clean baseline.

## `internal/git` — narrow wrapper around the binary

```go
// after
func Detect(dir string) (*Repo, errs.DomainError)
func BinaryAvailable() errs.DomainError
func (r *Repo) Commit(pathspec []string, msg string) (string, errs.DomainError)
```

`Commit` stages the pathspec, silently skips on empty diff, and
classifies non-zero exits as `HookFailedError` when a commit-time hook
is installed and `CommitError` otherwise. Wildcards `foo/**` in the
pathspec are converted to the corresponding literal directory when
handed to `git add`; the wildcard is used only by the semantic
`Covers` helper.

## `app.GitCommitter` seam + `CommitOutcome`

```go
// after
type GitCommitter interface {
    Commit(dir string, pathspec []string, msg string) (string, errs.DomainError)
}

type CommitOutcome struct {
    SHA string
    Err errs.DomainError
}
```

Three service methods now return `CommitOutcome` alongside their
existing outputs:

```go
// after
func (s *Service) UpdateAsset(profileRef string, manifest *asset.Manifest) (CommitOutcome, errs.DomainError)
func (s *Service) SaveAssetFilesEdit(profileRef string, manifest *asset.Manifest) (CommitOutcome, errs.DomainError)
func (s *Service) Apply(profileRef, projectID string, r Resolutions) (*Preview, CommitOutcome, errs.DomainError)
```

`Service.commitEnabled()` + `Service.runCommit()` centralize the
"settings on + committer wired" check so every trigger has the same
skip/error semantics.

## Constructor rename: `app.New` → `app.NewWithStores`

```go
// before
svc := app.New(reg, proj)
```

```go
// after — settings store + initial settings + committer wired at composition root
svc := app.NewWithStores(reg, proj, settingsStore, loadedSettings, app.NewGitCommitter())
```

The rename makes every existing call site an intentional compile
error so no wiring path is missed. Test helpers (`newSvc`, fixtures in
`internal/actions/*_test.go` and `internal/tui/shell/*_test.go`) route
through `newSvcWith` which injects a `fakeCommitter`.

## Settings TUI screen rewrite

The placeholder body is replaced with a `huh.NewSelect[bool]()` git
toggle plus `[Save]` (mnemonic `e`) / `[Back]` (mnemonic `b` / `esc`)
buttons following the `edit_asset` pattern. Save calls
`actions.UpdateSettings` and emits a `Settings saved` toast; the dirty
Back opens a `Discard unsaved changes?` confirm modal.

## Merged save-plus-commit toast helper

New `internal/tui/shell/commits.go`:

```go
// after
func notificationText(base string, outcome app.CommitOutcome) (string, tea.Cmd)
func commitOutcomeCmd(base string, outcome app.CommitOutcome) tea.Cmd
```

Edit Asset composes `Asset "x" saved (committed abc1234)` when the
commit succeeded; Plan Project uses the same helper for the
`Project synced` toast. A hard commit failure fires a second warn
toast so the base save toast stays unchanged.

## `cmd/af/main.go`

```go
// after
settingsStore := settings.NewStore(*settingsPath)
loadedSettings, _ := settingsStore.Load()
svc := app.NewWithStores(profileStore, projectStore, settingsStore, loadedSettings, app.NewGitCommitter())
```

Adds a `--settings` flag mirroring the existing `--registry` /
`--projects` flags.
