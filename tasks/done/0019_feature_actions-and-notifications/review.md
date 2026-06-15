# Actions + Notifications review

Multi-agent review of task `0019` (commit `ac91ac3`, branch already merged to
`master`). Build, lint, and `go test ./...` all pass on the current tree. The
work is well-scoped: a thin `internal/actions` forwarding layer plus a small
`internal/tui/notifications` subsystem, with no incursions into existing
packages.

Security review: no findings. The bridge stringifies errors but rendering
happens behind the existing `safe()` sanitizer, so no terminal-escape vector
opens. The `FolderAction` enum defaults to `KeepFolders`, and the destructive
variant routes through `Service.DeleteProfileWithFolder` (already gated by
`isUnsafeProfilePath`). No bypass.

The two largest issues are domain-vocabulary forks: the bridge collapses
typed `errs.DomainError` (with its three-way `Severity`) into a binary
`Level` + free-form string at the message boundary, so downstream
consumers — including the notifications modal in task 0023 — can no longer
introspect the error. Second, three test-only methods leak onto the
production `*Toast` API.

The remaining findings are tighter: a duplicated `safe()`/styles block
(self-acknowledged in the plan as 0021-blocked), a no-op `NotificationArea`
wrapper, hand-rolled `itoa`/`containsText` instead of stdlib, a few
vocabulary deviations (`RegisterProject` vs Service's `AddProject`,
`FolderAction` outside the glossary), redundant doc comments, an unused
variable in one stale-expire test, a few testing hygiene items, and a
plan/impl divergence (plan claimed "table-driven", tests are
sequential).

## Bridge collapses typed DomainError + Severity into binary Level + string

> [!WARNING]
>
> - [docs/guidelines/domain_model.md](../../../docs/guidelines/domain_model.md) — _Use The Project Language_: "let UI labels, docs, and code drift into different meanings".
> - [docs/glossary.md → Severity](../../../docs/glossary.md) — "Severity is part of the domain (not presentation)… Every typed domain error implements `Severity() errs.Severity`; the TUI consumes the result to pick icon and color."
> - [docs/glossary.md → Typed Domain Error](../../../docs/glossary.md) — typed leaves must reach the TUI.

The notifications subsystem invents a parallel severity vocabulary
(`Level` / `LevelInfo` / `LevelError`) that is a strict subset of the
canonical `errs.Severity` (`SeverityInfo` / `SeverityWarning` /
`SeverityError`). The bridge collapses every error — including
`SeverityWarning` — into `LevelError`, then immediately stringifies the
underlying typed error via `err.Error()`. The typed leaves are lost at the
edge: `Log.Entries()` and the upcoming notifications modal (task 0023)
have no way to call `errors.As`, query `Severity()`, or re-render via
`tui.RenderError` to get the warning icon/color the rest of the TUI uses.

```go
// internal/tui/notifications/log.go:11-19
type Level string

const (
    LevelInfo  Level = "INFO"
    LevelError Level = "ERROR"  // warnings get folded into ERROR
)

// internal/tui/notifications/bridge.go:30-43
if err != nil {
    return NotificationMsg{Notification: Notification{
        Level:     LevelError,   // any errs.Severity → LevelError
        Text:      err.Error(),  // typed leaf flattened to a string
        CreatedAt: now,
    }}
}
```

This is the choke point where the typed-error discipline established by
`docs/guidelines/errors.md` gets thrown away. Once it crosses the bridge,
nothing downstream can recover it.

## Solution

- [x] **Replace `Level` with `errs.Severity` end-to-end**: store
      `Severity errs.Severity` on `Notification`, drop the `Level` type,
      route `iconAndStyleFor` through the same severity switch
      `tui.severityStyle` uses. Bridge maps success → `SeverityInfo`,
      error → `err.Severity()`. Single severity vocabulary across the
      whole codebase.
- [ ] **Keep `Level` but mirror all three severities**: add
      `LevelWarning`, map the bridge through `err.Severity()`. Two
      parallel vocabularies remain (worse than option 1) but at least
      they no longer lose information.
- [ ] **Carry the typed error**: add `Err errs.DomainError` on
      `Notification` (alongside or instead of `Text`), and render
      through the existing `tui.RenderError` path so severity icon and
      color match the rest of the TUI. Toast/log/modal can all
      introspect.
