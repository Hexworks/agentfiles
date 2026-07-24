# 0009 changes

Replaced the stringly-typed agent identifier with a single domain type
`agent.Agent` and pushed it through the whole model: `asset`, `project`,
`render`, `app`, `actions`, and the TUI. Unknown agent ids are now rejected at
manifest **load** time with a typed error instead of silently rendering as "no
compatible agents".

> The identifier initially landed in `internal/config`; the task-0009 review
> extracted it into its own leaf package `internal/agent`. See
> **Review follow-up** below for that and the other applied review fixes.

A leaf package `internal/agent` holds `type Agent string`, the four constants,
`All() []Agent`, map-backed `IsKnown`, the order-preserving dedup collector
`Unknown`, and the nil-safe `FromStrings`/`Strings` bridges. `asset.Manifest.CompatibleAgents`,
`asset.Projection.Agent`, and `project.Manifest.EnabledAgents` are now typed;
their `Validate()` methods collect every unknown id in one pass and return a
single typed `DomainError` (`asset.UnknownCompatibleAgentError`,
`project.UnknownEnabledAgentError`). `surfaces` stays string-keyed — render
bridges with `string(agent)` at its two call sites. The TUI keeps its huh
multiselect form state as `[]string` (huh binds strings) and converts at the
modal extract/hydrate boundary.

On-disk JSON is unchanged: a named string type marshals as its underlying
string, so `enabled_agents` / `compatible_agents` round-trip identically. No
migration, no `state.json` change.

## Decisions

- One canonical `Agent` type in `config` (the existing agent source of truth) —
  **Why:** avoids a second parallel type and keeps every consumer pointing at
  one definition.
- Load-time rejection in `Manifest.Validate()` for both `asset` and `project` —
  **Why:** both are already invoked on load (`asset.Load` → `Validate`;
  `project` via the wired `proj.Validator`), so it is the natural reject point.
- Each error carries **all** unknown ids in a slice field rather than
  `errors.Join` — **Why:** `errors.Join` cannot return a `DomainError`, and the
  batch keeps the message actionable ("unknown compatible agent(s): bogus, nope").
- `surfaces` left string-keyed — **Why:** it is a path registry, not a domain
  owner of the agent concept; bridging with `string(agent)` avoids churn and an
  extra import.
- No new ADR, no glossary term — **Why:** an implementation-level type
  refinement, not a reversible decision; the glossary has no "Agent" entry.

## Assumptions

- Go marshals a named string type as its underlying string — **Why:**
  `encoding/json` spec behaviour; verified by the real-fs round-trip test
  `TestLoad_AcceptsKnownAgents` and the unchanged `migrate` suite.

## Other Notes

- `CLAUDE.md` `config` bullet refreshed to note it owns the typed `Agent` and
  that the TUI converts at the modal boundary.
- Test files across `app`, `actions`, `render`, `sync`, `migrate`, `project`,
  `projectstore`, and the TUI were compile-fixed to the typed boundaries; TUI
  form-state literals stayed `[]string`, with `config.AgentStrings`/`ToAgents`
  inserted at three comparison seams (`register_project_test.go:45`,
  `edit_asset_test.go:566`).
- New tests: asset load-time reject (real fs) + happy-path round-trip + unknown
  projection agent; project unknown-enabled-agent reject; `config` bridge
  round-trip / nil-safety / `IsKnownAgent`.
- Gate: `make fmt && make lint && make build && make test` all green (782 pass).

## Review follow-up (task 0009 review-apply)

The review selected the following fixes, applied on top of the implementation
above:

- **Extracted the agent value object into its own leaf package `internal/agent`**
  (was `internal/config`). `config` returns to holding only filename/default
  constants; every consumer imports `internal/agent`. Constants are the
  idiomatic `agent.Codex` / `agent.ClaudeCode` / `agent.Cursor` / `agent.OpenCode`
  (no `Agent` stutter); the bridges are `agent.FromStrings` / `agent.Strings`.
