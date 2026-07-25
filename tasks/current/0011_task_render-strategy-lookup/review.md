# render-strategy-lookup review

The refactor is clean and lands all 13 acceptance criteria: `make fmt && make test && make lint && make build` are green, the `a.Type` dispatch switch is gone from `render.go`, `assetProjectionDirs`/`owningAssetSourceRelFor` are deleted from `sync.go`, the reverse mapping is consolidated behind `render.ProjectPlan.ReverseLookup`, and ADR 0021 + the mermaid diagrams shipped. **Security review found nothing** — the `CLAUDE.md` fence widening is symmetric with `AGENTS.md`, and the write-back path is still contained by `asset.ResolveRelative`. The DoD gate passed on all three substeps (contract sections present, every criterion met, no scope creep).

Findings below are all **minor or optional** — none block the task. The strongest signals are two testing gaps (the golden was hand-authored rather than captured from the pre-refactor `Build`, and the one intentional behavior change — the uniform `SupportsAgent` gate — has no direct coverage) and two documentation drifts (glossary + project `CLAUDE.md` still describe pre-refactor `agents_doc` behavior). Pick one checkbox per issue.

## Unresolved `// should not happen` comment in `walkProjectionFiles`

> [!WARNING]
>
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) — leave touched code clearer than you found it
> - [docs/guidelines/errors.md](../../../docs/guidelines/errors.md) — exception #2 (programming/environment bug → fail fast)

`internal/render/render.go:204` carries a speculative question committed into shipping code, above the `filepath.Rel` fallback. It predates the commit but this task moved/renamed the function into the strategy structure, so it is touched code. The reader cannot tell whether `rel = filepath.Base(path)` is a deliberate safe degradation or a known-incomplete stub. In practice `source` is always an ancestor of `path` inside a `WalkDir` rooted at `source`, so `Rel` cannot fail; if it ever did, `filepath.Base` would flatten a nested file to its bare name, corrupting both `Path` and `SourceRel`.

```go
rel, relErr := filepath.Rel(source, path)
// should not happen, maybe produce error here instead?   // <- decide this
if relErr != nil {
    rel = filepath.Base(path)
}
```

Choose one:

- [ ] Replace the question with a settled intent comment stating the invariant, e.g. `// filepath.Rel cannot fail: source is always an ancestor of path; Base is a defensive no-op.`
- [x] Treat a failure as a bug per errors.md exception #2: append a typed `AssetReadError{Op: "rel"}` (via `classifyFileError`) and `return nil` to skip the file, dropping the lossy `Base` fallback.

## `ReverseLookup` returns a three-value tuple instead of a named struct

> [!WARNING]
>
> - [docs/guidelines/go.md](../../../docs/guidelines/go.md) — "Prefer Structs Over Tuples" (three or more values must be a named struct)

`internal/render/reverse.go:30` exports `func (p *ProjectPlan) ReverseLookup(repoPath string) (assetID, sourceRel string, ok bool)`. go.md exempts only two-value idioms (`(T, error)`, `(value, ok)`, `(index, found)`); this is a three-value return whose first two positions are same-typed strings — exactly the silent argument-order-swap risk the guideline names. The shape was inherited verbatim from the deleted `sync.owningAssetSourceRelFor`, but the refactor promoted it to an exported method, which raises the bar. The `sync.go:323` call site already discards two returns (`assetID, _, _`), which a `.AssetID` field would make self-documenting. The neighboring `owningDir` / `ReverseContext` / `reverseIndex` structs already follow the rule; only the public entry point regresses.

```go
type ReverseMatch struct {
    AssetID   string
    SourceRel string
}
func (p *ProjectPlan) ReverseLookup(repoPath string) (ReverseMatch, bool) // (value, ok) — exempt
```

Choose one:

- [x] Introduce `ReverseMatch{AssetID, SourceRel}` and return `(ReverseMatch, bool)`; update both `sync.go` call sites and the tests.
- [ ] Keep the tuple as a documented exception (the two strings are always consumed together) and add a one-line note on the method explaining why the struct rule is waived.

## `ReverseContext` is a covert union whose exact-match arm silently trusts `SourceRel != ""`

> [!WARNING]
>
> - [docs/guidelines/solid.md](../../../docs/guidelines/solid.md) — LSP/ISP (a type whose valid field-combinations are enforced only by caller discipline)
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) — avoid hidden ordering/population requirements