- [ ] Accept the lossy projection; add `Level` and the mapping rule to
      `docs/glossary.md` and document that notifications are deliberately
      lossy so future consumers know not to introspect.

## Test-only methods exposed on production `*Toast` API

> [!WARNING]
>
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) — _Data And Objects_: "Hide internal structure when callers should not rely on it"; _Naming_: "Avoid encoding type or scope into names unless it is an established Go convention".
> - [docs/guidelines/clean_architecture.md](../../../docs/guidelines/clean_architecture.md) — abstractions should not exist solely to support tests.
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md) — internal access belongs in `package X` internal tests or `export_test.go`, not on the production type.

Three methods on `*Toast` exist solely for the test suite:
`ExpireNowForTest`, `StaleExpireForTest`, and `Duration`. The `ForTest`
suffix makes the intent unambiguous, but they are part of the public
package surface — every IDE autocomplete on `*Toast` (including the shell
in task 0021) lists them. Production code can call them and produce
behavior no test exercises.

```go
// internal/tui/notifications/toast.go:46-48
func (t *Toast) Duration() time.Duration { return t.duration }

// internal/tui/notifications/toast.go:103-115
func (t *Toast) ExpireNowForTest() tea.Msg {
    return expireMsg{seq: t.seq}
}

func (t *Toast) StaleExpireForTest() tea.Msg {
    return expireMsg{seq: t.seq - 1}
}
```

The root cause is that the test files live in `package notifications_test`,
which forces every helper they need to be exported.

## Solution

- [x] **`export_test.go` pattern (stdlib idiom)**: rename the methods to
      unexported (`expireNow`, `staleExpire`), add
      `internal/tui/notifications/export_test.go` containing
      `var ExpireNow = (*Toast).expireNow` (etc.) so tests in
      `notifications_test` still reach them. Production surface shrinks.
- [ ] **Switch tests to `package notifications`** (white-box) and access
      `expireMsg`/`seq`/`duration` directly. Helpers disappear entirely.
- [ ] **Inject the clock**: replace the `tea.Tick`/`time.Now()`
      coupling with a `func(time.Duration) tea.Cmd` field on `Toast`
      (default `tea.Tick`, override in tests). Tests stop forging
      `expireMsg` values at all, and the production type loses the
      "ForTest" baggage.
- [ ] Keep as-is and document the exposure as deliberate — least
      preferred; the leak persists.

## `NotificationArea` is a no-op wrapper around `Toast`

> [!WARNING]
>
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) — _Code Smells_: "Needless complexity: abstractions, options, or layers that do not pay for themselves".
> - [docs/guidelines/solid.md](../../../docs/guidelines/solid.md) — soft-LSP note: `NotificationArea.Update` returns `(*NotificationArea, tea.Cmd)`, not `(tea.Model, tea.Cmd)`, so the wrapper isn't substitutable for the bubbletea `tea.Model` contract.

Every `NotificationArea` method delegates straight through to `*Toast`
with no behavior added. The doc comment promises "a single home for
per-screen padding/width concerns," but no padding, width, or per-screen
state exists. The extra constructor + nil-panic + four delegate methods
buy nothing today, and there is no consumer outside the area's own
test file.

```go
// internal/tui/notifications/area.go:27-31
func (a *NotificationArea) Update(msg tea.Msg) (*NotificationArea, tea.Cmd) {
    var cmd tea.Cmd
    a.toast, cmd = a.toast.Update(msg)
    return a, cmd
}
```

Bonus naming asymmetry: the type is `NotificationArea` but the constructor
is `NewArea`. Go convention is `New<TypeName>`; mounting reads
`notifications.NewArea(...) *notifications.NotificationArea`.

## Solution

- [x] **Delete `NotificationArea`** and its tests; have the shell mount
      `*Toast` directly. Reintroduce a wrapper if and when an actual
      padding/width concern arrives.
- [ ] **Keep the type but rename to `Area`** (so it reads
      `notifications.NewArea() *notifications.Area`, matching
      `notifications.NewToast() *notifications.Toast`). Defer the
      delete until 0021 proves the wrapper still adds nothing.
- [ ] **Give the wrapper real behavior now** (e.g. empty-row
      suppression, fixed-width clamp) so the abstraction earns its
      keep before the shell starts depending on it.

