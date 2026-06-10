# 0019 changes

Adds two new packages that future TUI screens (tasks 0021–0029) will sit
on: an `internal/actions` factory that wraps `*app.Service` behind a
uniform single-parameter shape, and an `internal/tui/notifications`
subsystem that owns the in-memory log, FIFO toast queue, notification
area component, and the action → notification bridge helper.

No existing files were modified beyond the task description frontmatter
and the new plan/changelog. The shell (0021) is responsible for
constructing the actions factory and notification stack in
`cmd/af/main.go` and routing `NotificationMsg` to the `Log` + `Toast` —
that wiring is explicitly out of scope here.

## Decisions

- **`Actions` is stateless; `profileRef` rides in each input struct.** —
  **Why:** Matches the 0019 spec ("one factory, threaded through the
  TUI"), avoids a per-screen `ScopedActions` flavor, and mirrors the
  Service-side convention of accepting `profileRef` as the first
  argument to every profile-touching method.
- **Tests use a real `*app.Service` against `t.TempDir()`.** — **Why:**
  Matches `docs/guidelines/testing.md` and every existing
  `internal/app/service_*_test.go`. Introducing a `Service` interface
  and a fake duplicates Service for one consumer; integration-style
  tests prove the forwarding plus the `[]DomainError` collapse without
  the extra surface.
- **`FolderAction` enum lives in `internal/actions`, not `internal/app`.**
  — **Why:** The current Service exposes two separate methods
  (`DeleteProfile` and `DeleteProfileWithFolder`); the actions layer
  owns the user-facing enum and dispatches to the right Service call.
  The zero value is `KeepFolders` so an uninitialized input never
  accidentally destroys data.
- **`SyncProjectInput` carries `Drift` + `Unknown` slices, not a single
  `FileResolution` slice.** — **Why:** The 0019 description text writes
  one slice (`sync.FileResolution`), but the actual sync API (post-0017)
  is two separate slices that the Service translates internally. We
  ship the actual shape. If single-slice consolidation lands later, it
  lives in `internal/sync` + `internal/app` and the action input
  collapses then.
- **Bridge produces `NotificationMsg`; shell routes.** — **Why:** Keeps
  the bridge pure (no `*Log`/`*Toast` references), matches bubbletea
  idiom (commands return messages, components handle them), and yields
  a trivial test surface — assert the message produced, not side
  effects on a mutable target.
- **`From[T]` discards `T`.** — **Why:** Screens that need both the
  value and a notification call the action directly and dispatch two
  messages. A `ResultFrom[T]` variant that emits both is deferred until
  a real consumer needs it (YAGNI).

### Considered but not done

- Introducing a `Service` interface for fake-testing the actions
  package — rejected per the testing decision above.
- Lifting `safe()` and the info/error styles into a shared
  `internal/tui/styles` package now — deferred until 0021 lands and
  becomes the first consumer that would benefit.
- Modifying `cmd/af/main.go` to wire the new packages in — deferred to
  0021 (TUI shell). Out of scope here.

## Assumptions

- **`Service.LoadProfile`'s `ref` parameter accepts id, name, or path.**
  The `LoadProfileInput.ProfileRef` field name reflects this resolver
  semantics. TUI call sites always pass the canonical id.
- **`Service.UpdateAsset` takes `*asset.Manifest`, not `*asset.Asset`.**
  `asset.Asset.Dir` is derived from the loaded profile and is not
  user-editable, so the action input mirrors the Service signature.
- **Test patterns follow the existing `internal/app/service_*_test.go`
  style** (`t.TempDir()`, typed-error assertions via `errors.As`, no
  external mocking framework).

## Other notes

- The `safe()` sanitizer is duplicated between
  `internal/tui/styles.go` and `internal/tui/notifications/render.go`
  to avoid the `tui → notifications → tui` import cycle that task 0021
  introduces. The two copies must be kept in sync until the helper is
  lifted into a shared package (follow-up after 0021 lands).
- `Toast` exposes test-only helpers (`ExpireNowForTest`,
  `StaleExpireForTest`, `Duration`) so the suite can drive expiry
  without waiting on real wall-clock ticks. The helpers are part of
  the public API but their names make their test-only intent
  explicit.

## New: `internal/actions` package

Thin forwarding layer between TUI screens and the application service.
Every method has either zero or exactly one parameter and returns
`(T, errs.DomainError)`, including for side-effect-only actions
(`struct{}`). Service accumulator methods (`LoadProfiles`, `AddProject`)
have their `[]errs.DomainError` return collapsed into `errs.Errors`
through a small generic helper.

```go
// internal/actions/actions.go
type Actions struct{ svc *app.Service }

func New(svc *app.Service) *Actions { /* nil-check + panic */ }

func collapse[T any](v T, es []errs.DomainError) (T, errs.DomainError) {
    if len(es) == 0 {
        return v, nil // nil interface, not typed-nil errs.Errors
    }
    return v, errs.Errors(es)
}
```

Action set (15 methods across 3 files):

- **profiles.go** — `LoadProfiles`, `LoadProfile`, `CreateProfile`,
  `RegisterProfile`, `DeleteProfile`.
- **projects.go** — `RegisterProject`, `LoadProject`, `UpdateProject`,
  `DeleteProject`, `PlanProject`, `SyncProject`.
- **assets.go** — `LoadAsset`, `CreateAsset`, `UpdateAsset`,
  `DeleteAsset`.

24 tests across `{profiles,projects,assets}_test.go` exercise each
action against a real `*app.Service` with `t.TempDir()`, including the
slice-error collapse paths (corrupt profile manifest, unknown asset id
on `AddProject`).

## New: `internal/tui/notifications` package

### Ring buffer (`log.go`)

```go
const LogCap = 500

type Log struct { /* ring buffer + sync.Mutex */ }

func NewLog() *Log
func (l *Log) Add(n Notification)
func (l *Log) Entries() []Notification // newest-first, defensive copy
```

500-entry cap with in-place eviction. Mutex covers the bridge-cmd
goroutine vs. modal-View goroutine concurrency. `Entries()` returns a
fresh slice so callers can sort/filter without locking.

### Toast (`toast.go`)

```go
const DefaultToastDuration = 5 * time.Second

type Toast struct { /* FIFO queue + seq guard */ }

type PushMsg     struct{ Notification Notification }
type expireMsg   struct{ seq uint64 } // private
```

FIFO queue, one visible toast at a time, 5-second default duration
(injectable for tests via `NewToast(d)` — 0 maps to default). The seq
guard means a stale `expireMsg` from a previous front-of-queue toast
that arrives after a new toast has taken its place is a no-op.

### Notification area (`area.go`)

Thin wrapper around `Toast`. Empty queue renders as `""` so the shell
can omit the row entirely.

### Bridge (`bridge.go`)

```go
type NotificationMsg struct{ Notification Notification }

func From[T any](
    action func() (T, errs.DomainError),
    successText string,
) tea.Cmd
```

`From` runs the action, then returns a `tea.Cmd` whose message is a
`NotificationMsg` carrying `LevelInfo` (`successText`) on success or
`LevelError` (`err.Error()`) on failure. The shell routes the message
to `*Log.Add` and `*Toast.Update(PushMsg{...})`. `From` does not hold
references to the log or toast.

### Render (`render.go`)

`iconAndStyleFor(Level)` + `renderNotification(Notification)`. INFO
uses cyan + `ℹ`; ERROR uses red bold + `✗`. Style choices mirror
`internal/tui/render_errors.go::severityStyle`. The two style values
and the `safe()` sanitizer are duplicated locally to avoid the
`tui → notifications → tui` import cycle 0021 introduces.

18 tests across `{log,toast,area,bridge}_test.go` cover cap
enforcement, eviction order, FIFO ordering, seq-stale guard, empty-
queue rendering, default-duration mapping, bridge success/error
shapes, timestamp population, and `errs.Errors` rendering through
`Error()`.

## Out of scope (follow-ups)

- Shell mounting of the actions factory + notification stack into
  `cmd/af/main.go` and the root `tea.Model` (task 0021).
- Notifications Modal that reads `Log.Entries()` (task 0023).
- Wiring actions into existing forms — those forms are deleted by 0021
  and rewritten in 0024–0029.
- `ResultFrom[T]` variant that emits both a typed result and a
  `NotificationMsg`.
- Lifting `safe()` + INFO/ERROR styles into a shared
  `internal/tui/styles` package.
- Persistent event-log storage (revive task 0016 if desired).
- Single-slice `FileResolution` consolidation in `internal/sync`.
