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

## Available Asset

The complement of Selected Asset for one project: any asset present in the
profile whose id is not in the project's `SelectedAssetIDs`. The Select
Project Assets screen partitions `Profile.AssetList()` into Selected and
Available using exactly this rule. `Compatible Agents` filters and
`Exclusive Group` conflicts are *not* applied here — they are reported at
plan time so the selection screen never silently hides a chosen asset.

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

## Ignored Path

A repo-relative directory key recorded in the `ignored_paths` list of the
managed state. It marks an all-unknown folder the user chose to suppress on
the Plan Project screen. Each `sync.Plan` drops any `ChangeUnknown` whose
path sits under an ignored path, so the folder no longer appears. On apply
the list is unioned with the previously-persisted paths, never dropped. See
ADR 0010.

## Drift

A condition where a previously managed file was changed locally after apply and
now differs from the managed-state hash. Drift defaults to *keep* during
apply; the user must explicitly resolve a drift entry to `DriftOverwrite`
to let apply replace the local edits. Choosing `DriftKeep` adopts the
on-disk content as the new managed baseline so future plans do not flag
the same path as drift again. See ADR 0010.

## First-Apply Clean Slate

A project with no `.agentfiles/state.json` is treated as fresh: every desired
file is classified as `ChangeCreate` (overwriting whatever exists at that
path), and stray files inside managed surfaces are ignored. The first
successful apply writes the initial state; subsequent plans then
distinguish drift from unknown normally. Surfaced on `Preview.FirstApply`
so the TUI can show a clean-slate banner. See ADR 0010.

## ChangeDelete

A pending change emitted by `sync.Plan` for a file recorded in the
previous `ManagedState` but missing from the new desired output. The
user already opted in to managing the file during a previous apply, so
removal is auto-applied on the next `sync.Apply`; the preview itself is
the opt-in surface. See ADR 0010.

## ChangeUnknown

A pending change emitted by `sync.Plan` for a file that lives inside a
managed surface but was never tracked in `ManagedState`. Unlike
`ChangeDelete`, removal is **not** automatic: the file is kept unless the
user supplies a `UnknownDelete` resolution for its path. The first-apply
clean slate suppresses `ChangeUnknown` entirely so adopting `agentfiles`
in an existing repo does not flood the preview with noise. An all-unknown
folder can also be permanently suppressed by adding it to the [Ignored
Path](#ignored-path) list, a third outcome distinct from the transient
keep/delete resolution. See ADR 0010.

## Orphaned File

A managed file left behind in a project repository after the asset or
project that produced it was deleted from its profile. `DeleteAsset` and
`DeleteProject` deliberately do not touch project-side files; the next
`sync.Plan` for the affected project surfaces the leftovers as
`ChangeDelete` (when the file was recorded in `ManagedState`) or
`ChangeUnknown` (when it was not). The user can confirm removal through
the standard preview / apply flow instead of having the delete cascade
inline at the source.

## Resolution

The user's per-file decision for a `ChangeDrift` or `ChangeUnknown`
entry. The sync engine models the two cases as separate types because
their valid choices do not overlap: `DriftDecision` is `DriftOverwrite`
or `DriftKeep`; `UnknownDecision` is `UnknownDelete` or `UnknownKeep`.
Paths absent from the resolution slices fall back to the safe default
(drift → keep, unknown → keep). A `ChangeUnknown` has a third, persisted
outcome beyond this transient decision: ignoring the folder (see [Ignored
Path](#ignored-path)). See ADR 0010.

## File Resolution

The slice element that pairs a target path with one resolution
decision. The engine exposes `sync.DriftResolution` and
`sync.UnknownResolution`; the app layer mirrors them as
`app.DriftResolution` and `app.UnknownResolution` so the TUI never
imports `internal/sync` directly. Paths in either slice must be the
forward-slash relative key matching `FileChange.Path` — absolute or
OS-separated paths are rejected with `InvalidPathError`.

## Apply

The act of writing a preview's desired outputs into a target repository and then
persisting fresh managed state.

## Source Of Truth

The authoritative location for reusable content and selection state. In
`agentfiles`, this is the profile folder, not the generated project files.

## Project Ownership

The rule that one target project path may belong to only one profile.

## Change Kind

The classification of a pending change inside a Preview. One of
`create`, `update`, `drift`, `delete`, or `unknown`. Owned by the sync
layer as `sync.ChangeKind`.

## File Change

The sync-layer entry that pairs a target path with its `ChangeKind` and
a `ReasonKind` constant. Produced inside `sync.Preview.Changes`.

## Reason Kind

The domain-level explanation field on `sync.FileChange`. Values are
constants (`ReasonFirstApply`, `ReasonFileMissing`, `ReasonContentDiffers`,
`ReasonDriftDetected`, `ReasonStateRecordedDelete`, `ReasonUnknown`)
emitted by the sync engine; the TUI translates each value into
user-facing prose so a copy-edit (or future i18n pass) touches one
file. Tests assert against the constants, not the translated text.

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
