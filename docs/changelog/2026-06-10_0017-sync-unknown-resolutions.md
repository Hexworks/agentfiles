# 0017 changes

Split the sync engine's delete-candidate classification into two kinds and
replaced the all-or-nothing `deleteCandidates bool` flag with a per-file
`FileResolution` slice. `Plan` now emits `ChangeUnknown` for files inside a
managed surface that were never tracked in `ManagedState`, separate from
`ChangeDelete` for state-recorded files missing from desired. `Apply` reads
each `Changes` entry, looks up the user's resolution, and applies the right
default behavior — drift kept unless `ResolveOverwrite`, unknown kept unless
`ResolveDelete`, create/update/delete applied automatically.

First-time applies (no `.agentfiles/state.json`) are treated as a clean
slate: every desired file is `ChangeCreate` (overwriting whatever exists at
that path), and stray files in managed surfaces are ignored. This stops
fresh projects from flooding the preview with `ChangeUnknown` noise before
`agentfiles` has ever written anything.

`Preview.DeleteCandidates` was deleted (its `TODO` is finally honored) and
`app.Service.Apply`'s signature changed to take `[]llmsync.FileResolution`.
The existing TUI passes an empty slice — the Plan Project Screen with toggle
buttons ships in task 0029.

## Decisions

- **`ChangeDelete` auto-applies; `ChangeUnknown` requires explicit
  `ResolveDelete`** — **Why:** the two cases carry different safety
  profiles. A state-recorded file means the user opted in to managing it
  during a previous apply, so removing it once the render no longer wants
  it is the same decision as updating it; the preview itself is the opt-in.
  An unknown file was never tracked, so removing it crosses a stronger
  trust line and demands a deliberate per-file resolution.
- **First-apply suppresses `ChangeUnknown` entirely** — **Why:** a project
  with no state has never had a conversation with `agentfiles` about what's
  managed. Showing stray files as "unknown" before any commitment exists is
  noise that would punish the common case of adopting `agentfiles` in a
  repo that already has an `AGENTS.md` or `.claude/`.
- **First-apply emits `ChangeCreate` for desired files even when they
  already exist** — **Why:** clean-slate apply should be visible in the
  preview. Falling back to `ChangeUpdate` or `ChangeDrift` for pre-existing
  files would imply we'd compared against a baseline we don't have.
- **`Apply` iterates `preview.Changes`, not `preview.Files`** — **Why:**
  with per-file resolutions, the change list is the source of truth for
  what `Apply` does. The parallel `Files` slice survives as a lookup table
  for bodies (used for create/update/overwrite-drift) and as the input to
  the new `ManagedState`.
- **`ResolveAuto` exists but is implicit** — **Why:** the user never picks
  it; it's the default for `ChangeCreate` / `ChangeUpdate` / `ChangeDelete`
  when no resolution is supplied. Having a named constant makes the type
  self-documenting and lets future code (e.g. a doctor mode that lists
  what would be applied) talk about the auto kinds.

Considered but not done:

- Wiring the new per-file UX (`[Overwrite]` / `[Keep]` / `[Delete]`
  toggle buttons) into the existing TUI. The task description explicitly
  defers that to 0029; the current TUI calls `Apply` with an empty
  resolutions slice so the build stays green and defaults apply.
- Adding a typed error for "resolution refers to a path not in `Changes`".
  Silently ignoring stray entries is fine and matches the slice/map fold
  semantics; the UI is the only producer.

## Assumptions

- **`modal.ResolutionState` (TUI modal lifecycle) and `sync.Resolution`
  (per-file apply decision) can coexist without renaming.** — **Why:**
  they live in different packages with no overlapping call sites; the
  parent task 0015 already plans to consume both from the Plan Project
  screen and refers to `Resolution` for the sync concept.
- **Doctor should surface `ChangeUnknown` rather than swallow it.** —
  **Why:** doctor's role is to report the state of every project; an
  unknown file is part of that state. The task body didn't spell this
  out, but suppressing it would have made `convertKind` lossy.

## Other Notes

- ADR 0010 (`docs/adr/0010-sync-resolutions-and-first-apply.md`) was added
  to capture the decision model.
- `docs/architecture/08-concepts.md` "Drift Detection" and "Safety-First
  Deletion" sections were rewritten; a new "First-Apply Clean Slate"
  subsection was added.
- `docs/architecture/05-building-block-view.md` `sync` paragraph now lists
  all five `ChangeKind` values and references the new ADR.
- `docs/architecture/09-architecture-decisions.md` index gained the new
  ADR entry.
