---
id: 0038
type: bug
status: done
topics: asset_authoring, errors
---

# Reject nested asset registration

A folder can currently be registered as an asset even when it sits **inside**
an already-managed asset folder. `RegisterableDirs` (`internal/app/service.go`)
returns **every ancestor** of an all-unknown subtree, so for an unknown skill at
`.claude/skills/foo/bar/…` it offers `.claude`, `.claude/skills`,
`.claude/skills/foo`, `.claude/skills/foo/bar`, … all as registerable.

Only a **top-level folder directly under a known asset-container root** should be
registerable: `.claude/skills/foo/` is OK; `.claude/skills/foo/bar/` is not, nor
is the root `.claude/skills` itself.

Rule (chosen): a dir is registerable **iff its parent path is a known
asset-container root** AND every descendant leaf under it is an unknown change.
The known roots are the skill container dirs (`.claude/skills`, `.codex/skills`,
`.opencode/skills`, `.cursor/commands`) — the only folder-shaped asset
containers. `agents_doc`/`settings` render to single files, so they have no
child-folder to register and are out of scope here.

The root list is scattered in `internal/render/render.go` (`skillRoots`,
`.cursor/commands`). Centralize it as one exported source of truth in `render`
(e.g. `AssetContainerRoots()`) and have `app.RegisterableDirs` consume it, so no
second hard-coded copy drifts.

`CreateAssetFromFolder` (service.go) already re-asserts
`RegisterableDirs(...)[dirKey]` and returns `FolderNotRegisterableError` on a
miss; under the new rule a nested/ancestor `dirKey` is simply absent from the
set, so that guard rejects it with no new error type. The TUI listing
(service.go:465) consumes the same function, so nested folders stop being
offered with no TUI change.

## Acceptance Criteria

- [ ] `RegisterableDirs` returns a dir **iff** its parent path is a known
      asset-container root **and** every descendant leaf under it is an unknown
      change (partly-managed folders stay excluded — existing invariant).
- [ ] Table holds: `.claude/skills/foo` (all-unknown) → registerable;
      `.claude/skills/foo/bar` → not; `.claude/skills` (root itself) → not;
      `.claude` → not; an unknown dir outside any asset root (e.g.
      `docs/whatever`) → not.
- [ ] A partly-managed `.claude/skills/foo` (≥1 managed leaf) is **not**
      registerable.
- [ ] Known asset-root list lives in **one** exported place in `internal/render`
      (`AssetContainerRoots()`), derived from the existing `skillRoots` map plus
      `.cursor/commands`; `app.RegisterableDirs` consumes it — no duplicated
      hard-coded list.
- [ ] `CreateAssetFromFolder` called with a nested `dirKey`
      (e.g. `.claude/skills/foo/bar`) returns `FolderNotRegisterableError` and
      writes nothing.

## Out of scope

- `agents_doc` / `settings` folder-registration (single-file assets, no child folder).
- Any change to render output or projection targets.
- New error types — reuse `FolderNotRegisterableError`.

## Verification

- Baseline: `make build && make test && make lint` pass.
- `go test ./internal/app -run TestRegisterableDirs` — asserts the OK/NOT-OK
  table above (rewritten from the old `a`/`b`/`c/deep` fixtures to
  `.claude/skills/...` paths).
- `go test ./internal/app -run TestCreateAssetFromFolder` — a nested `dirKey`
  returns `FolderNotRegisterableError` and no asset/profile write occurs.
- Smoke: `./bin/af` → project with unknown `.claude/skills/foo/bar/` → the
  register-asset list offers `.claude/skills/foo` only (not `bar`, not
  `.claude/skills`).

## Plan

[plan.md](./plan.md)
