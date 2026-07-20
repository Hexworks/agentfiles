# 0037 changes

Adds the reusable **path selector modal** at
`internal/tui/modals/pathselector`, opened by callers through the thin
`modals.NewSelectPath(opts) (*modal.Modal, errs.DomainError)` wrapper
one directory up. The modal lets a caller ask the user to pick a folder
or file inside a **constraint root**, enforcing that navigation cannot
escape it and — when `Options.FollowSymlinks` is enabled — that
symlink targets are `filepath.EvalSymlinks`-resolved and refused if
they resolve outside the constraint. The selection is delivered as a
typed `pathselector.Result{Path, IsDir}` through `modal.ResolvedMsg`
and extracted with `pathselector.ResultFromMsg`.

Screen-level mnemonic buttons under the tree cover `[Select current]`
(mnemonic `c`), `[Show hidden] / [Hide hidden]` (mnemonic `h`), and
`[Cancel]` (mnemonic `n`, with `esc` as an extra binding). A per-row
`[Select]` (mnemonic `s`) sits on the treetable's actions column,
suppressed on the header row and on the `<empty>` placeholder. Reads
are synchronous — `os.ReadDir` errors surface through an error
`notifications.NotificationMsg` and leave the modal on the previous
folder; the modal never resolves on an I/O error.

Every acceptance criterion from `tasks/current/0037_.../description.md`
maps to a named test in the package (see `## Verification` below).

## Decisions

- **`FollowSymlinks bool` kept as spelled in the task description.** —
  **Why:** the task's "true (default)" wording cannot be encoded in a
  plain `bool` (zero value is `false`). Renaming to
  `NoFollowSymlinks bool` or promoting to `*bool` diverges from the
  spec; documenting the zero-value on the field is the least
  surprising Go idiom and keeps callers in control.
- **`[Cancel]` mnemonic = `n`.** — **Why:** `s`, `c`, and `h` are
  already claimed by `[Select]`, `[Select current]`, and
  `[Show hidden]`. `n` is the most readable choice from `ca[N]cel`,
  and the button also carries `esc` as an extra key so raw `Esc` and
  the mnemonic resolve identically.
- **Errors are struct types, not `Err…` sentinels.** — **Why:** every
  other `errors.go` in this repo (`app`, `asset`, `render`, …)
  defines structs with `Error()` + `Severity()` and callers use
  `errors.As`. `docs/guidelines/errors.md` calls this out as the
  house style. The task's `Err…` shorthand was documentation prose,
  not a promise about the code shape.
- **Root row is a header, not `..`.** — **Why:** treetable requires a
  non-empty root label and does not expose `SetCursor`, so the root
  will always render at index 0. Using it as a "you are here" header
  keeps the enumerated cursor states (`..`, folder, file, empty
  folder, folders-only) matching the task description verbatim and
  lets the header row skip both the per-row `[Select]` button and any
  Enter behavior, avoiding a fifth mnemonic case in
  `TestMnemonicUniqueness`.

Considered but not adopted:

- **Sentinel `var Err… = &…Error{}` alongside the struct types.**
  Adds surface without a caller need — no code in-tree uses
  `errors.Is` on the modal's construction errors — and blurs the
  house style (`errors.As` on struct types).
- **A new ADR for the constraint / symlink policy.** The mechanics
  (`filepath.Abs` → `EvalSymlinks` → `filepath.Rel` → `..` check)
  already have precedent in `internal/asset/files.go`
  (`ResolveRelative`) and `internal/app/service.go`
  (`isUnderRoot`/`isAncestor`), and `docs/guidelines/security.md`
  spells out the rule. Adding an ADR would restate existing
  conventions.
- **A dedicated `pathselector/testhelpers_test.go` file.** The
  behavioural helpers (`mustNew`, `moveCursorTo`, `keyPress`,
  `drainNotifications`, `assertConfirmed`) fit next to the tests that
  use them in `pathselector_test.go`; splitting them out would ship
  ceremony without value.

## Assumptions

- **Not-following-symlinks means "Enter is a silent no-op on directory
  symlinks", never "hide them from the listing".** — **Why:** the
  task doc says "symlinks are shown but Enter on a symlink is a
  no-op" for `FollowSymlinks=false`. The classifier still surfaces
  symlink rows with the `@` suffix; only the navigation handler
  short-circuits.
- **The header row is a valid cursor position but never selectable.**
  — **Why:** treetable's `Cursor()` starts at 0 and no API moves it
  off the root without a user key press. The `s` press on the header
  is a no-op (guarded by `entry.isSelectable()`), so the user still
  has to move down to make a per-row selection or use
  `[Select current]` to pick the browsed folder.
- **`Constraint == ""` means "unconstrained" everywhere in the
  package.** — **Why:** the task says the empty string opts out of the
  constraint. `safety.isUnderRoot` returns `true` for `root == ""`,
  `buildEntries` prepends `..` whenever `dir != opts.constraint` (and
  `""` never equals a real dir path), and `navigate` skips the
  containment check when the constraint is empty.

## Other Notes

- **New package**: `internal/tui/modals/pathselector` — see
  `docs/architecture/05-building-block-view.md` (Level 2 additions)
  for the responsibility description.
- **New wrapper**: `internal/tui/modals/select_path.go` matches the
  `create_*.go` filename convention already in that directory, so
  screen code can invoke the modal with a one-liner.
- **New glossary entries**: `Constraint Root` and
  `Path Selector Modal` in `docs/glossary.md`.
- **CLAUDE.md**: package-layout table gains a bullet for the new
  package.
- **No ADR added** and **no new guideline file** — see the "Decisions"
  section for the rationale.
- **Reuses** the same safety idiom (`EvalSymlinks` + `filepath.Rel` +
  `..` check) as `internal/asset/files.go` and
  `internal/app/service.go`. The helpers are duplicated (not
  imported) so the modal stays inside the TUI-layer dependency
  direction called out in `docs/guidelines/clean_architecture.md`.
- **Out of scope (per task)**: wiring the modal into any existing
  screen (register project, register asset, edit profile, …). Each
  integration is a follow-up task.

## Verification

- `make build && make test && make lint` clean on
  `feature/path-selector-modal`.
- `go test ./internal/tui/modals/pathselector/...` — 48 tests pass;
  every acceptance criterion in `description.md` maps to a named test:
  `TestConstraintRootUpwardNoop`, `TestSymlinkEscapeRejected`,
  `TestSymlinkNoFollow`, `TestConstructorValidatesInputs`,
  `TestListingSortOrder`, `TestFoldersOnlyMode`,
  `TestExtensionFilterCaseInsensitive`, `TestHiddenToggle`,
  `TestEnterSemantics`, `TestRowSelectResolvesResult`,
  `TestSelectCurrentDir`, `TestReadDirDeniedNotifies`,
  `TestCancelPaths`, `TestMnemonicUniqueness`, `TestResultFromMsg`.
- Security-focused subset from the task's `## Verification`:

  ```
  go test ./internal/tui/modals/pathselector \
      -run 'TestConstraintRootUpwardNoop|TestSymlinkEscapeRejected|TestSymlinkNoFollow|TestConstructorValidatesInputs'
  ```

  green (10 tests).
