# Plan — Task 0019: Actions factory + Notifications subsystem

Links:

- Task description: [./description.md](./description.md)
- UI design source: `tasks/done/0015_task_refactor_ui/description.md` (§Actions, §Notifications)
- Service CRUD baseline: `tasks/current/0018_feature_service-crud-methods/`
- Sync resolutions baseline: `tasks/done/0017_task_sync-unknown-resolutions/`
- Downstream consumers: `tasks/current/0021_feature_tui-shell/`, `tasks/current/0023_feature_system-modals/`
- Guidelines: `docs/guidelines/errors.md`, `docs/guidelines/go.md`, `docs/guidelines/tui.md`, `docs/guidelines/testing.md`, `docs/guidelines/clean_architecture.md`, `docs/guidelines/clean_code.md`, `docs/guidelines/domain_model.md`, `docs/guidelines/solid.md`

## Context

Task 0019 delivers two coupled subsystems that the new TUI screens will sit on:

1. **`internal/actions`** — uniform factory exposing every relevant `*app.Service` op as `(T, errs.DomainError)`, with **at most one parameter** per method (input structs for multi-input cases).
2. **`internal/tui/notifications`** — in-memory 500-entry ring buffer (`Log`), FIFO 5-second toast queue (`Toast`), notification area component (`NotificationArea`), and a small bridge that lets screens fire an action and have its outcome land in both via a single `tea.Cmd`.

Neither is mounted into any UI in this task. Shell (0021) mounts the area + toast; system modals (0023) consume the log; per-screen tasks (0024–0029) consume the actions.

## Branch

`feature/actions-and-notifications` — created from clean master after `git status --porcelain` is empty.

## Verified facts about current code

- `*app.Service` (`internal/app/service.go`) exposes (relevant subset, real signatures):
  ```
  CreateProfile(name, path string) (*registry.ProfileRef, errs.DomainError)
  RegisterProfile(path string) (*registry.ProfileRef, errs.DomainError)
  LoadProfile(ref string) (*profile.Profile, errs.DomainError)
  LoadProfiles() ([]*profile.Profile, []errs.DomainError)
  DeleteProfile(profileRef string) errs.DomainError
  DeleteProfileWithFolder(profileRef string) errs.DomainError
  AddProject(profileRef, name, path string, agents, assetIDs []string) (*project.Manifest, []errs.DomainError)
  LoadProject(profileRef, projectID string) (*project.Manifest, errs.DomainError)
  UpdateProject(profileRef string, p *project.Manifest) errs.DomainError
  DeleteProject(profileRef, projectID string) errs.DomainError
  Plan(profileRef, projectID string) (*llmsync.Preview, errs.DomainError)
  Apply(profileRef, projectID string, []app.DriftResolution, []app.UnknownResolution) (*llmsync.Preview, errs.DomainError)
  InitAsset(profileRef string, manifest asset.Manifest) (string, errs.DomainError)
  LoadAsset(profileRef, assetID string) (*asset.Asset, errs.DomainError)
  UpdateAsset(profileRef string, manifest *asset.Manifest) errs.DomainError
  DeleteAsset(profileRef, assetID string) errs.DomainError
  ```
- `app.DriftResolution{Path, Decision DriftDecision}`, `app.UnknownResolution{Path, Decision UnknownDecision}` already exist (`internal/app/service.go:193-206`). `Service.Apply` internally re-runs `Plan` then calls `sync.Apply(preview, …)`.
- No `app.FolderAction` enum exists. Delete-with-folder is a **separate Service method** (`DeleteProfileWithFolder`). The actions layer owns its own `FolderAction` enum and dispatches to the right Service call.
- `errs.Errors` is itself a `DomainError` (`internal/errs/errs.go`), so collapsing `[]errs.DomainError` from Service accumulator methods is a one-liner.
- `internal/tui/styles.go` defines color indices (cyan=14, red=9) and a `safe()` sanitizer; `internal/tui/render_errors.go` defines `severityStyle(Severity) (icon, style)`.
- Bubbletea v2, lipgloss v2, huh v2 (`charm.land/...`). Module `github.com/hexworks/agentfiles`.

