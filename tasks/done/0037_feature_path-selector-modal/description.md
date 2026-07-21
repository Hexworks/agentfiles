---
id: 0037
type: feature
status: done
topics: tui, charm, go, security
---

# Path selector modal

Reusable TUI modal that lets the user pick a filesystem path (folder or file)
under a caller-configurable constraint. Opened by other screens via the modal
component (`internal/tui/components/modal`), returns the selected absolute path
back to the caller through `modal.ResolvedMsg`.

## Location and shape

- Package: `internal/tui/modals/pathselector/`. Owns the stateful
  `modal.Content` implementation, directory reads, filtering, and rendering.
- Thin wrapper: `internal/tui/modals/select_path.go` exposes
  `modals.NewSelectPath(opts) (*modal.Modal, errs.DomainError)` matching the
  other modal constructors in that directory (`create_asset.go`,
  `create_file.go`, …).
- The list uses `internal/tui/components/treetable` with a flat root (no
  hierarchy) and a per-row actions column — same shape as the Profiles and
  Assets tables.

## Constructor options

```go
type Options struct {
    // Caption shown in the modal's top border, e.g. "Selecting project root".
    // Empty defaults to "Select path".
    Caption string

    // Constraint is the absolute directory the user cannot navigate above.
    // Empty means no constraint — user may browse the entire filesystem.
    Constraint string

    // StartFolder is the folder shown when the modal opens. Empty defaults to
    // Constraint when set, otherwise to $HOME (falling back to "/" if HOME
    // is unset). Must lie inside Constraint when Constraint is set.
    StartFolder string

    // ShowFiles: when false, files are hidden and only folders can be
    // selected. Not toggleable at runtime.
    ShowFiles bool

    // AllowedExtensions restricts visible files when ShowFiles is true.
    // Nil/empty = all extensions allowed. Match is case-insensitive and
    // leading-dot canonicalized ("md" and ".MD" both match ".md").
    // Silently ignored when ShowFiles is false.
    AllowedExtensions []string

    // FollowSymlinks: when true (default), Enter follows directory symlinks
    // and constraint checks use filepath.EvalSymlinks so symlinks that
    // resolve outside Constraint are rejected. When false, symlinks are
    // shown but Enter on a symlink is a no-op.
    FollowSymlinks bool

    // ShowHiddenInitially: initial state of the runtime hidden-files toggle
    // (mnemonic `h`). Defaults to false (dotfiles hidden).
    ShowHiddenInitially bool
}
```

Construction errors (typed per `docs/guidelines/errors.md`, package
`pathselector/errors.go`):

- `ErrStartOutsideConstraint` — `StartFolder` is set and resolves outside
  `Constraint`.
- `ErrStartUnreadable` — `StartFolder` does not exist, is not a directory, or
  cannot be read.
- `ErrConstraintUnreadable` — `Constraint` is set but does not exist / is not
  a directory.

## Behavior

- **Layout**: caption in the modal's top border; single-column treetable
  labelled `Name`; per-row actions column carrying `[Select]` on the cursor
  row; screen-level mnemonic buttons below the table for
  `[Select current]` (returns the folder currently browsed),
  `[Show hidden]` / `[Hide hidden]` (runtime toggle), and `[Cancel]`.
- **Row rendering**:
    - `..` at the top of the listing (absent when browsing at the constraint
      root, since there is nowhere to go up to).
    - Folders rendered as `<name>/`.
    - Symlinks to directories rendered as `<name>@`.
    - Files rendered as bare name.
    - Empty folder → single italic `<empty>` placeholder row (still non-
      selectable).
- **Sort**: `..` first, then directories A→Z (case-insensitive), then files
  A→Z. Symlinks sort with the type they resolve to (directory or file).
- **Keys**:
    - `Enter` on a folder or `..`: navigate into it. `Enter` on a file: no-op.
    - `s` (`[Select]`): select current row → resolve modal with
      `Result{Path, IsDir}`. On `..` row, selects the parent folder.
    - `c` (`[Select current]`): resolve modal with the folder currently being
      browsed.
    - `h` (`[Show hidden]` / `[Hide hidden]`): toggle dotfile visibility.
    - `Esc` and `[Cancel]` (mnemonic): resolve with `Confirmed=false`,
      `Value=nil`.
- **Navigation safety**:
    - Attempting to navigate above `Constraint` (via `..` at the root, or via
      a symlink that resolves outside) is rejected. Modal stays on the
      current folder and emits a `notifications.NotificationMsg` with error
      severity ("Cannot leave <constraint>").
    - `FollowSymlinks=false`: `Enter` on a symlink is a silent no-op.
- **I/O errors** (permission denied, folder disappeared, partial read):
  modal stays on the previous folder, emits a
  `notifications.NotificationMsg` (error severity) naming the failed path
  and the error class. Modal never resolves on an I/O error.
- **Sizing**: `SetSize(w, h)` recomputes visible-row count from available
  height minus (caption + button row + optional inline error line + panel
  borders). Name column stretches to fill the interior width. Below a
  minimum (roughly 40×10), a "Terminal too small" placeholder replaces the
  table body.

## Result type

```go
package pathselector

// Result carries the selection back to the caller.
type Result struct {
    Path  string // absolute, cleaned path (symlinks resolved when FollowSymlinks)
    IsDir bool
}

// ResultFromMsg extracts a Result from a modal.ResolvedMsg. Returns (r, true)
// when Confirmed and the Value is a Result; (Result{}, false) otherwise.
func ResultFromMsg(m modal.ResolvedMsg) (Result, bool)
```

