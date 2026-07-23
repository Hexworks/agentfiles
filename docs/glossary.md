# Domain Glossary

This glossary defines the canonical vocabulary for the `agentfiles` bounded
context. The project uses these terms to describe how reusable LLM workspace
content is modeled, selected, rendered, and synchronized into repositories.

Following the bounded-context idea, the glossary prefers one precise meaning for
each term inside this project. If implementation details evolve, the glossary
should be updated so the language stays internally consistent.

## User Config Dir

The centralized directory `~/.agentfiles/` that holds the persistent CLI
state: the profile registry (`profiles.json`), the projects store
(`projects.json`), and the [Settings Store](#settings-store)
(`settings.json`). Its name intentionally matches the target-repo
[Managed State](#managed-state) dir — the two live under different
anchors ($HOME vs repo root), so both can be called `.agentfiles/` without
ambiguity at the file-system level. Introduced by ADR 0017.

## Settings Store

The persistent user-preferences aggregate rooted at
`~/.agentfiles/settings.json` inside the [User Config Dir](#user-config-dir).
Schema `{version:1, git:{enabled}}`. Loaded once by `cmd/af/main.go` and
swapped in on `Service.UpdateSettings`; missing file → defaults with no
error. Only owns the settings that the user is meant to change through the
Settings TUI screen. See ADR 0019.

## Commit Trigger

One of the three points where `af` records an automated
[Git-Aware Commit](#git-aware-commit): asset-manifest save (Save button
on Edit Asset), asset-files edit (return from the external editor), and
plan-apply. Each has a distinct pathspec and Conventional-Commits
subject; the trigger fires only when git integration is enabled in the
[Settings Store](#settings-store) and the mutated folder is a git
repository.

## Git-Aware Commit

The scoped Conventional-Commits commit `af` records after a mutation
when git integration is enabled. Produced by the `app.GitCommitter`
seam and materialized by `internal/git` via `os/exec`. Silent-skips
when the folder is not a git repo or the pathspec has no diff; refuses
when the index already carries staged paths outside the pathspec so
unrelated user work is never rolled into an automated commit. See
ADR 0019.

## Registry

The global profile index stored at `~/.agentfiles/profiles.json` inside
the [User Config Dir](#user-config-dir). It contains profile references
and is used for discovery and resolution.

## Projects Store

The per-user aggregate that owns every project manifest across all
registered profiles. Its root is `ProjectsStore{profileID → []Manifest}`
persisted at `~/.agentfiles/projects.json`. The store enforces its own
invariants — one target repo path per project globally, project ids
unique within one profile group, no orphan groups — rather than
delegating them to callers. Per-user selections live here so the
profile folder can be shared without leaking machine-specific state.
Introduced by ADR 0017.

## Profile

The shareable aggregate whose root is `Profile{Manifest, Assets}` on
disk in the profile folder (`profile.json` + `assets/`). It is the
authoritative source of asset content and enforces asset-id uniqueness
within one profile. Per-user project selections live in the
[Projects Store](#projects-store) aggregate; `app.Service.LoadProfile`
composes the two aggregates at read time via `app.LoadedProfile`
(ADR 0017).

## Migration

The one-shot startup process that upgrades a user's on-disk state from
the v1 layout (`~/.agentprofiles.json` plus `<profile>/projects/`) to
the v2 layout (`~/.agentfiles/profiles.json` +
`~/.agentfiles/projects.json`). Runs from `cmd/af/main.go` before the
TUI opens; presence-based detection makes it a no-op after the first
successful run. The v1 filename lives only as a private constant inside
`internal/migrate` so no other package accidentally reintroduces it.
See ADR 0017.

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

An entry in the [Projects Store](#projects-store) that records the
project path, enabled agents, selected asset ids, and metadata such as
creation time. Grouped in `projects.json` under the owning profile id.

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
The tighter inner fence is the [Asset Container Root](#asset-container-root)
set — a strict subset whose direct child folders are eligible for
folder-based asset registration.

## Asset Container Root

The strict subset of [Managed Surfaces](#managed-surfaces) whose direct
child folders are eligible for folder-based asset registration:
`.claude/skills`, `.codex/skills`, `.opencode/skills`, and
`.cursor/commands`. These are the only folder-shaped asset containers;
`agents_doc` and `settings` render to single files and have no child
folder to register. The set is exposed as
`surfaces.AssetContainerRoots()` (and its O(1) predicate
`surfaces.IsAssetContainerRoot`) so no caller duplicates the list.

## Registerable Folder

A directory key in a plan whose parent path is an
[Asset Container Root](#asset-container-root) and whose every
descendant leaf is a [ChangeUnknown](#changeunknown) — i.e. a
top-level, all-unknown skill or command folder that the user can
adopt as a new asset through the Plan Project screen's "Register as
asset" action. Ancestors above a container root and folders nested
deeper than a direct child are never registerable. Enforced by
`surfaces.RegisterableFolders` and re-asserted at the service
boundary in `app.Service.CreateAssetFromFolder`; a stale or nested
`dirKey` is rejected with `app.FolderNotRegisterableError`.

## Managed State

The `.agentfiles/state.json` file written into a target repository. It stores
managed-file hashes and generation metadata for the last successful apply.
The containing directory `<repo>/.agentfiles/` shares its name with the
[User Config Dir](#user-config-dir) intentionally; the two are anchored at
the repo root and $HOME respectively, so callers disambiguate by anchor,
not by name.

## Ignored Path

A repo-relative directory key recorded in the `ignored_paths` list of the
managed state. It marks an all-unknown folder the user chose to suppress on
the Plan Project screen. Each `sync.Plan` drops any `ChangeUnknown` whose path
sits under an ignored path, so the folder no longer appears. The Plan Project
screen sees the full persisted set (via `app.Preview.IgnoredPaths`) and sends
the **complete desired set** on apply; `sync.Apply` writes it **verbatim**
(replace, not union — `normalizeIgnoredPaths`). Dropping a key from the desired
set therefore un-ignores that folder. See ADR 0010 (task-0032 addendum).

## Un-ignore

Removing a folder from the project's *Ignored Paths* so its contents are no
longer suppressed. On the Plan Project screen the **Show Ignored** toggle
reveals already-persisted ignored folders as collapsed `! ignored` leaves;
pressing **Show** on one un-ignores it (drops it from the desired set sent on
apply) and **pins** the row visible — *pinned* meaning kept on screen for the
rest of the session regardless of the Show/Hide toggle. Such a revealed folder
is termed *persisted-ignored* (it came from `ignored_paths`, not from a live
`ChangeUnknown`). After apply, an un-ignored folder's files reappear as
`? unknown` on the next plan. See ADR 0010 (task-0032 addendum).

## Drift

A condition where a previously managed file was changed locally after apply and
now differs from the managed-state hash. Drift defaults to *keep* during
apply. `DriftKeep` leaves the file alone **and preserves the prior managed
baseline**, so a kept drift stays classified as drift on every subsequent plan
until the user resolves it. `DriftOverwrite` replaces the local edits with the
rendered body. Keep never adopts the on-disk hash as the new baseline (doing so
would silently flip drift to update — see bug 0033). Promoting local edits into
the profile is a separate, future operation (*Adopt*), not Keep. See ADR 0015.

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

## Acceptance Criteria

The hybrid checklist that lives under `## Acceptance Criteria` in a task's
`description.md`. Each item is a verifiable statement — either a named test
+ expected assertion, an observable input→output pair, or a reproducible
CLI/TUI smoke step. The checklist **is** the [Definition of Done](#definition-of-done)
for that task; a task is done when every `[ ]` becomes `[x]` and
`## Verification` passes. Authored during `af.create-task` Step 8 (grilling
gate) and consumed by `af.task.review` Step 6.5b (DoD evidence table).

## Definition of Done

The condition under which a task is considered complete. In `agentfiles`
this is defined by the task's own [Acceptance Criteria](#acceptance-criteria)
checklist plus its `## Verification` bullets — there is no separate DoD
section restating them. `af.task.review` Step 6.5b enforces the DoD by
producing a per-criterion table (met / unmet / unverifiable) against the
diff before dispatching the review subagents.

## Legacy Task

A task in `in-review` whose `description.md` is missing one or more of the
three required body sections (`## Acceptance Criteria`, `## Out of scope`,
`## Verification`) mandated by the task-workflow contract (see
`af.create-task` Step 7). `af.task.review` Step 6.5a and `af.task.implement`
Step 2.5 both stop with this outcome and instruct the author to add the
missing section before the pipeline can continue. The name refers to the
task itself, not to a producer skill, so tasks authored by hand or by a
future import skill fall under the same label.

## Typed Domain Error

A struct value (e.g. `render.AssetNotFoundError`,
`app.ProjectPathOwnedError`) returned in place of an ad-hoc `fmt.Errorf`
string. Each type implements both `Error()` and `Severity()` so it
satisfies `errs.DomainError`. Accumulator functions return
`[]errs.DomainError` directly; non-accumulator functions return
`error` and let `errs.Collect` flatten domain leaves. See
[`docs/guidelines/errors.md`](./guidelines/errors.md).

## Constraint Root

An absolute filesystem directory that bounds a
[Path Selector Modal](#path-selector-modal). The user cannot navigate
above it, and — with `Options.FollowSymlinks` enabled — symlink
targets that resolve outside it are rejected with an error
notification. Set the empty string to allow browsing the entire
filesystem. The value is `EvalSymlinks`-resolved and cleaned during
option validation so subsequent containment checks compare canonical
paths.

## Path Selector Modal

The reusable TUI modal implemented in
`internal/tui/modals/pathselector` and opened by callers through
`modals.NewSelectPath(opts)`. It lets the user pick a folder or file
inside a [Constraint Root](#constraint-root), filters files by
extension when `Options.ShowFiles` is set, and offers a runtime
`Show hidden / Hide hidden` toggle (mnemonic `h`). The selection is
delivered as a typed `pathselector.Result{Path, IsDir}` on the
`modal.ResolvedMsg` — extract it with `pathselector.ResultFromMsg`.