`ReverseLookup`'s exact-hit branch (`reverse.go:38-42`) populates only `RepoPath`/`ExactSourceRel`/`AssetID`; the sibling branch (`reverse.go:57-62`) populates only `RepoPath`/`TargetRoot`/`SourceRoot`/`AssetID`. So `ReverseContext` is really a union where only one subset of fields is ever valid, and `suffixReverse.Reverse` (`strategies.go:23-28`) branches on which subset the caller filled. This works _because_ every strategy's `Render` stamps a non-empty `SourceRel`; if a future bodyless render ever left it empty, the exact arm would fall through to `reverseSuffixPreserving` with empty roots and return `ctx.RepoPath` as the source — a wrong-but-`ok=true` answer. The exact arm also re-runs `strategyFor(rf.Agent, rf.Type)` (`reverse.go:34-37`), a lookup that provably cannot miss (the file only exists because `Build` found a strategy), so the two directions independently re-resolve provenance through the global map instead of sharing one handle.

```go
src, rok := strat.Reverse(ReverseContext{
    RepoPath:       repoPath,
    ExactSourceRel: rf.SourceRel,   // if "", the empty-root fallback returns a wrong ok=true
    AssetID:        rf.AssetID,
})
```

Choose one:

- [ ] Document on `ReverseContext` that `ExactSourceRel` and (`TargetRoot`,`SourceRoot`) are mutually exclusive population modes, so the contract is explicit for future strategies.
- [ ] Guard the exact arm: if `rf.SourceRel == ""`, treat it as no exact hit (fall to the owner walk) so a future bodyless render cannot produce a silently-wrong reverse.
- [x] Carry the resolved `Strategy` on `RenderedFile`/`owningDir` so `Reverse` dispatches off a non-nil value and the second `strategyFor` lookup disappears.

## `suffixReverse` gives single-file strategies an unreachable sibling-walk arm

> [!WARNING]
>
> - [docs/guidelines/solid.md](../../../docs/guidelines/solid.md) — SRP/ISP (expose only the behavior a type can exercise)

`suffixReverse` (`strategies.go:21-55`) has two arms: exact-match and `reverseSuffixPreserving` (tail arithmetic for an untracked sibling). It is embedded by four strategies, but the two single-file, root-level ones — `agentsDocStrategy` (`CLAUDE.md`/`AGENTS.md`) and `settingsStrategy` (`.codex/config.toml`) — have no directory to host a sibling, so their `reverseSuffixPreserving` arm is unreachable. `agentsDocStrategy`'s doc comment (`strategies.go:129-133`) even has to document _around_ the inherited-but-inapplicable arm. Under SRP the embed bundles two reasons to change (exact-match policy vs. dir-tail arithmetic) into strategies that only need the first. This is cheap coupling, not a bug.

```go
func (suffixReverse) Reverse(ctx ReverseContext) (string, bool) {
    if ctx.ExactSourceRel != "" {
        return ctx.ExactSourceRel, true   // only reachable arm for agents_doc/settings
    }
    return reverseSuffixPreserving(ctx)    // dead for single-file root targets
}
```

Choose one:

- [x] Split into an `exactReverse` embeddable (exact-match only) for agents_doc/settings and keep `suffixReverse` (both arms) for the dir-shaped skill/generic strategies.
- [ ] Leave as-is (the coupling is cheap and tested) and keep the per-strategy doc note.

## `sync.Apply` reconstructs a `render.ProjectPlan` from `preview.Files`

> [!WARNING]
>
> - [docs/guidelines/clean_architecture.md](../../../docs/guidelines/clean_architecture.md) — Stable Dependencies (don't hide a "Files is the whole plan" assumption behind a reconstruction)

`newApplyLoop` builds `&render.ProjectPlan{Files: preview.Files}` (`internal/sync/sync.go:549`) to get a `ReverseLookup` receiver instead of threading the original `*render.ProjectPlan` from `Plan` through `Preview`. It is correct today because `ReverseLookup` depends only on `Files` (and the lazily-built `revIndex`, derived from `Files`). The coupling direction is right (volatile `sync` → stable `render`), but the reconstruction hides an assumption that isn't type-enforced: a future `ProjectPlan` field that `ReverseLookup` comes to need would be silently zero here.

```go
plan: &render.ProjectPlan{Files: preview.Files}, // sync.go:549 — reconstructs rather than carries the plan
```

Choose one:

- [ ] Add a one-line comment on `ProjectPlan` stating `ReverseLookup` must depend only on `Files` so the sync-side reconstruction stays valid.
- [x] Thread the real `*render.ProjectPlan` into `Preview` (e.g. `Preview.Plan`) so Apply reuses the same instance and its memoized `revIndex`.

## Golden test is hand-authored inline, not captured from the pre-refactor `Build`

> [!WARNING]
>
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md) — don't let both sides of an equality drift together; capture the oracle independently

