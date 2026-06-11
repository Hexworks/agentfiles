# System modals review (task 0023)

Seven parallel reviews (security, clean code, clean architecture, SOLID, DDD,
testing, Go) of the system-modals branch. Security cleared. The remaining
agents converge on one **critical correctness bug** plus a cluster of design
and test issues. Highest-impact items first.

**Critical**

1. Info modal always renders a `NotFoundError` — `ManualOverview` is double-joined against `help.ManualRoot`.
2. Modal does not resize on `tea.WindowSizeMsg`; `Screen.Body` parameters discarded.

**Architecture / structure** 3. Two sibling packages named `notifications` force aliases at every shared importer. 4. Severity → label vocabulary belongs with `styles.SeverityStyle`, not inside the modal. 5. `modalSize` is shell-wide but lives in `info.go`; `notifications.go` calls into a peer adapter. 6. `infoScreen` and `notificationsScreen` are structurally identical. 7. SRP: `notifications.go` mixes lifecycle, layout arithmetic, and severity vocabulary.

**Interface design** 8. ISP/CCP: shell `notifier` bundles `Add` + `Entries` for disjoint callers. 9. Topic registry split across `shell/topics.go` and `components/help`; path syntax invariant lives in only one half. 10. `topicFor(_ Screen)` ignores its argument — extension seam without dispatch. 11. Three labels for the same concept: screen title (`Info`), help tab (`Help: Info`), manual H1 (`Overview`).

**Tests** 12. Info modal happy-path is not exercised (description requirement uncovered). 13. `topicFor` has no test. 14. `modalSize` floor (40×10) has no test. 15. `TestUpdate_NonCloseKeyKeepsActive` does not actually verify key forwarding to the table. 16. Newest-first ordering test only compares first/last entries. 17. Level styling (`INFO neutral, ERROR red/bold`) not asserted. 18. `notifier.Entries()` consumer-interface contract is implicit; no end-to-end test of the modal reading the shell log.

**Smaller items** 19. Stale `pushScreen` comment still references "three global-key stubs". 20. Magic numbers `40`/`10`/`4` in `modalSize` are unnamed. 21. `EmptyMessage` is exported only so tests can substring-match.

---

## 1. Info modal always renders NotFoundError — path double-join

> [!WARNING]
>
> - [`docs/guidelines/clean_code.md#design-rules`](../../../docs/guidelines/clean_code.md)
> - [`docs/guidelines/domain_model.md#use-the-project-language`](../../../docs/guidelines/domain_model.md)
> - [`docs/guidelines/testing.md#start-with-the-smallest-useful-test`](../../../docs/guidelines/testing.md)

`internal/tui/components/help/help.go:34` defines `const ManualRoot = "docs/manual"` and `Request.Path` is documented as relative-to-ManualRoot. `loadManual` joins them at `help.go:171`:

```go
fullAbs := filepath.Join(rootAbs, clean)
```

`internal/tui/shell/topics.go:7` returns the **full repo-relative** path:

```go
const ManualOverview = "docs/manual/overview.md"
```

Verified locally — the join produces `<repo>/docs/manual/docs/manual/overview.md`, which does not exist. Pressing `?` opens the Info modal showing `Failed to load manual: help: "docs/manual/overview.md" not found`. The acceptance criterion `./bin/af # ? opens Info modal` is not met. Existing tests miss this because they assert only `Title() == "Info"` and `ResolvedMsg` pops the screen — no test calls `View()` from the repo root against the real `overview.md`.

```go
// internal/tui/shell/topics.go
const ManualOverview = "docs/manual/overview.md"

func topicFor(_ Screen) string { return ManualOverview }

// help.New then passes "docs/manual/overview.md" as Path,
// which gets joined with rootAbs ("…/docs/manual") to produce
// "…/docs/manual/docs/manual/overview.md" — file not found.
```

All options below fix the bug — `?` opens the rendered manual instead of an error. They differ in how much structural cleanup they bundle in.

Choose one:

- [x] **Minimal fix.** Change `ManualOverview` from `"docs/manual/overview.md"` → `"overview.md"`. The constant now matches the documented "relative to `help.ManualRoot`" contract; `help.New` joins it with `ManualRoot` and reads the real file. One-line edit, no other code moves.
- [ ] Replace `topicFor` with a `Topic{Label, File}` struct and resolve the file through `help.ManualRoot` at the call site. Same user-visible fix; also folds finding #11 (three labels for one concept) into the same change.
- [ ] Move the topic registry into `internal/tui/components/help/` (e.g. `help.Topics.Overview` constructed against `ManualRoot` directly). Same fix; also closes finding #9 (split boundary) by giving "topic" one owner.

