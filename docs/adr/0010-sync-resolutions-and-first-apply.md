# Sync Resolution Model And First-Apply Clean Slate

## Status

accepted

## Context

The sync engine originally classified every non-desired file inside a managed
surface as a single `ChangeDelete` bucket, and `Apply` took one `bool`
`deleteCandidates` flag to decide whether all of them should be removed.

Two problems compounded:

1. **One bucket conflated two safety profiles.** A file that the previous
   apply wrote and that the current render no longer wants is a *recognized*
   removal — the user has already seen its hash in `.agentfiles/state.json`
   and is intentionally dropping it. A file that simply happened to live
   inside a managed surface (say a stray `.cursor/notes.md` the user dropped
   in by hand) was never tracked by `agentfiles` and demands a separate, more
   cautious decision. The bool flag forced both into the same yes/no.
2. **First-time applies were noisy.** A fresh project that already had a
   pre-existing `.claude/` folder produced a long list of "delete candidate"
   entries before a single file had ever been written by `agentfiles`. The
   user couldn't tell apart "files we'd be removing because they were ours"
   from "files we'd be removing because we noticed them".

The parent UI refactor (task 0015) introduces a Plan Project Screen with
per-file toggle buttons (`[Overwrite]` / `[Keep]` for drift,
`[Delete]` / `[Keep]` for unknown). That UX requires the sync engine to
already expose those distinctions; this ADR pins down the data model that
both the screen (task 0029) and the engine (task 0017) agree on.

## Decision

### ChangeKind gains `ChangeUnknown`

The constant existed but was never emitted. We now split the existing
delete-candidate pass into two:

- **`ChangeDelete`** — file was recorded in `ManagedState.ManagedFiles` and
  is missing from the current desired output. The user already opted in to
  managing this file. Apply removes it automatically.
- **`ChangeUnknown`** — file lives inside a `surfaces.Roots()` path but is
  neither in `desired` nor in `ManagedState.ManagedFiles`. We never managed
  it. Apply leaves it alone unless the user explicitly resolves it to
  `ResolveDelete`.

### Per-file `Resolution` replaces the bool flag

```go
type Resolution int

const (
    ResolveAuto      Resolution = iota // create / update / delete — always applied
    ResolveOverwrite                    // drift only
    ResolveKeep                         // drift or unknown — no-op
    ResolveDelete                       // unknown only
)

type FileResolution struct {
    Path       string
    Resolution Resolution
}

func Apply(preview *Preview, resolutions []FileResolution) errs.DomainError
```

Defaults when a path is absent from `resolutions`:

- `ChangeCreate`, `ChangeUpdate`, `ChangeDelete` — always applied. These are
  the "auto" kinds; the user's opt-in is the act of looking at the preview
  and approving the apply.
- `ChangeDrift` — kept (no write). Overwriting requires explicit
  `ResolveOverwrite`.
- `ChangeUnknown` — kept (no removal). Deleting requires explicit
  `ResolveDelete`.

The TUI emits a `FileResolution` only when the user toggles away from the
default; an empty slice produces the safest behavior.

### First-apply clean slate

When `.agentfiles/state.json` is absent for the project:

- Every desired file is classified as `ChangeCreate`, regardless of whether
  something already exists at that path. The apply will overwrite.
- No `ChangeUnknown` entries are emitted; the surface walk is skipped.
- The first successful `Apply` writes the initial `ManagedState`. Subsequent
  plans then run the full classification.

The reasoning: a project that has never been applied has no prior agreement
with the user about what is or isn't managed. Treating stray surface files
as `ChangeUnknown` on the very first preview would punish users for the
common case of adopting `agentfiles` in a repo that already has an
`AGENTS.md`. The clean slate moves that decision into the first deliberate
apply.

## Consequences

- `Preview.DeleteCandidates` (the `TODO` field) is gone. All delete and
  unknown entries flow through `Preview.Changes` with the appropriate
  `ChangeKind`.
- `sync.Apply` is no longer a single-decision write loop. It iterates the
  `Changes` slice as the source of truth, with a parallel
  `map[string]render.RenderedFile` lookup for the bodies it needs to write.
  Files in `preview.Files` that are not referenced by a `Changes` entry
  (i.e. nothing to do for them) are still hashed into the new `ManagedState`
  so the next plan can detect drift correctly.
- `app.Service.Apply` accepts `[]llmsync.FileResolution`. The existing TUI
  passes an empty slice — defaults are safe — until the Plan Project Screen
  in task 0029 wires the toggle UI.
- `internal/doctor` and `internal/tui/render_preview` learn the
  `ChangeUnknown` kind. Doctor surfaces it; preview rendering shows a `?`
  glyph styled like drift (both are "attention required" states).
- The change does not introduce new typed errors; `DeleteError`,
  `StatError`, `SurfaceWalkError`, and `StateMissingError` already cover
  every failure mode.

The Plan Project Screen (task 0029) consumes this model and presents the
toggle buttons described in task 0015. Until that ships, the engine is
already correct and the existing TUI keeps applying with the new defaults.

## Addendum: ignored paths (task 0031)

`ManagedState` gains an `ignored_paths []string` list — repo-relative,
forward-slash directory keys validated like `managed_files`. It records
all-unknown folders the user chose to suppress on the Plan Project screen
(the inverse of the Register action from task 0030).

Unlike a `Resolution`, ignoring is purely additive, so it carries no Decision
enum — a plain `[]string` flows through `Apply`. On apply the persisted list is
the **union** of the prior state's `ignored_paths` and this session's ignores
(deduplicated and sorted), so already-persisted ignores survive even after
their folders vanish from the preview. Each subsequent `sync.Plan` suppresses
any `ChangeUnknown` whose path sits under an ignored path, so the folder no
longer appears at all. No new typed error is introduced beyond reusing
`StateCorruptError` for an invalid persisted key.
