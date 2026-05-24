# Modal component review (task 0014)

Seven parallel reviews (security, clean code, clean architecture, SOLID, DDD,
testing, Go) ran against
[`internal/tui/components/modal`](../../../internal/tui/components/modal).
Security came back empty — the package is an in-process leaf UI component
with no I/O, no path/manifest parsing, no secrets, and no execution
surface. `make fmt`/`make lint`/`go test ./internal/tui/components/modal/...`
all pass; coverage is **77.6 %** with notable gaps. The findings below are
real defects worth fixing before consumers start depending on the
component (the upcoming `0002 use-wizards` task is the first such
consumer).

The ten issues fall into three buckets:

1. **Contract fragility** — `Content.Done()`'s tri-state-as-two-bools, the
   silent `nil`-content panic, and `huh.Form` leaking through
   `ResolvedMsg.Value`. These compound as more `Content` types arrive.
2. **Test gaps** — `WithStyle`/`WithZ`/`Layer`-clamps have zero
   coverage; `drainResolved`'s batched-cmd branch is dead code that the
   changelog claims is exercised.
3. **Style and doc drift** — `defaultStyle` hard-codes a hex literal
   divergent from `internal/tui/styles.go`, the arc42 building-block view
   was not updated, and several godoc comments restate the code instead of
   capturing intent.

Pick **one** checkbox under each `## issue` block before coming back.

## `New(id, nil, …)` silently panics on first use

> [!WARNING]
>
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) (Understandability — boundary checks in one obvious place)

`New` at
[`internal/tui/components/modal/modal.go:81-92`](../../../internal/tui/components/modal/modal.go)
accepts `content Content` and stores it without any precondition check.
`Init`, `Update`, and `View` then dereference `m.content` directly. A caller
that hands `New` a `nil` content (easy to do during refactors or when an
upstream constructor returns `nil` on error) gets a nil-pointer panic deep
inside `Modal.Update`, not at the construction site. The asymmetry with the
package doc — which says "nil = closed" for the _parent's_ modal field —
makes this worse: nil-as-sentinel is meaningful one level up but unsafe
inside the constructor.

```go
func New(id string, content Content, opts ...Option) *Modal {
    m := &Modal{
        id:      id,
        content: content, // no nil check
        style:   defaultStyle,
        z:       10,
    }
    ...
}
```

Suggestions (pick one):

- [x] Add `if content == nil { panic("modal: nil content") }` at the top of `New` so misuse fails loudly at construction.
- [ ] Document the invariant on `New` ("`content` must be non-nil") and trust callers. No code change.

## `Content.Done()` encodes a tri-state through two booleans

> [!WARNING]
>
> - [docs/guidelines/clean_architecture.md](../../../docs/guidelines/clean_architecture.md) (Stable Abstractions Principle)
> - [docs/guidelines/solid.md](../../../docs/guidelines/solid.md) (LSP — illegal combos cannot be expressed by the type)

The `Content` contract at
[`internal/tui/components/modal/modal.go:28-36`](../../../internal/tui/components/modal/modal.go)
declares `Done() (done, confirmed bool, value any)`. The shape encodes
three states (not-done, confirmed, cancelled) as `(bool, bool, any)` with
two illegal combinations (`(false, true, *)` and `(false, false, non-nil)`)
that the type system does not forbid. The doc has to spell out the filter
in prose.

```go
// Done reports whether the content has resolved. When done is true the
// modal will emit a ResolvedMsg carrying confirmed and value. A
// cancelled resolution should return (true, false, nil).
Done() (done bool, confirmed bool, value any)
```

Every future `Content` implementation (form is the first; confirm dialog,
picker, custom wizard will follow as 0002 lands) has to re-discover the
encoding. A small enum or struct collapses the surface and lets
`Modal.Update` switch on a single tag.

Suggestions (pick one):

- [x] Replace `Done() (bool, bool, any)` with `Resolution() (state ResolutionState, value any)` where `ResolutionState` is `Active`, `Confirmed`, `Cancelled` — mirrors `huh.FormState`, no ambiguous bool pair, and the test sentinel from the form-test smell below disappears.
- [ ] Keep three returns but introduce a `type Resolution struct { Done, Confirmed bool; Value any }` and return that single value, so future call sites can pattern-match on a named record.
- [ ] Leave as-is for now (one implementation) but lock the contract by panicking inside `Modal.Update` if `done == false && confirmed == true` (or the inverse), turning the prose invariant into a runtime check.

## `huh.Form` leaks out through `ResolvedMsg.Value` (weak anti-corruption layer)

> [!WARNING]
>
> - [docs/guidelines/domain_model.md](../../../docs/guidelines/domain_model.md) (Keep boundaries clear)
> - [docs/guidelines/charm.md](../../../docs/guidelines/charm.md) §Modal Overlays — "Value carries the typed payload the caller needs"