## 2. Modal does not resize on `WindowSizeMsg`

> [!WARNING]
>
> - [`docs/guidelines/charm.md#window-size-propagation`](../../../docs/guidelines/charm.md)
> - [`docs/guidelines/tui.md`](../../../docs/guidelines/tui.md)

Modal dimensions are frozen at push time. `notificationsScreen.Body` and `infoScreen.Body` ignore their `(w, h)` arguments and render the modal at construction-time size. `bubbles/table.Update` does not consume `tea.WindowSizeMsg`, and `content.Update` (`internal/tui/components/notifications/notifications.go:82-95`) never calls `c.table.SetWidth/SetHeight`. The shell forwards resizes to every stacked screen, but the screen adapters drop them.

A user who resizes the terminal with a modal open sees a stale-sized modal. The notifications table cannot grow or shrink to fit; the help viewport keeps its old wrap width.

```go
// internal/tui/shell/notifications.go
func (s *notificationsScreen) Body(_ int, _ int) string { return s.modal.View() }

// internal/tui/components/notifications/notifications.go (Update)
var cmd tea.Cmd
c.table, cmd = c.table.Update(msg) // forwards key/scroll, not resize
return c, cmd
```

Choose one:

- [x] On `tea.WindowSizeMsg`, store `(w, h)` on the screen adapter and rebuild the wrapped `*modal.Modal` (or expose a `SetSize` on `Modal` / `content` that re-runs `innerSize` and calls `c.table.SetWidth/SetHeight`).
- [ ] Use `Body(w, h)` parameters per frame: track the latest size in the screen adapter via Update, and rebuild only when the size actually changes.
- [ ] Acknowledge as known limitation, file follow-up task, document in plan/changelog. Acceptable only if team agrees frozen modals are tolerable for now.

## 3. Two sibling packages named `notifications`

> [!WARNING]
>
> - [`docs/guidelines/clean_architecture.md#common-reuse-principle`](../../../docs/guidelines/clean_architecture.md)
> - [`docs/guidelines/domain_model.md#use-the-project-language`](../../../docs/guidelines/domain_model.md)
> - [`docs/guidelines/clean_code.md#naming`](../../../docs/guidelines/clean_code.md)

`internal/tui/notifications` (ring-buffer log) and `internal/tui/components/notifications` (modal view) share the same package identifier. Every shared importer must alias one of them, and the alias convention is inconsistent: the modal imports the log as `notlog`; the shell imports the modal as `notmodal`. A reader greps for `notifications` and has to discover which is meant from context.

```go
// component file
notlog "github.com/hexworks/agentfiles/internal/tui/notifications"
// shell file
notmodal "github.com/hexworks/agentfiles/internal/tui/components/notifications"
```

Choose one:

- [x] Rename the component package to a role-name (e.g. `notificationsmodal`, `notificationsview`, `notifylist`). Drops the alias requirement everywhere; matches `help` which is role-named.
- [ ] Move the modal into `internal/tui/notifications/` as `notifications.NewModal(...)`, one package per domain concept. Removes the collision entirely; couples the view to the log package.
- [ ] Keep current naming; document `notlog`/`notmodal` aliasing convention in `docs/glossary.md` so future contributors find it.

## 4. Severity → label vocabulary lives in the modal, not `styles`

> [!WARNING]
>
> - [`docs/guidelines/clean_architecture.md#common-closure-principle`](../../../docs/guidelines/clean_architecture.md)
> - [`docs/guidelines/domain_model.md#use-the-project-language`](../../../docs/guidelines/domain_model.md)

`internal/tui/components/notifications/notifications.go:167-175` introduces `levelLabel(errs.Severity) string` returning `"INFO" / "WARN" / "ERROR"`. The rest of the codebase already uses `errs.Severity` as the canonical noun and centralises severity presentation in `styles.SeverityStyle` (icon + style switch). The label table is shared policy that the next severity renderer (a settings filter, a future log view) will need; locking it inside the modal forces a copy.

The table column header is also named `level` while the underlying field is `Severity`.