## `safe()` + INFO/ERROR styles duplicated from `internal/tui`

> [!WARNING]
>
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) — _Code Smells_: "Needless repetition: duplicated rules that can drift apart".
> - [docs/guidelines/clean_architecture.md](../../../docs/guidelines/clean_architecture.md) — copied code drifts; pre-emptive duplication for a not-yet-existing cycle is debt today.

`internal/tui/notifications/render.go:35-68` ships a byte-identical copy
of `safe`/`isTerminalSafeRune` from `internal/tui/styles.go:58-99`, plus a
parallel `infoStyle`/`errorStyle` pair using raw `lipgloss.Color("14")` and
`lipgloss.Color("9")` instead of the existing `colorCyan`/`colorRed`
constants. The justification (`render.go:10-15`) names the
`tui → notifications → tui` cycle that **task 0021 will introduce** when
the shell mounts the area — the cycle does **not** exist on `master`
today.

```go
// internal/tui/notifications/render.go:17-19
infoStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
errorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)

// internal/tui/notifications/render.go:35-45 — copy of internal/tui/styles.go:58-68
func safe(s string) string {
    if s == "" { return s }
    for _, r := range s {
        if !isTerminalSafeRune(r) {
            return strconv.Quote(s)
        }
    }
    return s
}
```

The plan (D8) and changelog both acknowledge this as deferred debt slated
for cleanup "after 0021 lands." But the canonical fix — extract a
leaf-package — works equally well today and would prevent the cycle from
ever appearing.

## Solution

- [x] **Extract now (preferred)**: lift `safe`, `isTerminalSafeRune`,
      and the INFO/ERROR style+icon vocabulary into a new
      `internal/tui/styles` package (or `internal/safe`). Have both
      `internal/tui` and `internal/tui/notifications` import it. Cycle
      never materializes; no copy ever drifts. Deletes the four
      duplicated declarations and the deferred-follow-up TODO.
- [ ] **Keep the duplication, add a sync test**: leave both copies but
      add `styles_sync_test.go` that asserts byte-equality between the
      two `safe` implementations so the next aesthetic edit fails loudly.
- [ ] **Defer with a tracked follow-up**: keep as-is, document the
      cleanup as a new backlog task (rather than a free-text reference
      to "after task 0021") so it can't fall off the radar.

## `itoa` reimplements `strconv.Itoa`

> [!WARNING]
>
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) — _General Rules_: "Prefer the simplest design that solves the current problem well"; _Tests_: "Make tests readable: clear setup, one action, visible assertions".
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md) — keep test code boring.

`internal/tui/notifications/log_test.go:18-39` reimplements integer
formatting with a comment claiming it "avoids strconv import just for
tiny test labels." `strconv` is part of the Go stdlib — there is no
dependency cost. Twenty-two lines of reviewable surface for zero
benefit.

```go
// internal/tui/notifications/log_test.go:18-39
// itoa avoids strconv import just for tiny test labels.
func itoa(i int) string {
    if i == 0 {
        return "0"
    }
    var buf [20]byte
    pos := len(buf)
    neg := i < 0
    // ...
}
```

## Solution

- [x] Replace every `itoa(i)` with `strconv.Itoa(i)` and delete the
      helper.

## `containsText` reimplements `strings.Contains`

> [!WARNING]
>
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) — _Code Smells_: "Needless repetition" / "Needless complexity".
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md) — keep test code boring; assert on state, not on rendered text where reasonable.

`internal/tui/notifications/toast_test.go:101-108` reimplements
`strings.Contains` with a manual byte-slice loop. `bridge_test.go` in
the same package already imports `strings`, so even the (rejected)
"avoid the import" defense from `itoa` does not apply. The byte-slicing
also creates a latent fragility — the rendered output prepends the
multi-byte glyph `ℹ`, so any future non-ASCII needle would slice
mid-rune.

```go
// internal/tui/notifications/toast_test.go:101-108
func containsText(rendered, needle string) bool {
    for i := 0; i+len(needle) <= len(rendered); i++ {
        if rendered[i:i+len(needle)] == needle {
            return true
        }
    }
    return false
}
```

## Solution

- [x] Replace with `strings.Contains(toast.View(), "first")`; delete
      `containsText`.
- [ ] Go further: assert against `t.queue[0]` (white-box internal test)
      or a `Front() Notification` accessor instead of the rendered
      string. Couples the test to state, not to rendering.