The AC (`description.md:131-134`) requires `TestBuild_UnchangedPairs_Golden` to be byte-/path-identical to a golden **captured from the pre-refactor `Build`**. What shipped in `strategy_test.go:63-135` is a hand-authored inline `golden` map (the test's own comment says "hand-authored golden"); no snapshot was ever taken from `58af094`. The values are independently checkable against `agent.Descriptors()`/`surfaces`, so the risk is low — but the AC's drift-safety mechanism (a human cannot re-encode the same mistake the refactor made) was substituted for a weaker one.

```go
// AC wanted: capture from 58af094 Build, then diff. What shipped: values typed by hand.
golden := map[string]want{
    ".claude/settings.local.json": {"claude-cfg", "claude-code.json"},
    // ...
}
```

Choose one:

- [x] Regenerate the golden by checking out `58af094`, running `Build` over the same fixture, serializing `plan.Files` into a `testdata/*.json`, and asserting against that file (note the capture in a comment).
- [ ] Keep the inline form but update the `description.md`/`plan.md` AC wording to "hand-authored golden independently derived from the descriptor/surfaces tables" so the test matches its stated contract.

## The one intentional behavior change — the uniform `SupportsAgent` gate — has no direct test

> [!WARNING]
>
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md) — cover the behavior you deliberately changed, not just the unchanged ones

The changelog (`2026-07-24_0011-...md:47-51`) flags a real semantic change: `Build` now applies `asset.SupportsAgent` uniformly at the dispatch gate (`render.go:146`), where the old codex `agents_doc` arm bypassed `compatible_agents`. The golden fixture uses empty `compatible_agents`, so **no test exercises the new restriction path**. A regression that turned the `continue` on `!SupportsAgent` into an `UnsupportedRenderingError` (or vice-versa) would ship green — and the distinction matters: an unsupported agent must be silently skipped, not reported as a missing strategy.

```go
if !asset.SupportsAgent(a, ag) {
    continue // untested: skip vs. UnsupportedRenderingError
}
```

Choose one:

- [x] Add a test: an `agents_doc` asset with `compatible_agents:["claude-code"]` enabled for codex+claude renders only `CLAUDE.md`, emits no `AGENTS.md`, and produces **no** `UnsupportedRenderingError`.
- [ ] Add a `compatible_agents`-restricted asset to the existing golden fixture so the skip path is pinned alongside the byte-identical pairs.

## Missing-pair test proves a single error, not accumulation

> [!WARNING]
>
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md) — name the test after the behavior it actually verifies

`TestStrategyFor_MissingPair_AggregatesUnsupportedError` (`strategy_test.go:141-176`) drives the error path with one bogus agent and one selected asset, yielding exactly one error. The assertion style is correct (`errors.As` on the typed error, per errors.md), but with a single failing element there is nothing to short-circuit past, so the "aggregates / never short-circuits" claim in the name is not demonstrated.

```go
EnabledAgents:    []agent.Agent{agent.Agent("bogus")},
SelectedAssetIDs: []string{"review"},   // one asset → one error; accumulation not proven
```

Choose one:

- [x] Add a second selected asset (or a second bogus agent) and assert `len(buildErrs) == 2` with both `UnsupportedRenderingError`, proving the loop keeps going after the first miss.
- [ ] Rename the test to `..._YieldsUnsupportedError` to match what it verifies.

## Settings/agents_doc reverse round-trip only exercises the exact-match arm

> [!WARNING]
>
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md) — add a non-happy-path variant so the assumption can't ride the exact-hit path

Both settings and both agents_doc assertions in `TestReverse_Settings_And_AgentsDoc_RoundTrip` (`strategy_test.go:242-245`) resolve through the exact-hit arm, which returns the stored `ExactSourceRel` verbatim. For the two strategies whose whole point is a filename change (`codex.toml`→`config.toml`, `AGENTS.md`→`CLAUDE.md`), the reverse test therefore only proves "the field we stored round-trips," never that the strategy offers no spurious Adopt for a stray neighbor. Single-file renames legitimately have no sibling, so exact-match is the only production path — this is a coverage note, not a bug. (The genuinely independent reverse coverage lives in the skill/generic **sibling** cases; a future editor should not delete those thinking the exact-hit assertions suffice.)