```go
// notifications.go — private switch the next consumer will copy
func levelLabel(s errs.Severity) string {
    switch s {
    case errs.SeverityError:   return "ERROR"
    case errs.SeverityWarning: return "WARN"
    }
    return "INFO"
}
```

Choose one:

- [x] Move the label to `internal/tui/styles/styles.go` as `SeverityLabel(s)` (or extend `SeverityStyle` to return `(label, icon, style)`), have the modal call the shared helper, rename the column header to `severity`.
- [ ] Keep the helper in the modal but rename `levelLabel` → `severityLabel` and the column to `severity` so vocabulary matches the glossary even if the mapping stays duplicated.

## 5. `modalSize` lives in `info.go` but is shared

> [!WARNING]
>
> - [`docs/guidelines/clean_architecture.md#common-closure-principle`](../../../docs/guidelines/clean_architecture.md)
> - [`docs/guidelines/clean_code.md#source-structure`](../../../docs/guidelines/clean_code.md)
> - [`docs/guidelines/solid.md#single-responsibility-principle`](../../../docs/guidelines/solid.md)

`internal/tui/shell/info.go:52-62` defines `modalSize` and `internal/tui/shell/notifications.go:24` calls it. The helper encodes shell-wide layout (reserve 4 rows for title + status, floor at 40×10) — independent of either screen's content. A reader changing the floor for one modal has to discover it sits in the peer adapter file.

```go
// info.go — but called from notifications.go too
func modalSize(width, height int) (int, int) {
    w := width
    if w < 40 { w = 40 }
    h := height - 4
    if h < 10 { h = 10 }
    return w, h
}
```

Choose one:

- [x] Move `modalSize` into `screen.go` (where the screen contract is documented) next to `pushCmd`/`popCmd`.
- [ ] Move it next to `Model.View` in `shell.go` where the chrome heights are already encoded; lets a future chrome change update both numbers atomically.
- [ ] Create `internal/tui/shell/layout.go` for shell-wide layout helpers.

## 6. `infoScreen` and `notificationsScreen` are structurally identical

> [!WARNING]
>
> - [`docs/guidelines/clean_architecture.md#reuse--release-equivalence-principle`](../../../docs/guidelines/clean_architecture.md)
> - [`docs/guidelines/clean_code.md#code-smells`](../../../docs/guidelines/clean_code.md)

Both screens share identical `Init / Update / Body / StatusKeys` shape around a `*modal.Modal`, differing only in title and constructor call. A future modal-hosting screen (settings, about, confirm-quit) produces another copy. A fix to the resolve→pop dispatch (e.g. only pop on cancel and emit a value-bearing message on confirm) has to land in every copy.

```go
func (s *infoScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
    if _, ok := msg.(modal.ResolvedMsg); ok { return s, popCmd() }
    var cmd tea.Cmd
    s.modal, cmd = s.modal.Update(msg)
    return s, cmd
}
// notifications.go is the same body
```

Choose one:

- [ ] Introduce `modalScreen` (or `dialogScreen`) that wraps `*modal.Modal` + a title string. `newInfoScreen`/`newNotificationsScreen` become factory functions returning `*modalScreen`. Single test for ResolvedMsg dispatch.
- [x] Defer until a third modal-hosting screen lands (0024+). Acceptable per "start simple"; revisit on the third copy.

## 7. SRP: `notifications.go` mixes lifecycle, layout, vocabulary

> [!WARNING]
>
> - [`docs/guidelines/solid.md#single-responsibility-principle`](../../../docs/guidelines/solid.md)
> - [`docs/guidelines/clean_code.md#source-structure`](../../../docs/guidelines/clean_code.md)

One file holds: the `content` struct implementing `modal.Content` (lines 44-106), modal-frame + column-width constants and sizing math `innerSize`/`buildTable` (lines 111-162), and the severity vocabulary in `levelLabel`/`renderLevel` (lines 167-180). Three independent reasons to change live in one place. The dedicated `TestLevelLabel_Vocabulary` already treats the vocabulary as a separable concern.

Choose one:

- [x] Split into `notifications.go` (content + New), `layout.go` (constants + `innerSize` + `buildTable`), `severity.go` (`levelLabel` + `renderLevel`). Bundles cleanly with finding #4 if severity moves to `styles`.
- [ ] Accept current cohesion; the file is still under 200 lines.