`formContent.Done()` at
[`internal/tui/components/modal/form.go:27-36`](../../../internal/tui/components/modal/form.go)
returns the entire `*huh.Form` as `value`, and the package doc instructs
callers to "read field values via `form.GetString(key)` etc." in their
`ResolvedMsg` handler. The lifecycle adaptation is a clean ACL — but the
payload channel breaks the seal: every consumer of `NewForm` will
type-assert to `*huh.Form` and call `huh` methods directly. If `huh`
changes its accessor surface (`v2` → `v3`, key rename), the blast radius
becomes every TUI screen that opened a form-modal, not this one adapter.

```go
case huh.StateCompleted:
    return true, true, f.form // the huh.Form itself escapes
```

Suggestions (pick one):

- [x] Have `NewForm` accept an extractor `extract func(*huh.Form) any` stored on `formContent`; `Done()` returns `extract(f.form)`. Each caller defines a typed result struct (e.g. `CreateProfileResult{Name, Path string}`) so `*huh.Form` does not leave `package modal`. Update the `NewForm` godoc accordingly.
- [ ] Keep the current signature but rename `Value` → `Payload`, document in `ResolvedMsg`'s godoc that for `NewForm`-built modals the payload is `*huh.Form`, and add an example in the package godoc showing the screen-side conversion at the boundary.
- [ ] Accept the leak as a deliberate cost (caller convenience > seal strength) and add a one-line note in `charm.md` §Modal Overlays calling it out as a known trade-off.

## `defaultStyle` hard-codes a hex literal and lives as package-level mutable state

> [!WARNING]
>
> - [docs/guidelines/charm.md:676-700](../../../docs/guidelines/charm.md) — "Define styles once in a shared module ... Avoid inline `lipgloss.NewStyle()` chains"
> - [docs/guidelines/clean_architecture.md](../../../docs/guidelines/clean_architecture.md) (Common Closure Principle)
> - [docs/guidelines/solid.md](../../../docs/guidelines/solid.md) (OCP — two ways to vary the same behavior)

[`internal/tui/components/modal/modal.go:74-77`](../../../internal/tui/components/modal/modal.go)
declares the default modal style:

```go
var defaultStyle = lipgloss.NewStyle().
    Border(lipgloss.RoundedBorder()).
    BorderForeground(lipgloss.Color("#874BFD")).
    Padding(1, 2)
```

Two problems land in one declaration:

1. **Hex literal divergent from the project palette.** `internal/tui/styles.go`
   is the single edit-point for the palette and deliberately uses ANSI
   indices, not hex. A future theme change will touch `styles.go` and miss
   this — the next palette tweak will leave the modal off-theme.
2. **Mutable package-level state.** The `var` (it cannot be `const` for a
   `lipgloss.Style`) invites the alternative path "mutate
   `defaultStyle` once at startup", which collides with the documented
   `WithStyle` extension point. Two ways to vary the same behavior is
   exactly the OCP smell.

The plan argues the modal must stay a leaf to avoid an import cycle into
`internal/tui` — that's correct. The resolution is to invert the
dependency: callers pass `modal.WithStyle(styles.ModalBorder)` from
`internal/tui`, and the package-level fallback uses no color at all.

Suggestions (pick one):

- [x] Replace the `var` with `func defaultStyle() lipgloss.Style { return ... }` (no hex literal — just border + padding) invoked from `New`. Add `Modal` (border) entry to `internal/tui/styles.go` and have callers pass it via `WithStyle`. Closes both problems at once.
- [ ] Inline the literal directly into `New` (drop the package-level identifier) and keep the hex temporarily; add a `// also touch: internal/tui/styles.go` cross-reference so the next palette change is not silently inconsistent.
- [ ] Leave `defaultStyle` as a `var` but drop the hex (`BorderForeground` removed entirely), making the default near-monochrome so any caller wanting color must opt in via `WithStyle`. The next palette refactor then naturally adds the project color through that knob.

## `Layer` clamp branches and `WithStyle` / `WithZ` options have zero test coverage

> [!WARNING]
>
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md) (Start with the smallest useful test; cover edge branches)

`go test ./internal/tui/components/modal/... -cover` reports **77.6 %**
with the gaps concentrated in code paths the package's own contract
mentions:

- `modal.go:62 WithStyle` and `modal.go:68 WithZ` — the options loop in
  `New` is at 75 % because no test passes an option. `WithZ` controls
  overlay stacking, the package's entire reason for existing.
- `modal.go:141-146` — the `if x < 0 { x = 0 }` / `if y < 0 { y = 0 }`
  clamps in `Layer` are never hit.
  `TestModal_RenderCompositesOverBackground` uses a 60×20 canvas against a
  small modal, well inside bounds.
