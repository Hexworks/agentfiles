# Move rendering logic to TUI — review

The task hits its main goals: domain packages return data + typed errors, the
TUI is the only producer of styled text, ADR 0007 captures the rationale, and
the refactor resolves all `// FIX: task#0005` markers. Tests for the new
behavior are present and pass; `go vet ./...` is clean.

The findings below cluster into four groups:

- **Correctness gaps that should be fixed before merge**: `flattenJoined`
  does not follow `Unwrap() error`, so a future joined cause inside
  `AssetRenderError` would render as a single mangled line; `doctor.CheckProfile`
  short-circuits on the first project failure, contradicting the ADR's
  "every problem at once" promise; map iteration in `render.Build` and
  `ensureProjectPathAvailable` is non-deterministic; `go mod tidy` was not
  run (termenv direct-require missing); the test-file `unwrapJoined`
  helpers have already drifted (`app/service_errors_test.go` is
  non-recursive while `render/render_test.go` is).
- **Design issues that limit the contract**: `severityOf` enumerates every
  typed error from four domain packages with a silent `sevInfo` fallback;
  `assetIDNameRequiredError` mixes a sentinel with the otherwise-uniform
  struct-error pattern, forcing a special `errors.Is` branch in the TUI;
  `changeStyle`/`severityStyle` return an ad-hoc anonymous interface
  instead of the concrete `lipgloss.Style`; `RenderError` silently
  downgrades unknown errors to info severity.
- **Domain/vocabulary**: `doctor.ProjectStatus.Changes` re-exports
  `llmsync.FileChange`; `app.UnknownAssetError` and
  `render.AssetNotFoundError` model the same situation with two names and
  two field names; the glossary did not gain entries for several new
  user-facing terms.
- **Test-coverage gaps and minor hygiene**: `resolveAssets` accumulation,
  per-asset `AssetRenderError` I/O wrapping, `TargetOutsideSurfacesError`,
  multi-conflict `ensureProjectPathAvailable`, mixed clean+dirty doctor
  report — all promised by §F of the plan but partially absent. Plus: bare
  lipgloss color codes, leftover `WARN`/`FIX`/`TODO` markers in touched
  files, and absolute filesystem paths leaking through wrapped
  `*os.PathError`s.

## flattenJoined ignores Unwrap() error chains

> [!WARNING]
>
> - [docs/guidelines/errors.md](../../../docs/guidelines/errors.md) — "the TUI walks that slice and dispatches per typed error so each entry gets its own icon and severity"

`internal/tui/render_errors.go:42` only recognizes `Unwrap() []error`.
`render.AssetRenderError` implements `Unwrap() error` (single). When
`addAssetOutputs` ever returns an `errors.Join`-built error wrapped in
`AssetRenderError{Err: joined}`, `flattenJoined` treats the wrapper as a
leaf — every inner error is fused into one styled line via
`%s` of `e.Err.Error()`, with literal `\n` characters embedded inside one
"cell". This is dormant today (no inner `Join` produced) but will silently
break the per-error icon/severity contract the moment it is.

```go
// internal/tui/render_errors.go
func flattenJoined(err error) []error {
    type multi interface{ Unwrap() []error }
    if m, ok := err.(multi); ok {
        var out []error
        for _, e := range m.Unwrap() {
            out = append(out, flattenJoined(e)...)
        }
        return out
    }
    return []error{err} // AssetRenderError ends here even if Err is a Join
}
```

- [x] Extend `flattenJoined` to also follow `Unwrap() error` so single-wrap typed errors do not hide a joined cause underneath.
- [ ] Or document explicitly on `AssetRenderError.Err` that it must always be a single typed error and assert it in render.

## doctor.CheckProfile short-circuits on the first project failure

> [!WARNING]
>
> - [docs/guidelines/errors.md](../../../docs/guidelines/errors.md) — "Loops over a collection should not short-circuit when one element fails"
> - [docs/adr/0007-rendering-belongs-to-tui.md](../../../docs/adr/0007-rendering-belongs-to-tui.md) — "The 'first error wins' surprises in render and app go away."

`internal/doctor/doctor.go:38-41` returns on the first `llmsync.Plan`
error. Doctor is the most important place to surface every problem at
once — the user wants the full picture before fixing anything. Neither
of the two documented short-circuit exceptions (shared I/O resource;
programming bug) applies; each project's plan is independent.