## 8. ISP: shell `notifier` bundles Add + Entries for disjoint callers

> [!WARNING]
>
> - [`docs/guidelines/solid.md#interface-segregation-principle`](../../../docs/guidelines/solid.md)
> - [`docs/guidelines/clean_code.md#naming`](../../../docs/guidelines/clean_code.md)
> - [`docs/guidelines/go.md#keep-packages-cohesive`](../../../docs/guidelines/go.md)

`internal/tui/shell/shell.go:19-22`:

```go
type notifier interface {
    Add(notifications.Notification)
    Entries() []notifications.Notification
}
```

`Add` is used only by `Model.notify` (shell.go:148). `Entries` is needed only when `newNotificationsScreen` passes the log to `notmodal.New`. No caller uses both. The component package already enforces the principle with its own one-method `LogReader` (`notifications.go:29-31`). The shell's wider interface is exactly the bundling the guideline warns against — the changelog itself acknowledges this growth.

The name `notifier` also no longer matches the responsibility now that it includes a read side.

Choose one:

- [ ] Split into `notificationAdder { Add(...) }` and `notificationReader { Entries() ... }`; store both halves separately (or embed) so each call site declares only what it uses. Rename `notifier` field accordingly.
- [x] Drop `Entries()` from the shell interface; pass `m.log` directly into `notmodal.New`. The shell keeps a write-only `notifier`; modal's `LogReader` is the only read-side contract. Subsumes the duplication and the naming issue at once.
- [ ] Keep the combined interface but rename `notifier` → `notificationLog` to better reflect both sides; accept the bundle.

## 9. Topic registry placement splits the same boundary

> [!WARNING]
>
> - [`docs/guidelines/domain_model.md#model-consistency-boundaries`](../../../docs/guidelines/domain_model.md)
> - [`docs/guidelines/clean_architecture.md#common-closure-principle`](../../../docs/guidelines/clean_architecture.md)

The "manual topic" concept is split between `internal/tui/shell/topics.go` (path map) and `internal/tui/components/help/` (`ManualRoot`, path validation, `Request`). Adding a topic forces the author to know an undocumented contract that lives in another package. This is the same root cause behind finding #1 — the shell author wrote the full `docs/manual/...` path because the registry has no reference to `help.ManualRoot`.

Choose one:

- [x] Move the topic registry into `internal/tui/components/help/` (e.g. `help.Topics.Overview`). Topic + path validation share one home; shell calls `help.TopicFor(focus)`. Fixes #1 by construction.
- [ ] Keep the registry in the shell but reference `help.ManualRoot` directly: `ManualOverview = "overview.md"` and the call site assembles `help.Request{Path: ManualOverview}` (already ManualRoot-relative). Smallest change, also fixes #1.

## 10. `topicFor(_ Screen)` advertises an extension seam it cannot use

> [!WARNING]
>
> - [`docs/guidelines/solid.md#open-closed-principle`](../../../docs/guidelines/solid.md)
> - [`docs/guidelines/clean_code.md#design-rules`](../../../docs/guidelines/clean_code.md)

`internal/tui/shell/topics.go:13` ignores its argument and always returns `ManualOverview`. There is no dispatch table, no registration mechanism, and no test exercising a non-default path. A future contributor will likely add a `switch s.(type)` in the function body — which is exactly the "modify the stable rule" path OCP says to avoid. A genuine extension point would let a screen own its own topic.

Choose one:

- [x] Give Screens an optional `Topic() string` method (or a small `Topical` interface); have `topicFor` type-assert. Each screen owns its topic; no central edits when new screens land.
- [ ] Drop the parameter (`topicFor() string`) until per-screen overrides exist. Re-introduce the seam with the second topic.
- [ ] Inline `ManualOverview` at the call site in `info.go`; delete `topics.go`. Same end-state, fewer indirections; revives when needed.

## 11. Three labels for the same concept

> [!WARNING]
>
> - [`docs/guidelines/domain_model.md#model-consistency-boundaries`](../../../docs/guidelines/domain_model.md)
> - [`docs/guidelines/clean_code.md#naming`](../../../docs/guidelines/clean_code.md)

`infoScreen.Title()` returns `"Info"` (info.go:44). The help component tab renders `"Help: Info"` (help.go:123). The manual H1 is `"# agentfiles — Overview"` (`docs/manual/overview.md:1`). Three labels for the same page. `topicFor` returns only a path — no slot for a label, so the label is a string literal in the screen and will drift from the manual page name when overrides land.