## `FolderAction` enum dispatch in `DeleteProfile`

> [!WARNING]
>
> - [docs/guidelines/solid.md](../../../docs/guidelines/solid.md) — Open/Closed: adding a third variant means editing both the enum and the dispatch.
> - [docs/guidelines/domain_model.md](../../../docs/guidelines/domain_model.md) — _Use The Project Language_: "update the glossary when a new durable domain term appears"; _Put Domain Rules In Domain Code_.

`Actions.DeleteProfile` introduces an enum (`FolderAction { KeepFolders,
DeleteFolders }`) that fans out into the two existing Service methods
(`DeleteProfile` vs `DeleteProfileWithFolder`). The enum encodes a
destruction-policy decision (a domain rule about what "delete" means)
and exists only in the actions layer — not in the glossary, not in
`internal/app`, not in any ADR. The Service already exposes the
intention via two named methods; the actions layer is re-collapsing
them.

```go
// internal/actions/inputs.go:12-21
type FolderAction int

const (
    KeepFolders FolderAction = iota
    DeleteFolders
)

// internal/actions/profiles.go:35-43
func (a *Actions) DeleteProfile(in DeleteProfileInput) (struct{}, errs.DomainError) {
    var err errs.DomainError
    if in.FolderAction == DeleteFolders {
        err = a.svc.DeleteProfileWithFolder(in.ProfileRef)
    } else {
        err = a.svc.DeleteProfile(in.ProfileRef)
    }
    return struct{}{}, err
}
```

## Solution

- [x] **Split into two actions**: drop the enum, expose
      `Actions.DeleteProfile(DeleteProfileInput)` and
      `Actions.DeleteProfileWithFolder(DeleteProfileInput)` mirroring
      the Service. Caller decides at the call site based on the modal
      outcome. Removes a domain term that exists in only one layer.
- [ ] **Promote `FolderAction` to `internal/app`** (next to the two
      delete methods) and refactor Service to take a single
      `DeleteProfile(ref string, action FolderAction)`. Action layer
      becomes a one-line forward. Add `FolderAction` to
      `docs/glossary.md`.
- [ ] Keep current shape but add `FolderAction` / `KeepFolders` /
      `DeleteFolders` to `docs/glossary.md` so the term has a single
      canonical definition.

## `RegisterProject` action diverges from Service `AddProject`

> [!WARNING]
>
> - [docs/guidelines/domain_model.md](../../../docs/guidelines/domain_model.md) — _Use The Project Language_: "let UI labels, docs, and code drift into different meanings".

The glossary uses "register" for the registry-side operation that adds a
profile to `~/.agentprofiles.json`. Projects are _added_ to a profile
(per `Service.AddProject` and `docs/glossary.md → Project`). Renaming
the verb to `RegisterProject` in the action layer conflates two
different concepts — registry registration vs. per-profile project
creation — under the same verb.

```go
// internal/actions/projects.go:11-14
func (a *Actions) RegisterProject(in RegisterProjectInput) (*project.Manifest, errs.DomainError) {
    manifest, es := a.svc.AddProject(in.ProfileRef, in.Name, in.Path, in.EnabledAgents, in.AssetIDs)
    return collapse(manifest, es)
}
```

## Solution

- [x] Rename `Actions.RegisterProject` → `Actions.AddProject` (and
      `RegisterProjectInput` → `AddProjectInput`) so the action layer
      tracks Service vocabulary. One-line change, eliminates the fork.
- [ ] Rename `Service.AddProject` → `Service.RegisterProject` for the
      mirror direction; would need a glossary entry that distinguishes
      project-registration from registry-registration. Higher blast
      radius.

## Redundant comments restating the type name

> [!WARNING]
>
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) — _Comments_: "Do not repeat what the next line of code already says".
> - CLAUDE.md: "Default to writing no comments. Only add one when the WHY is non-obvious".

Eleven near-identical doc comments in `internal/actions/inputs.go` of
the form `// XInput is the input for Actions.Y.`, plus matching
restated comments on every wrapper method in `profiles.go` / `projects.go`
/ `assets.go`. The type name and method name already convey this. The
genuinely informative comments — the `LoadProfileInput.ProfileRef`
resolver-semantics note, the `DeleteProfileInput.FolderAction`
zero-value note, the `collapse` typed-nil note, the `From` CreatedAt
timing note — survive on their own.

