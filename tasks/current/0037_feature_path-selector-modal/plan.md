# Plan — Task 0037: Path Selector Modal

Cross-links: [description.md](./description.md)

## Context

We need a **reusable** TUI modal for picking a filesystem path (folder or
file). Callers open it, tell it what to browse under (a `Constraint`
root), and receive back a `Result{Path, IsDir}` through
`modal.ResolvedMsg`. This task builds the modal itself — no caller-side
wiring; every integration (register project, register asset, edit
profile `Path`, …) becomes a follow-up task.

The modal is safety-critical: a mis-implementation lets a user (or a
symlink) browse outside the intended root and pick a path the caller
then acts on. Two concrete rejection rules must hold: **`..` at the
constraint root is a no-op**, and **symlinks that resolve outside the
constraint are refused**.

## Package layout

```
internal/tui/modals/pathselector/                  ← new package
    pathselector.go       Content impl + View/Update/Init/Lifecycle
    options.go            Options struct + validation (normalize)
    entries.go            os.ReadDir → filter + sort → row model
    safety.go             constraint & symlink resolution helpers
    result.go             Result type + ResultFromMsg
    errors.go             typed DomainErrors
    strings.go            static labels ("Cannot leave …", "<empty>", …)

    pathselector_test.go  Behavior tests driving Content.Update
    options_test.go       Constructor validation tests
    entries_test.go       Sort / filter / hidden / extension tests
    safety_test.go        constraint / symlink unit tests
    result_test.go        ResultFromMsg tests
    testhelpers_test.go   makeTree, sendKey, drainNotifications

internal/tui/modals/
    select_path.go        Thin wrapper: NewSelectPath(opts) (*modal.Modal, errs.DomainError)
```

Rationale for the split (`select_path.go` in `modals/` + package under
`modals/pathselector/`):

- Every other modal has a `create_*.go` file directly under
  `internal/tui/modals/`. `select_path.go` matches that convention so
  screen code stays a one-liner (`modals.NewSelectPath(...)`).
- The `pathselector` package owns state, filesystem access and
  filtering. Isolating it lets `clean_architecture.md`'s "I/O at the
  edges" rule apply naturally — the filesystem is the edge the modal
  presents to the user.

## Public API

### `internal/tui/modals/select_path.go`

```go
package modals

func NewSelectPath(opts pathselector.Options) (*modal.Modal, errs.DomainError) {
    content, err := pathselector.New(opts)
    if err != nil {
        return nil, err
    }
    caption := opts.Caption
    if caption == "" {
        caption = "Select path"
    }
    return modal.New("select-path", content, modal.WithCaption(caption)), nil
}
```

### `internal/tui/modals/pathselector/options.go`

`Options` matches the task description verbatim (`Caption`,
`Constraint`, `StartFolder`, `ShowFiles`, `AllowedExtensions`,
`FollowSymlinks`, `ShowHiddenInitially`).

`FollowSymlinks bool` stays as spelled. Doc comment on the field
records the deliberate choice: *"Zero-value is `false`; callers must
set it to `true` to follow directory symlinks."* No inversion, no
`*bool`.

`(Options).normalize() (Options, errs.DomainError)`:

- Trim `Constraint` and `StartFolder`.
- If `Constraint != ""`: `filepath.Abs` → `filepath.EvalSymlinks` →
  `os.Stat` must be a directory. Otherwise return
  `ConstraintUnreadableError{Path, Err}`.
- Resolve `StartFolder`:
  - Empty → `Constraint` when set, else `os.UserHomeDir()`, else `"/"`.
  - Non-empty → `filepath.Abs` → `EvalSymlinks` → `os.Stat` must be a
    directory. Otherwise `StartUnreadableError{Path, Err}`.
- When `Constraint != ""` verify the resolved start is under it via
  `safety.isUnderRoot(start, constraint)`. Otherwise
  `StartOutsideConstraintError{Start, Constraint}`.
- Lower-case + leading-dot canonicalize each entry of
  `AllowedExtensions`; drop empties.

### `internal/tui/modals/pathselector/pathselector.go`

