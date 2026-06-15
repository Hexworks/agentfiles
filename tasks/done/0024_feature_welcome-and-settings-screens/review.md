# Welcome + Settings screens review

Task 0024 replaces the placeholder Welcome/Settings stubs with real screens and
adds a `profilesStub` placeholder for the future Profiles screen (task 0025).
The diff is small, well-scoped, and stays inside the TUI boundary the
guidelines and ADR 0011 draw for shell-level screens.

Headline checks pass:

- **Security**: no findings. No I/O, no parsing, no external input beyond
  bounded key presses. Stack growth is bounded by the `sameScreenType` dedup.
- **Clean Architecture / DDD**: no findings. New screens import only
  `charm.land/*`, `internal/tui/styles`, `internal/tui/components/mnemonic`.
  No domain leakage, no policy in the TUI.
- **SOLID**: no real violations. One borderline observation about Welcome
  redeclaring `up`/`down` bindings already declared on the global key map
  (see issue #7 below).

Issues found:

1. **`settingsScreen.Body` diverges from plan**: branches on `width` (and on a
   nearly-unreachable condition) instead of `height` as the plan specified;
   the short-width fallback is effectively dead code with a divergent layout
   shape.
2. **ADR 0011 documentation drift**: still names `settingsStub` as a live
   placeholder after this task deletes it.
3. **`welcomeScreen.Body` silently ignores `width` and `height`**: inconsistent
   with `Screen`'s documented contract; no comment justifies the discard.
4. **Asymmetric navigation test coverage + cursor reset inside loop**: `j` is
   not tested wrapping from bottom (only `down`); `DownMovesCursor` /
   `UpWrapsFromTop` reuse one screen and reset `s.cursor = 0` between
   iterations, muddying Given/When/Then.
5. **`[Back]` body assertion is brittle**: splits the substring check across
   `"["` and `"ack]"` to dodge ANSI between them, and would pass on bogus
   output like `[Xack]`.
6. **`TestWelcomeScreen_TitleAndBody` bundles two unrelated promises**: plan
   listed Title and Body assertions as separate tests; impl fused them.
7. **Esc handled via direct `kp.Code == tea.KeyEsc` instead of `key.Matches`**:
   inconsistent with the rest of the shell (welcome, keys, confirm) which
   routes everything through `bubbles/key`.
8. **Welcome `up` / `down` bindings duplicate `globalKeyMap` key strings**:
   borderline SRP/DIP nit — same key strings declared on both sides; if either
   side drifts the status-bar hint will stop matching the screen behavior.

## Settings body branches on width, not height; short-width branch is effectively dead

> [!WARNING]
>
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) — § "Understandability", § "Code Smells: Opacity"
> - [plan.md § Design / settingsScreen](./plan.md)

`plan.md` lines 116–117 say: _"If height is small (< 3), drop the spacing and
stack vertically — the shell already clamps `bodyH` to 0 when overdrawn."_ The
implementation instead branches on **width** — and on a degenerate condition
(`width <= lipgloss.Width(back)`), which would only fire if the terminal is
narrower than `[Back]` (~6 columns). The real fragility the plan called out —
`height` shrinking below the 3 rows the spacer + button assume — is silently
ignored, and `Body(width, _ int)` discards `height` entirely so the asymmetry
is invisible at the call site.

The two return branches also produce different layout shapes: the "narrow"
branch returns `msg\nback` (left-aligned, tight); the "normal" branch returns
`msg\n\nback-right-aligned`. The fallback is not a graceful degradation — it's
a different composition. Without a comment that says "narrow path drops the
spacer and right-alignment because there is no horizontal room", a reader has
to reverse-engineer the intent.

```go
func (s *settingsScreen) Body(width, _ int) string {
    msg := " Coming soon."
    back := s.back.View()
    if width <= lipgloss.Width(back) {
        return lipgloss.JoinVertical(lipgloss.Left, msg, back)
    }
    return lipgloss.JoinVertical(lipgloss.Left, msg, "", lipgloss.PlaceHorizontal(width, lipgloss.Right, back))
}
```