```go
for _, proj := range p.ProjectList() {
    preview, err := llmsync.Plan(p, proj)
    if err != nil {
        return nil, err  // first failure wins, rest of profile invisible
    }
    report.Projects = append(report.Projects, ProjectStatus{...})
}
```

- [x] Accumulate per-project failures via `errors.Join`; attach successful previews to a partial report and return both `(*Report, error)` so the TUI can render clean projects alongside failing ones. Wrap each per-project failure in a typed `doctor.ProjectCheckError{ProjectName, Err}` so the TUI can render "project X: <err>" without parsing strings.
- [ ] Or document explicitly that doctor intentionally fails fast (and update the ADR consequence section accordingly).

## Non-deterministic accumulation order in render.Build and ensureProjectPathAvailable

> [!WARNING]
>
> - [docs/guidelines/go.md](../../../docs/guidelines/go.md) — "Use Behavior-Focused Tests" (deterministic ordering is part of the contract)
> - [docs/guidelines/errors.md](../../../docs/guidelines/errors.md)

Two loops iterate maps before appending to the joined error:

- `internal/render/render.go:61` — `for group, ids := range groupSelections`. With multiple offending groups, the order of `ExclusiveGroupConflictError` entries inside the joined error is non-deterministic.
- `internal/app/service.go:210` — `loaded.Projects` is a `map[string]*project.Manifest`. Iteration order is non-deterministic; multi-conflict accumulation is therefore order-flaky.

Single-conflict tests pass today, but multi-conflict tests would flake,
and multi-line user output reorders between runs.

```go
for group, ids := range groupSelections { // map iteration: non-deterministic
    if unique := dedupSorted(ids); len(unique) > 1 {
        errs = append(errs, ExclusiveGroupConflictError{Group: group, AssetIDs: unique})
    }
}
```

- [x] Sort map keys before appending. For `groupSelections`, build a sorted slice of keys via `slices.Sorted(maps.Keys(...))` and iterate that. For `loaded.Projects`, reuse the existing `Profile.ProjectList()` helper (already name-sorted) and apply the same pattern in `ensureProjectPathAvailable`'s inner loops.
- [ ] Or accept order-instability and add a note that `errors.Join` order is unspecified — but this loses the "every issue, predictable order" UX promise.

## go mod tidy not run — termenv missing from direct requires

> [!WARNING]
>
> - [docs/guidelines/go.md](../../../docs/guidelines/go.md)

`internal/tui/main_test.go:9` imports `github.com/muesli/termenv`
directly (used to force `termenv.Ascii` for stable test output). Per
`go mod tidy -diff`, termenv must be promoted from indirect to direct
require alongside the (correct) lipgloss promotion.

```diff
 require (
   github.com/charmbracelet/lipgloss v1.1.0
+  github.com/muesli/termenv v0.16.0
 )
 ...
-  github.com/muesli/termenv v0.16.0 // indirect
```

- [x] Run `go mod tidy` and commit the updated `go.mod`/`go.sum`.

## unwrapJoined test helpers duplicated and already drifted

> [!WARNING]
>
> - [docs/guidelines/clean_architecture.md](../../../docs/guidelines/clean_architecture.md) — "Reuse / Release Equivalence Principle"
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md)

Three near-identical implementations of the joined-error walker exist:

- `internal/tui/render_errors.go:42` (`flattenJoined`, recursive — production)
- `internal/render/render_test.go:153` (`unwrapJoined`, recursive)
- `internal/app/service_errors_test.go:52` (`unwrapJoined`, **non-recursive**)

The two test copies have already drifted. A nested `errors.Join` in the
`app` test path would silently lose leaves. This is exactly the failure
mode REP/DRY warns about: "Copied code drifts, bug fixes are missed."

```go
// app/service_errors_test.go — single-level only; SILENTLY differs
func unwrapJoined(err error) []error {
    type multi interface{ Unwrap() []error }
    if m, ok := err.(multi); ok {
        return m.Unwrap()  // does not recurse
    }
    return []error{err}
}
```

- [x] Make the production `tui.flattenJoined` exported (or move to a small shared package) and have both test files call it. Delete the local copies.
- [ ] Or, if duplication is preferred, copy the recursive form into `app/service_errors_test.go` so all three implementations behave identically.

## severityOf enumerates every domain typed error in one switch

> [!WARNING]
>
> - [docs/guidelines/solid.md](../../../docs/guidelines/solid.md) — Open/Closed
> - [docs/guidelines/clean_architecture.md](../../../docs/guidelines/clean_architecture.md) — Stable Abstractions Principle