```go
// internal/actions/inputs.go:23-27
// CreateProfileInput is the input for Actions.CreateProfile.
type CreateProfileInput struct {
    Name string
    Path string
}
```

## Solution

- [x] Strip the comments that merely restate type/method names; keep
      only the ones that document semantics, invariants, or
      surprises.
- [ ] Keep a one-line `// CreateProfileInput.` tag on each exported
      type (gofmt-minimum) and drop the body. Less aggressive
      compromise.

## Plan claims table-driven tests; impl is sequential

> [!WARNING]
>
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md) — guideline does not require table-driven; plan/impl divergence is what matters.

The plan (`plan.md:190`) explicitly says: *"Pattern: table-driven; one
helper per file constructs a fresh `*app.Service`…"*. Every test in the
diff is a top-level `func TestX`with no`for \_, tc := range tests`.
Functionally the suite passes — the divergence is a documentation
mismatch.

```go
// e.g. internal/actions/profiles_test.go:24-72 — sequential, not table-driven
func TestActions_LoadProfiles_ReturnsAll(t *testing.T) { ... }
func TestActions_LoadProfiles_CollapsesErrorsIntoErrsErrors(t *testing.T) { ... }
func TestActions_LoadProfile_ResolvesByID(t *testing.T) { ... }
```

## Solution

- [x] Amend the plan: drop the "table-driven" claim; current
      one-test-per-behavior shape matches the
      `docs/guidelines/testing.md` philosophy of focused tests anyway.
- [ ] Convert the genuinely repetitive cases — the "missing returns
      XNotFoundError" family across `profiles_test.go`,
      `projects_test.go`, `assets_test.go` — into a single table per
      file.

## Dead variable in `TestToast_StaleExpireIgnored`

> [!WARNING]
>
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md) — keep test code boring and intent-clear.
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) — _Source Structure_: declare variables close to where they are used.

The test reads as if an earlier formulation was abandoned mid-edit.
`stale := toast.StaleExpireForTest()` is computed, immediately
discarded (`_ = stale`), and replaced by `preStale` further down.
Reader has to parse three lines before discovering the first variable
is dead.

```go
// internal/tui/notifications/toast_test.go:76-92
func TestToast_StaleExpireIgnored(t *testing.T) {
    toast := notifications.NewToast(time.Millisecond)
    toast, _ = toast.Update(push("first"))
    stale := toast.StaleExpireForTest() // seq for #1 -1; older still
    _ = stale
    // Properly: expire #1 to advance, then push #2 and replay a stale
    // expireMsg generated *before* push #1 — must not displace #2.
    toast, _ = toast.Update(toast.ExpireNowForTest())
    toast, _ = toast.Update(push("second"))

    preStale := toast.StaleExpireForTest()
    toast, _ = toast.Update(preStale)
    // ...
}
```

## Solution

- [x] Delete lines 79-81 (`stale := …; _ = stale`) and the obsolete
      "Properly:" preface in the comment; keep the rest.
- [ ] Rewrite the test as Given/When/Then with one explicit setup, one
      stale replay, one assertion.

## Toast tests use `time.Millisecond` duration with no safety margin

> [!WARNING]
>
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md) — keep tests isolated; avoid fragility from hidden timing.

`toast_test.go` constructs every `Toast` with `time.Millisecond`. The
returned `tea.Cmd` is discarded so the wall-clock tick never fires
today — the tests drive expiry via `ExpireNowForTest()`. The setup is
sound in current bubbletea but gives zero margin: any future change
that eagerly schedules cmds at construction would make these tests
race the timer.

```go
// internal/tui/notifications/toast_test.go:18, 28, 40, 50, 77, 95
toast := notifications.NewToast(time.Millisecond)
```

## Solution

- [x] Use `time.Hour` (or `math.MaxInt64`) so an accidentally-
      executed tick cannot fire during the test.
- [ ] Inject the tick command: replace `tea.Tick` in `scheduleExpire`
      with a `func(time.Duration) tea.Cmd` field on `Toast` (default
      `tea.Tick`, override in tests with a no-op). Decouples tests
      from the real clock entirely.