- `modal.go:98 Modal.Init` and `form.go:15 formContent.Init` — the
  documented "open the modal" pattern (`m.modal = newWizard();
return m, m.modal.Init()`) is what kicks off huh's startup commands,
  but no test exercises that wiring.

```go
func (m *Modal) Layer(parentW, parentH int) *lipgloss.Layer {
    view := m.View()
    w := lipgloss.Width(view)
    h := lipgloss.Height(view)
    x := (parentW - w) / 2
    y := (parentH - h) / 2
    if x < 0 { x = 0 } // never executed by any test
    if y < 0 { y = 0 } // never executed by any test
    return lipgloss.NewLayer(view).ID(m.id).X(x).Y(y).Z(m.z)
}
```

Suggestions (pick one):

- [x] Add three tests: `TestModal_LayerClampsToZeroWhenContentExceedsCanvas` (assert `X()/Y() == 0` for an oversized modal), `TestModal_OptionsApply` (`WithZ(99)` → `m.Layer(...).Z() == 99`; `WithStyle(custom)` → rendered output contains the custom border), and `TestModal_InitDelegatesToContent` (fake returns a sentinel cmd; assert `m.Init()()` produces its message). Lifts coverage and pins the contracts named in godoc.
- [ ] Add only the `Layer` clamp test and the options test; leave `Init` uncovered as a known gap.

## `drainResolved`'s batched-cmd branch is dead code

> [!WARNING]
>
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md) (Keep test code simple; test what runs)

[`modal_test.go:148-172`](../../../internal/tui/components/modal/modal_test.go)
contains a `tea.BatchMsg` walking branch in `drainResolved`, and the
changelog at
[`docs/changelog/2026-05-24_0014-create-modal-component.md:65-68`](../../../docs/changelog/2026-05-24_0014-create-modal-component.md)
claims the helper "has to be able to pull a `ResolvedMsg` out of either a
bare cmd or a batch." But `fakeContent.returnCmd` is declared at
`modal_test.go:18` and never assigned in any test. Every test therefore
takes the `cmd == nil` branch in `Modal.Update` at `modal.go:120-122`. The
batch path in production code, and the matching arm of `drainResolved`,
are both untested.

```go
type fakeContent struct {
    view      string
    updates   int
    lastMsg   tea.Msg
    doneFn    func() (bool, bool, any)
    returnCmd tea.Cmd // declared, never set
}
```

Suggestions (pick one):

- [x] Add `TestModal_BatchesContentCmdWithResolve`: set `fake.returnCmd = func() tea.Msg { return "from-content" }`, mark `doneFn` to return done, drive one `Update`, assert the resulting cmd produces a `tea.BatchMsg` containing both the inner msg and a `ResolvedMsg`. Closes the gap and validates the changelog claim.
- [ ] Delete the `tea.BatchMsg` arm of `drainResolved` _and_ the matching claim in the changelog, on the grounds that no test exercises it and no consumer relies on it yet. The arm comes back when the first consumer needs it.

## Sentinel `"form"` in `form_test.go` table smuggles a second assertion

> [!WARNING]
>
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md) (Test one behavior at a time)
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) (Opacity)

[`form_test.go:29-77`](../../../internal/tui/components/modal/form_test.go)
sets `wantValue: "form"` as a string sentinel that the assertion block
reinterprets via `switch want := tc.wantValue.(type) ... if want == "form"
{ if value != form { ... } }` to mean "check pointer identity". The table
case for `StateCompleted` therefore asserts two unrelated things at once:
the resolution mapping _and_ the identity preservation. A reader has to
scroll to the switch to understand the literal is not a value but a tag
for a special assertion path. The same row leaves `confirmed` and `done`
correctness coupled to identity correctness — any single regression drops
the whole row.

The end-to-end test `TestNewForm_ResolvedMsgCarriesFormOnCompletion` at
`form_test.go:82-100` already exercises identity through `Update`, so the
table is duplicating the assertion under a worse encoding.

```go
{
    name:      "completed → confirmed with form payload",
    state:     huh.StateCompleted,
    wantDone:  true,
    wantConf:  true,
    wantValue: "form", // sentinel, not the actual expected value
},
```

Suggestions (pick one):

- [ ] Drop the `string` arm of the assertion entirely; let the table assert only `(done, confirmed, value == nil)` and rely on `TestNewForm_ResolvedMsgCarriesFormOnCompletion` for identity.
- [x] Replace the sentinel with a typed flag — `wantFormIdentity bool` on the case struct — so what the row asserts is visible from the data row, not buried in a type switch.
- [ ] Resolves automatically if the `Done()` redesign in the contract issue above is taken (the table itself goes away).

