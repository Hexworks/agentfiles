# 0011 changes

Replaced the per-`(asset-type, agent)` render rules — a `switch a.Type` in
`render.addRenderedFilesFor` plus per-agent branching in
`render.addSkillOutputs` — with a **global lookup table of strategies** keyed
by the typed `(agent.Agent, asset.Type)` pair. Dispatch is now a map lookup,
never a type switch. Each strategy is a small self-contained unit that owns
**both** directions: `Render` (forward) and `Reverse` (repo path → asset
source, for Adopt). Because both directions live in one unit, a forward-render
change can no longer silently break the reverse Adopt mapping.

The reverse mapping that Adopt (ADR 0020) previously re-derived inside
`internal/sync` (`assetProjectionDirs` + `owningAssetSourceRelFor`) is deleted;
`sync` now delegates to `render.ProjectPlan.ReverseLookup`, so forward render
and reverse Adopt share one owner. `RenderedFile` gained `Agent` / `Type`
provenance so the reverse lookup can select the producing strategy.

One behavior change ships alongside the refactor: `agents_doc` now renders
**per agent** — `CLAUDE.md` for claude-code, `AGENTS.md` for codex/cursor/
opencode — where before it rendered only for codex. This required widening the
managed-surface fence to include `CLAUDE.md` and adding
`config.ClaudeDocFileName`. Every other pair stays byte- and path-identical, as
pinned by a golden test.

## Decisions

- One strategy per `(agent, type)` pair, registered explicitly under its own
  key — **Why:** keeps dispatch a pure map lookup and makes a missing
  combination a typed `UnsupportedRenderingError`, not a silent switch
  fall-through. Pairs with identical logic (the three skill-folder agents, the
  twelve generic-projection pairs) share a parameterized struct but each still
  has its own table entry.
- Strategies own both `Render` and `Reverse`; `sync` delegates reverse to
  `render.ProjectPlan.ReverseLookup` — **Why:** the isomorphism Adopt needs
  (forward projection ↔ reverse source recovery) can only be guaranteed if one
  unit authors both; two independent implementations drift (that was the latent
  risk in the old sync-side re-derivation).
- `ReverseLookup` uses the recorded `SourceRel` for an exact rendered-file
  match and suffix-preserving re-derivation for an untracked sibling — **Why:**
  a rendered file's own provenance is authoritative and captures target
  renames (settings `codex.toml`→`config.toml`, agents_doc `AGENTS.md`→
  `CLAUDE.md`, a renamed single-file generic projection); only unknown siblings
  need the structural tail-preserving inverse.
- Cursor skill `Reverse` returns `ok=false` — **Why:** the flat
  `.cursor/commands/<id>.md` layout drops sibling filenames, so it is
  intentionally lossy and stays non-adoptable, matching prior behavior.
- Applied `asset.SupportsAgent` uniformly at the `Build` dispatch gate —
  **Why:** the old codex `agents_doc` arm bypassed the `compatible_agents`
  filter that settings and generic types already honored; the table makes every
  type consistent. Output is byte-identical for the common empty-
  `compatible_agents` case.

Considered but not done: adding a dry-run flag/method (render is already pure —
dry run = `render.Build`/`sync.Plan` without `sync.Apply`); moving
`asset.Type`/`agent.Agent` to a different package (out of scope); making cursor
skills adoptable (intentionally lossy).

## Assumptions

- The golden test's fixture uses empty `compatible_agents`, so the uniform
  `SupportsAgent` gate leaves every unchanged pair byte-identical — **Why:** the
  only behavior difference the gate introduces is for assets that *restrict*
  their agents, which the golden fixture does not exercise; that stricter
  behavior is the intended consistency fix, documented in ADR 0021.
- A stale/typo'd enabled agent id is the realistic trigger for the missing-pair
  error (all 24 real pairs are registered), so the test enables a bogus agent —
  **Why:** the real table is complete by construction; the error path guards
  against an unrecognized agent reaching dispatch.

## Other Notes

- **New ADR** `docs/adr/0021-render-reverse-strategy-table.md`; ADR 0020 gains a
  "superseded in part" pointer (reverse mechanics moved to `render`).
- **Diagrams:** component diagram of render in
  `docs/architecture/05-building-block-view.md`; forward-render and
  reverse-Adopt sequence diagrams in `docs/architecture/06-runtime-view.md`.
- **Docs:** `CLAUDE.md` render/sync/surfaces bullets updated; glossary gains a
  **Render Strategy** entry and `CLAUDE.md` added to Managed Surfaces + Adopt
  reverse pointer; `agent.go` Descriptor comment repointed from "task 0011" to
  ADR 0021.
- **Tests added:** golden unchanged-pairs, per-agent agents_doc (real
  render→sync on disk), `CLAUDE.md` fence, missing-pair error, skill/settings/
  agents_doc/generic reverse round-trips, cursor non-invertibility, and an
  end-to-end skill-drift Adopt through real `sync.Apply` + `app.Service.Apply`.
- **Invariants verified:** `grep "switch" internal/render/render.go` shows no
  `a.Type` dispatch; `assetProjectionDirs`/`owningAssetSourceRelFor` are gone
  from `internal/sync/sync.go`.

## Forward dispatch: type switch → strategy table

```go
// before — internal/render/render.go
func addRenderedFilesFor(files map[string]RenderedFile, a *asset.Asset, enabledAgents []agent.Agent) []errs.DomainError {
	switch a.Type {
	case asset.TypeSkill:
		return addSkillOutputs(files, a, enabledAgents)
	case asset.TypeAgentsDoc:
		if !slices.Contains(enabledAgents, agent.Codex) {
			return nil
		}
		// ... codex-only AGENTS.md ...
	case asset.TypeSettings:
		// ... loop agent.Descriptors() ...
	default:
		// ... generic projection walk ...
	}
}
```

```go
// after — dispatch by (agent, type) map lookup; a miss is a typed,
// accumulated error rather than a silent fall-through.
func addRenderedFilesFor(files map[string]RenderedFile, a *asset.Asset, enabledAgents []agent.Agent) []errs.DomainError {
	var domainErrs []errs.DomainError
	for _, ag := range enabledAgents {
		if !asset.SupportsAgent(a, ag) {
			continue
		}
		strat, ok := strategyFor(ag, a.Type)
		if !ok {
			domainErrs = append(domainErrs, UnsupportedRenderingError{Agent: ag, Type: a.Type})
			continue
		}
		rendered, renderErrs := strat.Render(a, ag)
		domainErrs = append(domainErrs, renderErrs...)
		for _, file := range rendered {
			files[file.Path] = file
		}
	}
	return domainErrs
}
```

## Reverse mapping: sync re-derivation → render.ProjectPlan.ReverseLookup

```go
// before — internal/sync/sync.go re-derived the mapping itself
assetDirs := assetProjectionDirs(rendered.Files)
// ...
assetID, sourceRel, ok := owningAssetSourceRelFor(change.Path, a.assetDirs)
```

```go
// after — sync delegates to the render plan, which asks the producing
// strategy to reverse; forward and reverse can no longer drift.
assetID, sourceRel, ok := a.plan.ReverseLookup(change.Path)
```