Choose one:

- [x] Make `topicFor` return a `Topic{Label, Path}` struct (or a full `help.Request`). One record per topic; label and path live together. Bundles with finding #9.
- [ ] Drop the per-call Topic label and have the help modal derive the tab title from the manual page's first H1.
- [ ] Accept current naming; rename screen title to `"Help"` (matches the global key binding label) so the user sees one consistent word.

## 12. Info modal happy-path is uncovered

> [!WARNING]
>
> - [`docs/guidelines/testing.md#start-with-the-smallest-useful-test`](../../../docs/guidelines/testing.md)

Description requires: _"Info modal: opening with a known `.md` file resolves and renders without error."_ `stubs_test.go:60-78` covers only `Title()` and synthetic `ResolvedMsg → pop`. Neither test calls `Body()` against a real `overview.md`. Finding #1 (the double-join bug) survived precisely because of this gap.

Choose one:

- [x] Add a test that builds `infoScreen` and asserts `Body(80, 20)` contains a substring from `overview.md` (e.g. `"Global keys"`). The test must run from the repo root (`t.Chdir`) so `docs/manual/` resolves. Catches finding #1 and any future drift.
- [ ] Add a helper that writes a temp `docs/manual/test.md` under `t.TempDir()` (after `t.Chdir`) and points `topicFor` at it. Locks the contract without depending on production manual content.

## 13. `topicFor` has no test

> [!WARNING]
>
> - [`docs/guidelines/testing.md#test-one-behavior-at-a-time`](../../../docs/guidelines/testing.md)

`internal/tui/shell/topics.go:13` is unexercised. The changelog flags it as a deliberate, future-proof seam — exactly the kind of decision a behavior test should pin.

Choose one:

