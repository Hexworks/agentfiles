---
id: 0046
type: feature
status: Pending
topics: domain_model, asset_authoring, sync_and_safety, documentation
depends_on:
---

# Shared-scripts asset type (per-agent script projection)

Skills across agents frequently need the same helper shell script, but
each agent expects it in a different place and there is no cross-agent
variable substitution: Claude Code expands `${CLAUDE_PROJECT_DIR}` /
`${CLAUDE_SKILL_DIR}`, Codex expands nothing. Today a user would copy the
script into each agent's tree by hand and reference two hardcoded paths.

Make the **profile** the canonical home for a shared script and let
`render` project it into each selected agent's managed surface, so one
authored script fans out to the right per-agent location and Adopt can
pull local edits back.

## Approach

Add a first-class asset type `script` rather than reusing the generic
`projections` mechanism (`mcp`/`rule`/`hook`). The target directory is
deterministic per agent (each agent owns its own convention), so
strategy-table dispatch keyed on `(agent.Agent, asset.Type)` fits better
than hand-written projection targets and keeps forward/reverse from
drifting.

Per-agent targets:

- `(claude-code, script)` → `<repo>/.claude/scripts/<...>`
- `(codex, script)`       → `<repo>/.agents/scripts/<...>`
- `(cursor, script)`      → `<repo>/.cursor/scripts/<...>` (confirm dir)
- `(opencode, script)`    → `<repo>/.opencode/scripts/<...>` (confirm dir)

## Scope

- **New asset type.** Add `script` to `internal/asset` (`AllTypes()`,
  type constant). Decide starter content: a `script` likely needs no
  templated starter body (like `mcp`/`rule`/`hook`) — the user supplies
  the `.sh`. Confirm `RequiredContentFile` behaviour.
- **Surface fence.** Add `.agents` as an allowed managed-surface root in
  `internal/surfaces/surfaces.go` (both the root list and the matcher).
  `.agents` is currently absent from the fence, so render would refuse
  the codex script target. Codex `agents_doc` still targets `AGENTS.md`;
  scripts add a second, distinct codex surface — this is intentional and
  widens the fence.
- **Render strategies.** Add `(agent, script)` strategies to
  `internal/render/strategies.go`, each owning both `Render` (forward)
  and `Reverse` (repo path → asset source) so Adopt works with no
  sync-side changes. A pair with no meaningful target reports
  `ok=false` / accumulates `UnsupportedRenderingError`.
- **Reverse / Adopt.** Confirm `render.ProjectPlan.ReverseLookup` picks
  up the new strategies with no `sync` edits (ADR 0021 invariant: sync
  delegates reverse to render).
- **Docs.** Update `docs/guidelines/asset_authoring.md` with the `script`
  type; add/adjust the domain-model type list; ADR if the new surface
  root or first-class-type decision is durable (per
  `docs/guidelines/documentation.md`).

## Out of scope

- **Rewriting script-reference paths inside rendered skills.** The larger
  win — having the skill strategy rewrite the `${CLAUDE_*}` / relative
  path that *calls* the script per agent — is a separate follow-up. This
  task only projects the script file itself.
- Codex auto-discovery semantics. Codex auto-runs helpers from
  `.agents/tools/`, not `.agents/scripts/`. These scripts are referenced
  by skills, not auto-discovered, so `.agents/scripts` is a deliberate
  symmetry choice; if a future need requires auto-discovery, revisit the
  target dir then.
- Executable-bit / permission propagation beyond what `sync.Apply`
  already does for managed files (confirm current behaviour; do not add
  new perms machinery unless a test shows scripts land non-executable).

## Acceptance Criteria

- [ ] `asset.AllTypes()` includes `script`; scaffolding a `script` asset
      via `profile.Init` path produces a valid `asset.json` (`type:
      script`) and no spurious starter body.
- [ ] `surfaces.IsAllowed` returns true for `.agents/...` and
      `.agents/scripts/...`; `surfaces.Roots()` includes `.agents`. Both
      the root list and matcher updated in one place.
- [ ] Render of a project selecting claude-code + codex for a `script`
      asset produces `.claude/scripts/<file>` and `.agents/scripts/<file>`
      with identical body; a table-driven test asserts both targets.
- [ ] `render.ProjectPlan.ReverseLookup` maps each rendered script path
      back to its owning asset; an Adopt of a locally edited
      `.claude/scripts/<file>` writes the body back via `asset.WriteFile`
      (existing sync path, no sync edits).
- [ ] Selecting an agent with no `script` strategy accumulates
      `UnsupportedRenderingError` rather than panicking or silently
      dropping.
- [ ] `docs/guidelines/asset_authoring.md` documents the `script` type;
      domain-model type list updated; ADR added if the surface-root /
      first-class-type decision warrants one.

## Verification

- `make build && make test && make lint` (baseline gate).
- New table test in `internal/render` covering claude-code + codex
  script projection (forward) and reverse lookup for both.
- `grep -n '\.agents' internal/surfaces/surfaces.go` shows the new root
  in both list and matcher.
- Manual: scaffold a `script` asset in a scratch profile, register a
  project with claude-code + codex, run Plan → Apply, confirm both
  `.claude/scripts/` and `.agents/scripts/` files appear; edit one, run
  Plan, confirm `drift`; Adopt, confirm the profile asset body updates.

## Open questions (resolve during /plan-task)

- First-class `script` type vs generic `projections` — this description
  assumes first-class; confirm before implementing.
- Cursor / opencode target dirs — verify each agent's real convention
  before wiring those strategies (may ship claude-code + codex first).
- Whether the same mechanism should later carry non-shell shared assets
  (arbitrary files), i.e. is `script` too narrow a name vs `file`.