`internal/tui/render_errors.go:64` enumerates typed errors from `render`,
`asset`, `profile`, and `app`. Adding a new typed error in any domain
package requires editing this switch; if the edit is forgotten the new
error silently degrades to `sevInfo`. ADR 0007 deliberately rejected a
`Severity()` method on each error to keep presentation out of the domain
— that rejection is correct, but the centralized switch is the cost of
that rejection and the cost is currently uncovered.

```go
switch {
case errors.As(err, new(render.ExclusiveGroupConflictError)),
    errors.As(err, new(render.AssetNotFoundError)),
    errors.As(err, new(render.AssetRenderError)),
    errors.As(err, new(asset.UnsupportedAssetTypeError)),
    errors.As(err, new(profile.DuplicateAssetIDError)),
    errors.As(err, new(profile.DuplicateProjectIDError)),
    errors.As(err, new(app.UnknownAssetError)),
    errors.As(err, new(app.AssetExistsError)),
    errors.As(err, new(app.ProjectNotFoundError)):
    return sevError
case errors.As(err, new(render.TargetOutsideSurfacesError)),
    errors.As(err, new(app.ProjectPathOwnedError)):
    return sevWarning
}
if errors.Is(err, asset.ErrAssetIDNameRequired) { // dispatch style asymmetry
    return sevError
}
return sevInfo // unknown -> info: silent downgrade of real failures
```

- [ ] Replace the switch with a `map[reflect.Type]severity` (or per-package helper functions) and add a TUI test that constructs every domain typed error and asserts the rendered icon — so a new typed error without a registration entry fails the build.
- [ ] Or keep the switch but change the default to `sevError` (no operation that returns an unknown error reasonably wants an info icon) and add a coverage-style test that lists every typed error.
- [ ] Either way, document in `docs/guidelines/errors.md` that adding a typed domain error requires updating `severityOf`.
- [x] Severity is a domain concern, so it should be part of the domain object (the error that is produced). Remove the switch completely and assign severities to all errors.

## RenderError silently downgrades unknown errors to info severity

> [!WARNING]
>
> - [docs/guidelines/tui.md](../../../docs/guidelines/tui.md) — "render structured errors with severity, location, and likely next step when available"

`severityOf` falls through to `sevInfo` for any unknown error type, then
`renderOneError` prints with the `ℹ` icon. A user who hits an unexpected
I/O failure (e.g. `*os.PathError` from `Plan` or `Apply`) sees a cyan
informational note — visually identical to "no changes". The fallback
should at minimum be `sevError` because by definition the operation
failed.

```go
// internal/tui/render_errors.go
default:
    return sevInfo // an os.PathError now renders as info
```

- [ ] Default unknown error types to `sevError`, with a generic "internal" icon.
- [ ] Or introduce a fourth `sevUnknown` rendered as warning so it is visually distinct without claiming a known-bad classification.
- [x] severity is part of the domain so it can never be unknown (see previous issue). This is solved by adding severity to the domain object(s)

## assetIDNameRequiredError mixes sentinel with the rest of the typed-error pattern

> [!WARNING]
>
> - [docs/guidelines/errors.md](../../../docs/guidelines/errors.md) — "Each domain package owns an `errors.go` file with one struct per failure mode"
> - [docs/guidelines/solid.md](../../../docs/guidelines/solid.md) — LSP

`internal/asset/errors.go:8` exports `ErrAssetIDNameRequired` as a
sentinel of the **unexported** struct `assetIDNameRequiredError`. Every
other error in this task is an exported struct used with `errors.As`.
Out-of-package callers cannot do
`errors.As(err, new(assetIDNameRequiredError))` because the type is
unexported, so the TUI's `severityOf` is forced into a special trailing
`errors.Is` clause — two dispatch styles for one mechanism.

```go
var ErrAssetIDNameRequired = assetIDNameRequiredError{}
type assetIDNameRequiredError struct{}

// tui/render_errors.go — special case just for this one error
if errors.Is(err, asset.ErrAssetIDNameRequired) {
    return sevError
}
```

- [x] Export the struct as `AssetIDNameRequiredError struct{}` and use `errors.As` like every other typed error. Optionally retain `var ErrAssetIDNameRequired = AssetIDNameRequiredError{}` as a sentinel for `errors.Is` ergonomics.
- [ ] Or document explicitly in `docs/guidelines/errors.md` when a sentinel is preferred over a struct, with this case as the canonical example, and group all sentinel checks into a single `errors.Is` block in `severityOf` so the dispatch shape is uniform.

