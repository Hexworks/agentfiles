# Ignore unknown asset folder review

Seven parallel reviews (security, clean code, clean architecture, SOLID, DDD, testing, Go) ran against the 0031 diff. **Security: clean** — the `isUnderIgnored` prefix guard (`ig+"/"`), the symmetric `validatePathKey` on both write and load, and the fact that suppression is purely subtractive on the unknown set (never touches deletes or managed files) all hold up. The feature is well-built and well-tested on its headline paths (Plan suppression, Apply union/persist, typed errors, TUI collapse/restore).

The findings below are about robustness and consistency, not correctness of the happy path. The highest-value items are: (1) ignore eligibility is enforced only at the TUI edge, so the trusted layer will persist any path a caller hands it; (2) the load-bearing prefix-boundary line has no test; (3) the empty-set serialization is `null`, not the omitted key the code comment claims. The rest are convention/readability/naming polish.

## Ignore eligibility is enforced only in the TUI; `sync.Apply` trusts the caller's `ignoredPaths`

> [!WARNING]
>
> - [docs/guidelines/clean_architecture.md](../../../docs/guidelines/clean_architecture.md)
> - [docs/guidelines/sync_and_safety.md](../../../docs/guidelines/sync_and_safety.md)
> - [docs/guidelines/domain_model.md](../../../docs/guidelines/domain_model.md)

The domain rule "a folder may only be ignored if its whole subtree is unknown" lives only in `app.RegisterableDirs`, consulted by `treeActionsFn` to decide whether to render the `[Ignore]` button. The trusted write path does not re-assert it: `sync.Apply` validates each `ignoredPaths` entry with `validatePathKey` (path-traversal safety) and unions it straight into persisted state. The sibling write path `CreateAssetFromFolder` deliberately does the opposite — it re-derives `RegisterableDirs` from a fresh plan "rather than trusting the caller."

A caller of `app.Service.Apply` / `actions.SyncProject` with a hand-built `Ignored` (e.g. `.claude/skills`) could permanently persist an ancestor of managed files into `ignored_paths`. `classifyDeleteOrUnknown` would then silently suppress real unknown drift under it forever, weakening the "show everything inside managed surfaces, opt-in only" guarantee.

```go
// sync.Apply — only path-safety is checked, not domain eligibility
for _, p := range ignoredPaths {
    if err := validatePathKey(p); err != nil {
        validationErrs = append(validationErrs, err)
    }
}
// CreateAssetFromFolder, by contrast, re-asserts the rule from a fresh plan:
if !RegisterableDirs(previewFromSync(syncPreview).Changes)[dirKey] {
    return "", FolderNotRegisterableError{DirKey: dirKey}
}
```

Choose one:

- [x] In `Service.Apply`, re-assert each **newly selected** ignored key is registerable against the freshly computed `syncPreview` before persisting (skip the prior-state union members, whose folders have legitimately vanished), returning a typed error otherwise.
- [ ] Leave enforcement at the TUI but add an explicit comment on `sync.Apply` / `SyncProjectInput.Ignored` stating the eligibility check is a UI-only convenience and the engine intentionally trusts the caller (documents the trust boundary).

## Prefix-boundary of `isUnderIgnored` is untested

> [!WARNING]
>
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md)

`isUnderIgnored` guards the prefix with `ig+"/"` so that an ignored key `.codex/ig` does **not** suppress the sibling `.codex/ignore-me`. This is the single subtlest line in the change — a naive `strings.HasPrefix(rel, ig)` would silently over-suppress. `TestPlan_SuppressesUnknownUnderIgnoredPath` only covers a true descendant and an unrelated sibling; it never exercises the case where one key is a string-prefix of another path. Dropping the `+"/"` would pass every existing test.

```go
// missing: a string-prefix sibling must NOT be suppressed
func TestIsUnderIgnored_PrefixBoundary(t *testing.T) {
    if isUnderIgnored(".codex/ignore-me", []string{".codex/ig"}) {
        t.Error(".codex/ig must not suppress sibling .codex/ignore-me")
    }
    if !isUnderIgnored(".codex/ig/x.md", []string{".codex/ig"}) {
        t.Error(".codex/ig must suppress its descendant")
    }
}
```