- [x] Drop the short-width branch entirely. `lipgloss.PlaceHorizontal` already
      degrades gracefully on small widths (it just won't right-align), so the
      single-line "normal" path covers both cases without the divergent layout.
- [ ] Honor the plan: rename `_` → `height`, switch the guard to `height < 3`,
      and stack `msg` directly above `back` when the body has fewer than three
      rows. Add a `TestSettingsScreen_BodyStacksVerticallyWhenShort` covering
      it.
- [ ] Keep the width branch but document it: rename to a named constant
      (`const minBackButtonWidth = ...`) and add a one-line comment explaining
      why width was chosen over height after all.

## ADR 0011 still references the deleted `settingsStub`

> [!WARNING]
>
> - [docs/guidelines/documentation.md](../../../docs/guidelines/documentation.md) — § "document current reality, not planned state"

ADR 0011 Consequences (line 108) still says: _"Three placeholder stubs
(`notificationsStub`, `settingsStub`, `infoStub`) ship inside the shell
package so the global keys can push something the user sees before tasks
0023 and 0024 land."_ That paragraph was already partially stale after task
0023 turned `notificationsStub` and `infoStub` into real screens; task 0024
deletes `settingsStub` outright. A future reader landing in the ADR will find
prose that describes a world that no longer exists.

The 0024 changelog likewise does not note this ADR touch-up, so the drift is
untraceable from the changelog side. Per the project's documentation
guideline, the Consequences section should describe the _current_ shell state,
not the wiring 0021 originally shipped.

```text
docs/adr/0011-tui-screen-router.md:108
Three placeholder stubs (`notificationsStub`, `settingsStub`,
`infoStub`) ship inside the shell package so the global keys can
push something the user sees before tasks 0023 and 0024 land.
```

- [x] Update the ADR Consequences paragraph: after 0023/0024 the only
      remaining placeholder is `profilesStub` (replaced by 0025). Mention the
      ADR edit in the 0024 changelog so the trail is preserved.
- [ ] Leave the ADR as-is and add a short "Subsequent tasks" note at the
      bottom of the ADR that reads "0023 replaced `notificationsStub` and
      `infoStub`; 0024 replaced `settingsStub`; only `profilesStub` remains
      pending 0025."

## `welcomeScreen.Body` ignores its `width` and `height` parameters

> [!WARNING]
>
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) — § "Functions", § "Understandability"
> - [internal/tui/shell/screen.go](../../../internal/tui/shell/screen.go) (contract)

`Body(_ int, _ int)` discards both dimensions. The shell's documented
contract (`screen.go:23-26`) says _"Body receives the content-area dimensions
the shell has reserved … The shell controls layout; the screen renders into
the area it is given."_ This screen acknowledges neither: a tall window
leaves the 4-row menu floating at the top; a narrow window can wrap the
longest label onto a second line and break cursor alignment.

For an MVP this is acceptable, but the discarded parameters make the screen
quietly inconsistent with the rest of the shell (`notificationsScreen`,
`infoScreen`, `settingsScreen` all consume at least one dimension). No
comment notes the deliberate omission, so a reader can't tell whether the
screen is buggy or just lazy.

```go
func (s *welcomeScreen) Body(_ int, _ int) string {
    bar := styles.MutedStyle.Render("┃")
    rows := make([]string, 0, len(s.items)+1)
    rows = append(rows, bar+" "+styles.HeaderStyle.Render("Choose a task"))
    ...
}
```

- [ ] Add a one-line comment ahead of `Body` explaining that the menu is a
      fixed top-anchored block and the shell-supplied dimensions are
      intentionally unused.
- [x] Use `width` to truncate / pad label rows so a narrow terminal cannot
      wrap `Settings` and break cursor alignment.

## Navigation tests: missing `j`-wrap coverage and cursor-reset-inside-loop

> [!WARNING]
>
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md) — § "Test One Behavior At A Time", § "Keep Tests Isolated", § "Keep Test Code Simple"

Two issues bundle here because they share root cause (one screen reused
across multiple iterations):