- [x] Add `TestTopicFor_DefaultsToOverview` asserting `topicFor(nil) == ManualOverview` and `topicFor(welcomeStub{}) == ManualOverview`.
- [ ] Skip — defer until the first override exists (folds with finding #10's "drop the seam" option).

## 14. `modalSize` floor has no test

> [!WARNING]
>
> - [`docs/guidelines/testing.md#start-with-the-smallest-useful-test`](../../../docs/guidelines/testing.md)

Changelog headlines the 40×10 floor as preventing a `bubbles/table` crash on pre-`WindowSizeMsg` opens. A regression that lowers the floor or removes it would not fail any test.

Choose one:

- [x] Add `TestModalSize_FloorsBelowMinimum` with cases `(0,0)→(40,10)`, `(30,12)→(40,10)`, `(120,40)→(120,36)`.
- [ ] Extract `const modalMinWidth = 40; modalMinHeight = 10; chromeHeight = 4` and assert against the constants.

## 15. `TestUpdate_NonCloseKeyKeepsActive` does not verify forwarding

> [!WARNING]
>
> - [`docs/guidelines/testing.md#assert-behavior-not-mock-mechanics`](../../../docs/guidelines/testing.md)

`internal/tui/components/notifications/notifications_test.go:99-109` claims a non-close key "must reach the underlying table" but only asserts the lifecycle state. A regression that swallowed every non-close key (returning early without calling `c.table.Update`) would still pass.

```go
c := newContent(log, 80, 20)
_, _ = c.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
if state, _ := c.Lifecycle(); state != modal.Active { ... }
// table.Cursor() never observed
```

Choose one:

- [x] Assert `c.table.Cursor()` advanced after `j` (or `c.View()` differs from the pre-send view). Locks the actual forwarding behaviour.
- [ ] Rename to `TestUpdate_NonCloseKeyDoesNotClose` to match what the assertion actually checks; add a separate test for forwarding.

## 16. Newest-first ordering test only compares first and last

> [!WARNING]
>
> - [`docs/guidelines/testing.md#test-one-behavior-at-a-time`](../../../docs/guidelines/testing.md)

`internal/tui/components/notifications/notifications_test.go:36-57` asserts only `boom < hello`; `careful` is never compared against either neighbour. A reversal of the middle two rows passes silently.

Choose one:

- [x] Extend the ordering check to all three rows: `boomIdx < carefulIdx < helloIdx`.
- [ ] Iterate over the expected entries and assert strictly increasing indexes via a loop.

## 17. Level styling not asserted

> [!WARNING]
>
> - [`docs/guidelines/testing.md#assert-behavior-not-mock-mechanics`](../../../docs/guidelines/testing.md)

Description: _"Levels rendered with a small style helper: INFO neutral, ERROR red/bold."_ `TestLevelLabel_Vocabulary` covers the unstyled string; `renderLevel` has no test. The test could assert the styled output is longer than the bare label (proves some style was applied) without binding to specific ANSI codes.

Choose one:

- [x] Add `TestRenderLevel_AppliesStyle` asserting `renderLevel(errs.SeverityError)` contains `"ERROR"` and is longer than `"ERROR"` (style added something).
- [ ] Use `ansi.Strip(renderLevel(s))` to assert the visible text equals `levelLabel(s)` and the raw form differs.

## 18. `notifier.Entries()` contract has no end-to-end test

> [!WARNING]
>
> - [`docs/guidelines/testing.md#assert-behavior-not-mock-mechanics`](../../../docs/guidelines/testing.md)

The changelog calls the `Entries()` addition the load-bearing design decision. No test substitutes a fake `notifier` and asserts the modal mounted by the shell actually reads its log entries. `TestUpdate_NotificationMsgFeedsLog` casts back to `*notifications.Log`, which would break under the substitution the interface is meant to enable.

Choose one:

- [ ] Add `TestNewNotificationsScreen_ReadsShellLog`: call `m.notify(...)` once, then `m.newNotificationsScreen()`, then assert the resulting modal's `Body()` contains the notification text.
- [x] Drop `Entries()` from `notifier`; pass the log directly to `notmodal.New` from `keys.go`. Folds with finding #8.

## 19. Stale comment on `pushScreen`

> [!WARNING]
>
> - [`docs/guidelines/clean_code.md#comments`](../../../docs/guidelines/clean_code.md)

`internal/tui/shell/shell.go:154-157` says _"Idempotent for the three global-key stubs"_, but two of the three are no longer stubs (only `settingsStub` remains). Future readers will look for three stubs and find one.

```go
// pushScreen … Idempotent for the three global-key stubs: if s has the same
// concrete type as the current top, the push is a no-op so auto-repeat on
// n/s/? cannot grow the stack indefinitely.
```

Choose one:

- [ ] Rephrase to `"Idempotent on concrete type: pushing the same screen kind twice in a row is a no-op, so global-key auto-repeat cannot grow the stack."`
- [x] Drop the explanatory line; the invariant is implied by `sameScreenType`.

## 20. Magic numbers in `modalSize`

> [!WARNING]
>
> - [`docs/guidelines/clean_code.md#naming`](../../../docs/guidelines/clean_code.md)

`internal/tui/shell/info.go:54-60` hard-codes `40`, `10`, `4` inline. The comment explains them in prose; the code does not. The `4` is the inverse of the chrome reservation computed in `View()` — two encodings of the same number.

Choose one:

- [x] Extract `const (modalMinWidth = 40; modalMinHeight = 10; chromeHeight = 4)`; use named constants in `modalSize` and reference `chromeHeight` in `View` so they stay in sync. Folds with finding #14.
- [ ] Compute chrome reservation at call time from `lipgloss.Height(title)` + `lipgloss.Height(status)` so `modalSize` no longer encodes layout twice.

## 21. `EmptyMessage` exported solely so tests can assert it

> [!WARNING]
>
> - [`docs/guidelines/clean_code.md#data-and-objects`](../../../docs/guidelines/clean_code.md)
> - [`docs/guidelines/clean_code.md#tests`](../../../docs/guidelines/clean_code.md)

`internal/tui/components/notifications/notifications.go:22-24` exports `EmptyMessage` with the rationale "Exported so tests can assert the exact string." Exporting purely for test reach is a code smell — the modal's contract is "show something readable when empty", not "the literal string `No notifications yet`". A future caller that imports `notifications.EmptyMessage` couples to a UI string.

Choose one:

- [x] Lower-case to `emptyMessage`; the test declares its own `wantEmpty := "No notifications yet"`. Captures the user-visible contract without exposing the constant.
- [ ] Keep exported but find a non-test consumer (e.g. shell shows a count badge against it). If no production caller materializes, pick option A.

---

## How to apply

Tick exactly one `[x]` under every `##` block, then return here. I'll apply the chosen solutions, run `make fmt && make lint && make build && make test`, and commit.
