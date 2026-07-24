# Typed compatible-agent value — review

The implementation is correct, faithful to the approved plan, and green on
`make fmt && make build && make test && make lint` (782 tests pass). All ten
acceptance criteria are backed by real, typed-error assertions. Security and
clean-architecture reviews returned **no findings**: `config` stays at the
bottom of the import graph (only stdlib `slices`), the `surfaces` string-bridge
is a narrow one-directional `string(agent)` at two render call sites, the new
load-time validation _strengthens_ the input boundary, and error messages echo
only closed-vocabulary agent tokens (no paths/secrets).

The findings below are refinements, not defects. Two are worth acting on before
merge (the duplicated collector logic and the duplicated `Error()` loop, which
can reuse the bridge this very task introduced). Several testing gaps leave
deliberate behavior (dedup, cross-source merge, `SupportsAgent` empty-means-all,
project-side JSON round-trip) unverified. The rest are judgment calls (domain
home of `Agent`, glossary sync) and optional idiom nits.

Tick exactly one checkbox per issue for the solution you want applied.

## Duplicated unknown-agent collector logic across `asset` and `project`

> [!WARNING]
>
> - [`docs/guidelines/clean_code.md`](../../../docs/guidelines/clean_code.md) — needless repetition of a rule that can drift apart
> - [`docs/guidelines/solid.md`](../../../docs/guidelines/solid.md) — SRP; the "recognized-set" rule wants one owner

`asset.unknownAgents` (`internal/asset/asset.go:147`) and
`project.unknownEnabledAgents` (`internal/project/project.go:74`) implement the
identical algorithm — walk `[]config.Agent`, skip `config.IsKnownAgent`,
dedup via a `seen` map, preserve order. The only difference is `asset` folds in
projection agents as a second source. The membership+dedup core is byte-similar
and can silently drift. `config` already owns `IsKnownAgent`/`AllAgents`, so the
pure collection logic has a natural home next to them.

```go
// asset.go — collect over two sources through one seen map
func unknownAgents(m Manifest) []config.Agent {
	var unknown []config.Agent
	seen := map[config.Agent]bool{}
	collect := func(a config.Agent) {
		if config.IsKnownAgent(a) || seen[a] { return }
		seen[a] = true
		unknown = append(unknown, a)
	}
	for _, a := range m.CompatibleAgents { collect(a) }
	for _, p := range m.Projections { collect(p.Agent) }
	return unknown
}

// project.go — same core, one source. Duplicated dedup rule.
func unknownEnabledAgents(m *Manifest) []config.Agent { /* identical seen/append loop */ }
```

Choose one:

- [x] Add `config.UnknownAgents(as []config.Agent) []config.Agent` (order-preserving, deduped) next to `IsKnownAgent`; `asset` calls it over its two concatenated sources, `project` over `EnabledAgents`. Each package keeps its own typed error and only wraps the result.
- [ ] Accept the duplication as intentional per-package ownership (two call sites, ~10 lines) and add a one-line comment in each collector noting the deliberate non-abstraction, consistent with the guideline's "don't add indirection for pattern completeness".

## Duplicated `Error()` id-join loop reinvents the new `config.AgentStrings` bridge

> [!WARNING]
>
> - [`docs/guidelines/clean_code.md`](../../../docs/guidelines/clean_code.md) — duplicated transformation logic
> - [`docs/guidelines/go.md`](../../../docs/guidelines/go.md) — reuse the package's own helpers; return actionable errors without hand-rolled plumbing

`UnknownCompatibleAgentError.Error()` (`internal/asset/errors.go:103`) and
`UnknownEnabledAgentError.Error()` (`internal/project/errors.go:52`) contain a
byte-identical `[]config.Agent → comma-joined string` loop, differing only in
the message prefix. This same task added `config.AgentStrings`, which does
exactly that conversion, nil-safely — both methods can reuse it.

```go
// both errors.go files, twice:
ids := make([]string, len(e.Agents))
for i, a := range e.Agents {
	ids[i] = string(a)
}
return fmt.Sprintf("unknown compatible agent(s): %s", strings.Join(ids, ", "))

// could be, reusing the bridge this task introduced:
return fmt.Sprintf("unknown compatible agent(s): %s", strings.Join(config.AgentStrings(e.Agents), ", "))
```

Choose one:

- [x] Replace both hand-rolled loops with `strings.Join(config.AgentStrings(e.Agents), ", ")` (drops the `make`/loop in both files).
- [ ] Leave as-is — two tiny loops, no functional defect — if you prefer the error packages not depend on the bridge for formatting.

## `IsKnownAgent` allocates a fresh `AllAgents()` slice on every call

> [!WARNING]
>
> - [`docs/guidelines/go.md`](../../../docs/guidelines/go.md) — cohesive, non-wasteful domain helpers

