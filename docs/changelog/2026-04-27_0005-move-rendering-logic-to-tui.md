# 0005 changes

Moved every user-facing string out of the domain layer and replaced ad-hoc
`fmt.Errorf` strings with typed error structs that carry their context as
fields. Loops that previously short-circuited on the first failure now
accumulate via `errors.Join` and surface every issue together. The TUI is
now the only layer that turns domain values into styled output, with a new
`internal/tui/styles.go` for lipgloss styles and three render entry points
(`RenderPreview`, `RenderReport`, `RenderError`).

The refactor resolves all 11 `// FIX: ... @see task#0005` markers across
`internal/{asset,profile,render,app,doctor,sync}` and is documented in
[ADR 0007](../adr/0007-rendering-belongs-to-tui.md). New error convention
in [`docs/guidelines/errors.md`](../guidelines/errors.md).

## Decisions

- **Typed errors live in their package, not in a shared `errs/` package.** —
  **Why:** clean architecture says rules belong with the data they constrain.
  Each domain package's `errors.go` lists exactly the failure modes that
  package can produce, which keeps the surface discoverable without forcing
  a cross-cutting import.
- **Aggregate with `errors.Join`, not a custom `MultiError`.** — **Why:**
  the standard library already exposes `Unwrap() []error` on the joined
  value, so `errors.As` walks it for free. Adding our own collector would
  reinvent that mechanism without payoff.
- **`AssetRenderError` keeps fail-fast semantics inside `addAssetOutputs`.**
  — **Why:** the inner loop runs filesystem reads on the same asset; one
  failure already invalidates the result, so accumulation belongs at the
  `Build` level. The outer `Build` collects per-asset errors so multiple
  asset failures still produce one bundled error.
- **`doctor.CheckProfile` returns `*Report`, not `(string, error)`.** —
  **Why:** doctor was the last domain function building a string. Returning
  structured data lets the TUI add icons/colors and lets future doctor
  features (drift counts, last-applied timestamp) extend the report without
  breaking callers.
- **TUI tests force ASCII via `lipgloss.SetColorProfile(termenv.Ascii)`.** —
  **Why:** rendering output with embedded ANSI escapes is brittle to assert
  on. Stripping color in tests keeps assertions on plain substrings while
  keeping production styling intact.

Considered and rejected:

- A separate `internal/errs` package for shared error helpers — pulled all
  domain packages into a circular import path and offered no real reuse,
  since each error type carried package-specific fields.
- Keeping `sync.FormatPreview` and adding a parallel TUI renderer — would
  have left two formatting functions to maintain; deletion was cleaner.
- Returning typed errors as `interface{ Error() string; Severity() int }`
  with TUI-aware methods — would have leaked presentation back into the
  domain. Severity assignment now lives only in `render_errors.go`.

## Assumptions

- **lipgloss is safe to promote to a direct require.** — **Why:** it was
  already a transitive dependency via `huh`; the TUI render functions
  import it directly, so `go mod tidy` promotes it explicitly.
- **No external consumer relied on `sync.FormatPreview`.** — **Why:** it
  was only called from `internal/tui/forms.go`; `grep` found no other
  usage. Removing it is internal API churn only.
- **`asset.ErrAssetIDNameRequired` as a sentinel value (not a struct) is
  acceptable.** — **Why:** there is no extra context to attach beyond the
  bare condition; callers introspect with `errors.Is`. Using a singleton
  matches the standard-library convention (`io.EOF`, `os.ErrNotExist`).

## Bug fix surfaced during refactor

`ensureProjectPathAvailable` previously only blocked **cross-profile**
project-path collisions. Adding a second project at the same path inside
the same profile silently overwrote `.agentfiles/state.json` on apply.
The check now iterates the active profile's projects too, so any
collision (same profile or cross profile) returns
`ProjectPathOwnedError`. The error type gained a `ProjectName` field so
the message names the offending owner. Covered by
`TestProjectPathCannotBeSharedWithinSameProfile`.

## Other Notes

- arc42 §5 (`docs/architecture/05-building-block-view.md`) updated: render
  / sync / doctor / app / tui blocks now describe the metadata-and-typed-
  errors split.