Choose one:

- [x] Add a negative case: `ReverseLookup(".claude/settings.local.json.bak")` (an untracked neighbor of a single-file target) returns `ok=false`, pinning that settings offers no spurious Adopt.
- [ ] Accept as-is and add a comment on the test noting the exact-hit arm is the only reachable inverse for single-file targets, and that the sibling cases carry the independent coverage.

## Glossary "Exclusive Group" example contradicts per-agent `agents_doc`

> [!WARNING]
>
> - [docs/guidelines/domain_model.md](../../../docs/guidelines/domain_model.md) — "Use The Project Language" (docs must not drift from code behavior)

`docs/glossary.md` (Exclusive Group, ~lines 190-201) still justifies the concept with "two `agents_doc` assets that both populate `AGENTS.md`." After this commit `agents_doc` renders per-agent, so `(ClaudeCode, AgentsDoc)` populates `CLAUDE.md` — two such assets collide on `CLAUDE.md` for claude-code and on `AGENTS.md` for the others. The commit updated the glossary in other spots (added `Render Strategy`, `CLAUDE.md` managed surface, the Adopt pointer) but left this example describing pre-refactor reality. The nearby `Adopt` entry (~lines 319-342) is similarly incomplete — an adopted `AGENTS.md` edit now has `CLAUDE.md` as a sibling projection sharing one source.

Choose one:

- [x] Update the Exclusive Group prose to note the target is now per-agent (a shared group prevents a collision on whichever target the enabled agents share), and optionally add the `CLAUDE.md`↔`AGENTS.md` case to the Adopt entry.
- [ ] Replace the example with a neutral one that does not hinge on a fixed single target.

## Project `CLAUDE.md` render bullet omits the now-uniform `compatible_agents` enforcement

> [!WARNING]
>
> - [docs/guidelines/domain_model.md](../../../docs/guidelines/domain_model.md) — "Model Constraints Explicitly" / durable behavior changes belong in authoritative docs

The project-root `CLAUDE.md` `render` bullet documents `exclusive_group` and `compatible_agents` resolution generally but was not updated to record that `agents_doc` now honors `compatible_agents` uniformly via the dispatch gate (where the old codex arm bypassed it — ADR 0021, changelog). Because `CLAUDE.md` is itself an authoritative instruction file, this durable behavior change deserves a one-line mention there, not only in the changelog/ADR.

Choose one:

- [x] Add a short clause to the `render` bullet in `CLAUDE.md` noting `agents_doc` now honors `compatible_agents` uniformly via the strategy dispatch gate (ADR 0021).
- [ ] Leave as-is and treat the changelog + ADR 0021 as the canonical record of the change.

## Minor: `reverseSuffixPreserving` hand-rolls a prefix check `strings.HasPrefix` expresses

> [!WARNING]
>
> - [docs/guidelines/go.md](../../../docs/guidelines/go.md) — readability (idiom nit, not a violation)

`internal/render/reverse.go:42` writes the "is `RepoPath` under `TargetRoot`" test as `len(ctx.RepoPath) > len(ctx.TargetRoot) && ctx.RepoPath[:len(ctx.TargetRoot)+1] == ctx.TargetRoot+"/"`. Correct, but it forces the reviewer to re-derive the `+1` off-by-one; the file already imports `strings`. (The tagless `switch` on path shape is idiomatic and does not conflict with the "map lookup never a switch" dispatch requirement, which governs dispatch, not local control flow.)

```go
// current
case len(ctx.RepoPath) > len(ctx.TargetRoot) && ctx.RepoPath[:len(ctx.TargetRoot)+1] == ctx.TargetRoot+"/":
    tail = ctx.RepoPath[len(ctx.TargetRoot)+1:]
// clearer
case strings.HasPrefix(ctx.RepoPath, ctx.TargetRoot+"/"):
    tail = strings.TrimPrefix(ctx.RepoPath, ctx.TargetRoot+"/")
```

Choose one:

- [x] Replace the manual slice/length check with `strings.HasPrefix` + `strings.TrimPrefix`.
- [ ] Leave as-is (functionally correct, covered by round-trip tests).
