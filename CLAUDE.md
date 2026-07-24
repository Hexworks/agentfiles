# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

All via `make` at repo root:

| Command             | Purpose                                                                                                                    |
| ------------------- | -------------------------------------------------------------------------------------------------------------------------- |
| `make build`        | Compile `./cmd/af` into `./bin/af` and install to `~/.local/bin/af`. Injects version/commit/date via `-ldflags -X main.*`. |
| `make test`         | `go test ./...`                                                                                                            |
| `make lint`         | `go vet ./...` (no golangci-lint configured)                                                                               |
| `make fmt`          | `gofmt -w .`                                                                                                               |
| `make run ARGS="…"` | Build then run `./bin/af` with args. Accepted flags: `--registry <path>`, `--projects <path>`, `--theme <path>`.            |
| `make clean`        | Remove `./bin/` and run `go clean`.                                                                                        |

Run a single test: `go test ./internal/sync -run TestName` (or any package path). Toolchain pinned by `mise.toml` (`go = "latest"`; repo targets Go 1.26.1 per `go.mod`).

## Architecture

`agentfiles` treats a **profile folder** (`~/profiles/<name>/`) as the single source of truth for AI-assistant config (skills, settings, hooks, MCP, rules, agents_doc). It **renders** selected assets into target repositories under a fixed set of **managed surfaces**.

### Package layout (all under `internal/`)

Domain packages are kept separable by design — do not blur them:

- `config` — package-level constants for filenames and default profile metadata. No internal deps; sits at the bottom of the import graph and is the single edit-point for those values.
- `agent` — the canonical agent value object: `type Agent string` + the four recognized constants (`Codex`, `ClaudeCode`, `Cursor`, `OpenCode`), `All`, map-backed `IsKnown`, the order-preserving dedup collector `Unknown` (shared by `asset`/`project` load-time validation), the `FromStrings`/`Strings` string bridges, and the per-agent settings `Descriptors` table consumed by `render`. A leaf package (stdlib only), so every domain package can import it cycle-free. Agent ids are typed `agent.Agent` across `asset`/`project`/`render`/`app`/`actions`; the TUI keeps huh form state as `[]string` and converts at the modal boundary (both project modals convert in-modal).
- `surfaces` — owns two nested fences: the outer managed-surface root list (`IsAllowed(target)`, `Roots()`) consumed by render/sync, and the inner asset-container-root set (`AssetContainerRoots`, `IsAssetContainerRoot`, `SkillRoot`, `CursorCommandsRoot`) plus the `RegisterableFolders` eligibility predicate and `ClassifyFolderRejection` helper consumed by `app.RegisterableDirs` and `render.addSkillOutputs`. The data and the rules live together.
- `registry` — global profile index at `~/.agentfiles/profiles.json` (discovery only, no asset content).
- `projectstore` — centralized per-user project selection file at `~/.agentfiles/projects.json`. Owns Load/Save + typed CRUD (`Add`, `Update`, `Remove`, `ListByProfile`, `RemoveByProfile`, `AllProjects`). Introduced by ADR 0017 so profile folders can be shared without leaking per-machine selections.
- `settings` — persistent user-preferences store at `~/.agentfiles/settings.json` (schema `{version:1, git:{enabled, run_hooks}}`). Same shape as `projectstore` / `registry`. Introduced by ADR 0019 so the git-integration toggle survives across launches; missing file → defaults with no error. `run_hooks` defaults to `false` so auto-commits pass `--no-verify`.
- `git` — narrow wrapper around the `git` binary via `os/exec` (per `docs/guidelines/external_tools.md`). Split into `git.go` (Repo/Detect/filtered exec), `commit.go` (Commit + phase helpers), `pathspec.go` (Covers + symlink-aware toRepoRelative), `hooks.go` (stderr marker classifier + installed-hook fallback), `errors.go`. `Repo.Commit(pathspec, msg, runHooks)` passes `--no-verify` unless `runHooks` is true, filters the child env to strip inherited `GIT_*` variables, and re-verifies the staged index after `git add` to catch a concurrent add. Consumed only by `internal/app` through the `GitCommitter` interface seam.
- `appapi` — leaf package holding the boundary value types shared by `app`, `actions`, and `tui/shell`: discriminated `CommitOutcome` (`Committed` / `Skipped{SkipReason}` / `Failed`), `Preview`, `FileChange`, `ChangeKind`, `DriftDecision` / `UnknownDecision`, `LoadedProfile`, `Resolutions`, plus helpers `RegisterableDirs`, `DesiredIgnored`, `DriftResolutionsFromMap`, `SanitizeSubject`. Keeps the documented `tui/shell → actions → app` edge honest — shell reads types from `appapi` and never imports `internal/app`.
- `migrate` — one-shot v1→v2 user-config migration invoked from `cmd/af/main.go` before the TUI opens. Idempotent, presence-based; injectable logger surfaces non-fatal warnings. See ADR 0017.
- `profile` — profile folder model (`profile.json` + `assets/`). `Profile.Projects` is retained as an in-memory projection populated by `app.Service.LoadProfile` from `projectstore`; profile folders no longer own projects on disk.
- `asset` — typed asset manifest (`asset.json`) + scaffolding. Types: `skill`, `agents_doc`, `settings`, `mcp`, `rule`, `hook`. Exposes `AllTypes()` so `profile.Init` can iterate them without duplicating the list. Per-type starter content lives in `starter.go`: a `starters` map (`Type → {filename, template}`) is the single source both `writeStarter` and `RequiredContentFile` derive from, backed by `//go:embed templates/*.tmpl` (`text/template`, executed against the `Manifest`). Generic types (`mcp`/`rule`/`hook`) have no entry → no starter file.
- `project` — per-project manifest struct (target path + selected agents + selected asset ids) plus `NewDraft`, `SelectAsset`, `Validate`, `Normalize`. Persistence lives in `projectstore`.
- `render` — **read-only**. Builds desired files from profile+project. Calls `surfaces.IsAllowed` to gate projection targets against the safety fence (`AGENTS.md`, `.claude`, `.cursor`, `.codex`, `.opencode`, `.mcp.json`). Resolves `exclusive_group` conflicts and `compatible_agents` filters.
- `sync` — compares render plan vs. repo, classifies as `create`/`update`/`drift`/`delete`, writes files, and rewrites `<repo>/.agentfiles/state.json` (hashes of managed files). Imported as `llmsync` in `internal/app` to avoid clashing with stdlib `sync`.
- `app` — thin orchestration layer called by the TUI. Holds both centralized stores and cascades project removal on profile deletion. Contains no business logic.
- `tui` — the only user interface. Menus + `huh` forms. `Esc` and `ctrl+c` both bound to Quit (see `runForm` in `tui/tui.go`) so Esc backs out one level.
- `utils` — shared path/IO/hashing helpers and small generic utilities (e.g. `Deduplicate`).
- `tui/modals/pathselector` — reusable file/folder picker modal opened via `modals.NewSelectPath(opts)`. Enforces a caller-supplied `Options.Constraint` root (no browsing above it) and, when `Options.FollowSymlinks` is true, `EvalSymlinks`-rejects symlink targets that resolve outside the root. Returns a typed `pathselector.Result{Path, IsDir}` on `modal.ResolvedMsg` — extract with `pathselector.ResultFromMsg`.

