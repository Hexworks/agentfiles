# Plan — Task 0023: System Modals (Notifications + Info)

Cross-links: [description.md](./description.md)

## Context

Task 0021 shipped the TUI shell with global key bindings (`n` → notifications,
`?` → help) wired to **no-op stubs** in `internal/tui/shell/{notifications,info}_stub.go`.
Task 0019 shipped the in-memory `notifications.Log` (500-entry ring buffer,
`Entries()` returns newest-first). Task 0023 replaces both stubs with real
modal experiences:

1. A **Notifications Modal** rendered as a `bubbles/table` over the log entries.
2. The existing **Help/Info modal** (`internal/tui/components/help/help.go`,
   markdown viewport) wired to the global `?` key with `docs/manual/overview.md`
   as the default topic.

The notifications log already flows into the shell, the help modal is already
implemented; this task is mostly construction of one new component
(notifications modal) plus shell wiring + a starter manual page + tests.

## File map

### New

| Path | Purpose |
|------|---------|
| `internal/tui/components/notifications/notifications.go` | `modal.Content` impl backed by `bubbles/table`. Constructor `New(id string, log *notlog.Log, width, height int) *modal.Modal`. |
| `internal/tui/components/notifications/notifications_test.go` | Table-driven tests: newest-first ordering, empty log message, esc/q close. |
| `internal/tui/shell/notifications.go` | Replaces `notifications_stub.go`. Screen that hosts `*modal.Modal` for notifications. |
| `internal/tui/shell/info.go` | Replaces `info_stub.go`. Screen that hosts `*modal.Modal` from `help.New(...)` using the topic registry. |
| `internal/tui/shell/topics.go` | Small topic→manual-path registry. MVP: a single global default → `docs/manual/overview.md`. |
| `docs/manual/overview.md` | Short intro (1 paragraph) + global key list (`n`, `s`, `?`, `q`). |
| `docs/changelog/2026-06-11_0023-system-modals.md` | Changelog entry. |

### Removed (replaced)

- `internal/tui/shell/notifications_stub.go`
- `internal/tui/shell/info_stub.go`

### Edited

| Path | Change |
|------|--------|
| `internal/tui/shell/keys.go` | `handleGlobalKey` returns the real Screen constructors instead of stubs. |
| `internal/tui/shell/stubs_test.go` | Remove the now-replaced stubs; add coverage for the new screens. Either rename to `screens_test.go` or split per-screen tests next to each new file. |
| `internal/tui/shell/shell.go` | Inject topic registry (if not held in `topics.go` as a package-level helper). No public API break expected. |

## Architecture choices

### A. Notifications modal lives in its own component package

Follow the `internal/tui/components/help/` pattern: dedicated package, exports a
`New(...) *modal.Modal` constructor that mounts a typed `modal.Content`.
Package name **`notifications`** matches user-facing terminology. Import in
shell uses an alias to avoid clash with the existing `internal/tui/notifications`
log package, e.g. `notlog "github.com/hexworks/agentfiles/internal/tui/notifications"`
in shell + new modal code.

### B. Shell mounts modals via Screen adapters, not native overlay

The shell currently has only a Screen stack (`tui/shell/screen.go`). Adding a
native modal-overlay layer is out of scope for this task. Instead, each new
shell file is a thin Screen that:

- Holds a `*modal.Modal`.
- Forwards `Update` to it.
- Renders `modal.View()` inside `Body(w, h)`.
- Pops itself when `modal.Lifecycle()` returns `Cancelled`/`Confirmed`.

Matches how `notifications_stub.go` already lived — the stub just exits on `esc`.
This keeps task 0023 strictly additive to the shell's contract; native overlays
remain a future refactor (track via a new ADR if/when shipped, not in this task).

### C. Topic registry

`topics.go` exposes:

```go
const ManualOverview = "docs/manual/overview.md"

// topicFor returns the manual file for the focus screen. Default applies
// when no per-screen override is registered.
func topicFor(_ Screen) string { return ManualOverview }
```

MVP: ignore the focus screen; always return `overview.md`. Per-screen overrides
land when screens land (task description "Out of scope" clause).

### D. Reuse existing typed errors / styles

- `internal/tui/styles/styles.go` exposes `SeverityStyle(errs.Severity)` →
  reuse for table-row level coloring (INFO neutral cyan, WARNING yellow,
  ERROR red+bold). Time formatted via stdlib `time.Format("15:04:05")`.
- `internal/tui/components/modal/modal.go` exposes the `Content` /
  `Lifecycle` contract — implement directly, no custom modal frame.
- `internal/tui/components/help/help.go` already typed errors; no change.

## Step-by-step execution