## `Modal.Update` mixes four concerns; some godoc just restates the code

> [!WARNING]
>
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) (Functions — small, one job, one abstraction level; Comments — don't restate what code says)
> - [docs/guidelines/solid.md](../../../docs/guidelines/solid.md) (SRP)
> - Project `CLAUDE.md` — "default to no comments unless the WHY is non-obvious"

[`modal.go:105-124`](../../../internal/tui/components/modal/modal.go) does
four things in 20 lines: gate on `resolved`, delegate to content, poll
`Done()`, and branch on whether to issue a bare resolve cmd or batch it
with the inner cmd. The capture `id := m.id` exists for closure
correctness but reads as a puzzle (why not also `confirmed`/`value`?
because those are already locals from `Done()` — fine, but unexplained).
The `if cmd == nil { ... }` branch hand-rolls what `tea.Batch` already
handles in v2 (a single non-nil arg returns itself; nil entries are
dropped).

Separately, the godoc on `Init` at `modal.go:97-99` and `ID` at
`modal.go:94-95` restate what the code already says, against the project
CLAUDE rule.

```go
m.resolved = true
id := m.id
resolveCmd := func() tea.Msg {
    return ResolvedMsg{ID: id, Confirmed: confirmed, Value: value}
}
if cmd == nil {
    return m, resolveCmd
}
return m, tea.Batch(cmd, resolveCmd)
```

Suggestions (pick one):

- [ ] Extract `func (m *Modal) resolveCmd(confirmed bool, value any) tea.Cmd` and let `Update` end with `return m, tea.Batch(cmd, m.resolveCmd(confirmed, value))` unconditionally. Drop the `if cmd == nil` branch. Also drop the godoc on `Init` and `ID`.
- [x] Keep the structure but add two one-line comments: above `id := m.id`, explain "closure outlives Update; aliased because `m.id` is referenced through `m`"; above `tea.Batch(cmd, resolveCmd)`, note "independent; ResolvedMsg and follow-ups from content may interleave". Drop the restating godoc on `Init`/`ID`.
- [ ] Comment-only fix: leave the function shape, drop the godoc on `Init`/`ID`, leave the closure capture without comment.

## `z: 10` is an unnamed magic number duplicated across two sites

> [!WARNING]
>
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) (Naming — replace magic numbers with named constants)

The default z-index `10` appears in
[`modal.go:86`](../../../internal/tui/components/modal/modal.go) (inside
`New`) and is referenced as a literal in the godoc of `WithZ` at
`modal.go:66` ("Defaults to 10"). If the default ever changes, both sites
drift. The value `10` is also arbitrary; naming it makes the intent
("leave headroom for a background ID at 0 and let parents stack a few
layers below us") expressible in a comment.

```go
m := &Modal{
    id:      id,
    content: content,
    style:   defaultStyle,
    z:       10, // magic
}
```

Suggestions (pick one):

- [ ] Add `const defaultZ = 10` at package scope; reference it from `New` and rewrite the `WithZ` godoc as "Defaults to `defaultZ`."
- [x] Drop the explicit `z: 10` and let `z` default to `0`. Document in `WithZ` that callers using multiple stacked modals must set z explicitly. Removes the constant entirely.

## arc42 building-block view does not describe `internal/tui/components/`

> [!WARNING]
>
> - [docs/guidelines/documentation.md](../../../docs/guidelines/documentation.md) — "the first job of a document is to describe the current system accurately"
> - [docs/architecture/05-building-block-view.md](../../../docs/architecture/05-building-block-view.md)

[`05-building-block-view.md:126-139`](../../../docs/architecture/05-building-block-view.md)
describes `tui` as a flat package set. The Level 1 mermaid graph and the
package responsibilities section both omit `internal/tui/components/`. The
new sub-tree has its own dependency surface (`charm.land/{bubbletea,
lipgloss,huh}/v2`) and a new conceptual role (reusable leaf UI
components, no internal imports) — exactly the kind of structural change
arc42 chapter 5 is meant to capture. The changelog explicitly claims "no
doc updates beyond the changelog and plan.md", which contradicts
`documentation.md`.

Suggestions (pick one):

- [ ] Extend the existing `### tui` entry with one sentence: "Reusable Bubble Tea widgets live in `internal/tui/components/<name>/`; each is a leaf with no internal imports." Skip a graph node for now.
- [x] Add a short subsection to `docs/architecture/05-building-block-view.md` titled `### tui/components/modal`, plus a `tui --> tui/components/modal` edge in the Level 1 mermaid graph. More work, but the components/ namespace will grow as 0002–0003 land.
- [ ] Skip the doc update; accept the drift. (Records the user's decision so a future review does not re-flag it.)
