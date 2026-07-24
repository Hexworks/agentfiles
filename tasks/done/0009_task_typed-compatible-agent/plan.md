# Plan — 0009 · Typed compatible-agent value

Task: [./description.md](./description.md)

## Goal

Replace the stringly-typed agent identifier with a single domain type
`config.Agent`, and reject unknown agent values at manifest **load** time with
a typed `errs.DomainError`. Per approved scope decision (**Broad — retype
everywhere**), the type propagates through `asset`, `project`, `render`,
`app`, `actions`, and the TUI, with the four huh multiselect form-state fields
staying `[]string` (huh binds strings) and converting at the modal
extract/hydrate boundary.

## Decisions (approved)

1. **One canonical type in `config`** (the existing agent SoT). `type Agent string`;
   the four constants retype to `Agent`; `AllAgents() []Agent`. Add bridge
   helpers so `[]string` boundaries stay ergonomic.
2. **Load-time rejection** lives in `Manifest.Validate()` for both `asset`
   (`CompatibleAgents`) and `project` (`EnabledAgents`) — both are already
   invoked on load (`asset.Load` calls `manifest.Validate()`; `project`
   validates via `proj.Validator` wired at `internal/app/service.go:74`).
   Each package gets one typed error carrying **all** unknown values found
   (slice field), so a single `DomainError` reports every bad id without
   needing `errors.Join` (which cannot return a `DomainError`).
3. **`surfaces` stays string-keyed.** It is a path registry, not a domain
   owner of the agent concept; render bridges with `string(agent)` at the two
   call sites. Avoids `surfaces`→typed-agent churn and an extra import.
4. **JSON on disk is unchanged.** Go marshals a named string type as its
   underlying string, so `enabled_agents` / `compatible_agents` round-trip
   identically — **no migration, no state.json change**.
5. **No new ADR, no glossary term.** This is an implementation-level type
   refinement (no decision reversal); glossary has no "Agent" term entry.
   Changelog entry written by the implement skill. Optionally refresh the
   `config` bullet in `CLAUDE.md` to note it owns the typed `Agent`.

## Execution steps

### 1. `internal/config/agents.go` — the type + bridges
- `type Agent string`.
- Retype constants: `AgentCodex Agent = "codex"`, `AgentClaudeCode`,
  `AgentCursor`, `AgentOpenCode`.
- `AllAgents() []Agent` (return typed slice).
- Add `func (a Agent) String() string { return string(a) }`.
- Add `func IsKnownAgent(a Agent) bool` (`slices.Contains(AllAgents(), a)`).
- Add slice bridges:
  `func ToAgents(ss []string) []Agent` and `func AgentStrings(as []Agent) []string`
  (both nil-safe, defensive copies).
- Update the doc comment.

### 2. `internal/asset` — typed field + load-time reject
- `asset.go`: `CompatibleAgents []config.Agent` (json tag unchanged).
  `Projection.Agent config.Agent`.
- `asset.go` `Validate()`: after the type switch, collect unknown compatible
  agents and unknown projection agents; if any, return the new error.
- `asset.go` `SupportsAgent(a *Asset, agent config.Agent) bool` — retype param;
  body `slices.Contains(a.CompatibleAgents, agent)` still compiles.
- `errors.go`: add `UnknownCompatibleAgentError{ Agents []config.Agent }`
  implementing `Error()` (lists the ids) + `Severity() == SeverityError`.
- `files.go` `Equal`: `slices.Equal(m.CompatibleAgents, other.CompatibleAgents)`
  still typechecks on `[]config.Agent` — no change beyond compile.
- Update the `CompatibleAgents` doc block (drop the "see task 0009 follow-up"
  note; state the invariant now enforced).

### 3. `internal/project` — typed field + load-time reject
- `project.go`: `EnabledAgents []config.Agent`; `NewDraft(name, path string, agents []config.Agent)`;
  `Normalize` `slices.Sort(m.EnabledAgents)` still valid (`Agent` is `~string`, ordered).