1. `TestWelcomeScreen_DownWrapsFromBottom` only exercises `tea.KeyDown`. The
   matching `TestWelcomeScreen_UpWrapsFromTop` iterates both `up` and `k`, so
   the asymmetry is obvious side-by-side. The plan called out "wrap from top
   AND bottom" hitting both arrow + `j`/`k`; coverage is one-sided.
2. `TestWelcomeScreen_DownMovesCursor` and `TestWelcomeScreen_UpWrapsFromTop`
   reuse a single `newWelcomeScreen()` and call `s.cursor = 0` inside the
   loop body before each iteration. The "given" is half in construction and
   half buried in the loop body, and if the first iteration leaves state
   that affects the second, the test passes for the wrong reason.

```go
// missing j-key wrap
func TestWelcomeScreen_DownWrapsFromBottom(t *testing.T) {
    s := newWelcomeScreen()
    s.cursor = len(s.items) - 1
    _, _ = s.Update(tea.KeyPressMsg{Code: tea.KeyDown}) // no `j` case
    ...
}

// reset hidden in loop body
for _, kp := range []tea.KeyPressMsg{
    {Code: tea.KeyDown},
    {Code: 'j', Text: "j"},
} {
    s.cursor = 0
    _, _ = s.Update(kp)
    ...
}
```

- [x] Convert both tests to `t.Run` subtests with a fresh `newWelcomeScreen()`
      per case, mirroring `TestUpdate_GlobalKeysInterceptedBeforeScreen` in
      `shell_test.go`. Add the missing `j` case to `DownWrapsFromBottom`.
- [ ] Keep the loop shape but construct `s := newWelcomeScreen()` inside the
      loop instead of resetting `s.cursor`. Still add the `j` case to
      `DownWrapsFromBottom`.

## `[Back]` body assertion is brittle

> [!WARNING]
>
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md) — § "Assert Behavior, Not Mock Mechanics"

`TestSettingsScreen_TitleAndBody` asserts the back-button render with
`strings.Contains(body, "[") && strings.Contains(body, "ack]")` — split
because the mnemonic styling injects ANSI escapes between the highlighted
`B` and `ack]`. The workaround would also pass on bogus output such as
`[Xack]`, `[Quack]`, or `[ack]` — none of which contain a `B`. The assertion
only proves "a bracket and the letters `ack]` exist somewhere", not "the
mnemonic-styled [Back] button is rendered".

```go
if !strings.Contains(body, "[") || !strings.Contains(body, "ack]") {
    t.Errorf("Body missing [Back] button render\n%s", body)
}
```

- [x] Replace with `if !strings.Contains(body, s.back.View()) { ... }` so the
      test asserts the literal rendered button against the component's own
      output.
- [ ] Strip ANSI before substring matching (e.g. via a small local helper or
      `charm.land/x/ansi` if available) and assert `strings.Contains(plain,
"[Back]")`.

## `TestWelcomeScreen_TitleAndBody` bundles two unrelated promises

> [!WARNING]
>
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md) — § "Test One Behavior At A Time"

One test asserts (a) Title returns `"Agentfiles"`, (b) Body contains four
labels and the "Choose a task" header, (c) Body shows `> Settings` on the
selected row, and (d) Body does NOT show `> Profiles` on an unselected row.
Three of those are about body composition and one is about the title. The
plan listed them as separate tests (`TestWelcomeScreen_BodyContainsAllItems`
and `TestWelcomeScreen_TitleIsAgentfiles`); the implementation fused them. A
failure that breaks Body lumps the title in the test name and obscures which
rule actually broke.

```go
func TestWelcomeScreen_TitleAndBody(t *testing.T) {
    s := newWelcomeScreen()
    if got := s.Title(); got != "Agentfiles" { ... }
    s.cursor = 1
    body := s.Body(80, 10)
    for _, want := range []string{"Choose a task", "Profiles", "Settings", "Quit"} { ... }
    if !strings.Contains(body, "> Settings") { ... }
    if strings.Contains(body, "> Profiles") { ... }
}
```