## Acceptance Criteria

- [ ] Given constraint `/tmp/root/` populated with subdirs, navigating `..` at
      `/tmp/root/` is a no-op (stays put, no error notification) —
      `TestConstraintRootUpwardNoop`.
- [ ] Given constraint `/tmp/root/` and a symlink `/tmp/root/out → /tmp/other/`
      with `FollowSymlinks=true`, `Enter` on `out` emits an error
      `notifications.NotificationMsg` and does not change the current folder —
      `TestSymlinkEscapeRejected`.
- [ ] With `FollowSymlinks=false`, `Enter` on a directory symlink is a silent
      no-op — `TestSymlinkNoFollow`.
- [ ] Constructor with `StartFolder` outside `Constraint` returns
      `ErrStartOutsideConstraint`; unreadable `StartFolder` returns
      `ErrStartUnreadable`; unreadable `Constraint` returns
      `ErrConstraintUnreadable` — `TestConstructorValidatesInputs`.
- [ ] Listing sort in a folder containing `Bar/`, `alpha/`, `README.md`,
      `apple.txt`: rows in order `..`, `alpha/`, `Bar/`, `apple.txt`,
      `README.md` — `TestListingSortOrder`.
- [ ] `ShowFiles=false` hides every file regardless of `AllowedExtensions` —
      `TestFoldersOnlyMode`.
- [ ] `ShowFiles=true` with `AllowedExtensions=[".md",".json"]` shows only
      matching files; a file named `NOTES.MD` is visible under filter `.md` —
      `TestExtensionFilterCaseInsensitive`.
- [ ] `ShowHiddenInitially=false` hides dotfiles; pressing mnemonic `h`
      reveals them; pressing `h` again hides them — `TestHiddenToggle`.
- [ ] `Enter` on a folder navigates into it and updates the listing; `Enter`
      on a file is a no-op (listing unchanged, modal not resolved) —
      `TestEnterSemantics`.
- [ ] Per-row `[Select]` on a file resolves the modal with
      `Result{Path:"…/file.md", IsDir:false}`; on a folder resolves with
      `IsDir:true`; on `..` resolves with the parent folder path and
      `IsDir:true` — `TestRowSelectResolvesResult`.
- [ ] Screen-level `[Select current]` (mnemonic `c`) resolves the modal with
      the folder currently being browsed — `TestSelectCurrentDir`.
- [ ] Permission-denied on `Enter`ing a folder leaves the modal on the
      previous folder and emits an error `notifications.NotificationMsg` —
      `TestReadDirDeniedNotifies`.
- [ ] `Esc` and the screen-level `[Cancel]` mnemonic both emit
      `ResolvedMsg{Confirmed:false, Value:nil}` — `TestCancelPaths`.
- [ ] Walking every cursor state (`..` row, folder row, file row, empty
      folder, folders-only mode), no two labelled buttons share a mnemonic —
      `TestMnemonicUniqueness`.
- [ ] `ResultFromMsg(msg)` returns `(Result, true)` when `msg.Confirmed` and
      `msg.Value` is a `Result`; returns `(Result{}, false)` when
      `msg.Confirmed==false` or `msg.Value` is a different type —
      `TestResultFromMsg`.

## Out of scope

- Wiring the modal into any existing screen (register project, register
  asset, edit profile, …). Each integration is a follow-up task.
- Async / streaming directory reads. Reads are synchronous.
- File preview panel, size / modified-time columns.
- Multi-select — the modal returns exactly one path.
- Type-to-filter or fuzzy search inside the current folder.
- Creating new folders from within the modal.

## Verification

- Baseline: `make build && make test && make lint` pass.
- `go test ./internal/tui/modals/pathselector/...` green — every acceptance
  criterion above corresponds to a named test in this package.
- Security-focused subset — path-traversal safety gates:
  `go test ./internal/tui/modals/pathselector -run 'TestConstraintRootUpwardNoop|TestSymlinkEscapeRejected|TestSymlinkNoFollow|TestConstructorValidatesInputs'`
  green.

## Plan

[plan.md](./plan.md)

## Clarification

### Question

The `FollowSymlinks bool` field is documented as "true (default)" but
Go's zero value for `bool` is `false`. Should we invert the field
(`NoFollowSymlinks`), promote it to `*bool`, or leave the shape and
document the zero-value semantics?

### Answer

Keep the field named `FollowSymlinks bool`. The doc comment states that
the zero value is `false` and callers must set it to `true` explicitly
to follow directory symlinks. No inversion, no pointer bool.

### Question

The task requires a screen-level `[Cancel]` mnemonic button, but does
not pin a letter. `s`, `c`, and `h` are already taken by
`[Select]`, `[Select current]`, and `[Show hidden]`. Which unused
letter from "Cancel" should be the mnemonic?

### Answer

Use `n` (`ca[N]cel`). The button also carries `esc` as an extra key
binding so raw `Esc` and the mnemonic behave identically.

### Question

The task text uses names like `ErrStartOutsideConstraint` — a
Go-idiomatic prefix for **sentinel values**. Every other domain
package in this repo defines struct types (`AssetNotFoundError`,
`FilePathError`, …) compared via `errors.As`. Which shape should the
package expose?

### Answer

Struct types: `StartOutsideConstraintError`, `StartUnreadableError`,
`ConstraintUnreadableError`. Callers use `errors.As`. No `Err…`
sentinel values. This matches every other `errors.go` in the repo and
`docs/guidelines/errors.md`.