- `Validate()`: keep the empty check; add unknown-agent collection returning the new error.
- `errors.go`: add `UnknownEnabledAgentError{ Agents []config.Agent }` (same shape as asset's).

### 4. `internal/render` — typed signatures + constants
- `addRenderedFilesFor(files, a, enabledAgents []config.Agent)` and
  `addSkillOutputs(..., enabledAgents []config.Agent)`.
- Replace string literals with constants: `"codex"` → `config.AgentCodex`;
  the settings mapping struct field `Agent config.Agent` with
  `config.AgentClaudeCode/Codex/Cursor/OpenCode`; `agent == "cursor"` →
  `agent == config.AgentCursor`.
- `surfaces.SkillRoot(string(agent))` at its call site (bridge).
- `SupportsAgent` calls now pass `config.Agent` (mapping/projection/agent locals).

### 5. `internal/app` + `internal/actions` — typed boundary
- `service.go`: `AddProject(..., agents []config.Agent, assetIDs []string)`,
  `UpdateProject(..., enabledAgents []config.Agent)`; line ~1043
  `p.EnabledAgents = append([]config.Agent(nil), enabledAgents...)`;
  `project.NewDraft(name, path, agents)` now typed.
- `actions/inputs.go`: `AddProjectInput.EnabledAgents []config.Agent`,
  `UpdateProjectInput.EnabledAgents []config.Agent`. `projects.go` passes through unchanged.

### 6. TUI — convert at the form boundary (form state stays `[]string`)
- `modals/agents.go`: re-export constants as strings for the `[]string` form
  world (`AgentCodex = string(config.AgentCodex)`, etc.); `AgentOptions()`
  builds from `config.AllAgents()` via `a.String()`.
- `modals/register_project.go`: `RegisterProjectInput.EnabledAgents` stays `[]string`;
  extract returns `project.NewDraft(state.Name, state.Path, config.ToAgents(state.EnabledAgents))`.
- `modals/edit_project.go`: `EditProjectInput.EnabledAgents` stays `[]string`
  (huh-bound); no conversion inside the modal — the shell converts on the way out.
- `modals/create_asset.go`: `createAssetState.CompatibleAgents` stays `[]string`;
  prefill via `config.AgentStrings(initial.CompatibleAgents)`; extract
  `assetManifestFromState` sets `CompatibleAgents: config.ToAgents(state.CompatibleAgents)`.
- `shell/edit_asset.go`: `hydrateForm` `compatibleAgents: config.AgentStrings(a.CompatibleAgents)`;
  `composeManifest` `CompatibleAgents: config.ToAgents(s.form.compatibleAgents)`.
- `shell/edit_profile.go`: convert at the four seams —
  prefill `modals.RegisterProjectInput`/`EditProjectInput` from a manifest with
  `config.AgentStrings(...)`; build `actions.UpdateProjectInput` from an
  `EditProjectInput` with `config.ToAgents(in.EnabledAgents)`;
  `AddProjectInput` fed by a `*project.Manifest` (`draft.EnabledAgents` already
  `[]config.Agent`) passes through; `pendingRegisterProjectAgents` stays
  `[]string` (feeds the `[]string` modal init), set via `config.AgentStrings(draft.EnabledAgents)`.

### 7. Tests (update + add)
Compile-fix `[]string` agent literals → `[]config.Agent` (or leave untyped
string-constant literals inside `Projection{Agent: "..."}`, which still assign
to `config.Agent`) across:
`actions/projects_test.go`, `app/service_project_test.go`, `project/project_test.go`,
`projectstore/store_test.go`, `render/render_test.go`, `sync/sync_test.go`,
`migrate/migrate_test.go`, `tui/modals/{create_asset,edit_project,register_project,validators}_test.go`,
`tui/shell/{edit_asset,edit_profile}_test.go`.

New tests for the load-time reject boundary and bridges (see Acceptance Criteria).

### 8. Quality gate
`make fmt`, `make build`, `make test`, `make lint` all green.

## Assumption grounding

| Assumption | Source `file:line` | Verified line |
|---|---|---|
| Agent ids are untyped string consts + `AllAgents() []string`, and `config` is the declared SoT | `internal/config/agents.go:8` | `AgentCodex      = "codex"` |
| `CompatibleAgents` is `[]string`, only render-time membership checks it | `internal/asset/asset.go:92` | `CompatibleAgents []string \`json:"compatible_agents,omitempty"\`` |
| `asset.Load` calls `Validate()` on load → the reject point | `internal/asset/asset.go:145` | `if err := manifest.Validate(); err != nil {` |
| `project.Manifest.Validate` is invoked on load via the wired validator | `internal/app/service.go:74` | `proj.Validator = func(m *project.Manifest) errs.DomainError { return m.Validate() }` |
| `SupportsAgent` takes a string agent, compared with `slices.Contains` | `internal/asset/asset.go:305` | `func SupportsAgent(a *Asset, agent string) bool {` |
| Render compares `enabledAgents` against raw literals/`Projection.Agent` | `internal/render/render.go:140` | `if !slices.Contains(enabledAgents, "codex") {` |
| Settings mapping uses inline string agent literals | `internal/render/render.go:157` | `{"claude-code", "claude-code.json", ".claude/settings.local.json"},` |
| `surfaces.SkillRoot` is string-keyed (bridge target) | `internal/surfaces/surfaces.go:98` | `func SkillRoot(agent string) (string, bool) {` |
| `EnabledAgents` is `[]string`; `NewDraft` takes `[]string`; `Normalize` sorts it | `internal/project/project.go:24` | `EnabledAgents    []string  \`json:"enabled_agents"\`` |
| huh multiselect binds `*[]string` (form state must stay `[]string`) | `internal/tui/modals/create_asset.go:64` | `compatibleAgentsSelect(&state.CompatibleAgents, ...)` |
| `AgentOptions` builds `huh.Option[string]` from `AllAgents()` | `internal/tui/modals/agents.go:26` | `func AgentOptions() []huh.Option[string] {` |
| `AddProject`/`UpdateProject` carry the agent slice through app/actions | `internal/app/service.go:164` | `func (s *Service) AddProject(profileRef, name, path string, agents, assetIDs []string) (...)` |
| Named string type marshals as its underlying string (JSON unchanged, no migration) | Go spec `encoding/json` (external) — verified: `type Agent string` uses string kind | — |

## Acceptance Criteria (DoD)

- [ ] `config.Agent` defined; the four constants retyped; `AllAgents() []Agent`,
      `IsKnownAgent`, `ToAgents`, `AgentStrings`, `Agent.String()` present.
- [ ] `asset.Manifest.CompatibleAgents` and `Projection.Agent` are `config.Agent`;
      `project.Manifest.EnabledAgents` is `[]config.Agent`.
- [ ] **Load-time reject (real fs, asset):** `TestLoad_RejectsUnknownCompatibleAgent`
      writes an `asset.json` into `t.TempDir()` with
      `"compatible_agents":["codex","bogus","nope"]`, calls `asset.Load(dir)`,
      and asserts the returned error satisfies
      `errors.As(err, &asset.UnknownCompatibleAgentError{})` with `Agents`
      containing `bogus` and `nope` (not `codex`), `Severity()==SeverityError`.
- [ ] **Load-time reject (project):** `TestProjectValidate_RejectsUnknownEnabledAgent`
      builds a `project.Manifest` with a valid + a bogus agent, calls
      `Validate()`, asserts `errors.As(err, &project.UnknownEnabledAgentError{})`
      listing only the bogus id.
- [ ] **Happy path unchanged (real fs round-trip):** `TestLoad_AcceptsKnownAgents`
      writes an asset.json with all four known agents, `asset.Load` succeeds,
      and the loaded `CompatibleAgents` equals `config.AllAgents()`.
- [ ] `config.ToAgents`/`AgentStrings` round-trip: `AgentStrings(ToAgents(ss))`
      equals the known subset of `ss`; both nil-safe.
- [ ] Render behaviour is unchanged: existing `render` tests pass with typed
      signatures; a skill/settings/agents_doc asset renders to the same targets
      for the same enabled agents as before.
- [ ] TUI create/edit-asset and register/edit-project forms still round-trip
      agent selections (existing modal + shell tests pass after the
      `[]string`↔`[]config.Agent` conversions).
- [ ] `make fmt && make build && make test && make lint` all pass.
- [ ] No change to on-disk JSON keys or `state.json`; no migration added
      (asserted by an unchanged `migrate` test suite).