- arc42 §8 (`docs/architecture/08-concepts.md`) gained two cross-cutting
  concepts: "Rendering Lives In The TUI" and "Typed Errors With
  Accumulation".
- `docs/glossary.md` gained "Report" and "Typed Domain Error".
- `docs/guidelines/go.md` "Return Actionable Errors" section now points at
  `errors.md` and shows a typed return.
- `CLAUDE.md` "Open refactor markers" section was replaced with an
  "Errors and rendering" section reflecting the new convention.

## Change 1

Move rendering out of `sync`. `FormatPreview` is deleted; the TUI exposes a
styled equivalent.

```go
// before — internal/sync/sync.go
// FIX: task#0005
func FormatPreview(preview *Preview) string {
    var b strings.Builder
    fmt.Fprintf(&b, "Project: %s\n", preview.ProjectPath)
    if len(preview.Changes) == 0 {
        b.WriteString("No changes.\n")
        return b.String()
    }
    for _, change := range preview.Changes {
        fmt.Fprintf(&b, "- [%s] %s: %s\n", change.Kind, change.Path, change.Reason)
    }
    return b.String()
}
```

```go
// after — internal/tui/render_preview.go: lipgloss-styled, kind-aware icons
func RenderPreview(preview *llmsync.Preview) string {
    var b strings.Builder
    fmt.Fprintf(&b, "%s %s\n", headerStyle.Render("Project:"), preview.ProjectPath)
    if len(preview.Changes) == 0 {
        b.WriteString(cleanStyle.Render("No changes."))
        b.WriteByte('\n')
        return b.String()
    }
    for _, change := range preview.Changes {
        icon, style := changeStyle(change.Kind)
        b.WriteString(style.Render(fmt.Sprintf("%s [%s] %s: %s", icon, change.Kind, change.Path, change.Reason)))
        b.WriteByte('\n')
    }
    return b.String()
}
```

## Change 2

`doctor.CheckProfile` returns a structured `*Report`. The TUI renders.

```go
// before — internal/doctor/doctor.go
// FIX: return error object instaed of string @see task#0005
func CheckProfile(p *profile.Profile) (string, error) {
    var out strings.Builder
    fmt.Fprintf(&out, "Profile: %s\n", p.Manifest.Name)
    for _, proj := range p.ProjectList() {
        ...
        fmt.Fprintf(&out, "%s %s\n", change.Kind, change.Path)
    }
    return out.String(), nil
}
```

```go
// after — pure data; tui.RenderReport produces text
type Report struct {
    ProfileName string
    Projects    []ProjectStatus
}
type ProjectStatus struct {
    Name    string
    Changes []llmsync.FileChange
}

func CheckProfile(p *profile.Profile) (*Report, error) {
    report := &Report{ProfileName: p.Manifest.Name}
    for _, proj := range p.ProjectList() {
        preview, err := llmsync.Plan(p, proj)
        if err != nil {
            return nil, err
        }
        report.Projects = append(report.Projects, ProjectStatus{
            Name:    proj.Name,
            Changes: preview.Changes,
        })
    }
    return report, nil
}
```

## Change 3

`render.Build` accumulates exclusive-group conflicts, missing assets, and
per-asset render failures into one joined error.

```go
// before — internal/render/render.go: short-circuits
if current, exists := groupSelections[a.ExclusiveGroup]; exists && current != a.ID {
    // FIX: ... @see task#0005
    return nil, fmt.Errorf("multiple assets selected in exclusive group %s", a.ExclusiveGroup)
}
...
if err := addAssetOutputs(files, a, proj.EnabledAgents); err != nil {
    return nil, fmt.Errorf("%s: %w", a.ID, err)
}
```

