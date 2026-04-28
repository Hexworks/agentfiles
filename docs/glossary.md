# Domain Glossary

This glossary defines the canonical vocabulary for the `agentfiles` bounded
context. The project uses these terms to describe how reusable LLM workspace
content is modeled, selected, rendered, and synchronized into repositories.

Following the bounded-context idea, the glossary prefers one precise meaning for
each term inside this project. If implementation details evolve, the glossary
should be updated so the language stays internally consistent.

## Registry

The global profile index stored at `~/.agentprofiles.json`. It contains profile
references and is used for discovery and resolution.

## Profile

A root folder containing `profile.json`, `assets/`, and `projects/`. It is the
main source-of-truth unit in the system.

## Profile Reference

A registry entry that describes a profile by id, name, path, source,
`managed_by`, and timestamps.

## Profile Manifest

The `profile.json` file at the root of a profile. It records stable metadata
about that profile.

## Project

A target repository together with a per-profile selection of enabled agents and
selected assets.

## Project Manifest

A JSON file under `projects/<id>.json` that records the project path, enabled
agents, selected asset ids, and metadata such as creation time.

## Asset

A reusable, profile-scoped unit of content with an `asset.json` manifest and
optional files.

## Asset Type

The first-class category of an asset. Current types are `skill`, `agents_doc`,
`settings`, `mcp`, `rule`, and `hook`.

## Selected Asset

An asset that has been explicitly attached to a project through the project's
manifest.

## Enabled Agent

An LLM tool that the project should render for. Current names are `codex`,
`claude-code`, `cursor`, and `opencode`.

## Compatible Agents

An optional asset manifest field that limits which enabled agents can use that
asset.

## Projection

A mapping from an asset source file or directory to an agent-specific target
path. Used by generic asset types (`mcp`, `rule`, `hook`) whose render shape is
not hard-coded; `skill`, `agents_doc`, and `settings` derive targets from
per-agent conventions instead. Each projection names the agent it applies to,
the source path inside the asset directory, and the target path inside the
project — which must lie within the managed surfaces. When the source is a
directory, every file under it is projected, preserving the relative layout.

Example (hook asset):

```json
{
  "projections": [
    {
      "agent": "claude-code",
      "source": "pre-tool.sh",
      "target": ".claude/hooks/pre-tool.sh"
    }
  ]
}
```

## Exclusive Group

An asset manifest key that marks assets as mutually exclusive so only one chosen
asset from that group may render for a project. The render layer refuses to
produce a plan when two selected assets share the same non-empty group, which
forces the conflict to be resolved at selection time rather than silently
overwriting files.

Example: two `agents_doc` assets that both populate `AGENTS.md` declare the
same group so a project cannot select both:

```json
// codex-default/asset.json
{ "id": "codex-default", "type": "agents_doc",
  "exclusive_group": "main-agents-doc" }

// codex-strict/asset.json
{ "id": "codex-strict", "type": "agents_doc",
  "exclusive_group": "main-agents-doc" }
```

## Render Plan

The computed desired output set for a project after selected assets and enabled
agents are resolved.

## Preview

The sync-layer representation of pending changes, including creates, updates,
drift, and delete candidates.

## Managed Surfaces

The limited set of output locations that `agentfiles` is allowed to manage:
`AGENTS.md`, `.claude/`, `.cursor/`, `.codex/`, `.opencode/`, and `.mcp.json`.

## Managed State

The `.agentfiles/state.json` file written into a target repository. It stores
managed-file hashes and generation metadata for the last successful apply.

## Drift

A condition where a previously managed file was changed locally after apply and
now differs from the managed-state hash.

## Delete Candidate

A recognized LLM-tooling file in a managed surface that is present in the
repository but not in the current desired output set.

## Apply

The act of writing a preview's desired outputs into a target repository and then
persisting fresh managed state.

## Source Of Truth

The authoritative location for reusable content and selection state. In
`agentfiles`, this is the profile folder, not the generated project files.

## Project Ownership

The rule that one target project path may belong to only one profile.

## Profile Health Report

The structured result of a profile health check produced by
`doctor.CheckProfile`. It contains the profile name plus one
`ProjectStatus` per project, each with the project's name and the list of
pending changes (empty when the project is clean). Doctor never formats
output; the TUI renders the report. Often shortened to "report" in code.

## Project Status

The per-project entry inside a Profile Health Report. Carries the
project's display name plus the list of pending project changes. An
empty `Changes` slice means the project is clean.

## Project Change

The doctor-owned representation of one pending diff entry inside a
ProjectStatus: `{Path, Kind, Reason}`. Doctor exposes its own type so
callers can read a report without importing `internal/sync`. The
underlying values mirror `sync.FileChange` but the boundary is explicit.

## Change Kind

The classification of a pending change inside a Preview or Project
Status. One of `create`, `update`, `drift`, or `delete_candidate`. The
sync layer owns `sync.ChangeKind`; doctor mirrors it as
`doctor.ChangeKind` to keep its API independent.

## File Change

The sync-layer entry that pairs a target path with its `ChangeKind` and
a short reason string. Produced inside `sync.Preview.Changes` and
converted into `doctor.ProjectChange` by doctor's report builder.

## Severity

The classification of a domain failure into `info`, `warning`, or
`error`. Severity is part of the domain (not presentation) because it
answers "is this a hard error or a recoverable warning" — a question
about the kind of failure, not its rendering. Every typed domain error
implements `Severity() errs.Severity`; the TUI consumes the result to
pick icon and color.

## Typed Domain Error

A struct value (e.g. `render.AssetNotFoundError`,
`app.ProjectPathOwnedError`) returned in place of an ad-hoc `fmt.Errorf`
string. Each type implements both `Error()` and `Severity()` so it
satisfies `errs.DomainError`. Accumulator functions return
`[]errs.DomainError` directly; non-accumulator functions return
`error` and let `errs.Collect` flatten domain leaves. See
[`docs/guidelines/errors.md`](./guidelines/errors.md).