- [x] Split into `TestWelcomeScreen_TitleIsAgentfiles`,
      `TestWelcomeScreen_BodyContainsAllItems`, and
      `TestWelcomeScreen_BodyMarksSelectedRow`, matching the plan.
- [ ] Leave fused for compactness; rename to `TestWelcomeScreen_Render` so
      the test name no longer over-promises.

## Esc handled via direct `kp.Code == tea.KeyEsc` instead of `key.Matches`

> [!WARNING]
>
> - [docs/guidelines/charm.md](../../../docs/guidelines/charm.md) — § "Key Bindings — Consistent and Visible": _"match keys with `key.Matches`, not by `msg.String()`"_

Both `settingsScreen.Update` (`settings.go:35`) and `profilesStub.Update`
(`profiles_stub.go:18`) handle Esc with a direct rune comparison. While this
isn't a `msg.String()` compare (it touches `kp.Code`, the rune field), it
sidesteps the `bubbles/key` discipline the rest of the shell follows
(`keys.go::handleGlobalKey`, `welcomeScreen.Update`, `confirm.go` defining
`Cancel: key.NewBinding(key.WithKeys("esc"))`).

The pattern is inherited from the pre-existing stubs the task replaces, so
it isn't a regression, but the task is the natural moment to bring these two
screens in line with the rest of the shell — both ship as the real Settings
type and a deliberate stub respectively, and both already import
`bubbles/v2/key`. Doing it now also gives task 0025 a consistent skeleton to
inherit when `profilesStub` is replaced.

```go
// current
if kp.Code == tea.KeyEsc {
    return s, popCmd()
}

// suggested
back := key.NewBinding(key.WithKeys("esc"))
...
if key.Matches(kp, back) {
    return s, popCmd()
}
```

- [x] In `settingsScreen`, fold "esc" into the `back` mnemonic binding's key
      list (so the same `s.back.Matches(kp)` covers both `b` and `esc`) and
      remove the explicit `kp.Code == tea.KeyEsc` branch. Update the help
      text to keep `b back` as the displayed mnemonic.
- [ ] Add a dedicated `escBinding key.Binding` field on both screens and
      route Esc through `key.Matches(kp, s.escBinding)`. Leaves the back
      mnemonic and the back-out semantics visually distinct in code.

## Welcome `up` / `down` bindings duplicate `globalKeyMap` key strings

> [!WARNING]
>
> - [docs/guidelines/solid.md](../../../docs/guidelines/solid.md) — § "Single Responsibility Principle" (one reason to change)

`welcomeScreen` declares `up: key.NewBinding(key.WithKeys("up", "k"))` and
`down: key.NewBinding(key.WithKeys("down", "j"))`. `globalKeyMap` declares
the same key strings as display-only hints for the status bar. The bar
advertises `↑/k` and `↓/j`; if either side's key list is edited in
isolation, the bar and the screen drift apart silently.

The SOLID guideline cautions against adding indirection for one consumer —
welcome is the only cursor-driven screen today — so promoting this to a
shared route now would be premature. Worth recording so the next cursor
screen (Profiles, task 0025) doesn't independently re-declare the same
strings a third time.

```go
// welcome.go
up:     key.NewBinding(key.WithKeys("up", "k")),
down:   key.NewBinding(key.WithKeys("down", "j")),

// keys.go advertises ↑/k and ↓/j as display-only globals
```

- [ ] Leave as-is for this task; record the duplication in the changelog so
      task 0025 sees the prior art and promotes the bindings to a shared
      helper (or to passing `globalKeyMap` into screens) when the second
      cursor consumer lands.
- [ ] Add a one-line comment above the `up` / `down` field declarations in
      `welcome.go` noting that the key strings must stay in sync with
      `globalKeyMap.Up` / `Down` until a shared helper exists.
- [x] Promote now: have `welcomeScreen` accept (or pull from `Model`) the
      global key map and reuse `Up` / `Down` directly. Adds the abstraction
      one task earlier than strictly needed but eliminates the drift surface.
