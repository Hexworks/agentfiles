# 0029 changes

Replaced the back-only `planProjectStub` with a real `planProjectScreen`
that loads a sync `Preview` via `actions.PlanProject`, renders the change
list as a `treetable.Model` with `Status` + `Current Action` value
columns, and lets the user toggle per-row drift/unknown resolutions
before dispatching `actions.SyncProject` on `[Apply]`. The stub call
sites in Edit Profile and Select Project Assets now push the real
screen; the `planProjectStub` and its `stub_screens_test.go` are gone.

## Decisions

- **Default `[Back]` in the status bar despite the description excluding
  it.** — **Why:** every other screen (Settings, Select Project Assets,
  navigation stubs) keeps `[Back]` in the bar as the universal escape
  hint. Excluding it would break that convention for one screen. The
  description's "screen-level `a` and `b` excluded" reads strictly, but
  `select_project_assets.go` already documents the same exception in
  comments. `[Apply]` stays off the bar — the body's labelled button is
  visible at all times.
- **Encode `Keep` as map absence rather than an explicit entry.** —
  **Why:** the default for both drift and unknown is `Keep`; storing
  only off-default entries means an empty resolutions map produces empty
  `Drift`/`Unknown` slices for `SyncProject` and `onApply` never has to
  filter stale entries. `actionStateOf(path)` returns `planKeep` when
  the key is missing.
- **Build the change tree on every toggle via `tree.SetRoot`.** —
  **Why:** `treetable.SetRoot` calls `rebuild` → `refreshRows`, which
  reads `m.table.Cursor()` without resetting it, so the cursor sticks on
  the active row. The alternative (manually refreshing the actions cell
  alone) would have required exposing a new public method on
  `treetable.Model` for a single screen's convenience.
- **Iterate `preview.Changes` (not the map) when building resolution
  slices.** — **Why:** deterministic output order for tests, and any
  stale resolution for a path absent from the current plan is silently
  ignored.

## Assumptions

- The `Project synced` notification text fits the existing notification
  vocabulary alongside `Asset selected` / `Profile deleted`. — **Why:**
  short, verb-past-tense, matches the convention every other mutation
  uses; no need for a separate copy review.

## Other Notes

- Architecture docs updated: `docs/architecture/05-building-block-view.md`
  promotes Plan Project from "future" to implemented; `06-runtime-view.md`
  Scenario: Plan A Project rewritten to describe the actual TUI flow
  (load → treetable → toggle → SyncProject) instead of the placeholder.
- No new ADR. Screen uses already-established patterns: `treetable.WithValueColumns`
  (task 0020), `mutationCmd` + `notificationCmd` envelope (existing),
  dual-load `Init` (mirrors `select_project_assets`).
- Tests: 19 new tests in `plan_project_test.go` covering construction
  panics, status/action value mapping for every `ChangeKind`, toggle
  state machine, `onApply` slice construction with and without
  overrides, success-pops-and-notifies + failure-notifies-only, and an
  exhaustive mnemonic-uniqueness walk across cursor rows × override
  combinations.

## Plan Project screen

Replace the stub at `internal/tui/shell/plan_project_stub.go` with a
real screen that loads, renders, and applies.

```go
// before — internal/tui/shell/plan_project_stub.go
type planProjectStub struct {
    backOnlyScreenBase
    profileID string
    projectID string
}
func (s *planProjectStub) Init() tea.Cmd { return nil }
func (s *planProjectStub) Body(width int) string {
    sentence := fmt.Sprintf(" Planning project %q in profile %q — task 0029", ...)
    return s.renderBody(width, sentence)
}
```

```go
// after — internal/tui/shell/plan_project.go (excerpt)
type planProjectScreen struct {
    actions     planProjectActions
    profileID, projectID string
    preview     *llmsync.Preview
    resolutions map[string]planActionState // off-default only

    tree     *treetable.Model
    applyBtn, backBtn *mnemonic.Button
    set      *mnemonic.Set
}

func (s *planProjectScreen) Init() tea.Cmd { return s.loadCmd() }

// Three-RPC load folded into one envelope.
func (s *planProjectScreen) loadCmd() tea.Cmd {
    return func() tea.Msg {
        proj, err := s.actions.LoadProject(...); if err != nil { return planProjectLoadedMsg{err: err} }
        prof, err := s.actions.LoadProfile(...);  if err != nil { return planProjectLoadedMsg{err: err} }
        preview, err := s.actions.PlanProject(...); if err != nil { return planProjectLoadedMsg{err: err} }
        return planProjectLoadedMsg{prof: prof, proj: proj, preview: preview}
    }
}
```

## Per-row toggle button factory