```go
type Content struct {
    opts         Options              // frozen, post-normalize
    allowedExt   map[string]struct{}  // nil ⇒ allow all
    current      string               // absolute current folder
    entries      []entry              // filtered+sorted rows for `current`
    tree         *treetable.Model
    selectBtn    *mnemonic.Button     // per-row [Select] (s)
    selectCurBtn *mnemonic.Button     // [Select current] (c)
    hiddenBtn    *mnemonic.Button     // [Show hidden] / [Hide hidden] (h)
    cancelBtn    *mnemonic.Button     // [Cancel] (n) + esc extra key
    showHidden   bool
    width, height int
    state        modal.LifecycleState
    value        any                  // Result on Confirmed
}

func New(opts Options) (*Content, errs.DomainError)
func (c *Content) Init() tea.Cmd
func (c *Content) Update(msg tea.Msg) (modal.Content, tea.Cmd)
func (c *Content) View() string
func (c *Content) SetSize(w, h int)
func (c *Content) Lifecycle() (modal.LifecycleState, any)
```

### `internal/tui/modals/pathselector/result.go`

```go
type Result struct {
    Path  string
    IsDir bool
}

func ResultFromMsg(m modal.ResolvedMsg) (Result, bool) {
    if !m.Confirmed {
        return Result{}, false
    }
    r, ok := m.Value.(Result)
    if !ok {
        return Result{}, false
    }
    return r, true
}
```

### `internal/tui/modals/pathselector/errors.go`

Struct-per-failure per `docs/guidelines/errors.md`; every type
implements `Error()` and `Severity() errs.Severity`.

```go
type StartOutsideConstraintError struct{ Start, Constraint string }
type StartUnreadableError        struct{ Path string; Err error }  // Unwrap() error
type ConstraintUnreadableError   struct{ Path string; Err error }  // Unwrap() error
```

Callers use `errors.As`. No `Err…` sentinel values.

## Behavior model (Content.Update)

Key order in `Update`:

1. `tea.KeyPressMsg`:
   - `Esc` → `state = Cancelled`, `value = nil`.
   - `Enter` on cursor row:
     - `..` → `navigate(parent)`.
     - Folder / dir-symlink (when `FollowSymlinks`) → `navigate(row.abs)`.
     - Dir-symlink with `FollowSymlinks == false` → no-op.
     - File → no-op.
     - `<empty>` placeholder → no-op.
   - Otherwise walk the mnemonic buttons and trigger the first that
     matches (`selectBtn`, `selectCurBtn`, `hiddenBtn`, `cancelBtn`).
   - Otherwise forward to `tree.Update(msg)` (arrow keys / paging).
2. Non-key messages: forward to `tree.Update(msg)`.

Button actions:

- `selectBtn` (`s`): resolve with the cursor row.
  - `..` → parent path, `IsDir=true`.
  - folder / dir-symlink → row path, `IsDir=true`.
  - file → row path, `IsDir=false`.
  - `<empty>` placeholder → no-op.
- `selectCurBtn` (`c`): resolve with `Result{current, true}`.
- `hiddenBtn` (`h`): toggle `showHidden`; rebuild `entries`; refresh
  tree root; swap label between `Show hidden` and `Hide hidden`.
- `cancelBtn` (`n`) with extra binding `esc`: same as raw `Esc`.

`Lifecycle()` returns `(state, value)` per convention.

## Navigation

Single `navigate(target string)` helper:

1. Resolve `target` with `safety.resolve(target, opts.FollowSymlinks)`.
2. If `Constraint != ""` and the resolved path escapes the constraint
   → emit a `notifications.NotificationMsg` (severity `Error`) with
   text `"Cannot leave <constraint>"`. Do **not** change `current`.
3. `os.ReadDir(resolved)`:
   - Failure → emit an error `NotificationMsg` naming the failed path
     and the OS error. Do not change `current`.
   - Success → set `current = resolved`, rebuild entries, refresh the
     tree root, reset cursor to top.

The modal never resolves on an I/O error.

## Entries pipeline (`entries.go`)

```go
type entryKind int
const (
    entryParent entryKind = iota // ".."
    entryDir
    entryDirSymlink
    entryFile
    entryFileSymlink
    entryEmpty                    // "<empty>" placeholder
)

type entry struct {
    Name   string      // display name including trailing '/' or '@'
    Abs    string      // absolute path (target for nav / selection)
    Kind   entryKind
    Hidden bool        // starts with '.'
}
```

`buildEntries(dir string, opts Options, allowed map[string]struct{}, showHidden bool) ([]entry, errs.DomainError)`:

