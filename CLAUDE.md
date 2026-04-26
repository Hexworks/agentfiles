# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

All via `make` at repo root:

| Command | Purpose |
| --- | --- |
| `make build` | Compile `./cmd/af` into `./bin/af` and install to `~/.local/bin/af`. Injects version/commit/date via `-ldflags -X main.*`. |
| `make test` | `go test ./...` |
| `make lint` | `go vet ./...` (no golangci-lint configured) |
| `make fmt` | `gofmt -w .` |
| `make run ARGS="…"` | Build then run `./bin/af` with args. Only flag the binary accepts is `--registry <path>`. |
| `make clean` | Remove `./bin/` and run `go clean`. |

Run a single test: `go test ./internal/sync -run TestName` (or any package path). Toolchain pinned by `mise.toml` (`go = "latest"`; repo targets Go 1.26.1 per `go.mod`).

## Architecture

`agentfiles` treats a **profile folder** (`~/profiles/<name>/`) as the single source of truth for AI-assistant config (skills, settings, hooks, MCP, rules, agents_doc). It **renders** selected assets into target repositories under a fixed set of **managed surfaces**.

### Package layout (all under `internal/`)

Domain packages are kept separable by design — do not blur them:

- `config` — package-level constants for filenames and default profile metadata. No internal deps; sits at the bottom of the import graph and is the single edit-point for those values.
- `surfaces` — owns the managed-surface root list and the `IsAllowed(target)` matcher. Render and sync both consume it; the data and the rule live together.
- `registry` — global profile index at `~/.agentprofiles.json` (discovery only, no asset content).
- `profile` — profile folder model (`profile.json`, `assets/`, `projects/`).
- `asset` — typed asset manifest (`asset.json`) + scaffolding. Types: `skill`, `agents_doc`, `settings`, `mcp`, `rule`, `hook`. Exposes `AllTypes()` so `profile.Init` can iterate them without duplicating the list.
- `project` — per-project manifest (target path + selected agents + selected asset ids). Lives inside a profile's `projects/`.
- `render` — **read-only**. Builds desired files from profile+project. Calls `surfaces.IsAllowed` to gate projection targets against the safety fence (`AGENTS.md`, `.claude`, `.cursor`, `.codex`, `.opencode`, `.mcp.json`). Resolves `exclusive_group` conflicts and `compatible_agents` filters.
- `sync` — compares render plan vs. repo, classifies as `create`/`update`/`drift`/`delete_candidate`, writes files, and rewrites `<repo>/.agentfiles/state.json` (hashes of managed files). Imported as `llmsync` in `internal/app` to avoid clashing with stdlib `sync`.
- `doctor` — read-only health check across every project in a profile.
- `app` — thin orchestration layer called by the TUI. Contains no business logic.
- `tui` — the only user interface. Menus + `huh` forms. `Esc` and `ctrl+c` both bound to Quit (see `runForm` in `tui/tui.go`) so Esc backs out one level.
- `fsutil` — shared path/IO helpers.

### Critical invariants

1. **Plan before apply.** Writes go through `sync.Plan` → `sync.Apply`. Do not add write paths that bypass preview.
2. **Managed surfaces fence.** Render refuses any target where `surfaces.IsAllowed` returns false. Keep both the root list and the matcher in `internal/surfaces/surfaces.go`.
3. **Drift vs. update.** `update` = desired content changed; `drift` = local file hash diverged from last `.agentfiles/state.json`. Never collapse them.
4. **Delete opt-in.** `delete_candidate` is surfaced in the preview but only removed when `Apply` is called with `deleteCandidates=true`.
5. **Single ownership.** One target repo path may belong to at most one profile. Enforced by `app.Service.ensureProjectPathAvailable`.
6. **Source of truth.** Profile content is authoritative; repo files are outputs. Never make render read from the repo as input.

### TUI-only

No CLI subcommands exist (see `docs/adr/0006`). The binary opens the menu; every input flows through `huh` forms. If adding a new operation, wire it into `internal/tui/` submenus and back it with a method on `app.Service`.

## Open refactor markers

Several `FIX: task#0005` comments still exist across the domain packages. They reference a ticket under `tasks/current/`:

- `task#0005` — replace hard-coded error strings with structured error types (per go_guidelines: implement `Error()` when returning non-string error values).

When touching those sites, prefer resolving the marker over adding new ones.

## Guidelines referenced from docs/

- `docs/guidelines/go_guidelines.md` — keep packages cohesive, explicit structs over `map[string]any`, actionable errors, I/O at edges (render computes, sync writes).
- `docs/guidelines/domain_model_guidelines.md` — registry/profile/asset/project/render/sync must remain separable; stable ids; model compatibility/exclusivity in manifests not ad-hoc checks.
- `docs/guidelines/sync_and_safety_guidelines.md` — the source of the invariants above.
- `docs/guidelines/asset_authoring_guidelines.md` — `asset.json` must have id/name/type; generic types use explicit `projections` whose targets stay inside managed surfaces.
- `docs/guidelines/documentation_guidelines.md` — arc42 for architecture views, ADRs for durable decisions, glossary for terms; document current reality, not planned state.

Full arc42 set in `docs/architecture/`, ADRs in `docs/adr/`, canonical vocabulary in `docs/glossary.md`.