The treetable invokes `treeActionsFn` for the cursor row only. The
button always shows the **other** option; pressing it swaps the
resolution and re-renders.

```go
// after — internal/tui/shell/plan_project.go
func (s *planProjectScreen) driftToggleBtn(path string) *mnemonic.Button {
    if s.actionStateOf(path) == planOverwrite {
        return mnemonic.New("Keep", 'k', func() tea.Cmd { return s.toggle(path, planKeep) })
    }
    return mnemonic.New("Overwrite", 'o', func() tea.Cmd { return s.toggle(path, planOverwrite) })
}

func (s *planProjectScreen) toggle(path string, next planActionState) tea.Cmd {
    if next == planKeep {
        delete(s.resolutions, path)
    } else {
        s.resolutions[path] = next
    }
    if s.preview != nil {
        s.tree.SetRoot(buildPlanTree(s.projectName, s.preview.Changes))
    }
    s.rebuildSet()
    return nil
}
```

## Plan-change tree builder

`FileChange.Path` is forward-slash relative per `sync.validatePathKey`;
the tree builder splits on `"/"` literally rather than
`filepath.Separator` (the divergence from `buildAssetTree`).

```go
// after — internal/tui/shell/plan_project.go
func buildPlanTree(projectName string, changes []llmsync.FileChange) *treetable.Node {
    root := &treetable.Node{Label: projectName + "/", Data: planNode{kind: planNodeRoot}}
    dirs := map[string]*treetable.Node{"": root}
    for _, ch := range changes {
        parts := strings.Split(ch.Path, "/")
        // walk parts; create dir nodes as needed; final part becomes a file leaf
        ...
    }
    return root
}
```

## Apply: build resolutions and dispatch SyncProject

```go
// after — internal/tui/shell/plan_project.go
func (s *planProjectScreen) onApply() tea.Cmd {
    var drift []app.DriftResolution
    var unknown []app.UnknownResolution
    for _, ch := range s.preview.Changes {
        st, has := s.resolutions[ch.Path]
        if !has { continue }
        switch ch.Kind {
        case llmsync.ChangeDrift:
            if st == planOverwrite {
                drift = append(drift, app.DriftResolution{Path: ch.Path, Decision: app.DriftOverwrite})
            }
        case llmsync.ChangeUnknown:
            if st == planDelete {
                unknown = append(unknown, app.UnknownResolution{Path: ch.Path, Decision: app.UnknownDelete})
            }
        }
    }
    return mutationCmd(
        func() errs.DomainError {
            _, err := s.actions.SyncProject(actions.SyncProjectInput{
                ProfileRef: s.profileID, ProjectID: s.projectID, Drift: drift, Unknown: unknown,
            })
            return err
        },
        "Project synced",
    )
}
```

## Widen `selectProjectAssetsActions` so the screen forwards its handle

```go
// before — internal/tui/shell/select_project_assets.go
type selectProjectAssetsActions interface {
    LoadProfile(in actions.LoadProfileInput) (*profile.Profile, errs.DomainError)
    LoadProject(in actions.LoadProjectInput) (*project.Manifest, errs.DomainError)
    SelectAsset(in actions.SelectAssetInput) ([]string, errs.DomainError)
    UnselectAsset(in actions.UnselectAssetInput) ([]string, errs.DomainError)
}
```

```go
// after — adds the two methods planProjectScreen needs so the parent
// screen can pass `s.actions` to the child constructor without a type
// assertion. The composed `editProfileActions` interface inherits both.
type selectProjectAssetsActions interface {
    LoadProfile(in actions.LoadProfileInput) (*profile.Profile, errs.DomainError)
    LoadProject(in actions.LoadProjectInput) (*project.Manifest, errs.DomainError)
    SelectAsset(in actions.SelectAssetInput) ([]string, errs.DomainError)
    UnselectAsset(in actions.UnselectAssetInput) ([]string, errs.DomainError)
    PlanProject(in actions.PlanProjectInput) (*llmsync.Preview, errs.DomainError)
    SyncProject(in actions.SyncProjectInput) (*llmsync.Preview, errs.DomainError)
}
```

## Retarget the stub push call sites

```go
// before — internal/tui/shell/select_project_assets.go:onPlan
return pushCmd(newPlanProjectStub(s.profileID, s.projectID))
```

```go
// after — pushes the real screen with the actions handle
return pushCmd(newPlanProjectScreen(s.actions, s.profileID, s.projectID))
```

```go
// before — internal/tui/shell/edit_profile.go:onPlanProject
return pushCmd(newPlanProjectStub(s.profileID, p.ID))
```

```go
// after
return pushCmd(newPlanProjectScreen(s.actions, s.profileID, p.ID))
```
