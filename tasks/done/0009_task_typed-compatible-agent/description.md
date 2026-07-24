---
id: 0009
type: task
status: done
topics: go, asset
---

# Replace `Manifest.CompatibleAgents []string` with a typed value

`asset.Manifest.CompatibleAgents` is currently `[]string`
(`internal/asset/asset.go`). Any string passes JSON parsing; only
`asset.SupportsAgent` checks membership at render time. A typo in a
manifest renders silently as "no compatible agents" without a clear
error.

Replace the field with a typed `CompatibleAgent` value (or enum-like
constant set) that mirrors the recognized agents list (`codex`,
`claude-code`, `cursor`, `opencode`). Validation should reject unknown
values at load time with a typed error.

Originally tracked as a `// FIX:` marker in `asset.go`.

## Acceptance Criteria

- [ ] `config.Agent` (`type Agent string`) defined; the four constants
      (`codex`, `claude-code`, `cursor`, `opencode`) retyped to `Agent`;
      `AllAgents() []Agent`, `IsKnownAgent`, `ToAgents`, `AgentStrings`, and
      `Agent.String()` present and nil-safe where they take slices.
- [ ] `asset.Manifest.CompatibleAgents` and `Projection.Agent` are
      `config.Agent`; `project.Manifest.EnabledAgents` is `[]config.Agent`.
- [ ] Load-time reject (asset): a test writes `asset.json` with
      `"compatible_agents":["codex","bogus","nope"]` into `t.TempDir()`, calls
      `asset.Load(dir)`, and asserts the error satisfies
      `errors.As(err, &asset.UnknownCompatibleAgentError{})` with `Agents`
      containing `bogus` and `nope` (not `codex`) and `Severity()==SeverityError`.
- [ ] Load-time reject (project): a test builds a `project.Manifest` with one
      valid + one bogus agent, calls `Validate()`, and asserts
      `errors.As(err, &project.UnknownEnabledAgentError{})` listing only the
      bogus id.
- [ ] Happy path unchanged: a test writes `asset.json` with all four known
      agents, `asset.Load` succeeds, and the loaded `CompatibleAgents` equals
      `config.AllAgents()`.
- [ ] `AgentStrings(ToAgents(ss))` equals the known subset of `ss`; both
      helpers nil-safe.
- [ ] Render output unchanged: existing `render` tests pass with the typed
      signatures — a skill/settings/agents_doc asset renders to the same targets
      for the same enabled agents as before.
- [ ] TUI create/edit-asset and register/edit-project forms still round-trip
      agent selections across the `[]string`↔`[]config.Agent` conversions
      (existing modal + shell tests pass).
- [ ] `make fmt && make build && make test && make lint` all pass.

## Out of scope

- No change to on-disk JSON keys (`compatible_agents` / `enabled_agents`) or to
  `.agentfiles/state.json`; the named string type marshals identically.
- No migration added — asserted by an unchanged `migrate` test suite.
- `internal/surfaces` stays string-keyed (path registry, not agent-concept
  owner); render bridges with `string(agent)` at its call sites.
- No new ADR and no glossary term — implementation-level type refinement, no
  decision reversal.

## Verification

- `make fmt && make build && make test && make lint` — all green.
- New tests pass: `TestLoad_RejectsUnknownCompatibleAgent`,
  `TestProjectValidate_RejectsUnknownEnabledAgent`, `TestLoad_AcceptsKnownAgents`,
  and the `ToAgents`/`AgentStrings` round-trip test.
- Grep confirms no remaining bare-string agent literals in domain code
  (`asset`, `project`, `render`, `app`, `actions`) outside the TUI form-state
  and `surfaces` bridge boundaries.

## Clarification

### Question

How wide should the typed-agent change go — narrow (add `config.Agent`
alongside the existing untyped consts; only `asset.CompatibleAgents` typed) or
broad (retype the agent identifier everywhere: config consts, `EnabledAgents`,
`Projection.Agent`, render, project, actions, TUI)?

### Answer

Broad — retype everything. One canonical `config.Agent` type across the
domain; huh form-state fields stay `[]string` and convert at the modal
boundary.
