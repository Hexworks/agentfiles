---
id: 0019
type: feature
status: pending
depends_on: 0017, 0018
---

# Actions factory + Notifications subsystem

This task delivers two tightly-coupled subsystems described in
`0015_task_refactor_ui/description.md` — **Actions** and **Notifications**.
They are bundled because actions surface their outcomes via notifications:
shipping one without the other leaves both incomplete.

> See also: task `0016_feature_event-log` (currently a one-line stub). This
> task supersedes it; coordinate before starting — either close 0016 or
> retarget its scope to a persistent event log layered on top of the
> in-memory ring buffer added here.

## Background

The new screens must invoke business operations uniformly and route
success/error feedback into a single in-memory notification stream. Today
there is no central abstraction for either.

## Part 1 — `internal/actions` package

### Shape

```go
package actions

type Actions struct { svc *app.Service }

func New(svc *app.Service) *Actions { return &Actions{svc: svc} }
```

### Strict rules

- Every action returns `(T, errs.DomainError)`, **uniformly**, including for
  side-effect-only actions. For those, `T` is `struct{}`.
- An action has either **no parameter** or **exactly 1 parameter** — strict.
  When the underlying `Service` function needs more than one input, the
  action wraps them in a dedicated input struct (e.g.
  `CreateProfileInput{Name, Path string}`).
- The factory is constructed once in `main.go` (after the Service is built)
  and threaded through the TUI. Individual screens hold the `*Actions`
  reference, not the Service.

### Action set

One method per entry in `0015_task_refactor_ui/description.md` section
**Actions Reference**:

- `LoadProfiles`, `LoadProfile(id)`, `CreateProfile(CreateProfileInput)`,
  `RegisterProfile(path)`, `DeleteProfile(DeleteProfileInput)` — input wraps
  id + `FolderAction`.
- `RegisterProject(project)`, `LoadProject(id)`, `UpdateProject(project)`,
  `DeleteProject(id)`, `PlanProject(id)`,
  `SyncProject(SyncProjectInput)` — input wraps `*Preview` +
  `[]sync.FileResolution`.
- `LoadAsset(id)`, `CreateAsset(asset.Manifest)`, `UpdateAsset(asset.Asset)`,
  `DeleteAsset(id)`.

## Part 2 — `internal/tui/notifications` package

### Ring buffer

```go
type Level string

const (
    LevelInfo  Level = "INFO"
    LevelError Level = "ERROR"
)

type Notification struct {
    Level     Level
    Text      string
    CreatedAt time.Time
}

type Log struct { /* cap=500 ring buffer */ }

func NewLog() *Log
func (l *Log) Add(n Notification)
func (l *Log) Entries() []Notification // newest first
```

Cap = 500. New entries past the cap drop the oldest. In-memory only — no
persistence across runs.

### Toast queue

- FIFO. Only one toast visible at a time.
- Each toast displays for **5 seconds** then is replaced by the next entry.
- The persistent **Notifications Modal** (task 0023) shows every entry
  regardless of queue state.
- Timer driven by a `tea.Tick` cmd; the toast widget exposes a `tea.Model`
  with `Init`/`Update`/`View`.

### Notification area component

The bar rendered under each screen's content. Empty queue → invisible.
Reusable from every screen, mounted by the shell (task 0021).

### Bridge between actions and notifications

A small helper (e.g. `notifications.From(action func() (T, errs.DomainError), successFmt string) tea.Cmd`)
runs the action, then enqueues:
- `LevelInfo` with `successFmt` on success.
- `LevelError` with the `errs.DomainError`'s rendered message on failure.

Screens use this helper rather than calling actions directly so the
INFO/ERROR pattern stays uniform.

## Tests

- `notifications/log_test.go`: cap enforcement, eviction order, newest-first
  iteration.
- `notifications/toast_test.go`: queue order, 5s expiry (use a fake clock or
  inject the tick duration).
- `actions/*_test.go`: at least one action per category exercised with a
  fake `*app.Service` (table-driven), asserting the input struct unpacking.

## Out of scope

- Notifications **Modal** UI (task 0023).
- Wiring into screens (later tasks).
- Persistent event-log file storage (defer to a revived task 0016 if
  decided).

## Verification

```
make build && make test && make lint
```