1. `os.ReadDir(dir)`. On error return a typed
   `ReadDirError{Path, Err}`.
2. For each dirent:
   - Skip when `!showHidden` and the name begins with `.`.
   - Symlinks (`Type&os.ModeSymlink != 0`): `os.Stat` the target;
     classify by result. If stat fails, classify as file-symlink and
     drop it in `ShowFiles == false` mode.
   - Directories: keep.
   - Files: apply `ShowFiles` and `AllowedExtensions` (case-insensitive
     extension match).
3. Sort directories A→Z case-insensitive, then files A→Z.
4. Prepend an `entryParent` unless `dir == opts.Constraint`.
5. If the result is empty, return a single `entryEmpty` row.

## Safety helpers (`safety.go`)

Duplicates (not imports) the shape found in `internal/app/service.go`
`isUnderRoot` / `isAncestor` and the resolve pattern from
`internal/asset/files.go`. Reason for duplication: the modal must not
depend on `internal/app` (see `docs/guidelines/clean_architecture.md`
"dependencies point inward").

```go
func isUnderRoot(path, root string) bool
func resolve(path string, follow bool) (string, error)
```

`resolve`:

- `filepath.Abs`.
- If `follow`: `filepath.EvalSymlinks` — fall back to the absolute path
  when the target does not yet exist, so callers can still surface a
  meaningful error later.
- `filepath.Clean`.

`isUnderRoot` uses `EvalSymlinks` on both sides, then checks equality
or `filepath.Rel` starts-with-`..`.

## Rendering

`internal/tui/components/treetable` in flat-root mode:

- Root node label is unused; its `Children` are the visible rows.
- Name column stretches to fill: `w - actionsWidth - borders`.
- Actions column width fixed at 10 (fits `[Select]`).
- Actions func returns a slice containing exactly `selectBtn` when the
  cursor row's `entry.Kind != entryEmpty`; otherwise an empty slice.

Below the tree, the screen-level button row uses
`lipgloss.JoinHorizontal` with `mnemonic.Button.View()`:

```
[Select current] [Show hidden] [Cancel]
```

(`[Hide hidden]` when `showHidden` is true.)

`SetSize(w, h)` recomputes tree height:
`h - 1 (caption) - 1 (button row) - 2 (panel borders)`. When
`w < 40 || h < 10`, replace the tree body with a
`"Terminal too small"` placeholder while keeping the buttons.

## Notifications

Import `internal/tui/notifications`. A helper
`emitNotify(severity errs.Severity, text string) tea.Cmd` wraps the
`NotificationMsg` returned by the command closure, so the callsites
stay uncluttered:

```go
return c, emitNotify(errs.SeverityError, "Cannot leave "+c.opts.Constraint)
```

## Tests — every acceptance criterion mapped

Every test uses `t.TempDir()` and, for symlink cases, `os.Symlink`.

| AC # | Test name                            | File                     |
|------|--------------------------------------|--------------------------|
| 1    | `TestConstraintRootUpwardNoop`       | `pathselector_test.go`   |
| 2    | `TestSymlinkEscapeRejected`          | `pathselector_test.go`   |
| 3    | `TestSymlinkNoFollow`                | `pathselector_test.go`   |
| 4    | `TestConstructorValidatesInputs`     | `options_test.go`        |
| 5    | `TestListingSortOrder`               | `entries_test.go`        |
| 6    | `TestFoldersOnlyMode`                | `entries_test.go`        |
| 7    | `TestExtensionFilterCaseInsensitive` | `entries_test.go`        |
| 8    | `TestHiddenToggle`                   | `pathselector_test.go`   |
| 9    | `TestEnterSemantics`                 | `pathselector_test.go`   |
| 10   | `TestRowSelectResolvesResult`        | `pathselector_test.go`   |
| 11   | `TestSelectCurrentDir`               | `pathselector_test.go`   |
| 12   | `TestReadDirDeniedNotifies`          | `pathselector_test.go`   |
| 13   | `TestCancelPaths`                    | `pathselector_test.go`   |
| 14   | `TestMnemonicUniqueness`             | `pathselector_test.go`   |
| 15   | `TestResultFromMsg`                  | `result_test.go`         |

Guardrail tests (not required by the AC list but earned by the
security guideline):

- `TestSafetyIsUnderRoot` — table test in `safety_test.go`: same path,
  ancestor, sibling with common prefix, symlink resolution.