Choose one:

- [x] Add a focused table-driven unit test on `isUnderIgnored` (exact match, true descendant, prefix-sibling, unrelated).
- [ ] Extend `TestPlan_SuppressesUnknownUnderIgnoredPath` with a `.codex/ignored-sibling/x.txt` fixture that must remain `ChangeUnknown`.

## Empty ignored set serializes as `"ignored_paths": null`, not an omitted key

> [!WARNING]
>
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md)
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md)

`mergeIgnoredPaths` returns `nil` when both inputs are empty, and its comment claims this is "so the state file omits the key cleanly." But `IgnoredPaths` has no `omitempty` json tag, so a `nil` slice marshals to `"ignored_paths": null` — the key is present, just null. The comment is factually wrong, and no test pins the empty/first-apply serialization. (Unmarshalling `null` back to a nil slice is harmless, so this is a comment-accuracy + missing-coverage issue, not a runtime bug. Note `managed_files` also omits `omitempty`, so adding it here would be a small inconsistency.)

```go
// mergeIgnoredPaths comment says "omits the key cleanly" — but:
IgnoredPaths []string `json:"ignored_paths"` // no omitempty → null, not omitted
```

Choose one:

- [x] Fix the comment to say it serializes as `null` (drop the "omits the key" claim), and add a test asserting the first-apply state contains `"ignored_paths": null` (or no managed ignores).
- [ ] Add `omitempty` to the `IgnoredPaths` tag so the key really is omitted when empty, update the comment, and add a test asserting the key is absent. (Accept the minor divergence from `managed_files`.)

## `onApply` uses `sort.Strings` + manual map drain, against the repo's `slices`/`maps` convention

> [!WARNING]
>
> - [docs/guidelines/go.md](../../../docs/guidelines/go.md)
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md)

The repo standardized on `slices`/`maps` generics — `sync.go` itself uses `slices.Sort`, and the `shell` package already collects sorted map keys this way elsewhere. The new `onApply` instead allocates a slice, ranges the map to append keys, then calls `sort.Strings`, and that is the _sole_ reason `plan_project.go` adds the `"sort"` import. The TUI sort is also redundant: `mergeIgnoredPaths` re-dedups and re-sorts authoritatively, so the on-disk order does not depend on the TUI.

```go
ignored := make([]string, 0, len(s.ignoredDirs))
for dir := range s.ignoredDirs {
    ignored = append(ignored, dir)
}
sort.Strings(ignored)
// → one line, drops the "sort" import:
ignored := slices.Sorted(maps.Keys(s.ignoredDirs))
```

Choose one:

- [x] Replace with `slices.Sorted(maps.Keys(s.ignoredDirs))`; drop `"sort"`, add `"maps"`/`"slices"` as needed.
- [ ] Keep the manual collection but swap `sort.Strings` → `slices.Sort` for package consistency.

## `Apply`'s positional resolution parameters keep growing

> [!WARNING]
>
> - [docs/guidelines/solid.md](../../../docs/guidelines/solid.md)

Both `llmsync.Apply` and `Service.Apply` now carry three "what the user resolved this apply" inputs — `driftResolutions`, `unknownResolutions`, `ignoredPaths` — plus the preview. One additive feature forced editing both signatures and ~10 call sites (the `, nil` churn across `sync_test.go`). The actions layer already recognized the cohesion with `SyncProjectInput`; the domain/app signatures did not follow. A `Resolutions` struct would localize future growth to one field and leave `Apply(preview, resolutions)` stable. (This is independent of the sound `[]string`-vs-`Decision`-enum choice the changelog already defends — that asymmetry is fine because ignore is single-mode.)

```go
// today: each new resolution kind widens the signature + all call sites
func Apply(preview *Preview, drift []DriftResolution, unknown []UnknownResolution, ignoredPaths []string) errs.DomainError

// bundled: additive growth stays inside one type
type Resolutions struct {
    Drift   []DriftResolution
    Unknown []UnknownResolution
    Ignored []string
}
func Apply(preview *Preview, r Resolutions) errs.DomainError
```

Choose one:

- [x] Introduce a `Resolutions` struct for both `llmsync.Apply` and `Service.Apply`; map `SyncProjectInput` into it once.
- [ ] Accept the positional growth as the practical ceiling (the actions `SyncProjectInput` already shields the TUI); leave signatures as-is.

## "Ignored path" concept is named four different ways across layers

> [!WARNING]
>
> - [docs/guidelines/domain_model.md](../../../docs/guidelines/domain_model.md)
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md)

The glossary defines a single concept, **Ignored Path**, but the code realizes it under at least four surface names: `ManagedState.IgnoredPaths`, the TUI `ignoredDirs map[string]bool`, `SyncProjectInput.Ignored`, and the bare `ignoredPaths` param. sync calls it a _path_, the TUI calls it a _dir_; the folder-vs-path distinction is never pinned. A reader cannot grep one name for the concept.

```go
type ManagedState struct{ IgnoredPaths []string }   // sync: "paths"
ignoredDirs map[string]bool                          // tui: "dirs"
type SyncProjectInput struct{ Ignored []string }     // actions: "ignored"
func Apply(..., ignoredPaths []string)               // bare param
```

Choose one:

- [ ] Rename `SyncProjectInput.Ignored` → `IgnoredPaths` so the actions field matches sync + glossary (smallest consistent fix).
- [x] Standardize on one term ("ignored path") across all four layers, renaming the TUI `ignoredDirs` accordingly.

## `Show` mnemonic `'w'` has no recorded rationale, and docs disagree on the key

> [!WARNING]
>
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md)

`showFolderBtn` binds **Show** to `'w'` (commit `61546b4`, to avoid the Settings `'s'` collision), but `'w'` has no relationship to the word "Show", so the next maintainer will re-litigate it. The docs are also inconsistent: `description.md` says the toggle is `s`, while `changelog` and `plan` were updated to `w`.

```go
func (s *planProjectScreen) showFolderBtn(dirPath string) *mnemonic.Button {
    return mnemonic.New("Show", 'w', func() tea.Cmd { return s.toggleIgnore(dirPath, false) })
}
```

Choose one:

- [x] Add a one-line WHY comment on `showFolderBtn` noting `'w'` avoids the Settings `'s'` collision, and reconcile `description.md` to say `w`.
- [ ] Reconcile the docs only (the commit message already records the WHY); leave the code uncommented.

## New comments lean toward restating the code

> [!WARNING]
>
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md)

Project convention is no comments unless the WHY is non-obvious. The genuine-WHY comments here are good and should stay (`mergeIgnoredPaths` union rationale, `toggleIgnore`'s SetRoot-vs-RefreshActions note). But `isUnderIgnored`'s comment largely restates the code ("either an exact match or a descendant path"), and part of the `buildPlanTree` addition narrates loop mechanics the code already shows.

```go
// isUnderIgnored reports whether rel sits inside one of the ignored folder
// keys: either an exact match or a descendant path. ...   // restates the code
```

Choose one:

- [x] Trim `isUnderIgnored`'s comment and the mechanical half of the `buildPlanTree` comment; keep the union/SetRoot WHYs.
- [ ] Leave the comments as-is (they aid first-time readers of the collapse logic).

## Glossary `Resolution` entry doesn't mention that unknowns can now be ignored

> [!WARNING]
>
> - [docs/guidelines/domain_model.md](../../../docs/guidelines/domain_model.md)

A `ChangeUnknown` row now has two independent resolution channels: a transient `UnknownResolution` (keep/delete) and a persisted ignored path (suppress). The new **Ignored Path** glossary entry was added, but the existing `Resolution`/`ChangeUnknown` entries were not updated to acknowledge that "ignore" is now a third outcome for an unknown, leaving the relationship implicit.

```go
unknownResolutions []UnknownResolution // keep | delete (transient)
ignoredPaths       []string            // suppress      (persisted)
```

Choose one:

- [x] Add a sentence to the glossary `Resolution`/`ChangeUnknown` entry noting an unknown can also be permanently ignored.
- [ ] Leave as-is — the **Ignored Path** entry plus the ADR 0010 addendum already document the mechanism.
