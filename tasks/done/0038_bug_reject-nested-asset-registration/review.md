# Reject nested asset registration — review

Task 0038 tightens `RegisterableDirs` so nested / above-root / outside directories can no
longer be registered as skill assets, and centralizes the asset-container root list into a
new exported `render.AssetContainerRoots()`. The DoD gate passes: all five acceptance
criteria have diff evidence, and every hunk maps to task scope (no scope creep).

The rule change itself is correct and well-tested. The findings below are structural and
API-shape observations from the seven parallel review agents (security, clean-code,
clean-architecture, SOLID, DDD, testing, Go):

- The biggest concern (raised by both the clean-architecture and SOLID passes) is the
  placement of asset-container roots inside `internal/render` and the new `app → render`
  import that comes with it. `render` is documented as an I/O-doing detail package and
  `app.RegisterableDirs` now transitively pulls in `asset`, `profile`, `project`,
  `surfaces`, and `render`'s file-reading helpers just to read four constants.
- The API shape of `AssetContainerRoots() []string` forces every caller to rebuild a
  `map[string]bool` for O(1) parent lookup. There is only one caller today; a second
  would duplicate the conversion.
- `skillRoots` moved from a function-local literal (drift-proof by construction) to a
  package-level `var` map — mutable shared state where a const-equivalent is intended.
- `RegisterableDirs`' rewritten inner loop mixes an opaque `i > 0` parent branch with a
  quadratic `strings.Join(parts[:i], "/")` pattern and two nameless counters (`total` /
  `unknown`) whose "all leaves unknown" invariant is only visible as `n > 0 && unknown[dir] == n`.
- Test coverage of the new `AssetContainerRoots()` export is only indirect (via
  `TestRegisterableDirs`), and the service-boundary regression only asserts the nested
  case — the container-root-itself (`.claude/skills`) and above-root (`.claude`) rows from
  the acceptance table have no `CreateAssetFromFolder`-level regression.
- A durable new domain term ("asset-container root") appears in the exported function
  name, docstring, task description, and changelog, but was intentionally kept out of the
  glossary. `FolderNotRegisterableError`'s docstring still describes only the old rule.

Pick one solution per issue below by ticking exactly one `- [ ]`. Run `af.task.review-apply 0038`
in a fresh session once done.

---

## Asset-container roots misplaced in `render`; new `app → render` edge