- **`agent.Unknown([]Agent) []Agent`** now owns the order-preserving,
  deduplicating "collect unknown agents" rule. `asset` feeds it its two
  concatenated sources (`CompatibleAgents` + projection agents); `project` feeds
  it `EnabledAgents`. Removes the duplicated `seen`-map loop from both packages.
- **`IsKnown` is backed by a package-level `map[Agent]struct{}`** built once, so
  validating an N-agent manifest no longer allocates N throwaway `All()` slices.
  `All()` still returns a fresh defensive copy for external iteration.
- **Both `Error()` methods reuse `agent.Strings`** instead of a hand-rolled
  `[]Agent → comma-joined` loop.
- **Per-agent settings conventions moved into `agent.Descriptors()`** — the
  inline `{agent, source, target}` table in `render` is gone; render iterates the
  descriptor table the `agent` package owns. (Skill container roots stay in
  `internal/surfaces`; consolidating them is deferred to task 0011.)
- **Both project modals now convert `[]string`↔`[]agent.Agent` in-modal.**
  `EditProjectInput.EnabledAgents` is typed `[]agent.Agent` (converted at the huh
  boundary like the Register modal); the shell no longer converts. Dropped the
  redundant defensive copy in `Service.UpdateProject`, and bound `a.String()`
  once in `AgentOptions`.
- **New tests:** repeated-unknown-id dedup and cross-source (`CompatibleAgents` +
  projection) merge-once for asset load; direct `SupportsAgent` empty-means-all
  and membership cases; `agent.Unknown` dedup/order/nil; migrate happy-path
  seeded from a **raw v1 `enabled_agents` JSON** string asserting the typed slice
  survives migration.
- **Glossary:** added a dedicated **Agent** entry (closed, load-validated id set)
  and cross-linked the existing `Enabled Agent` / `Compatible Agents` entries to
  it — superseding the original "no glossary term" decision.

## Typed agent constant + bridges (as first implemented in `internal/config/agents.go`)

_The review later moved this into `internal/agent` and renamed the symbols
(`agent.Codex`, `agent.All`, `agent.IsKnown`, `agent.FromStrings`,
`agent.Strings`) — see **Review follow-up** above._


```go
// before
const (
	AgentCodex      = "codex"
	AgentClaudeCode = "claude-code"
	AgentCursor     = "cursor"
	AgentOpenCode   = "opencode"
)

func AllAgents() []string { return []string{AgentCodex, AgentClaudeCode, AgentCursor, AgentOpenCode} }
```

```go
// after — named type + membership check + []string bridges for the huh form world
type Agent string

const (
	AgentCodex      Agent = "codex"
	AgentClaudeCode Agent = "claude-code"
	AgentCursor     Agent = "cursor"
	AgentOpenCode   Agent = "opencode"
)

func (a Agent) String() string { return string(a) }
func AllAgents() []Agent        { return []Agent{AgentCodex, AgentClaudeCode, AgentCursor, AgentOpenCode} }
func IsKnownAgent(a Agent) bool { return slices.Contains(AllAgents(), a) }
func ToAgents(ss []string) []Agent  { /* nil-safe string→Agent */ }
func AgentStrings(as []Agent) []string { /* nil-safe Agent→string */ }
```

## Load-time rejection (`internal/asset/asset.go`)

```go
// before — any string passed; only render-time membership ever looked
CompatibleAgents []string `json:"compatible_agents,omitempty"`

func (m Manifest) Validate() errs.DomainError {
	// ...type switch only...
	return nil
}
```

```go
// after — typed field; Validate collects every unknown id (compatible + projection) and rejects
CompatibleAgents []config.Agent `json:"compatible_agents,omitempty"`

func (m Manifest) Validate() errs.DomainError {
	// ...type switch...
	if unknown := unknownAgents(m); len(unknown) > 0 {
		return UnknownCompatibleAgentError{Agents: unknown}
	}
	return nil
}
```

## TUI boundary conversion (`internal/tui/modals/create_asset.go`)

```go
// before — form []string flowed straight into the manifest
CompatibleAgents: state.CompatibleAgents,
```

```go
// after — huh form state stays []string; convert to typed at extract
CompatibleAgents: config.ToAgents(state.CompatibleAgents),
```