## changeStyle / severityStyle return an ad-hoc anonymous interface instead of lipgloss.Style

> [!WARNING]
>
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) — "Make intent visible in names, types, and function boundaries"
> - [docs/guidelines/solid.md](../../../docs/guidelines/solid.md) — Interface Segregation: "Use this principle to reduce coupling, not to create an interface for every struct"
> - [docs/guidelines/go.md](../../../docs/guidelines/go.md) — "Prefer Explicit Types Over Loose Maps"

`internal/tui/render_preview.go:32` and `internal/tui/render_errors.go:87`
both return `(string, interface{ Render(...string) string })`. The
underlying value is always `lipgloss.Style`, the package already imports
lipgloss in `styles.go`, and there is no second implementer. The
anonymous interface buys nothing; it is duplicated in two unrelated
files; it forecloses chaining other lipgloss methods on the result.

```go
func changeStyle(kind llmsync.ChangeKind) (string, interface{ Render(...string) string }) {
func severityStyle(s severity) (string, interface{ Render(...string) string }) {
```

- [x] Return `lipgloss.Style` directly from both functions.
- [ ] Or define one named alias in `tui/styles.go` (e.g. `type renderer = lipgloss.Style`) and reuse it.

## AssetRenderError leaks absolute filesystem paths through wrapped \*os.PathError

> [!WARNING]
>
> - [docs/guidelines/security.md](../../../docs/guidelines/security.md) — "keep ... private paths out of logs, previews, errors"
> - [docs/guidelines/tui.md](../../../docs/guidelines/tui.md) — "don't print low-level implementation details without explaining the user impact"

`internal/render/render.go:71` wraps `addAssetOutputs`'s raw return error
in `AssetRenderError{AssetID, Err}`. `Err` is whatever `os.ReadFile`,
`os.Stat`, or `filepath.WalkDir` produced — typically a `*fs.PathError`
embedding the absolute path under the user's profile root. The TUI's
`flattenJoined` does not descend into `Unwrap() error`, so the leaf
rendered to the terminal is the full
`<asset-id>: open /home/alice/profiles/work/assets/skill/.../SKILL.md: permission denied`.

The asymmetry with `TargetOutsideSurfacesError` is also worth noting:
one specific filesystem-adjacent case is typed; everything else is an
opaque wrapper.

```go
errs = append(errs, AssetRenderError{AssetID: a.ID, Err: err})

func (e AssetRenderError) Error() string {
    return fmt.Sprintf("%s: %s", e.AssetID, e.Err.Error())
    // -> "skill-foo: open /home/alice/profiles/.../SKILL.md: permission denied"
}
```

- [x] Introduce typed sub-errors (`AssetSourceMissingError{AssetID, RelPath}`, `AssetReadError{AssetID, RelPath, Op, Err}`) at the I/O sites in `render.go`, carrying only the asset-relative path. The TUI then renders a humane message without absolute paths.
- [ ] Or, in `render.go`, redact the profile-root prefix from `*os.PathError` strings before wrapping them in `AssetRenderError`.

## Manifest-sourced strings reach the terminal without sanitization

> [!WARNING]
>
> - [docs/guidelines/security.md](../../../docs/guidelines/security.md) — "Treat External Input As Untrusted" / "reject unknown or unsafe path forms early"

User-controlled strings (project path, project name, asset id, profile
name, change paths/reasons, error messages) flow into the new render
helpers via `fmt.Fprintf(&b, "%s ...", style.Render(...))` and `fmt.Print`.
`lipgloss.Render` does not strip ANSI control bytes from its input, so a
manifest with `"name": "evil[2J]0;pwned"` passes
through verbatim, allowing arbitrary screen clears, OSC title changes,
BEL, cursor moves, or injected SGR sequences. `profile.Manifest`,
`project.Manifest`, and `asset.Manifest` do not validate string fields
for control characters at load time.

```go
// internal/tui/render_report.go
fmt.Fprintf(&b, "%s\n", headerStyle.Render("["+status.Name+"]"))
// status.Name comes from project.Manifest.Name (free-form file content)

// internal/tui/render_errors.go
return style.Render(fmt.Sprintf("%s %s", icon, err.Error()))
// err.Error() embeds e.Path / e.AssetID / e.ProfileName as raw strings
```