- `TestSafetyResolveHandlesMissingSuffix` — resolve behavior when a
  path's tail does not exist yet.

Helpers (`testhelpers_test.go`):

- `makeTree(t, root string, spec map[string]string)` — build a fixture
  tree from a `path → content` map; paths ending in `/` are dirs.
- `sendKey(c *Content, key string) tea.Cmd` — drive
  `Content.Update(tea.KeyPressMsg{...})`.
- `drainNotifications(cmd tea.Cmd) []notifications.Notification` —
  execute cmd and collect `NotificationMsg` output.

### AC #2 (symlink escape) test shape

Fixture:

```
tmp/root/           (constraint)
    inner/
    out -> ../other/
tmp/other/
    file.md
```

`Constraint = tmp/root`, `FollowSymlinks = true`. Cursor sits on
`out@`. Send `Enter`. Assertions:

1. Current folder unchanged (equals `tmp/root`).
2. Exactly one `NotificationMsg` emitted with severity `Error` and
   text `"Cannot leave <tmp/root>"`.
3. `Lifecycle()` still reports `Active`.

## Documentation updates

- `docs/architecture/05-building-block-view.md` — add
  `### tui/modals/pathselector` after the existing `tui/components/*`
  subsections in "Level 2: Package Responsibilities". 4-6 lines
  covering purpose, collaborators, and the constraint / symlink safety
  statement.
- `docs/glossary.md` — two entries: `Constraint Root` and
  `Path Selector Modal`.
- `CLAUDE.md` package-layout table — one bullet under `internal/`.
- **No ADR.** The safety mechanics (`EvalSymlinks` + `filepath.Rel` +
  `..` check) already have precedent in `internal/asset/files.go` and
  `internal/app/service.go` and are called out in
  `docs/guidelines/security.md`.
- **No new guideline.** `tui.md`, `charm.md`, `errors.md`, and
  `security.md` already cover everything the modal practices.

## Execution order

1. Create branch `feature/path-selector-modal` (working tree already
   clean).
2. `errors.go` — three typed errors.
3. `result.go` + `result_test.go` — smallest surface, land green first.
4. `safety.go` + `safety_test.go` — pure functions, real symlinks in
   `t.TempDir()`.
5. `options.go` + `options_test.go` — covers AC #4
   (`TestConstructorValidatesInputs`).
6. `entries.go` + `entries_test.go` — AC #5, #6, #7 (sort, folders-
   only, extension filter).
7. `pathselector.go` (Content impl) incrementally:
   1. Constructor + Init + View skeleton.
   2. Enter / navigation (AC #1, #2, #3, #9).
   3. Row select `s` (AC #10).
   4. `[Select current]` `c` (AC #11).
   5. `[Show hidden]` `h` toggle (AC #8).
   6. `Esc` + `[Cancel]` `n` (AC #13).
   7. I/O error path (AC #12).
   8. Mnemonic uniqueness check at construction time + test (AC #14).
8. `internal/tui/modals/select_path.go` wrapper.
9. `make fmt lint test`; iterate.
10. Docs — arc42 §5, glossary, CLAUDE.md.
11. Frontmatter `status: in-review`.
12. Changelog `docs/changelog/2026-07-20_0037-path-selector-modal.md`.

## Verification

- `make build && make test && make lint` clean.
- `go test ./internal/tui/modals/pathselector/... -v` — every
  AC-named test present and green.
- Security-focused subset (task line 197):

  ```
  go test ./internal/tui/modals/pathselector \
      -run 'TestConstraintRootUpwardNoop|TestSymlinkEscapeRejected|TestSymlinkNoFollow|TestConstructorValidatesInputs'
  ```

  green.
- Manual smoke: build; open the TUI. No integration exists yet
  (Out of Scope), so this reduces to "no panic on boot, no linter
  regressions in `internal/tui/*`."

## Confirmed design choices

1. `FollowSymlinks bool` kept as spec'd. Zero value `false`; the doc
   comment makes callers own the choice.
2. `[Cancel]` mnemonic is `n`, with `esc` as an extra binding.
3. Errors are struct types (`StartOutsideConstraintError`,
   `StartUnreadableError`, `ConstraintUnreadableError`). No `Err…`
   sentinel values. Matches every other `errors.go` in the repo.