```go
// after — every issue surfaced together via errors.Join
selected, resolveErr := resolveAssets(p, proj)

var errs []error
if resolveErr != nil {
    errs = append(errs, resolveErr)
}

groupSelections := map[string][]string{}
for _, a := range selected {
    if a.ExclusiveGroup != "" {
        groupSelections[a.ExclusiveGroup] = append(groupSelections[a.ExclusiveGroup], a.ID)
    }
}
for group, ids := range groupSelections {
    if unique := dedupSorted(ids); len(unique) > 1 {
        errs = append(errs, ExclusiveGroupConflictError{Group: group, AssetIDs: unique})
    }
}

for _, a := range selected {
    if err := addAssetOutputs(files, a, proj.EnabledAgents); err != nil {
        errs = append(errs, AssetRenderError{AssetID: a.ID, Err: err})
    }
}

if len(errs) > 0 {
    return nil, errors.Join(errs...)
}
```

## Change 4

Typed errors replace string-only `fmt.Errorf` sites. Every domain package
gets an `errors.go`. Example pair:

```go
// before — internal/app/service.go
// FIX: task#0005 return metadata for error instead of hard-coded error message
return "", fmt.Errorf("asset already exists: %s", manifest.ID)
```

```go
// after — internal/app/errors.go owns the type, service.go returns the value
type AssetExistsError struct {
    AssetID string
}

func (e AssetExistsError) Error() string {
    return fmt.Sprintf("asset already exists: %s", e.AssetID)
}

// service.go
return "", AssetExistsError{AssetID: manifest.ID}
```

## Change 5

TUI error rendering with severity, icon, and color. Joined errors render
one styled line per cause.

```go
// before — internal/tui/tui.go
if err != nil {
    fmt.Printf("\nerror: %v\n", err)
}
```

```go
// after — RenderError unwraps errors.Join recursively and dispatches by type
func RenderError(err error) string {
    if err == nil {
        return ""
    }
    var b strings.Builder
    for _, e := range flattenJoined(err) {
        b.WriteString(renderOneError(e))
        b.WriteByte('\n')
    }
    return b.String()
}

func severityOf(err error) severity {
    switch {
    case errors.As(err, new(render.ExclusiveGroupConflictError)),
         errors.As(err, new(render.AssetNotFoundError)),
         ...:
        return sevError
    case errors.As(err, new(render.TargetOutsideSurfacesError)),
         errors.As(err, new(app.ProjectPathOwnedError)):
        return sevWarning
    }
    return sevInfo
}
```

## Post-review changes (2026-04-28)

The /review-task pass surfaced several gaps that were addressed before
merge. The largest reversal was on severity placement: ADR 0007's
original "no Severity() method on errors" rule was removed because the
centralized switch had no compile-time gate — any new typed error in
any domain package silently degraded to `sevInfo`. Severity is now part
of `errs.DomainError`, the TUI dispatches off `err.Severity()`, and the
ADR's revision history records the reversal.

### Architecture

- Added `internal/errs` with `Severity`, `DomainError`, an
  `Errors` slice that satisfies `error`, and a `Collect` helper that
  flattens both `Unwrap() []error` and `Unwrap() error` chains.
- Every typed error in `internal/{app,asset,profile,render,doctor}`
  now implements `Severity() errs.Severity`. The TUI's `severityOf`
  switch is gone.
- Accumulator-shape functions return `[]errs.DomainError` directly
  instead of `errors.Join(...)`:
  - `render.Build` and `render.resolveAssets`.
  - `app.Service.AddProject` and `app.Service.ensureProjectPathAvailable`.
  - `doctor.CheckProfile`.

  Functions that consume an accumulator and expose a single `error`
  (`sync.Plan`, `app.Plan`, `app.Apply`) wrap the slice in
  `errs.Errors`; the TUI uses `errs.Collect` to walk the typed leaves.
- `doctor` no longer re-exports `sync.FileChange`. `doctor.ProjectChange`
  and `doctor.ChangeKind` are owned types; consumers of a doctor report
  no longer need to import sync.