`IsKnownAgent` calls `slices.Contains(AllAgents(), a)`
(`internal/config/agents.go:44`), and `AllAgents()` allocates a new 4-element
slice each call (`agents.go:32`). The collectors invoke `IsKnownAgent` once per
manifest agent, so a manifest with N agents allocates N throwaway slices. The
recognized set is a compile-time constant, so this is pure waste (functionally
correct, N is small — hence low severity).

```go
func IsKnownAgent(a Agent) bool {
	return slices.Contains(AllAgents(), a) // new []Agent allocated every call
}
```

Choose one:

- [x] Back membership with a package-level `var knownAgents = map[Agent]struct{}{...}` (built once); keep `AllAgents()` returning a defensive copy for external iteration.
- [ ] Leave as-is — N ≤ a handful, allocation is negligible in practice.

## `config.Agent` is a first-class domain value object living in the metadata-constants package

> [!WARNING]
>
> - [`docs/guidelines/domain_model.md`](../../../docs/guidelines/domain_model.md) — each domain concept should have one clear owner; value objects for concepts with rules

`config.Agent` is now a genuine value object: a closed set (`AllAgents`),
self-validating (`IsKnownAgent`), driving compatibility (`asset`) and enablement
(`project`) rules across the whole model. It sits in `internal/config`, the
package CLAUDE.md frames as "filename and default profile metadata … no internal
deps." Placing the single most cross-cutting domain concept in the infrastructure
grab-bag is a tension with the "one clear owner" principle. It is _defensible_
(`config` was the pre-existing agent SoT, has no internal deps so every package
can import it cycle-free, and moving it is churn) — but worth an explicit call.
Note the CLAUDE.md `config` bullet was already refreshed this task to acknowledge
the ownership.

```go
// internal/config/agents.go — a domain value object beside filename constants
type Agent string
func IsKnownAgent(a Agent) bool { return slices.Contains(AllAgents(), a) }
```

Choose one:

- [ ] Accept as-is (pragmatic: pre-existing SoT, no import cycles, minimal churn); the CLAUDE.md bullet update already documents that `config` now owns a domain value type.
- [x] Extract the agent value object into its own leaf package `internal/agent` (no I/O), leaving `config` for genuine filename/default constants; every consumer imports the new package. Cleaner boundary, larger diff.

## Glossary does not reflect that "Agent" is now a validated closed set

> [!WARNING]
>
> - [`docs/guidelines/domain_model.md`](../../../docs/guidelines/domain_model.md) — update the glossary when a durable domain term appears / stable identifiers

The plan and changelog decided "no glossary term — implementation-level type
refinement." But promoting "agent" from an incidental string to a closed,
self-validating, rejected-at-load type is exactly the kind of durable domain rule
the guideline says to record. The existing `Enabled Agent`
(`docs/glossary.md:135`) and `Compatible Agents` (`docs/glossary.md:140`) entries
now _understate_ the model: they describe the ids as loose "current names," but a
typo is no longer just another name — it is rejected at load time.

```text
Compatible Agents — "... Current names are codex, claude-code, cursor, opencode."
// drifted: does not say the set is now typed and validated-at-load
```

Choose one:

- [ ] Amend the `Enabled Agent` and `Compatible Agents` entries to state the identifier set is now typed (`config.Agent`) and rejected at load time if unknown — keeps the "no new term" decision while ending the drift.
- [x] Add a dedicated "Agent" glossary entry (closed, self-validating id set) and cross-link the two existing entries to it.
- [ ] Accept the plan's decision as-is; record here that the glossary was intentionally left unchanged.

## Dedup and cross-source merge of unknown agents are unverified

> [!WARNING]
>
> - [`docs/guidelines/testing.md`](../../../docs/guidelines/testing.md) — assert the domain rule, including behavior that crosses a merge boundary

Both validators explicitly dedup (`seen[a]`) and the asset collector merges two
sources (`CompatibleAgents` + `Projections`) through one `seen` map — but no test
exercises either. `TestLoad_RejectsUnknownCompatibleAgent` uses three _distinct_
ids (`asset_test.go:53`); `TestValidate_RejectsUnknownProjectionAgent` drives
only projections (`asset_test.go:91`). A regression that dropped the `seen` map
or the second-source scan would still pass green.

```go
// untested today — would catch a broken dedup / broken cross-source merge:
body := `{"id":"x","name":"X","type":"skill","compatible_agents":["bogus","bogus","nope"]}`
// expect typed.Agents == []config.Agent{"bogus","nope"}  (deduped, in order)

// and: same unknown id in BOTH compatible_agents and a projection → reported once
```

Choose one:

- [x] Add a case with a repeated unknown id (assert deduped output) **and** a case with the same unknown id in both `CompatibleAgents` and a `Projection.Agent` (assert reported once).
- [ ] Add only the repeated-id dedup case (accept cross-source merge as covered-by-construction).
- [ ] Accept current coverage; record that dedup/merge are exercised only indirectly.

## `SupportsAgent`'s "empty means all agents" rule has no direct test

> [!WARNING]
>
> - [`docs/guidelines/testing.md`](../../../docs/guidelines/testing.md) — start with the smallest useful unit test for a load-bearing rule