## Architectural decisions

### D1. `Actions` holds `*app.Service` only; `profileRef` rides in each input struct

Per-screen `Actions` would force two flavors and per-screen guesswork. The 0019 spec says "constructed once and threaded through the TUI". So one factory, stateless, and `profileRef` is a field on every input struct that touches a profile. Top-level entry actions (`LoadProfiles`, `CreateProfile`, `RegisterProfile`) don't need one.

### D2. Tests use a real `*app.Service` against `t.TempDir()`

Matches `docs/guidelines/testing.md` and every existing `internal/app/service_*_test.go`. No `Service` interface introduced — the actions package is a thin forwarding layer; integration-style tests prove forwarding + `[]DomainError` collapse without standing up a fake that duplicates Service.

### D3. `FolderAction` lives in `internal/actions`

```go
type FolderAction int
const (
    KeepFolders FolderAction = iota
    DeleteFolders
)
```

`Actions.DeleteProfile(DeleteProfileInput{ProfileRef, FolderAction})` dispatches to `Service.DeleteProfile` or `Service.DeleteProfileWithFolder` based on the enum. The zero value defaults to KeepFolders (safe default).

### D4. `SyncProjectInput` carries two resolution slices, not one

The 0019 description text writes `SyncProjectInput{*Preview, []sync.FileResolution}`. Current code has **no** `sync.FileResolution` type. The actual shape (post-0017) is two separate slices: `[]app.DriftResolution` + `[]app.UnknownResolution`. We ship the actual shape. The `*Preview` field is also dropped because `Service.Apply` re-plans internally (it doesn't take a preview). The 0019 description deviation is documented in [Risks](#risks).

### D5. `UpdateAsset` action takes `*asset.Manifest` (matches Service)

The description writes `UpdateAsset(asset.Asset)`. `asset.Asset = Manifest + Dir` and `Dir` is derived by Service. Action input mirrors Service: `*asset.Manifest`.

### D6. Bridge is message-based; shell routes

`From[T]` returns a `tea.Cmd` whose message is `NotificationMsg{Notification}`. The shell (0021) routes that message to **both** `*Log.Add` and `*Toast.Update(PushMsg{...})`. The bridge does not hold `*Log` or `*Toast` references — keeps the producer pure, matches bubbletea idiom, simplifies tests (assert message produced, not side effects).

### D7. Discarded `T` in `From` is fine for MVP

`From[T]` drops the action's return value — screens that need it call the action directly and dispatch both a typed-result message and a `NotificationMsg`. A future `ResultFrom[T]` (returns both) is deferred until a screen needs it (YAGNI).

### D8. No tui→notifications→tui import cycle: duplicate two styles + `safe`

`internal/tui` imports `internal/tui/notifications` (via 0021). So `notifications` cannot import `internal/tui`. We mirror the two needed styles + `safe()` locally in `internal/tui/notifications/render.go` (~25 LOC). A follow-up after 0021 lifts both into a shared `internal/tui/styles` package; out-of-scope here.

## Package layout — `internal/actions/`

```
internal/actions/
├── actions.go         // Actions + New
├── inputs.go          // every input struct + FolderAction enum
├── profiles.go        // 5 methods
├── projects.go        // 6 methods
├── assets.go          // 4 methods
├── profiles_test.go
├── projects_test.go
└── assets_test.go
```

### `internal/actions/actions.go`

```go
// Package actions exposes every relevant app.Service operation as a
// uniform (T, errs.DomainError) action with at most one parameter.
// The factory is constructed once and threaded through the TUI;
// screens hold *Actions, not *app.Service.
package actions

import "github.com/hexworks/agentfiles/internal/app"

type Actions struct{ svc *app.Service }

func New(svc *app.Service) *Actions {
    if svc == nil {
        panic("actions.New: nil service")
    }
    return &Actions{svc: svc}
}
```

### `internal/actions/inputs.go`

All input structs + `FolderAction` enum (see D3). Fields:

- `CreateProfileInput{Name, Path string}`
- `RegisterProfileInput{Path string}`
- `LoadProfileInput{ProfileRef string}` (resolver semantics: id/name/path; doc on the field)
- `DeleteProfileInput{ProfileRef string; FolderAction FolderAction}`
- `RegisterProjectInput{ProfileRef, Name, Path string; EnabledAgents, AssetIDs []string}`
- `LoadProjectInput{ProfileRef, ProjectID string}`
- `UpdateProjectInput{ProfileRef string; Project *project.Manifest}`
- `DeleteProjectInput{ProfileRef, ProjectID string}`
- `PlanProjectInput{ProfileRef, ProjectID string}`
- `SyncProjectInput{ProfileRef, ProjectID string; Drift []app.DriftResolution; Unknown []app.UnknownResolution}`
- `LoadAssetInput{ProfileRef, AssetID string}`
- `CreateAssetInput{ProfileRef string; Manifest asset.Manifest}`
- `UpdateAssetInput{ProfileRef string; Manifest *asset.Manifest}`
- `DeleteAssetInput{ProfileRef, AssetID string}`

### Action signatures — exact

#### `profiles.go`

```go
LoadProfiles() ([]*profile.Profile, errs.DomainError)            // collapses []errs.DomainError → errs.Errors
LoadProfile(in LoadProfileInput) (*profile.Profile, errs.DomainError)
CreateProfile(in CreateProfileInput) (*registry.ProfileRef, errs.DomainError)
RegisterProfile(in RegisterProfileInput) (*registry.ProfileRef, errs.DomainError)
DeleteProfile(in DeleteProfileInput) (struct{}, errs.DomainError) // dispatches on FolderAction
```

#### `projects.go`

```go
RegisterProject(in RegisterProjectInput) (*project.Manifest, errs.DomainError) // collapses slice
LoadProject(in LoadProjectInput) (*project.Manifest, errs.DomainError)
UpdateProject(in UpdateProjectInput) (struct{}, errs.DomainError)
DeleteProject(in DeleteProjectInput) (struct{}, errs.DomainError)
PlanProject(in PlanProjectInput) (*llmsync.Preview, errs.DomainError)
SyncProject(in SyncProjectInput) (*llmsync.Preview, errs.DomainError)
```

#### `assets.go`

```go
LoadAsset(in LoadAssetInput) (*asset.Asset, errs.DomainError)
CreateAsset(in CreateAssetInput) (string, errs.DomainError)       // returns asset dir from InitAsset
UpdateAsset(in UpdateAssetInput) (struct{}, errs.DomainError)
DeleteAsset(in DeleteAssetInput) (struct{}, errs.DomainError)
```

Slice-error collapse helper (in `actions.go`):
```go
func collapse[T any](v T, es []errs.DomainError) (T, errs.DomainError) {
    if len(es) == 0 { return v, nil }
    return v, errs.Errors(es)
}
```

### Tests — `internal/actions/{profiles,projects,assets}_test.go`

Pattern: one focused test func per behavior (not table-driven — the
per-action setup varies enough that a table would obscure intent); a
shared `fixtures_test.go` helper constructs a fresh `*app.Service` with
`t.TempDir()` plus optional seeded profile/asset/project; typed-error
assertions via `errors.As`.

`profiles_test.go`:
- `TestActions_LoadProfiles_ReturnsAll`
- `TestActions_LoadProfiles_CollapsesErrorsIntoErrsErrors` (corrupt one `profile.json`; `errors.As` against `errs.Errors`)
- `TestActions_LoadProfile_ResolvesByID`
- `TestActions_LoadProfile_MissingReturnsProfileNotFoundError`
- `TestActions_CreateProfile_UnpacksNameAndPath`
- `TestActions_RegisterProfile_UnpacksPath`
- `TestActions_DeleteProfile_KeepFoldersLeavesFolderOnDisk`
- `TestActions_DeleteProfile_DeleteFoldersRemovesFolder`
- `TestActions_DeleteProfile_ReturnsStructZeroOnSuccess`

`projects_test.go`:
- `TestActions_RegisterProject_UnpacksInputs`
- `TestActions_RegisterProject_CollapsesValidationErrors` (unknown asset id → `errs.Errors`)
- `TestActions_LoadProject_ReturnsManifest`
- `TestActions_LoadProject_MissingReturnsProjectNotFoundError`
- `TestActions_UpdateProject_PersistsChanges`
- `TestActions_DeleteProject_RemovesManifestOnly`
- `TestActions_PlanProject_ReturnsPreview`
- `TestActions_SyncProject_AppliesAndReturnsPreview` (empty resolutions)
- `TestActions_SyncProject_PassesDriftAndUnknownResolutions`

`assets_test.go`:
- `TestActions_LoadAsset_ReturnsExisting`
- `TestActions_LoadAsset_MissingReturnsAssetNotFoundError`
- `TestActions_CreateAsset_ReturnsAssetDir`
- `TestActions_CreateAsset_DuplicateReturnsAssetExistsError`
- `TestActions_UpdateAsset_PersistsManifest`
- `TestActions_DeleteAsset_RemovesAssetAndUnselectsFromProjects`

## Package layout — `internal/tui/notifications/`

```
internal/tui/notifications/
├── log.go
├── log_test.go
├── toast.go
├── toast_test.go
├── area.go
├── area_test.go
├── bridge.go
├── bridge_test.go
└── render.go        // local style/icon helpers + safe()
```

### `log.go`

```go
const LogCap = 500

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

type Log struct {
    mu      sync.Mutex
    entries []Notification // ring buffer, len<=LogCap
    next    int
    full    bool
}

func NewLog() *Log
func (l *Log) Add(n Notification)
func (l *Log) Entries() []Notification // newest first, defensive copy
```

Mutex covers the cross-goroutine concurrency (bridge `tea.Cmd` goroutine vs. modal `View`).

### `log_test.go`

- `TestLog_AddBelowCapReturnsNewestFirst`
- `TestLog_AddAtCapDropsOldest`
- `TestLog_AddPastCapEvictionOrder` (push 501; assert `Entries()[500]` is entry #1, `Entries()[0]` is newest)
- `TestLog_EntriesIsCopySafe`
- `TestLog_NewLogEmpty`

### `toast.go`

```go
const DefaultToastDuration = 5 * time.Second

type Toast struct {
    duration time.Duration
    queue    []Notification
    seq      uint64
}

type PushMsg struct{ Notification Notification }
type expireMsg struct{ seq uint64 } // private; carries seq so stale ticks no-op

func NewToast(duration time.Duration) *Toast // 0 → DefaultToastDuration
func (t *Toast) Init() tea.Cmd
func (t *Toast) Update(msg tea.Msg) (*Toast, tea.Cmd) // schedules tea.Tick on first push or chain
func (t *Toast) View() string                          // "" when empty
func (t *Toast) Empty() bool
```

Update logic:
- `PushMsg`: append; if queue was empty, bump `seq`, schedule `tea.Tick(duration) → expireMsg{seq}`.
- `expireMsg{seq}`: ignore if `seq != t.seq` (stale). Else pop front; if queue non-empty, bump `seq`, schedule next tick.

### `toast_test.go`

Tests pass via direct dispatch of `PushMsg` and `expireMsg` (no real wall-clock waits). Use short duration like `10 * time.Millisecond` only where Test specifically asserts a `tea.Tick` is produced.

- `TestToast_QueueOrderIsFIFO`
- `TestToast_FirstPushSchedulesExpire` (asserts `Update(PushMsg{})` returns non-nil cmd)
- `TestToast_SecondPushDoesNotResetTimer`
- `TestToast_StaleExpireIgnored` (push #1, expire fires, push #2, replay stale expire seq-of-#1; #2 still visible)
- `TestToast_EmptyRendersEmptyString`
- `TestToast_DefaultDurationUsedWhenZero`

### `area.go`

```go
type NotificationArea struct{ toast *Toast }

func NewArea(toast *Toast) *NotificationArea
func (a *NotificationArea) Init() tea.Cmd
func (a *NotificationArea) Update(msg tea.Msg) (*NotificationArea, tea.Cmd) // delegates
func (a *NotificationArea) View() string
func (a *NotificationArea) Empty() bool
```

Thin wrapper so the shell treats it as one component; isolates future per-screen padding/width clamp.

### `area_test.go`

- `TestArea_DelegatesPushToToast`
- `TestArea_EmptyAfterExpire`
- `TestNewArea_NilToastPanics`

### `render.go`

```go
// Style/icon vocabulary mirrors internal/tui/render_errors.go::severityStyle.
// Duplicated locally (instead of imported) to avoid the tui→notifications→tui
// import cycle introduced by task 0021. Lift into internal/tui/styles when
// 0021 lands.

var (
    infoStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
    errorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
)

func iconAndStyleFor(l Level) (string, lipgloss.Style) {
    if l == LevelError { return "✗", errorStyle }
    return "ℹ", infoStyle
}

func renderNotification(n Notification) string {
    icon, style := iconAndStyleFor(n.Level)
    return style.Render(icon + " " + safe(n.Text))
}

// safe is a local copy of internal/tui/styles.go::safe.
func safe(s string) string { /* identical body */ }
```

### `bridge.go`

```go
type NotificationMsg struct{ Notification Notification }

// From runs action and returns a tea.Cmd whose message is a
// NotificationMsg. INFO on success (Text=successText), ERROR on failure
// (Text=err.Error()). The T return is discarded; screens needing the
// value invoke the action directly and dispatch both messages.
func From[T any](action func() (T, errs.DomainError), successText string) tea.Cmd {
    return func() tea.Msg {
        _, err := action()
        now := time.Now()
        if err != nil {
            return NotificationMsg{Notification{LevelError, err.Error(), now}}
        }
        return NotificationMsg{Notification{LevelInfo, successText, now}}
    }
}
```

### `bridge_test.go`

- `TestFrom_SuccessProducesInfoNotificationMsg`
- `TestFrom_ErrorProducesErrorNotificationMsg`
- `TestFrom_TimestampPopulated`
- `TestFrom_ErrsErrorsRenderedViaErrorMethod` (action returns `errs.Errors{a,b}`; Text contains both)

## Style integration

INFO → cyan + `ℹ`. ERROR → red bold + `✗`. Mirrors `severityStyle` in `internal/tui/render_errors.go`. Duplication justified by import-cycle concern (D8).

## `cmd/af/main.go` — NOT touched in this task

The shell (0021) is responsible for:
- Calling `actions.New(svc)` after building Service.
- Building `notifications.NewLog()` + `notifications.NewToast(0)` + `notifications.NewArea(toast)`.
- Routing `NotificationMsg` in root `Update`: `log.Add(msg.Notification)` + forward `PushMsg{...}` to area.

This task only ships the building blocks; `make build` still works because the new packages stand-alone and compile under `./...`.

## Step-by-step execution plan

Each code step is paired with its tests; targeted `go test ./...` runs after each. Full `make build && make test && make lint` runs at the end.

1. **Mark task in-progress** — set `status: in-progress` in `tasks/current/0019_*/description.md`, add `## Plan` section linking `./plan.md`. Write the plan file itself (this content) at `tasks/current/0019_*/plan.md`.
2. **Branch** — `git checkout -b feature/actions-and-notifications` (after confirming working tree clean).
3. **actions package skeleton** — `actions.go` (+ `collapse` helper), `inputs.go` (all structs + `FolderAction` enum). Verify `go build ./internal/actions/...`.
4. **profiles.go** + `profiles_test.go` (9 tests). Run targeted tests.
5. **projects.go** + `projects_test.go` (9 tests). Run targeted tests.
6. **assets.go** + `assets_test.go` (6 tests). Run targeted tests.
7. **notifications/log.go** + `log_test.go` (5 tests).
8. **notifications/render.go** (local styles + `safe`). No standalone tests; coverage rides on area/toast View output.
9. **notifications/toast.go** + `toast_test.go` (6 tests).
10. **notifications/area.go** + `area_test.go` (3 tests).
11. **notifications/bridge.go** + `bridge_test.go` (4 tests).
12. **Changelog** — `docs/changelog/2026-06-10_0019-actions-and-notifications.md` (use the skill's changelog template).
13. **Mark task in-review** — `status: in-review` in description frontmatter.
14. **Verification** — `make build && make test && make lint`.

## Verification

```bash
make build
make test
make lint
```

Targeted:
```bash
go test ./internal/actions/...
go test ./internal/tui/notifications/...
```

No UI smoke run — task adds zero UI wiring; `./bin/af` behavior unchanged.

## Files touched

New:
- `internal/actions/{actions,inputs,profiles,projects,assets,profiles_test,projects_test,assets_test}.go`
- `internal/tui/notifications/{log,toast,area,bridge,render,log_test,toast_test,area_test,bridge_test}.go`
- `tasks/current/0019_feature_actions-and-notifications/plan.md`
- `docs/changelog/2026-06-10_0019-actions-and-notifications.md`

Modified:
- `tasks/current/0019_feature_actions-and-notifications/description.md` (status transitions + `## Plan` link)

NOT touched: `internal/app/`, `internal/sync/`, `internal/tui/styles.go`, `internal/tui/render_errors.go`, existing components, `cmd/af/main.go`. No ADR. No new guideline files.

## Out of scope

- Shell mounting (0021)
- Notifications modal body (0023)
- Wiring actions into existing forms — those are deleted by 0021; per-screen rewrites in 0024–0029
- Persistent event-log storage
- `ResultFrom[T]` bridge variant — defer until a screen demands it
- Sync API single-slice consolidation
- Lifting `safe()` + styles into shared `internal/tui/styles` package — follow-up after 0021

## Risks

1. **`SyncProjectInput` deviates from 0019 description text.** Description says one `[]sync.FileResolution` slice; reality (post-0017) has two slices (`app.DriftResolution`, `app.UnknownResolution`). We ship the actual shape. If single-slice consolidation is wanted later, it lives in `internal/sync` + `internal/app`, then the action input collapses.
2. **`UpdateAsset` input is `*Manifest`, not `*Asset`.** Service signature is `*asset.Manifest`; `Dir` is derived. Documented on the input field.
3. **`From` discards T.** Screens that need both the value and a notification call the action directly and dispatch both messages. Follow-up helper deferred.
4. **`safe()` duplication.** Same code in `internal/tui/notifications/render.go` and `internal/tui/styles.go`. Drift risk. Mitigated by the changelog entry calling it out as a temporary state and the follow-up to extract.
5. **`LoadProfile` resolver semantics.** `Service.LoadProfile(ref)` accepts id/name/path; we name the input field `ProfileRef` to reflect this rather than `ID`.
6. **`From` timestamp captured at `tea.Cmd` execution time**, not at construction. Document on the function.

## ADRs / guidelines

No ADR — strictly additive, follows existing patterns. No new guideline files.

## Critical files for implementation

- `/home/addamsson/projects/agentfiles/internal/actions/actions.go`
- `/home/addamsson/projects/agentfiles/internal/actions/inputs.go`
- `/home/addamsson/projects/agentfiles/internal/tui/notifications/log.go`
- `/home/addamsson/projects/agentfiles/internal/tui/notifications/toast.go`
- `/home/addamsson/projects/agentfiles/internal/tui/notifications/bridge.go`

Reused (read-only, for reference):
- `/home/addamsson/projects/agentfiles/internal/app/service.go`
- `/home/addamsson/projects/agentfiles/internal/errs/errs.go`
- `/home/addamsson/projects/agentfiles/internal/tui/render_errors.go`
- `/home/addamsson/projects/agentfiles/internal/tui/styles.go`
- `/home/addamsson/projects/agentfiles/internal/tui/components/modal/modal.go` (Init/Update/View pattern reference)