- [ ] Accept; tests pass and the cmds are never invoked. Add a
      one-line comment on the test fixture explaining why `1ms` is
      irrelevant.

## Constructor panics on `nil` instead of returning error

> [!WARNING]
>
> - [docs/guidelines/go.md](../../../docs/guidelines/go.md) — Go style: panic for programmer errors; OK for "wiring bug" where the caller is `main`.

`actions.New(nil)` and `notifications.NewArea(nil)` both `panic`. Both
are constructed exactly once in `cmd/af/main.go` (per the plan), so a
nil here is an unambiguous wiring bug — crashing loud is acceptable
Go practice. The codebase elsewhere returns `errs.DomainError`
throughout; the panic is a small consistency break.

```go
// internal/actions/actions.go:21-26
func New(svc *app.Service) *Actions {
    if svc == nil {
        panic("actions.New: nil service")
    }
    return &Actions{svc: svc}
}
```

## Solution

- [x] Accept as-is (recommended): panic for `nil` wiring bug is
      conventional Go; both call sites are in `main`.
- [ ] Convert to `(*Actions, error)` / `(*NotificationArea, error)`
      for stdlib consistency; `main` does `log.Fatal` on the error.

## `LogCap = 500` magic number with no rationale

> [!WARNING]
>
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) — _Naming_: "Replace magic numbers and strings with named constants when the value has meaning outside one local expression" — the value is named, but its _meaning_ (why 500?) is undocumented.

```go
// internal/tui/notifications/log.go:28-30
// LogCap is the maximum number of entries retained in the ring buffer.
// New entries past the cap evict the oldest.
const LogCap = 500
```

The constant is named, but a future maintainer touching it has no
signal whether `500` was chosen for memory bounds, modal-scroll budget,
expected session length, or "feels right." One sentence in the comment
would fix it.

## Solution

- [x] Add a rationale sentence to the comment ("≈ one full session of
      operations; tuned for the notifications modal scroll budget" or
      "tuneable; no hard requirement").
- [ ] Accept; trivial.

## Hidden substring assertion in `TestFrom_ErrsErrorsRenderedViaErrorMethod`

> [!WARNING]
>
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md) — _Assert Behavior, Not Mock Mechanics_: the behavior under test is "From renders typed errors via Error()"; substring scans couple the test to `errs.Errors.Error()`'s exact formatting.

```go
// internal/tui/notifications/bridge_test.go:74-77
if !strings.Contains(msg.Notification.Text, "first") ||
    !strings.Contains(msg.Notification.Text, "second") {
    t.Fatalf("expected both errors in text, got %q", msg.Notification.Text)
}
```

`errs.Errors.Error()` already has its own focused tests in `internal/errs`.
The bridge test only needs to confirm that the bridge calls `Error()` on
the slice — not that the slice formats two children with a newline join.

## Solution

- [x] Assert exact equality: `msg.Notification.Text == "first\nsecond"`.
      Makes the coupling explicit; one assertion instead of two.
- [ ] Replace with a behavior assertion: `Level == LevelError` and
      `Text != ""`. Treat `errs.Errors.Error()`'s exact join as
      `internal/errs`'s responsibility, not the bridge's.

## Fixture duplication across `internal/actions/*_test.go`

> [!WARNING]
>
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md) — "a little duplication is better than abstractions that make each test harder to read" — borderline; three near-identical fixtures lean toward over-duplication.

`newProfileFixture`, `newProjectFixture`, and `newAssetFixture` differ
only by seed state and return tuple shape. Functionally fine; mostly a
readability nit.

```go
// internal/actions/profiles_test.go:17
func newProfileFixture(t *testing.T) (*actions.Actions, *app.Service, string)

// internal/actions/projects_test.go:19
func newProjectFixture(t *testing.T) (a *actions.Actions, svc *app.Service, profilePath, root string)

// internal/actions/assets_test.go:16
func newAssetFixture(t *testing.T) (*actions.Actions, *app.Service, string)
```

## Solution

- [ ] Accept; helpers are file-local and self-documenting in context.
- [x] Consolidate into a single `fixtures_test.go` with an
      option-based builder (e.g. `newFixture(t, withProfile("Personal"))`).
      Higher upfront cost; pays off as the action surface grows.

---

After ticking one box per finding, ping back so the chosen fixes can be
applied, tests re-run, lint cleared, and the result committed.