- `docs/guidelines/sync_and_safety.md` "Require Explicit Deletion Opt-In"
  rule was rewritten to describe the split between auto-delete and
  opt-in-delete.
- Bug fix drive-by: `sync.ChangeUnknown` was declared as a bare string
  constant rather than `ChangeKind`. Now correctly typed.
- The doctor tests had to seed an empty `ManagedState` to opt out of the
  new first-apply behavior; without it they would have read every desired
  file as `ChangeCreate` instead of the `ChangeUpdate` they assert on.

## ChangeKind split: delete vs unknown

`Plan` previously routed every non-desired file inside a managed surface
into a single `ChangeDelete` bucket via `detectDeleteCandidates`. The pass
is now `detectExtraneous`, returning two slices: state-recorded deletes
and surface-only unknowns.

```go
// before
deleteCandidates, detectErrs := detectDeleteCandidates(proj.Path, desired, state)
if len(detectErrs) > 0 {
    return nil, errs.Errors(detectErrs)
}
for _, candidate := range deleteCandidates {
    changes = append(changes, FileChange{Path: candidate, Kind: ChangeDelete, Reason: "recognized llm file not selected"})
}
```

```go
// after — split into ChangeDelete (state-recorded) and ChangeUnknown
// (surface walk only); first-apply skips both entirely
if state != nil {
    deletes, unknowns, detectErrs := detectExtraneous(proj.Path, desired, state)
    if len(detectErrs) > 0 {
        return nil, errs.Errors(detectErrs)
    }
    for _, path := range deletes {
        changes = append(changes, FileChange{Path: path, Kind: ChangeDelete, Reason: "recognized llm file not selected"})
    }
    for _, path := range unknowns {
        changes = append(changes, FileChange{Path: path, Kind: ChangeUnknown, Reason: "unrecognized file in managed surface"})
    }
}
```

## Apply signature: bool flag → per-file resolutions

The previous `Apply(preview *Preview, deleteCandidates bool)` could only
say "remove every candidate or none". The new signature takes a slice the
TUI can build per file as the user toggles each row.

```go
// before
func Apply(preview *Preview, deleteCandidates bool) errs.DomainError {
    for _, file := range preview.Files {
        abs := filepath.Join(preview.ProjectPath, filepath.FromSlash(file.Path))
        if err := utils.WriteFile(abs, file.Body, file.Mode); err != nil {
            domainErrs = append(domainErrs, err)
        }
    }
    if deleteCandidates {
        for _, path := range preview.DeleteCandidates {
            // ...remove
        }
    }
    // ...rewrite state from preview.Files
}
```

```go
// after — Changes is the source of truth; resolutions[path] decides
// drift and unknown defaults
func Apply(preview *Preview, resolutions []FileResolution) errs.DomainError {
    resolutionsByPath := map[string]Resolution{}
    for _, r := range resolutions {
        resolutionsByPath[r.Path] = r.Resolution
    }
    bodiesByPath := map[string]render.RenderedFile{}
    for _, f := range preview.Files {
        bodiesByPath[f.Path] = f
    }
    for _, change := range preview.Changes {
        switch change.Kind {
        case ChangeCreate, ChangeUpdate:
            // always write
        case ChangeDrift:
            if resolutionsByPath[change.Path] == ResolveOverwrite {
                // write
            }
        case ChangeDelete:
            // always remove
        case ChangeUnknown:
            if resolutionsByPath[change.Path] == ResolveDelete {
                // remove
            }
        }
    }
    // ...rewrite state from preview.Files (unchanged)
}
```

## First-apply clean slate

`Plan` short-circuits the existence/hash comparison when no state file is
present, emitting `ChangeCreate` for every desired file regardless of
whether it exists on disk. The detect-extraneous pass is skipped entirely.

```go
// before — same code path for first and subsequent applies
abs := filepath.Join(proj.Path, filepath.FromSlash(file.Path))
if !utils.Exists(abs) {
    changes = append(changes, FileChange{Path: file.Path, Kind: ChangeCreate, Reason: "file missing"})
    continue
}
// ... drift / update comparison
```

```go
// after — first apply forces ChangeCreate even when file exists
if state == nil {
    changes = append(changes, FileChange{Path: file.Path, Kind: ChangeCreate, Reason: "first apply"})
    continue
}
abs := filepath.Join(proj.Path, filepath.FromSlash(file.Path))
if !utils.Exists(abs) {
    changes = append(changes, FileChange{Path: file.Path, Kind: ChangeCreate, Reason: "file missing"})
    continue
}
// ... drift / update comparison (unchanged)
```