`SupportsAgent` (`internal/asset/asset.go:329`) still encodes the convention
"empty `CompatibleAgents` → supports every agent," and its signature changed to
`config.Agent` this task. No test references `SupportsAgent` at all — the rule is
only exercised indirectly through render fixtures. This is the exact inverse of
the bug the task motivates ("silently rendering as no compatible agents"), so it
deserves a direct, cheap unit test.

```go
func TestSupportsAgent_EmptyMeansAll(t *testing.T) {
	a := &Asset{Manifest: Manifest{CompatibleAgents: nil}}
	for _, ag := range config.AllAgents() {
		if !SupportsAgent(a, ag) { t.Fatalf("empty should support %q", ag) }
	}
}
```

Choose one:

- [x] Add a direct `SupportsAgent` unit test covering empty (all-true) and non-empty (membership true/false) branches with the typed `config.Agent` param.
- [ ] Accept the indirect render-fixture coverage; record the decision.

## "No migration / JSON identical" is proven only on the asset side

> [!WARNING]
>
> - [`docs/guidelines/testing.md`](../../../docs/guidelines/testing.md) — assert against ground truth (actual bytes written/read), not the implementation's own constant

`TestLoad_AcceptsKnownAgents` (`asset_test.go:75`) genuinely round-trips raw
`compatible_agents` JSON off disk. The project/projectstore/migrate side has no
equivalent: `store_test.go`'s `TestAddLoadRoundTrip` asserts only `ID` survives,
and the `migrate` suite seeds via a typed Go struct (`EnabledAgents:
[]config.Agent{...}`), not a raw v1 JSON fixture — so it cannot detect a
marshalling change of the agent field. The "unchanged migrate suite proves no
migration" argument is therefore weaker than the acceptance criterion implies.

```go
// project side: no test reads raw enabled_agents bytes and asserts the typed slice survives.
// TestAddLoadRoundTrip asserts loaded.ID == want.ID only — never EnabledAgents.
```

Choose one:

- [ ] Add one assertion (to `TestAddLoadRoundTrip` or a new small test) that a loaded manifest's `EnabledAgents` equals what was written, giving the project-side JSON round-trip ground-truth coverage symmetric with the asset side.
- [x] Also seed the migrate happy-path from a raw v1 `enabled_agents` JSON string and assert the migrated v2 manifest still carries `[config.AgentCodex]`.
- [ ] Accept the asset-side proof as sufficient; record that the project-side round-trip is covered only by construction.

## OCP note: render settings mapping is a second per-agent edit site (out of scope)

> [!WARNING]
>
> - [`docs/guidelines/solid.md`](../../../docs/guidelines/solid.md) — Open/Closed: the "single extension point" claim is only partial

The typing work makes ids type-safe but the inline settings mapping in
`render.go:150-161` still independently re-lists all four agents with their
per-agent source/target conventions. Adding a fifth agent needs edits in
`config` (constant + `AllAgents`), here, and possibly `surfaces.SkillRoot`. So
`AllAgents` is the single extension point for _membership/validation_, not for
_render conventions_. This is pre-existing and explicitly deferred to task 0011
(`render.go:133` marker) — flagged only so the "one extension point" framing is
not over-read.

```go
for _, mapping := range []struct{ Agent config.Agent; Source, Target string }{
	{config.AgentClaudeCode, "claude-code.json", ".claude/settings.local.json"},
	// ...all four re-listed; a new agent needs a row here too
}
```

Choose one:

- [ ] Leave as-is; add a note to the plan/changelog that `AllAgents` is the extension point for membership only, with per-agent render conventions consolidated by task 0011.
- [x] Pull the per-agent settings/skill file conventions into an agent-descriptor table `config` owns (defer to 0011 if too broad for this task).

## Optional idiom nits (low severity)

> [!WARNING]
>
> - [`docs/guidelines/clean_code.md`](../../../docs/guidelines/clean_code.md) / [`docs/guidelines/go.md`](../../../docs/guidelines/go.md) — minor readability/idiom

Three small, non-blocking observations. `AgentOptions` calls `a.String()` twice
per option (`internal/tui/modals/agents.go`). `UpdateProject` re-copies an
already-fresh slice via `append([]config.Agent(nil), enabledAgents...)`
(`service.go:1043`) — the value already comes from `config.ToAgents`, which
copies. And the two project modals convert string→typed at different seams
(`register_project.go:56` in-modal vs `edit_profile.go:838` in-shell), a
documented but non-uniform choice.

```go
out = append(out, huh.NewOption(a.String(), a.String())) // same value twice
```

Tick any you want applied (independent):

- [x] Bind `s := a.String()` once in `AgentOptions` and reuse it for label+value.
- [x] Drop the redundant copy in `UpdateProject` for symmetry with `AddProject` (which relies on `NewDraft` to copy) — or keep it as a caller-independence guard.
- [x] Converge the two project modals on one conversion seam (both in-modal or both in-shell) so the boundary is uniform.
- [ ] Leave all three as-is.