### Critical invariants

1. **Plan before apply.** Writes go through `sync.Plan` → `sync.Apply`. Do not add write paths that bypass preview.
2. **Managed surfaces fence.** Render refuses any target where `surfaces.IsAllowed` returns false. Keep both the root list and the matcher in `internal/surfaces/surfaces.go`.
3. **Drift vs. update.** `update` = desired content changed; `drift` = local file hash diverged from last `.agentfiles/state.json`. Never collapse them.
4. **Delete opt-in.** `delete` is surfaced in the preview but only removed when `Apply` is called with `deleteCandidates=true`.
5. **Single ownership.** One target repo path may belong to at most one project across every registered profile. Enforced by `app.Service.ensureProjectPathAvailable` in one pass against `projectstore.Store.AllProjects()` — no per-profile filesystem walk. Adding or moving a project against a path already owned elsewhere returns `app.ProjectPathOwnedError`.
6. **Source of truth.** Profile content is authoritative; repo files are outputs. Never make render read from the repo as input. The single sanctioned exception is **Adopt** (ADR 0020): `sync.Apply` classifies `DriftAdopt` / `UnknownAdopt` rows and returns an `AdoptRequests` list; `app.Service.Apply` writes the local body back into the owning profile asset via `asset.WriteFile`. Render itself is untouched.

### TUI-only

No CLI subcommands exist (see `docs/adr/0006`). The binary opens the menu; every input flows through `huh` forms. If adding a new operation, wire it into `internal/tui/` submenus and back it with a method on `app.Service`.

## Errors and rendering

Domain packages return data and typed errors. User-facing strings are
produced in `internal/tui/` (`RenderPreview`, `RenderError`).
When adding a new failure mode, declare a struct in the package's
`errors.go` with an `Error()` method instead of using `fmt.Errorf`. Loops
should accumulate via `errors.Join` rather than short-circuit on the first
failure. Background: ADR 0007, `docs/guidelines/errors.md`.

## Guidelines referenced from docs/

- `docs/guidelines/go.md` — keep packages cohesive, explicit structs over `map[string]any`, actionable errors, I/O at edges (render computes, sync writes).
- `docs/guidelines/errors.md` — typed-error structs per package + `errors.Join` accumulation in loops; the TUI introspects with `errors.As` and renders with severity/icon/color.
- `docs/guidelines/domain_model.md` — registry/profile/asset/project/render/sync must remain separable; stable ids; model compatibility/exclusivity in manifests not ad-hoc checks.
- `docs/guidelines/sync_and_safety.md` — the source of the invariants above.
- `docs/guidelines/asset_authoring.md` — `asset.json` must have id/name/type; generic types use explicit `projections` whose targets stay inside managed surfaces.
- `docs/guidelines/documentation.md` — arc42 for architecture views, ADRs for durable decisions, glossary for terms; document current reality, not planned state.
- `docs/guidelines/clean_architecture.md` — dependencies point inward; keep the domain free of I/O; cross boundaries through interfaces.
- `docs/guidelines/clean_code.md` — code a future maintainer can understand, change, and verify without reconstructing the whole system.
- `docs/guidelines/solid.md` — SOLID as pragmatic design checks for understandable, changeable, testable code.
- `docs/guidelines/testing.md` — small, direct tests that describe the domain rule; failures should be easy to understand.
- `docs/guidelines/git.md` — one long-lived `main`; scope each commit so its purpose is easy to review.
- `docs/guidelines/security.md` — protect the user's repositories, profile content, local configuration, and credentials.
- `docs/guidelines/charm.md` — conventions for the Charmbracelet stack (bubbletea/lipgloss/huh) used by the TUI.
- `docs/guidelines/tui.md` — TUI structure and interaction conventions; keep business logic out of the view layer.
- `docs/guidelines/external_tools.md` — safely handing control to external binaries via `os/exec` (e.g. the `git` wrapper).

Full arc42 set in `docs/architecture/`, ADRs in `docs/adr/`, canonical vocabulary in `docs/glossary.md`.

**Make sure** that you never include the AI footer in commit messages