> [!WARNING]
>
> - [Clean Architecture — Common Reuse Principle](../../../docs/guidelines/clean_architecture.md#common-reuse-principle)
> - [Clean Architecture — Stable Dependencies Principle](../../../docs/guidelines/clean_architecture.md#stable-dependencies-principle)
> - [SOLID — Single Responsibility](../../../docs/guidelines/solid.md#single-responsibility-principle)
> - [SOLID — Dependency Inversion](../../../docs/guidelines/solid.md#dependency-inversion-principle)

The changelog frames the four roots as "a render-layer concern". But `render`'s stated
purpose (package doc + CLAUDE.md) is to turn a profile plus a project manifest into
concrete files — a computation, not a policy registry. `AssetContainerRoots()` is
domain policy about _where folder-shaped skill assets live in a target repo_, and its
only consumer is `app.RegisterableDirs` (registration/ignore-flow classification — not
rendering).

Consequences of the current placement:

1. `internal/app` now imports `internal/render` for the first time. `RegisterableDirs`
   is a helper used by the ignore flow and the TUI plan-project listing — flows that
   have no semantic relationship to rendering. Any future churn in `render` (new asset
   type, per-`(Type, Agent)` strategy from task 0011, changed entry-point signature)
   forces a rebuild-and-retest of `app.RegisterableDirs`.
2. `render` now has two exports with unrelated reasons to change: (a) the file-render
   pipeline, (b) the folder-registration-eligibility root list.
3. `internal/surfaces` — which already owns the outer safety-fence root list with the
   same shape (bare paths + an `IsAllowed` accessor) — is the natural sibling. CLAUDE.md
   already documents `surfaces` as the place where "the data and the rule live together."

```go
// current graph
//
//   app ──► render ──► asset, profile, project, surfaces, config, errs, utils
//    │
//    └────► sync (llmsync) ──► render, surfaces
//
// proposed
//
//   app ──► surfaces        (or new tiny package)
//   render ──► surfaces     (existing)
//   sync ──► surfaces       (existing)
```

Pick one:

- [x] Move `AssetContainerRoots()`, `skillRoots`, and `cursorCommandsRoot` into
      `internal/surfaces` and have both `render` and `app` consume them from there.
      Drops the new `app → render` edge and co-locates the outer (managed-surface)
      and inner (asset-container-root) fences.
- [ ] Create a new tiny `internal/assetroots` (or `internal/containers`) package that
      owns the four strings, the map, and the accessor; `render`, `app`, and any
      future caller depend on it. Keeps `surfaces` narrow if the two fences should
      stay conceptually separate.
- [ ] Keep the current placement but invert the dependency direction: have `render`
      accept a `[]string` container-roots parameter (or a small `RootsProvider`
      interface) from its callers, so both `app` and `sync` own the list. Overkill
      for four constants — only pick this if a second provider ever appears.
- [ ] Accept the placement and explicitly document the new edge in CLAUDE.md's
      "Package layout" section, plus extend the `render` package doc to state its
      second responsibility (asset-container-root registry). No code moves, but the
      documented reality catches up with the actual imports.

---

## `AssetContainerRoots()` slice shape forces per-call reshaping

> [!WARNING]
>
> - [Clean Code — Code Smells (needless complexity)](../../../docs/guidelines/clean_code.md#code-smells)
> - [SOLID — Interface Segregation](../../../docs/guidelines/solid.md#interface-segregation-principle)
> - [Go — Return Actionable Types](../../../docs/guidelines/go.md#prefer-explicit-types-over-loose-maps)

`AssetContainerRoots()` returns `[]string` sorted for determinism, but the sole caller
(`RegisterableDirs`) immediately turns it into `map[string]bool` for O(1) parent
lookup — and `assertIgnoredRegisterable` calls `RegisterableDirs` on every apply, so
the slice → sort → convert cycle runs on the hot path. The sort is not observable to
the caller; determinism is only needed if the slice were iterated for user-facing
output, which it never is. If a second caller lands, it will duplicate the same
adapter.

```go
// current — render.go
func AssetContainerRoots() []string { /* fresh copy, sorted */ }

// app/service.go
rootList := render.AssetContainerRoots()
roots := make(map[string]bool, len(rootList))
for _, r := range rootList {
    roots[r] = true
}
```

Pick one:

- [x] Add `render.IsAssetContainerRoot(path string) bool` (backed by a package-level
      `map[string]struct{}` built once at init or via `sync.OnceValue`) and drop the
      slice → map conversion in `app.RegisterableDirs`. Mirrors `surfaces.IsAllowed`.
- [ ] Change the return type of `AssetContainerRoots()` to `map[string]struct{}` and
      document that the caller must not mutate. Removes the pointless sort and the
      per-call `bool`-valued conversion.
- [ ] Memoize the sorted slice with `sync.OnceValue[[]string]` and return
      `slices.Clone` per call. Preserves the current API and public shape while
      dropping the repeated allocation + sort.
- [ ] Leave as-is — the cost is small and a second caller has not landed. Only worth
      picking if the placement issue above is being resolved by moving the API
      anyway.

---

## Package-level `skillRoots` is mutable shared state; comment describes refactor not domain

> [!WARNING]
>
> - [Security — Managed-surface fence single source of truth](../../../docs/guidelines/security.md)
> - [Clean Code — Understandability (hidden ordering)](../../../docs/guidelines/clean_code.md#understandability)
> - [Clean Code — Comments](../../../docs/guidelines/clean_code.md#comments)

Two related problems in the same block:

1. **Mutable shared state.** `skillRoots` was previously a function-local literal in
   `addSkillOutputs`, rebuilt on every render call and drift-proof by construction.
   The refactor promoted it to a package-level `var` so `AssetContainerRoots()` can
   read it. Any future mutation inside `render` (a stray `delete`, a test overwriting
   it, an init-time patch) would simultaneously alter render output and
   `app.RegisterableDirs` eligibility, with no compile-time guard.
   `cursorCommandsRoot` is a `const` (immutable). `skillRoots` is not.
2. **Comment describes refactor history, not domain shape.** The current godoc says
   "Kept at package scope so `AssetContainerRoots` can derive from it without
   duplicating strings" — a mechanical justification. The load-bearing detail a future
   reader needs is that Cursor is intentionally in a separate `const` because it
   _flattens_ skills to one `.md` per asset rather than a folder-per-skill.

```go
// before (function-local, drift-proof)
func addSkillOutputs(...) {
    skillRoots := map[string]string{ "codex": ".codex/skills", ... }
}

// after (package-level var, mutation-exposed)
var skillRoots = map[string]string{ "codex": ".codex/skills", ... }
const cursorCommandsRoot = ".cursor/commands"
```

Pick one:

- [x] Make the map immutable in practice: convert `skillRoots` to an unexported
      accessor `func skillRoot(agent string) (string, bool)` backed by a private
      slice + map built via `sync.OnceValue`, and rewrite `addSkillOutputs` to use
      it. Also rewrite the godoc to state the domain shape difference (folder-per-
      skill vs flat-file) instead of the refactor history.
- [ ] Add a `TestSkillRootsUnchanged` regression that pins the exact contents of
      `skillRoots` (and `cursorCommandsRoot`) so accidental mutation trips CI, and
      rewrite the godoc as above.
- [ ] Add a single `// immutable — do not mutate; consumed by app.RegisterableDirs`
      godoc line on the `var` and update the domain-shape comment. Cheapest
      mitigation; relies on convention only.

---

## `RegisterableDirs` inner loop is opaque and quadratic

> [!WARNING]
>
> - [Clean Code — Understandability (explanatory variables)](../../../docs/guidelines/clean_code.md#understandability)
> - [Clean Code — Functions (one level of abstraction)](../../../docs/guidelines/clean_code.md#functions)
> - [Go — Explicit domain helpers over string surgery](../../../docs/guidelines/go.md#return-actionable-errors)

Three intertwined smells in the rewritten body:

1. The `i > 0` / `parent = ""` conditional reads as a numeric quirk rather than its
   domain intent ("a directory whose parent is the empty root can never be under a
   container root, so skip it"). Container roots are two-segment paths
   (`.claude/skills`), so a leaf at `parts[0]` can never match — the branch exists
   only to bootstrap the ancestor walk.
2. The inner `strings.Join(parts[:i+1], "/")` allocates a new string per level of the
   walk, giving O(n²) allocations per change on deep paths. The Go standard library
   offers `path.Dir` for exactly this slash-key semantics; the ancestor walk collapses
   to two `path.Dir` calls.
3. The two counters `total` / `unknown` + the post-loop `total == unknown` predicate
   are load-bearing but nameless. A reader must reconstruct "all leaves unknown" from
   `n > 0 && unknown[dir] == n`.

```go
// current
for _, ch := range changes {
    parts := strings.Split(ch.Path, "/")
    for i := 0; i < len(parts)-1; i++ {
        parent := ""
        if i > 0 {
            parent = strings.Join(parts[:i], "/")
        }
        if !roots[parent] {
            continue
        }
        dir := strings.Join(parts[:i+1], "/")
        total[dir]++
        if ch.Kind == ChangeUnknown {
            unknown[dir]++
        }
    }
}
for dir, n := range total {
    if n > 0 && unknown[dir] == n {
        out[dir] = true
    }
}

// proposed (all three fixes)
type leafCounts struct{ total, unknown int }
counts := map[string]leafCounts{}
for _, ch := range changes {
    dir := path.Dir(ch.Path)     // ".claude/skills/foo"
    root := path.Dir(dir)        // ".claude/skills"
    if !roots[root] {
        continue
    }
    c := counts[dir]
    c.total++
    if ch.Kind == ChangeUnknown {
        c.unknown++
    }
    counts[dir] = c
}
for dir, c := range counts {
    if c.total > 0 && c.unknown == c.total {
        out[dir] = true
    }
}
```

Pick one:

- [x] Replace the ancestor walk with `path.Dir(path.Dir(ch.Path))`, drop the
      `i > 0` / `parent = ""` branch, and collapse the two counters into a
      `leafCounts` struct (proposed snippet above). Fixes all three smells in one
      edit.
- [ ] Keep the ancestor walk (in case a future rule allows deeper roots) but hoist
      the `strings.Join` out of the recomputation by tracking `dir` incrementally
      (`dir = parts[0]` then `dir += "/" + parts[i]`) and extract an
      `allLeavesUnknown(dir string) bool` helper. Preserves the shape, kills the
      allocations, and names the invariant.
- [ ] Start the loop at `i := 1` and drop the empty-parent branch — narrowest fix,
      only addresses the readability smell. Leaves the quadratic allocation and
      opaque counters in place.

---

## `sort.Strings` should be `slices.Sort`

> [!WARNING]
>
> - [Go — Idiomatic stdlib usage (Go 1.21+)](../../../docs/guidelines/go.md#keep-packages-cohesive)

`AssetContainerRoots()` uses `sort.Strings(out)` in a file that already imports and
uses `slices.Sort` and `slices.SortFunc`. The `"sort"` import exists only for this one
call. Since Go 1.21 the generic form is the idiomatic choice and matches the rest of
the file.

```go
// current
import "sort"
sort.Strings(out)

// proposed
slices.Sort(out)
```

Pick one:

- [x] Replace `sort.Strings(out)` with `slices.Sort(out)` and drop the `"sort"`
      import.
- [ ] Skip — pick this if the placement/shape issues above are being resolved by
      dropping the sort entirely (memoize + clone, or return a map).

---

## Domain rule is split across `render` (roots) and `app` (predicate)

> [!WARNING]
>
> - [Domain Model — Put Domain Rules In Domain Code](../../../docs/guidelines/domain_model.md#put-domain-rules-in-domain-code)

The registration-eligibility rule has two clauses: (a) the folder's parent path is a
known asset-container root, (b) every descendant leaf is unknown. Clause (a) is data
owned by `render.AssetContainerRoots()`; clause (b) is enforced inline in
`app.RegisterableDirs`. Reading the full rule requires opening two packages. This
mirrors the pattern `surfaces.IsAllowed` avoids (surface list + membership check live
together).

Pick one:

- [x] Promote the whole predicate into the same package as the roots:
      `render.RegisterableFolders(changes []render.FolderClassifier) map[string]bool`
      (or an equivalent predicate `IsRegisterableFolderKey(dirKey string, changes ...) bool`).
      `app.RegisterableDirs` becomes a thin adapter over `[]FileChange`, mirroring
      the existing `ChangeKind` / `Preview` mirror layer.
- [ ] Keep the split but add a single-place comment (either at
      `render.AssetContainerRoots` or at `app.RegisterableDirs`) that spells out
      **both** clauses of the rule as one sentence and cross-references the other
      package's contribution. Cheapest mitigation.
- [ ] Skip — this issue is a consequence of the placement decision above and is
      subsumed once the root list moves into `surfaces` (or a new package) that
      also hosts the predicate.

---

## "Asset-container root" is a durable term but is not in the glossary

> [!WARNING]
>
> - [Domain Model — Use The Project Language](../../../docs/guidelines/domain_model.md#use-the-project-language)

The plan defends "the term is coined locally in a Godoc and does not need elevation
to glossary for one internal helper." But the term now appears in an exported function
name (`render.AssetContainerRoots`), the docstring on `RegisterableDirs`, the task
description (twice), and the changelog (four times). Neighboring terms — Managed
Surfaces, Managed State, Projection — are already glossary-defined. Leaving
"asset-container root" out creates real ambiguity between three overlapping-but-
distinct sets (`skillRoots` map, `AssetContainerRoots`, "managed surfaces").

Pick one:

- [ ] Add an `## Asset Container Root` entry to `docs/glossary.md` defining it as a
      strict subset of Managed Surfaces (the four fixed folder-shaped subpaths whose
      direct children are eligible for asset registration), and cross-link it from
      the Managed Surfaces entry.
- [x] Add the glossary entry above **plus** a `## Registerable Folder` entry that
      states the full eligibility rule as one sentence, so the domain rule has one
      canonical statement.
- [ ] Skip — accept the plan's justification. Only pick if the term is being renamed
      or eliminated by one of the placement fixes.

---

## Naming inconsistency: `skillRoots` vs `AssetContainerRoots`

> [!WARNING]
>
> - [Domain Model — Use The Project Language](../../../docs/guidelines/domain_model.md#use-the-project-language)
> - [Clean Code — Naming (meaningful distinctions)](../../../docs/guidelines/clean_code.md#naming)

Two adjacent symbols in the same file name the same concept differently. `skillRoots`
is the per-agent skill-projection map. `cursorCommandsRoot` is a const. The new
`AssetContainerRoots()` returns the union of both. So `skillRoots` was renamed-in-
concept but not renamed-in-code — the new export treats all four as "asset-container
roots", yet the field name still says "skill". A future contributor adding a fifth
root will not know whether to append to `skillRoots` or introduce another package
symbol. Additionally, `.cursor/commands` is Cursor's _command_ container, not a skill
container per se; collapsing them under one umbrella name should be reflected in the
symbol names.

Pick one:

- [x] Introduce a package-level `assetContainerRoots []string` slice as the single
      source of truth and have `AssetContainerRoots()` return `slices.Clone` of it,
      while `skillRoots` (renamed `skillContainerRoots` if kept as an agent → path
      map) is used only by `addSkillOutputs`. Each name reflects its actual role and
      the "asset-container root" set has one greppable declaration.
- [ ] Rename `skillRoots` → `skillContainerRoots` and update the godoc so it does
      not claim to be the source of truth (the current phrasing "kept at package
      scope so AssetContainerRoots can derive from it" invites drift when the next
      root is added).
- [ ] Skip — the two names are close enough. Only pick if the mutable-var and
      placement issues above are being resolved together, and the renaming would
      cause churn.

---

## `FolderNotRegisterableError` docstring is stale and does not carry a reason

> [!WARNING]
>
> - [Domain Model — Use The Project Language](../../../docs/guidelines/domain_model.md#use-the-project-language)
> - [Errors — Actionable typed errors](../../../docs/guidelines/errors.md#use-typed-errors-for-domain-failures)

The typed error's docstring at `internal/app/errors.go` still describes only the old
rule ("not an all-unknown folder in the freshly computed plan"). Under the tightened
rule the same error fires for two distinct reasons: (a) parent is not a container
root (nested / above-root / outside), or (b) partly-managed folder. The TUI receives
identical `FolderNotRegisterableError{DirKey: "…"}` payloads for very different
corrective actions (register the parent vs. resolve the managed leaves), and the
docstring under-specifies the new dominant case.

```go
// current
type FolderNotRegisterableError struct { DirKey string }
// docstring: "not an all-unknown folder in the freshly computed plan"
```

Pick one:

- [ ] Update the godoc only: state both rejection modes ("parent is not a known
      asset-container root, OR the folder has at least one managed descendant") and
      cross-reference `render.AssetContainerRoots`. No behavior change.
- [x] Add a `Reason` field to `FolderNotRegisterableError` (e.g. constants
      `ReasonNotUnderContainerRoot`, `ReasonHasManagedDescendants`,
      `ReasonAbsentFromPlan`) so the TUI can produce a targeted message. `DirKey`
      stays as-is; callers that ignore `Reason` see no behavior change.
- [ ] Extend `Error()` to compute the reason at message-render time from
      `render.AssetContainerRoots()` and the plan's change set — no new field, but
      the returned string differentiates the two modes. Lighter than a `Reason`
      field but re-derives context that the caller already has.
- [ ] Skip — the description says "no new error type" and the current single-
      message form is acceptable. Only pick if a follow-up task is filed and
      recorded in the changelog under "Not done".

---

## `TestRegisterableDirs` is not a `t.Run` table

> [!WARNING]
>
> - [Testing — Test One Behavior At A Time](../../../docs/guidelines/testing.md#test-one-behavior-at-a-time)

The description literally frames the acceptance criteria as a table ("Table holds:
`.claude/skills/foo` → registerable; `.claude/skills/foo/bar` → not; …"). The
implementation runs two `for _, dir := range …` loops with shared fixture and
`t.Errorf`, so a regression on (say) `.claude/skills/nest/deep` reports only `dir
".claude/skills/nest/deep" must NOT be registerable` — no subtest name pins which
acceptance-criterion row broke, and the loop keeps executing.

```go
// proposed shape
cases := []struct {
    name   string
    dir    string
    wantOK bool
}{
    {"direct child all-unknown", ".claude/skills/foo", true},
    {"direct child partly-managed", ".claude/skills/bar", false},
    {"nested one level deeper", ".claude/skills/nest/deep", false},
    {"container root itself", ".claude/skills", false},
    {"above container root", ".claude", false},
    {"outside any container root", "docs/whatever", false},
}
for _, tc := range cases {
    t.Run(tc.name, func(t *testing.T) { /* assert against got */ })
}
```

Pick one:

- [x] Convert `TestRegisterableDirs` into a `t.Run`-per-row table so each acceptance-
      criterion row has its own named subtest and can be filtered/rerun
      individually.
- [ ] Keep the shared-fixture loops but embed the row _category_ into the error
      message (e.g. `dir %q [category=%s] must NOT be registerable`, category ∈
      {nested, partly-managed, root, outside}) so failure diagnosis does not require
      re-reading the fixture.
- [ ] Skip — the current shape works; only pick if adding rows becomes painful.

---

## `render.AssetContainerRoots()` has no direct unit test

> [!WARNING]
>
> - [Testing — Start With The Smallest Useful Test](../../../docs/guidelines/testing.md#start-with-the-smallest-useful-test)

`AssetContainerRoots()` is exported as the single source of truth for a domain rule.
Its only current coverage is indirect via `TestRegisterableDirs` — if someone omits
`.cursor/commands` (or adds a fifth root incorrectly), the failure surfaces far from
the cause, and any future consumer has no test to lean on. The function is trivially
unit-testable (pure, no I/O).

```go
// suggested — internal/render/render_test.go
func TestAssetContainerRoots(t *testing.T) {
    got := render.AssetContainerRoots()
    want := []string{
        ".claude/skills",
        ".codex/skills",
        ".cursor/commands",
        ".opencode/skills",
    }
    if !slices.Equal(got, want) {
        t.Fatalf("AssetContainerRoots() = %v, want %v", got, want)
    }
}
```

Pick one:

- [x] Add `TestAssetContainerRoots` in `internal/render/render_test.go` asserting
      the exact sorted contents and length, plus a mutate-and-recall check pinning
      the "caller may mutate" godoc promise.
- [ ] Add a minimal `TestAssetContainerRoots` asserting only the contents (skip the
      mutate-and-recall check). Cheapest guardrail.
- [ ] Skip — the indirect coverage via `TestRegisterableDirs` is judged sufficient.

---

## Missing service-boundary negative cases for `.claude/skills` and `.claude`

> [!WARNING]
>
> - [Testing — Test One Behavior At A Time](../../../docs/guidelines/testing.md#test-one-behavior-at-a-time)

`TestCreateAssetFromFolder_NestedFolderRejected` covers the nested case at the
service boundary (good). But two other rows from the acceptance table have **no
service-level regression test**: passing `.claude/skills` (the container root itself)
and `.claude` (a top-level managed surface) as `dirKey` to `CreateAssetFromFolder`.
`TestCreateAssetFromFolder_NonRegisterableFolderRejected` only covers
`.claude/skills/missing` (a nonexistent direct child) — a semantically different
case that trips a different branch.

Pick one:

- [x] Add `TestCreateAssetFromFolder_ContainerRootRejected` and
      `TestCreateAssetFromFolder_AboveContainerRootRejected` mirroring the
      structure of `_NestedFolderRejected` (assert `FolderNotRegisterableError` +
      `len(prof.Profile.Assets) == 0` + `!slices.Contains(p.SelectedAssetIDs, ...)`)
      for `dirKey = ".claude/skills"` and `dirKey = ".claude"` respectively.
- [ ] Fold all four rejection paths into one table-driven
      `TestCreateAssetFromFolder_RegisterabilityRejections` with rows for `nested`,
      `container-root`, `above-root`, and `outside`. Consolidates the existing
      `_NestedFolderRejected` and `_NonRegisterableFolderRejected` into the same
      table.
- [ ] Skip — the unit-level `TestRegisterableDirs` proves the rule at the classifier
      layer, and the service-level check is considered a redundant re-assertion.

---

## `TestCreateAssetFromFolder_NestedFolderRejected` does not assert project selection is unchanged

> [!WARNING]
>
> - [Testing — Assert Behavior, Not Mock Mechanics](../../../docs/guidelines/testing.md#assert-behavior-not-mock-mechanics)

Acceptance criterion 5 requires "returns `FolderNotRegisterableError` **and writes
nothing**." The new test asserts `len(prof.Profile.Assets) == 0` but does not check
`p.SelectedAssetIDs`. The sibling `TestCreateAssetFromFolder_InvalidSourceLeavesNoPartialState`
pins the stronger invariant that project selection is also unchanged; a regression
that mutates the project manifest but not the profile aggregate would slip past the
current assertion.

```go
// current
if len(prof.Profile.Assets) != 0 {
    t.Fatalf("expected zero assets after rejected registration, got %v", prof.Profile.Assets)
}

// proposed addition — parallels the InvalidSource sibling
p, _ := svc.LoadProject(profileID, projectID)
if slices.Contains(p.SelectedAssetIDs, "nested") {
    t.Fatal("asset selected despite rejection")
}
```

Pick one:

- [x] Extend `TestCreateAssetFromFolder_NestedFolderRejected` to also assert
      `!slices.Contains(p.SelectedAssetIDs, "nested")`, matching the
      `InvalidSource` sibling's shape.
- [ ] Skip — the acceptance criterion says "writes nothing" and the current profile-
      side assertion is judged sufficient because `SelectAsset` is only called after
      the guard passes (see `CreateAssetFromFolder` at `service.go:293`).
