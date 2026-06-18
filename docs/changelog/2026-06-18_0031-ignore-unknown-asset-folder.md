# 0031 changes

Added an **Ignore** escape hatch to the Plan Project screen, the inverse of
the **Register** action from task 0030. When the cursor sits on a directory
whose every descendant is `? unknown`, pressing `i` collapses the folder
(subtree removed, trailing `/` dropped from the label), hides **Register**, and
flips the button to **Show** (`s`) which restores it. Ignoring is an in-memory
toggle with no modal, mirroring the drift/unknown resolution UX.

On **Apply** the ignored folders persist into a new `ignored_paths` list in
`<repo>/.agentfiles/state.json`, unioned with any already-persisted paths so
they survive later applies even after vanishing from the preview. Every
subsequent **Plan** suppresses any `ChangeUnknown` whose path sits under an
ignored folder, so the folder no longer appears at all.

## Decisions

- Plain `[]string` for ignored paths through the whole call chain (no Decision
  enum) — **Why:** ignoring is purely additive; a typed `Resolution` would be
  ceremony for a list that only ever grows.
- Union-on-apply in `mergeIgnoredPaths` — **Why:** Apply rewrites state from
  scratch, and ignored folders disappear from later previews; without the union
  with `preview.ManagedState.IgnoredPaths` a second apply would silently drop
  previously-persisted ignores.
- Reused `app.RegisterableDirs` for Ignore eligibility — **Why:** the "whole
  subtree is unknown" rule is identical to Register eligibility; a duplicate
  predicate would drift out of sync.
- Collapse implemented entirely in `buildPlanTree` (break on ignored dir) —
  **Why:** keeps the `treetable` component generic; no component change needed.

Not done (out of scope per task): reclassifying how `ChangeUnknown` is computed
(task 0017), a screen to un-ignore already-persisted paths (follow-up),
mutating the ignored folder's files on disk.

## Assumptions

- Ignored keys are validated with the existing `validatePathKey` and a corrupt
  persisted key surfaces as `StateCorruptError` — **Why:** matches the existing
  `managed_files` validation; no new typed error was warranted.

## Other Notes

Docs updated: `docs/manual/plan_project.md` (Ignore/Show toggle and
`ignored_paths` persistence), `docs/glossary.md` (new **Ignored Path** entry),
`docs/adr/0010-sync-resolutions-and-first-apply.md` (addendum describing the
additive, union-on-apply suppression list). No new ADR — mirrors task 0030.

## Domain — `internal/sync/sync.go`

`ManagedState` gained an `IgnoredPaths` field; `loadState` validates each key;
`classifyDeleteOrUnknown` suppresses unknowns under an ignored path; `Apply`
gained an `ignoredPaths` parameter and merges them into the persisted state.

```go
// before
type ManagedState struct {
    ManagedFiles map[string]string `json:"managed_files"`
    // ...
}

func Apply(preview *Preview, driftResolutions []DriftResolution, unknownResolutions []UnknownResolution) errs.DomainError
```

```go
// after — ignored_paths persisted and unioned across applies
type ManagedState struct {
    ManagedFiles map[string]string `json:"managed_files"`
    IgnoredPaths []string          `json:"ignored_paths"`
    // ...
}

func Apply(preview *Preview, driftResolutions []DriftResolution, unknownResolutions []UnknownResolution, ignoredPaths []string) errs.DomainError

func isUnderIgnored(rel string, ignored []string) bool {
    for _, ig := range ignored {
        if rel == ig || strings.HasPrefix(rel, ig+"/") {
            return true
        }
    }
    return false
}
```

## App / Actions layers

`Service.Apply` and `actions.SyncProject` forward the ignored set through to
the engine.

```go
// after
func (s *Service) Apply(profileRef, projectID string, driftResolutions []DriftResolution, unknownResolutions []UnknownResolution, ignoredPaths []string) (*Preview, errs.DomainError)

type SyncProjectInput struct {
    // ...
    Ignored []string
}
```

## TUI — `internal/tui/shell/plan_project.go`

Added an `ignoredDirs map[string]bool` toggle map, `Ignore`/`Show` buttons, a
`toggleIgnore` command that rebuilds the tree, and `onApply` collects the sorted
ignored keys into `SyncProjectInput.Ignored`. `buildPlanTree` breaks out of the
subtree for any dir in `ignoredDirs`, rendering the label without a trailing
slash.