1. **Component package** — create
   `internal/tui/components/notifications/notifications.go`:
   - `type content struct { id string; table table.Model; empty bool; closed lifecycleState; resolved bool }`
   - Implements `modal.Content`: `Init`, `Update`, `View`, `Lifecycle()`.
   - Constructor snapshots `log.Entries()` on open into a `[]table.Row`
     with columns `level | content | time`. Level rendered with the styles
     helper; time rendered `HH:MM:SS`.
   - Empty log → render `"No notifications yet"` body instead of a table.
   - `esc` / `q` set state to `Cancelled` (consistent with confirm/help).
   - `New(id, log, width, height)` constructs the content and wraps with
     `modal.New(content, modal.WithStyle(styles.ModalStyle))`.

2. **Tests for component**
   (`internal/tui/components/notifications/notifications_test.go`):
   - Fixed log with 3 mixed-level entries (INFO/WARN/ERROR, distinct timestamps);
     assert `View()` output orders newest-first and contains each text +
     `HH:MM:SS`.
   - Empty log → `View()` contains `"No notifications yet"`.
   - `esc` / `q` → `Lifecycle()` reports `Cancelled`.
   - Non-close key (e.g. `down`) forwards to the table without closing.

3. **Shell — info screen**
   (`internal/tui/shell/info.go`): wraps `*modal.Modal` returned by
   `help.New("info", help.Request{Path: topicFor(focus)}, w, h)`. Pops on
   `Lifecycle() != Active`.

4. **Shell — notifications screen**
   (`internal/tui/shell/notifications.go`): wraps `*modal.Modal` returned by
   `notmodal.New("notifications", m.log, w, h)`. Pops on lifecycle exit.

5. **Shell — topics registry** (`internal/tui/shell/topics.go`).

6. **Shell — key map updates** (`keys.go`): swap `newNotificationsStub`
   for `m.newNotificationsScreen()`, `newInfoStub` for `m.newInfoScreen()`.
   The constructors need access to `m.log` + the focused screen for topic
   lookup, so make them methods on `Model`.

7. **Delete** `notifications_stub.go`, `info_stub.go`.

8. **Shell tests** — update `stubs_test.go`:
   - Keep coverage for `welcomeStub` / `settingsStub` (still present).
   - Add tests for new info + notifications screens: esc pops; modal mount
     is non-nil; the global key path produces the expected screen type.

9. **Manual page** — create `docs/manual/overview.md`:

   ```md
   # agentfiles — Overview

   `af` is a TUI for managing AI-assistant profiles and projecting them into
   target repositories. Press `?` from any screen for help.

   ## Global keys

   - `n` — Notifications
   - `s` — Settings
   - `?` — Help
   - `q` / `ctrl+c` — Quit
   ```

10. **Quality gate** — `make build && make test && make lint`. Manually
    run `./bin/af` and verify `n` opens the notifications modal and `?`
    opens the info modal.

11. **Changelog** — write
    `docs/changelog/2026-06-11_0023-system-modals.md` from the skill's
    template.

12. **Frontmatter** — flip `description.md` `status` from `pending` →
    `in-progress` (step 9 of the skill) → `in-review` (step 13).

## ADR / docs touches

- **No new ADR.** The shell's modal hosting choice (Screen adapter) is a
  task-scoped tactical decision, not a durable architectural commitment.
  If the team later wants native overlay support, that change deserves its
  own ADR.
- **No guideline updates.** The implementation follows existing patterns
  in `components/help/` and the modal package; nothing new to codify.
- **Building-block view (`docs/architecture/`):** no edit needed — the
  notifications-modal node already exists in the diagram as part of the
  TUI subsystem.

## Verification

```bash
make build && make test && make lint
./bin/af
# 1. Press `n` — notifications modal opens; if log is empty, body reads
#    "No notifications yet". Press `esc` — closes.
# 2. Trigger any action that emits a Notification (e.g. an invalid input
#    on the existing welcome screen — none yet → seed by re-pressing `n`
#    is fine for visual). Press `n` again — entries listed newest first.
# 3. Press `?` — info modal opens with rendered overview.md. Press `q`/`esc`
#    — closes.
```

Unit tests cover the empty/non-empty rendering paths, the close key bindings,
and ordering. The shell tests cover the global-key wiring.

## Risk / open items

- **Package name collision** for `notifications`. Mitigation: alias the log
  package as `notlog` in any file importing both (only shell + the new
  component file, since the component depends on the log).
- **Table sizing** under small terminals. Mitigation: let `bubbles/table`
  manage scrolling; clamp column widths via `lipgloss` helpers already used
  in `help.go` (no custom width math).
- **Focused-screen topic mapping** beyond the global default is explicitly
  out of scope (task description). Keep `topicFor` ignoring its argument
  for now.