- [ ] Validate manifest text fields at load time: reject ASCII control characters (0x00-0x08, 0x0b-0x1f, 0x7f) with a typed error in `Manifest.Validate()`. Untrusted manifests then never reach the renderer.
- [x] Or add a `safe(s string) string` helper in `internal/tui/` that strips control bytes before any `style.Render` / `fmt.Fprintf`, and apply it to every user-supplied string in the three render entry points.

## Lipgloss color codes are bare numeric strings

> [!WARNING]
>
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) — "Replace magic numbers and strings with named constants when the value has meaning outside one local expression"

`internal/tui/styles.go` uses raw ANSI palette indices with comment-only
color names. The same code (`"9"` = red) appears in two styles
(`deleteStyle`, `errorStyle`) and `"10"` in two more (`cleanStyle`,
`createStyle`); a palette change must be done by hand twice.

```go
cleanStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("10")) // bright green
createStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10")) // green
deleteStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))  // red
errorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true) // also red
```

- [x] Define named color constants (`colorRed lipgloss.Color = "9"`, `colorGreen = "10"`, etc.) and reference those.
- [ ] Or accept the comments as sufficient documentation and keep the palette inline.

## Pre-existing WARN / FIX / TODO comments left in touched files

> [!WARNING]
>
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) — "Leave touched code clearer than you found it" + "Do not keep commented-out code. Remove it."

The plan's "Notes on scope" section leaves these markers explicitly out
of scope. They are still present in files this task modified:

```go
// internal/asset/asset.go — Manifest.CompatibleAgents
// FIX: This should be a concrete type (eg: CompatibleAgent) ...

// internal/asset/asset.go — Init
// TODO: This is only temporary. We need to create a new module ...

// internal/profile/profile.go — scanProjects
// WARN: Not too readable, extract this to an expressive function

// internal/render/render.go — addAssetOutputs
// TODO: Refactor this function **and `addSkillOutputs`** ...
```

These are vague meta-comments; either resolve them or move them to the
issue tracker.

- [x] Convert each marker to a tracked task in `tasks/backlog/` and link by id from a one-line comment that survives.
- [ ] Or do the trivial extraction the `WARN` in `profile.scanProjects` asks for: introduce `func isProjectManifestFile(entry os.DirEntry) bool` and drop the WARN.
- [ ] Or accept current state and document in CLAUDE.md that these markers describe non-task#0005 work.

## doctor.ProjectStatus.Changes re-exports llmsync.FileChange

> [!WARNING]
>
> - [docs/guidelines/clean_architecture.md](../../../docs/guidelines/clean_architecture.md) — Stable Dependencies Principle
> - [docs/guidelines/domain_model.md](../../../docs/guidelines/domain_model.md) — "Keep Boundaries Clear"

`internal/doctor/doctor.go:24` exports `Changes []llmsync.FileChange`.
Any consumer of doctor's report — TUI today, future "export-to-JSON"
command tomorrow — must also import `internal/sync`. doctor and sync are
sibling packages; doctor effectively re-exports sync's vocabulary so
future changes to `FileChange` ripple through both packages and every
TUI render path.

```go
type ProjectStatus struct {
    Name    string
    Changes []llmsync.FileChange  // sync vocabulary surfaced via doctor
}
```

- [x] Define a doctor-owned `ProjectChange` value object (kind, path, reason) and convert `llmsync.FileChange -> doctor.ProjectChange` in `CheckProfile`. doctor stops re-exporting sync types.
- [ ] Or move `FileChange`/`ChangeKind` into a neutral package (e.g. `internal/changes`) so both `sync.Preview.Changes` and `doctor.ProjectStatus.Changes` reference the same type without doctor importing sync.
- [ ] Or document the chosen direction in ADR 0007 ("doctor consumes sync deliberately") and add a code comment on `ProjectStatus.Changes` saying so.

## UnknownAssetError vs AssetNotFoundError model the same situation twice

> [!WARNING]
>
> - [docs/guidelines/domain_model.md](../../../docs/guidelines/domain_model.md) — "Use The Project Language" / "don't invent parallel names for established concepts"

Two error types describe the same concept — a referenced asset id that
does not exist in the profile — with two names and two field names:

```go
// internal/app/errors.go
type UnknownAssetError struct{ AssetID string }
// "unknown asset: %s"

// internal/render/errors.go
type AssetNotFoundError struct{ ID string }
// "selected asset not found: %s"
```