- `render.AssetRenderError` was replaced by two purpose-built typed
  errors that carry asset-relative paths (never absolute filesystem
  paths leaking the user's profile root): `AssetSourceMissingError` and
  `AssetReadError`. A small `redact()` helper strips embedded absolute
  paths from `*os.PathError`-style values that still surface through
  `Unwrap`.
- Map iteration in `render.Build` and `Service.ensureProjectPathAvailable`
  is now sorted, so accumulated error order is deterministic. The
  cross-profile pass uses `Profile.ProjectList()`'s name-sorted helper.
- `flattenJoined` is replaced by `errs.Collect`; the duplicate
  `unwrapJoined` helpers in `render_test.go` and `service_errors_test.go`
  are gone.

### Errors and naming

- `app.UnknownAssetError` was renamed `app.AssetNotFoundError` to match
  the render package's name and stdlib `os.ErrNotExist` style. The
  field is `AssetID` everywhere.
- `asset.assetIDNameRequiredError` (unexported struct + sentinel) was
  replaced by exported `asset.AssetIDNameRequiredError struct{}` with a
  retained `ErrAssetIDNameRequired` sentinel value, so callers can use
  either `errors.Is` or `errors.As` uniformly with the rest of the
  package.
- New `app.InternalError` wraps non-domain failures (filesystem,
  registry I/O) so accumulator-shape returns stay uniform without
  classifying every infrastructure error individually.

### Presentation

- `changeStyle` and `severityStyle` return `lipgloss.Style` directly
  instead of an ad-hoc anonymous interface. The TUI imports lipgloss
  already; there was no boundary worth preserving.
- `internal/tui/styles.go` exposes named `colorRed/Green/Yellow/...`
  constants and a `safe()` helper that strips ASCII control characters
  from manifest-sourced strings before they reach the terminal — so a
  hostile or accidentally-edited manifest cannot inject ANSI escape
  sequences.
- `addSkillOutputs` collapsed three near-identical agent branches into
  a single `skillRoots` map iteration; the `cursor` single-file branch
  is the remaining special case.

### Tests

- `internal/errs/errs_test.go` covers `Collect` (joined + single
  unwrap + non-domain leaves) and `Errors`'s `Error`/`Unwrap`.
- `render_test.go` adds `TestResolveAssets_AccumulatesMissingIDs`,
  `TestBuild_WrapsPerAssetFailureWithAssetSourceMissing`, and
  `TestBuild_TargetOutsideSurfacesIsTyped`.
- `app/project_ownership_test.go` adds
  `TestEnsureProjectPathAvailable_AccumulatesAcrossMultipleProfiles`.
- `app/service_errors_test.go` adds
  `TestAddProject_RejectsMixedKnownAndUnknownAssets` (asserts the
  project file is not persisted when any asset id is unknown).
- `tui/render_report_test.go` adds
  `TestRenderReport_MixedCleanAndDirtyProjects`.
- `tui/render_errors_test.go` is rewritten around `RenderErrors`; the
  unknown-error fallback is now `sevError` (was `sevInfo`).

### Documentation

- `docs/glossary.md` gains `Profile Health Report`, `Project Status`,
  `Project Change`, `Change Kind`, `File Change`, and `Severity`.
- `docs/guidelines/errors.md` rewritten around the new convention
  (Severity in domain, `[]DomainError` for accumulators, `errs.Errors`
  wrapper for cascade callers).
- ADR 0007 gains a "Revision history" section documenting the reversal
  on Severity placement.

### Backlog

The pre-existing `// FIX:`/`// TODO:`/`// WARN:` markers that this
task left in place (out of scope per plan §"Notes on scope") were
converted into tracked backlog tasks and the comments shortened to one
line referencing the task number:

- `tasks/backlog/0009_task_typed-compatible-agent` — replace
  `Manifest.CompatibleAgents []string` with a typed value.
- `tasks/backlog/0010_task_asset-init-strategy` — replace
  `asset.Init`'s switch with per-Type strategy + templates.
- `tasks/backlog/0011_task_render-strategy-lookup` — replace
  `render.addAssetOutputs`/`addSkillOutputs` switches with
  `(Type, Agent)` strategy lookup.
- `tasks/backlog/0012_task_extract-project-manifest-filter` — extract
  `profile.scanProjects` filter into a named helper.

### Dependency hygiene

`go mod tidy` was run; `github.com/muesli/termenv` (used by
`tui/main_test.go` to force `termenv.Ascii` for stable test output) is
now a direct require alongside lipgloss.