The TUI lists both as `sevError`, so callers see no semantic difference
— only inconsistent vocabulary. Other typed errors in the task already
standardize on `AssetID` (`AssetExistsError`, `AssetRenderError`).

- [x] Pick one name (`AssetNotFoundError` aligns with stdlib `os.ErrNotExist` style) and use it from both packages, normalizing the field to `AssetID`.
- [ ] Or keep two distinct types but rename them to express _why_ they differ (e.g. `app.AssetSelectionUnknownError` for "user picked an id that isn't in the profile yet" vs `render.AssetMissingError` for "manifest references an id that disappeared after load") — and standardize the field name.

## Glossary missing entries for several new user-facing terms

> [!WARNING]
>
> - [docs/guidelines/domain_model.md](../../../docs/guidelines/domain_model.md) — "update the glossary when a new durable domain term appears"

`Report` and `Typed Domain Error` were added; several others were not.
Concepts users will see in error output but cannot look up:

- `ProjectStatus` (referenced from `Report` glossary entry but never defined)
- `Severity` (the user-visible classification driving every error icon and color)
- `Change Kind` / `File Change` (`create`/`update`/`drift`/`delete` are the primary user-facing vocabulary in previews)

The `Report` entry itself is also a generic noun; if a future feature
emits any other structured summary the vocabulary will drift.

- [x] Add `ProjectStatus`, `Severity`, `Change Kind`, and `File Change` glossary entries.
- [ ] Rename the `Report` glossary entry to `Profile Health Report` (keep `Report` as a short form) and update ADR 0007 to use the precise term.
- [ ] Or accept the gap and add only the entries that match concrete code symbols (`ProjectStatus`, `Severity`).

## Missing render tests promised by plan §F

> [!WARNING]
>
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md) — "Start With The Smallest Useful Test" / "name tests after the behavior and expected outcome"

Plan §F lists `TestResolveAssets_AccumulatesMissingIDs` alongside
`TestBuild_AccumulatesMissingAssets`. Only the latter is present.
`AssetRenderError` (the per-asset filesystem-failure wrap) is never
exercised — no test scaffolds an asset whose `addAssetOutputs` step
fails and asserts `errors.As(err, &AssetRenderError{}) && typed.AssetID
== "..."`. `TargetOutsideSurfacesError` likewise only has a
severity-rendering test in `render_errors_test.go`; no render-level
test asserts it is actually returned when a generic projection points
outside the fence.

```go
// internal/render/render.go
errs = append(errs, AssetRenderError{AssetID: a.ID, Err: err})
// no test reaches this line
```

- [x] Add `TestResolveAssets_AccumulatesMissingIDs` (calls the unexported `resolveAssets` directly), `TestBuild_WrapsPerAssetFailureWithAssetRenderError` (skill manifest with no `SKILL.md` body), and `TestBuild_TargetOutsideSurfacesIsTyped` (rule asset with a projection target outside `surfaces.IsAllowed`).
- [ ] Or accept the gaps and update plan §F to remove the unfulfilled entries.

## Missing app + doctor tests for accumulation paths

> [!WARNING]
>
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md)

Plan §F also lists `TestEnsureProjectPathAvailable_AccumulatesConflicts`
— but `project_ownership_test.go` covers only single-conflict cases;
the accumulation behavior (the entire reason
`ensureProjectPathAvailable` was rewritten) has no test.
`TestAddProject_AccumulatesUnknownAssets` proves both-missing but never
the mixed valid+invalid case (which `service.go:127` rejects atomically
— a meaningful contract not pinned by any test).
`render_report_test.go` covers a one-project clean and a one-project
dirty report separately; the realistic doctor case (one profile with
multiple projects, some clean, some dirty) is uncovered.

- [x] Add `TestEnsureProjectPathAvailable_AccumulatesAcrossMultipleProfiles` (three profiles, two own the path, third tries to register), `TestAddProject_RejectsMixedKnownAndUnknownAssets` (asserts the project file is not written), and `TestRenderReport_MixedCleanAndDirtyProjects`.
- [ ] Or accept the gaps as "loops are trivial" and note this in the changelog.

## Make sure that only domain errors leave the service

Currently there are some service functions that accumulate errors into strings. This is not what the intent was behind this change. The domain errors should be accumulated then returned so as opposed to using `Join` and returning an `error` string we should return `(FunctionResult, DomainError[])` instead.

- [x] Refactor functions that return joined error strings to returning slices containing domain errors and let the TUI handle the conversion to a string representation
