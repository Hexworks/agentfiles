# Git-aware commits review

Feature ships correctly against every acceptance criterion. Seven parallel review agents produced findings clustered on four themes: (1) the `GitCommitter` seam is incomplete — pre-flight, ACL translation, and outcome shape all leak; (2) the commit-message templates + pathspec derivation duplicate rules that should live once; (3) testing still relies on fake-recorded-tuple assertions whose expected values copy implementation constants — exactly the shape that let the shipped bug pair land in production; (4) security: auto-commits execute repo hooks with no opt-out, inherit `GIT_*` env vars, and interpolate unsanitized user strings into commit subjects.

Nineteen distinct issues below. Tick exactly one checkbox per issue.

## Commit-message template + Commit Trigger vocabulary scattered inline

> [!WARNING]
>
> - [`docs/guidelines/clean_code.md`](../../../docs/guidelines/clean_code.md)
> - [`docs/guidelines/solid.md`](../../../docs/guidelines/solid.md)
> - [`docs/guidelines/domain_model.md`](../../../docs/guidelines/domain_model.md)

The subject-line policy `chore(agentfiles): <verb> ...` is inlined via `fmt.Sprintf` in three sites (`internal/app/service.go:468, :966, :989`) and re-inlined verbatim in test expectations (`internal/app/git_committer_test.go:71, :95, :180`). The glossary elevates **Commit Trigger** to a first-class domain term with three named triggers, but there is no `CommitTrigger` type, enum, or constant — the three "triggers" the glossary promises are ungrepable. Also the `Apply` subject encodes `len(mutated)-1` (`service.go:468`) because `mutatedPaths` silently appends `state.json` at the end — a hidden ordering coupling documented only in a comment.

```go
// internal/app/service.go — three inline templates + off-by-one
msg := fmt.Sprintf("chore(agentfiles): sync project %s (%d files)", proj.Name, len(mutated)-1)
msg := fmt.Sprintf("chore(agentfiles): update asset %s manifest", manifest.ID)
msg := fmt.Sprintf("chore(agentfiles): edit asset %s files", manifest.ID)
```

- [x] Extract a `commitTrigger` struct (`Subject func(...) string`, `Pathspec func(...) []string`) in `internal/app/git.go` and define three package-level values (`triggerSyncProject`, `triggerAssetManifest`, `triggerAssetFiles`). `Service.Apply/UpdateAsset/SaveAssetFilesEdit` consume them; tests iterate them table-driven. The Commit Trigger vocabulary surfaces in code.
- [ ] Lighter fix: extract three helper functions (`syncProjectMsg(name, n)`, `updateAssetMsg(id)`, `editAssetFilesMsg(id)`) into a new `internal/app/commit_messages.go` — one edit-point for prefix + shape, tests share the same helpers.
- [ ] Also change `mutatedPaths` to return `struct { Files []string; StateSnapshot string }` so the `-1` arithmetic disappears; the subject count uses `len(pair.Files)` directly.

## `Service.UpdateSettings` bypasses the `GitCommitter` seam by calling `git.BinaryAvailable()` directly

> [!WARNING]
>
> - [`docs/guidelines/clean_architecture.md`](../../../docs/guidelines/clean_architecture.md)
> - [`docs/guidelines/solid.md`](../../../docs/guidelines/solid.md)

`internal/app/git.go:16` sets up the seam so `app.Service` never depends on the concrete `internal/git`. But `internal/app/service.go:19` imports `internal/git` anyway and `service.go:559` calls `git.BinaryAvailable()` inline in `UpdateSettings`. The test-side tell is `git_committer_test.go:214-224` — a test named `TestUpdateSettings_PreflightRefusesWhenBinaryMissing` that cannot fake the pre-flight and instead asserts a no-op update. That test's name lies about its assertion.

```go
// internal/app/service.go:554-568 — direct concrete call bypasses seam
if next.Git.Enabled && !s.settings.Git.Enabled {
    if err := git.BinaryAvailable(); err != nil {
        return err
    }
}
```

- [x] Extend `GitCommitter` with `BinaryAvailable() errs.DomainError` (or add sibling `GitProbe` interface). `gitBinaryCommitter` delegates to `git.BinaryAvailable()`. `service.go` drops its `internal/git` import. Fake committer implements `BinaryAvailable` returning a stub `BinaryMissingError` unconditionally — pre-flight-refused path becomes testable without touching `$PATH`.
- [ ] Alternative: introduce a function-valued field on `Service` (`binaryChecker func() errs.DomainError`) wired at construction; tests swap it. Smaller change but adds a second seam shape next to `GitCommitter`.

## No full-stack real test for any commit trigger

> [!WARNING]
>
> - [`docs/guidelines/testing.md`](../../../docs/guidelines/testing.md)

Every commit-trigger service test in `internal/app/git_committer_test.go` uses `fakeCommitter`. `app.NewGitCommitter()` appears only in `cmd/af/main.go` — never in a test. `internal/git/git_test.go` tests `Repo.Commit` alone with hand-crafted pathspec strings (`git_test.go:107` `filepath.Join(dir, "assets", "foo") + "/**"`) — proves the wrapper works when caller passes the right thing; does not prove `app.Service.UpdateAsset` computes the right thing. That is exactly the gap the shipped bug pair rode through (`8e7b638`): asset pathspec was `assets/<id>/asset.json` but real layout is `assets/<type>/<id>/asset.json`, every unit test green, first live run failed. `docs/guidelines/testing.md:129-134` cites this task by name as the motivation for the Cross-Boundary Integration section.

```go
// missing test — no equivalent exists today:
func TestUpdateAsset_RealGitRecordsManifestCommit(t *testing.T) {
    requireGit(t)
    root := t.TempDir()
    profileDir := filepath.Join(root, "profile")
    // git init profileDir + user.email + user.name + seed commit
    svc := app.NewWithStores(..., enabledSettings, app.NewGitCommitter())
    // svc.CreateProfile → svc.InitAsset → edit manifest → svc.UpdateAsset
    // Assert via `git -C profileDir log --name-only HEAD`:
    // exactly "assets/skill/<id>/asset.json" (ground truth, not a copy).
}
```

- [ ] Add three real-stack tests (`TestUpdateAsset_RealGit`, `TestSaveAssetFilesEdit_RealGit`, `TestApply_RealGit`) that wire the production `NewGitCommitter`, seed a real git repo, and assert against `git log --name-only HEAD` output. Also add a nested-placement variant (profile several levels deep in the outer repo) mirroring the wrapper-level `TestCommit_NestedProfile` at `git_test.go:184`. Skip via `exec.LookPath` when git is absent.
- [x] Additionally rename `TestUpdateSettings_PreflightRefusesWhenBinaryMissing` → `TestUpdateSettings_NoOpUpdateSucceeds` so its name matches its assertion until the seam finding above is fixed.

## Test assertions copy implementation string constants

> [!WARNING]
>
> - [`docs/guidelines/testing.md`](../../../docs/guidelines/testing.md)
> - [`docs/guidelines/clean_code.md`](../../../docs/guidelines/clean_code.md)

`internal/app/git_committer_test.go:67` builds `wantSpec := filepath.Join(a.Dir, "asset.json")` — same recipe the implementation uses at `service.go:965` (`filepath.Join(target.Dir, config.AssetManifestFileName)`). Similarly `git_committer_test.go:91` computes `wantSpec := a.Dir + "/**"` — mirror of `service.go:988`. And `git_committer_test.go:173` uses the literal `"AGENTS.md"` when `config.AgentsDocStarterFileName` exports that exact value. If any of those flip, tests drift in lockstep and pass silently. Testing guideline explicitly forbids: "assert against ground truth ... not against the string constant the implementation also produced."

```go
// git_committer_test.go:67-73 — expected value composed the same way as the code
wantSpec := filepath.Join(a.Dir, "asset.json")
if len(call.Pathspec) != 1 || call.Pathspec[0] != wantSpec {
    t.Fatalf("pathspec = %v, want [%q]", call.Pathspec, wantSpec)
}
```

- [x] Keep fake tuple checks as smoke tests but pair each with the real-git assertion from the previous finding — ground truth becomes `git log`, not a string composed by the test.
- [ ] Alternative if the real-stack tests are not adopted: hardcode expected suffixes (`"asset.json"`, `"/**"`) as literals with a comment linking to the config constant, so a constant rename must break the test rather than silently passing.

## `GitCommitter` port speaks primitives, not `CommitOutcome` — ACL is misplaced

> [!WARNING]
>
> - [`docs/guidelines/domain_model.md`](../../../docs/guidelines/domain_model.md)
> - [`docs/guidelines/clean_architecture.md`](../../../docs/guidelines/clean_architecture.md)

The task summary calls `internal/app/git.go` the anti-corruption layer. But the seam returns `(sha string, err DomainError)` — same shape `internal/git.Repo.Commit` produces. `gitBinaryCommitter.Commit` is a bare pass-through that only swallows `NotARepoError`. The actual translation into the domain value `CommitOutcome` happens one hop later in `Service.runCommit` (`service.go:534-543`), re-shaping every call at the caller.

```go
// internal/app/git.go:16
type GitCommitter interface {
    Commit(dir string, pathspec []string, msg string) (string, errs.DomainError)
}
```

- [x] Change `GitCommitter.Commit` to return `CommitOutcome`. Move the `NotARepoError` swallow into `gitBinaryCommitter.Commit` as the sole ACL responsibility. `runCommit` shrinks to `if !s.commitEnabled() { return CommitOutcome{} }; return s.committer.Commit(...)`.
- [ ] Alternative narrower fix: keep the primitives-returning port but add `Unwrap() error` on `CommitError` / `HookFailedError` so callers can drill in via `errors.As`; document the pass-through nature of `gitBinaryCommitter` so future readers don't mistake it for a translation layer.

## `CommitOutcome` zero-value conflates three distinct domain outcomes

> [!WARNING]
>
> - [`docs/guidelines/domain_model.md`](../../../docs/guidelines/domain_model.md)
> - [`docs/guidelines/errors.md`](../../../docs/guidelines/errors.md)

`CommitOutcome{}` (zero-value) currently means all of: (a) feature disabled in settings, (b) dir not a git repo, (c) empty diff on the pathspec, (d) "no commit was attempted." Callers rediscover this protocol every time (`shell/commits.go:18-40`). The `Err` field is a silent second error channel outside the function signature — the compiler cannot enforce that a caller checks it.

```go
type CommitOutcome struct {
    SHA string
    Err errs.DomainError
}
```

- [x] Refactor to a discriminated result: `Committed{SHA string}`, `Skipped{Reason SkipReason}` (with `SkipDisabled`, `SkipNotARepo`, `SkipEmptyDiff`), `Failed{Err errs.DomainError}`. Callers pattern-match; the type system enforces coverage of the three skip reasons the glossary already distinguishes.
- [ ] Lighter fix: add `Skipped bool` + `SkipReason string` fields alongside `SHA` / `Err` so the three skip flavors are at least visible; document the zero-value protocol on the type doc comment.

## Auto-commit executes repo-supplied hooks with no user opt-out

> [!WARNING]
>
> - [`docs/guidelines/security.md`](../../../docs/guidelines/security.md) — "Avoid Unsafe Execution Paths"

`internal/git/git.go:116` runs `git commit -m msg` without `--no-verify`. Every `UpdateAsset` / `SaveAssetFilesEdit` / `Apply` executes whatever `pre-commit`, `prepare-commit-msg`, and `commit-msg` scripts sit at `<repo>/.git/hooks/` (or `core.hooksPath`). Target-repo hooks come from user-checked-out code — a hostile branch or a helper script post-configuring a hook can slip a script that runs the next time the user opens agentfiles. User's mental model: "typed a comment into the TUI." Actual behavior: ran arbitrary code from the checked-out branch. `HookFailedError.Stderr` also carries the hook's stderr through to a TUI toast — small but nonzero exfiltration surface.

```go
// internal/git/git.go:116 — hooks run by default
if _, err := r.runCommit("-C", r.Root, "commit", "-m", msg); err != nil {
```

- [x] Pass `--no-verify` by default so auto-commits never trigger repo-supplied code. Add a second setting `git.run_hooks: bool` for users who explicitly opt in. Document in ADR 0019.
- [ ] Alternative: use `git commit-tree` + `git update-ref` to record commits without invoking the hook machinery at all; also removes the "hook installed → HookFailedError" heuristic.
- [ ] Alternative if keeping hooks: truncate/redact `HookFailedError.Stderr` in the toast (show only "hook rejected commit; run `git commit` manually for details") to reduce exfiltration surface.

## Inherited environment lets `GIT_*` vars redirect commits

> [!WARNING]
>
> - [`docs/guidelines/external_tools.md`](../../../docs/guidelines/external_tools.md) — "the package owns … environment lookup"
> - [`docs/guidelines/security.md`](../../../docs/guidelines/security.md)

`internal/git/git.go:216, 238` build `exec.Command("git", ...)` and never set `cmd.Env`. Child inherits full parent env, including `GIT_DIR`, `GIT_WORK_TREE`, `GIT_INDEX_FILE`, `GIT_CONFIG_COUNT`/`GIT_CONFIG_KEY_n`/`GIT_CONFIG_VALUE_n`, `GIT_ALTERNATE_OBJECT_DIRECTORIES`, `GIT_TEMPLATE_DIR`. The wrapper advertises "commits are scoped to the given work-tree" but a stray `GIT_DIR` silently overrides the `-C r.Root` semantics. `GIT_CONFIG_COUNT` can inject config for the child. Guideline is explicit that the package owns env lookup.

```go
cmd := exec.Command("git", args...)
// cmd.Env == nil → child inherits os.Environ() unfiltered
```

- [x] Set `cmd.Env` explicitly: filtered copy of `os.Environ()` that drops `GIT_*` variables the wrapper does not want the child to honor; allow-list `HOME`, `PATH`, `USER`, `LANG`, `TZ`, `SSH_AUTH_SOCK` (for signing). Force `GIT_OPTIONAL_LOCKS=0`. Add a targeted test that sets `GIT_DIR=/tmp/somewhere` in the test process and confirms the wrapper still commits into `r.Root`.
- [ ] Lighter fix: whitelist only `HOME` + `PATH` (minimum for git to work) and let signing/etc. break if configured — smaller change but likely to surface user friction.

## Unsanitized `proj.Name` / `manifest.ID` interpolated into commit subject

> [!WARNING]
>
> - [`docs/guidelines/security.md`](../../../docs/guidelines/security.md)

`service.go:468, 966, 989` build subjects via `fmt.Sprintf` with `proj.Name` and `manifest.ID` unbounded and unsanitized. Newline in `proj.Name` splits the subject across the body silently; a paste-accident value like `password=foo` lands verbatim in permanent git history; unbounded length blows past the 50-char conventional-commits subject norm. No shell-injection risk (args are separated), but the guideline forbids unsanitized user data in generated content.

```go
msg := fmt.Sprintf("chore(agentfiles): sync project %s (%d files)", proj.Name, len(mutated)-1)
// proj.Name is a free-form user string; \n, \r, control chars pass through
```

- [x] Add a `sanitizeSubject(s string, max int) string` helper (strip control bytes, collapse whitespace, truncate) inside the commit-message helper introduced in the first finding. Apply it to every interpolated user field. Unit-test with newline / control-char / 200-char inputs.
- [ ] Alternative: reject `\n`, `\r`, control bytes at the boundary (`project.Manifest.Validate`, `asset.Manifest.Validate`) so the values can never carry them by the time they reach the commit path.

## Symlink not resolved before repo-root escape check in `toRepoRelative`

> [!WARNING]
>
> - [`docs/guidelines/security.md`](../../../docs/guidelines/security.md) — "Keep File Access Inside Intended Roots"

`internal/git/git.go:131-165` uses purely lexical `filepath.Abs` + `filepath.Rel` + `..` prefix check to gate "is this pathspec entry inside the work-tree?" Neither `Abs` nor `Rel` resolves symlinks. `r.Root` also comes from `git rev-parse --show-toplevel` without symlink resolution on the input `-C dir`. A profile folder (or ancestor) that is a symlink whose target lives elsewhere can produce a pathspec that looks "inside" `r.Root` lexically while the symlink-resolved target is unrelated.

```go
abs, err := filepath.Abs(base)              // lexical only
rel, relErr := filepath.Rel(r.Root, abs)    // lexical only
if rel == ".." || strings.HasPrefix(rel, "../") {
    outside = append(outside, p)            // catches "../" but not symlinks
}
```

- [x] `EvalSymlinks` `r.Root` once in `Detect` and store the resolved value. `EvalSymlinks` the caller's `abs` in `toRepoRelative`. Both containment checks compare canonical paths. Add table-test: profile root = symlink, pathspec entry that lexically matches the symlink source but resolves outside → rejected.
- [ ] Alternative: reject symlinked profile roots entirely at `Detect` with a typed `SymlinkedRootError` — simpler but changes UX for users who legitimately symlink their profile folder.

## TOCTOU race between staged-changes gate and `git add`

> [!WARNING]
>
> - [`docs/guidelines/security.md`](../../../docs/guidelines/security.md)

`internal/git/git.go:79-124` reads the staged-index at T0 (`stagedPaths`), calls `git add` at T1, calls `git commit` at T2. Nothing locks the index in between. A concurrent `git add` in another shell — or a second running `af` instance — can stage an unrelated file between T0 and T1, and the commit at T2 sweeps it in. Directly contradicts the invariant "unrelated user work is never rolled into an automated commit" (docstring at `git.go:66-77`).

```go
staged, stagedErr := r.stagedPaths()      // T0: read
// coverage check on T0 snapshot
if _, err := r.run(addArgs...); err != nil { ... }  // T1: write
// no re-check
if _, err := r.runCommit(...); err != nil { ... }   // T2: commit
```

- [x] Between `git add` and `git commit`, re-run `git diff --cached --name-only` and re-apply `Covers`; abort with `UnrelatedStagedChangesError` if a new entry appeared. Same helper, two calls.
- [ ] Alternative: pass pathspec after `--` to `git commit` (`git commit -m msg -- <files>`) so the commit itself constrains what gets included regardless of index state. Simpler and moves the guarantee to git rather than a re-check.
- [ ] Alternative: take an advisory filesystem lock (`.agentfiles/commit.lock`) around the read-check-add-commit sequence so concurrent `af` invocations serialize.

## `hookInstalled` heuristic misclassifies non-hook failures

> [!WARNING]
>
> - [`docs/guidelines/clean_code.md`](../../../docs/guidelines/clean_code.md)

`internal/git/git.go:252-268` classifies a failed commit as `HookFailedError` whenever ANY executable non-empty file exists at `.git/hooks/{pre-commit,prepare-commit-msg,commit-msg}`. Docstring admits this is a heuristic ("Git does not include a stable 'hook failed' line"). A commit that failed for an unrelated reason (dirty tree, gpg failure, config error) will be reported as `HookFailedError` if any hook happens to be installed. TUI renders it as authoritative fact.

```go
func (r *Repo) hookInstalled() bool {
    hooksDir := filepath.Join(r.Root, ".git", "hooks")
    for _, name := range []string{"pre-commit", "prepare-commit-msg", "commit-msg"} {
        info, err := os.Stat(filepath.Join(hooksDir, name))
        ...
        if info.Mode()&0o111 != 0 && info.Size() > 0 { return true }
    }
    return false
}
```

- [x] Parse the actual stderr text for well-known hook markers (`hint: The '.*' hook exited`, `error: cannot spawn .git/hooks/`) before falling back to the heuristic. False positives become tied to a stderr signal.
- [ ] Alternative: rename `HookFailedError` → `LikelyHookFailedError` (or add `Certain bool`) so callers cannot render it as authoritative fact; adjust toast text accordingly.
- [ ] Alternative if `--no-verify` is adopted (see hook-execution finding): drop `hookInstalled` and `HookFailedError` entirely — no hooks run, no need to classify.

## `CommitError` / `HookFailedError` drop underlying `*exec.ExitError`

> [!WARNING]
>
> - [`docs/guidelines/errors.md`](../../../docs/guidelines/errors.md)
> - [`docs/guidelines/go.md`](../../../docs/guidelines/go.md)

`internal/git/git.go:197, 207, 246, 248` map exec errors into `CommitError{Stderr: err.Error()}` via string. `stagedIsEmpty` (line 191-200) also uses `err.(*exec.ExitError)` type assertion instead of `errors.As`, bypassing any wrapper. Neither `CommitError` nor `HookFailedError` (`git/errors.go:63-88`) has an `Err error` field or `Unwrap()`. Callers cannot `errors.As` into the exec error, and when stderr is empty the fallback `err.Error()` is a useless `"exit status 128"` stored as `Stderr`.

```go
func (r *Repo) run(args ...string) (string, errs.DomainError) {
    out, err := runCapture(args...)
    if err != nil {
        return "", CommitError{Stderr: err.Error()}  // exec error lost
    }
    return out, nil
}
```

- [ ] Add `Err error` field + `Unwrap() error` to `CommitError` and `HookFailedError` (mirroring `BinaryMissingError` at `git/errors.go:14-28`). Change `stagedIsEmpty` to `errors.As(err, &exitErr)`.
- [x] Also rename the `Stderr` field to something honest (`Detail` or `Message`) since it can hold either stderr text or the exec fallback summary.

## `runErr` unexported wrapper adds allocation with no consumers

> [!WARNING]
>
> - [`docs/guidelines/clean_code.md`](../../../docs/guidelines/clean_code.md)
> - [`docs/guidelines/go.md`](../../../docs/guidelines/go.md)

`internal/git/git.go:231-233` defines `runErr{msg string}` with `Error()` returning `msg`. Produced once at line 224. Every consumer immediately calls `.Error()` and stores the string in a domain error's `Stderr` field. The type never escapes the package, never appears in `errors.As`, and adds one indirection for nothing.

```go
type runErr struct{ msg string }
func (e *runErr) Error() string { return e.msg }
```

- [x] Delete `runErr`; replace `&runErr{msg: s}` at line 224 with `errors.New(s)`.
- [ ] Alternative: if a typed wrapper is desired, promote it to a package-exported error and actually `errors.As` for it at the call site so it earns its keep.

## `Repo.Commit` is a 46-line 5-phase procedure

> [!WARNING]
>
> - [`docs/guidelines/clean_code.md`](../../../docs/guidelines/clean_code.md)

`internal/git/git.go:79-124` inlines five phases: pathspec conversion, coverage check, has-changes short-circuit, `git add` + `stagedIsEmpty` re-check, commit + SHA lookup. The docstring above documents them as a numbered list — strong tell that each phase deserves a named helper.

- [x] Extract private helpers: `r.ensureCoveredBy(relSpec) errs.DomainError`, `r.commitOrSkip(gitSpec, msg) (string, errs.DomainError)`. Top-level `Commit` reads as the five-line summary the docstring already describes.
- [ ] Alternative: leave the shape but split `internal/git/git.go` into `git.go` (exec + `Detect` + `Commit`), `pathspec.go` (`Covers`, `toRepoRelative`, `toGitPathspec`), `hooks.go` (`hookInstalled` + related). Related SRP finding below.

## `mutatedPaths` re-derives sync's write knowledge inside `app`

> [!WARNING]
>
> - [`docs/guidelines/clean_architecture.md`](../../../docs/guidelines/clean_architecture.md)

`internal/app/service.go:479-520` walks `Preview.Changes` and applies the drift/unknown resolution rules a second time to reconstruct what sync just wrote, then appends `.agentfiles/state.json`. The CLAUDE.md invariant #6 says "render computes, sync writes" — corollary: "what sync wrote" is sync's knowledge. Any future change to sync's write rules (new `ChangeKind`, additional side-writes, split state files) has to be mirrored here or the commit pathspec silently misses files. The `.agentfiles/state.json` hardcoded at `service.go:480, 517` is sync's bookkeeping detail leaking into `app`.

```go
// service.go:480 — sync's side-write hardcoded in app
stateAbs := filepath.Join(projRoot, config.StateDirName, config.StateFileName)
// service.go:502-515 — drift/unknown decision matrix mirrored from sync
```

- [x] Have `llmsync.Apply` return the absolute path list of files it mutated (creates + updates + deletes + resolved drifts/unknowns + `state.json`). `Service.Apply` hands the list straight to `runCommit` without reproducing the rule.
- [ ] Alternative: export `llmsync.MutatedPaths(preview, resolutions, root) []string` as a public helper in `sync`; `app.Apply` calls it. Same rule, one home.

## `tui/shell` imports `internal/app` directly, contradicting `05-building-block-view.md`

> [!WARNING]
>
> - [`docs/guidelines/clean_architecture.md`](../../../docs/guidelines/clean_architecture.md)

`docs/architecture/05-building-block-view.md:15, 58-61` documents the edge `tui_shell → actions → app` (no direct `tui_shell → app`). Reality: `plan_project.go:16`, `edit_asset.go:14`, `select_project_assets.go:12`, `edit_profile.go:13`, `profiles.go:14`, and the new `commits.go:6` all `import "internal/app"` directly and consume `app.CommitOutcome`, `app.LoadedProfile`, `app.Preview`, `app.FileChange`, `app.DriftDecision`, `app.RegisterableDirs`, etc. Task 0042 extends this by threading `app.CommitOutcome` through every commit-touched screen. Pre-existing but doubled down.

- [ ] Update `docs/architecture/05-building-block-view.md` to describe reality: "`tui_shell` imports `app` for value types; mutations go through `actions`" — the documented invariant stops lying.
- [x] Alternative: move `CommitOutcome`, `Preview`, `FileChange`, `DriftDecision`, `UnknownDecision`, `RegisterableDirs`, `DriftResolutionsFromMap`, `DesiredIgnored` into `actions` (or a new `internal/appapi` leaf package) so the shell honors the single `tui → actions` edge.

## Building block diagram missing new package edges

> [!WARNING]
>
> - [`docs/guidelines/documentation.md`](../../../docs/guidelines/documentation.md)

`docs/architecture/05-building-block-view.md:8-45` Level-1 mermaid diagram does not include `app → git`, `app → settings`, `cmd/af → settings`, `tui_shell → settings`, `settings → config/errs/utils`, `git → errs`. Level-2 prose sections describe the new packages but the diagram reviewers use for import-direction spot-checks is out of sync.

- [ ] Add the missing edges to the mermaid graph. Mark `git` and `settings` as leaf-ish infrastructure alongside `utils`/`config` (blue class) so the visual convention stays truthful.
- [x] Also add `docs/architecture/09-architecture-decisions.md` ADR-0019 to the diagram's decision-reference list if you keep one.

## `internal/git/git.go` mixes exec + pathspec + diff + hook heuristics in one file

> [!WARNING]
>
> - [`docs/guidelines/solid.md`](../../../docs/guidelines/solid.md)

290 lines mixing four unrelated concerns: exec adapter (`run`, `runCapture`, `runCommit`, `runErr`), pathspec translation (`toRepoRelative`, `toGitPathspec`), diff-state queries (`hasChanges`, `stagedPaths`, `stagedIsEmpty`), hook-detection heuristic (`hookInstalled`, `firstNonEmpty`). Evidence they are independent concerns already exists — `pathspec.go` was split out for `Covers`.

- [x] Move `toRepoRelative` and `toGitPathspec` next to `Covers` in `pathspec.go`. Move `hookInstalled` + `firstNonEmpty` into `hooks.go`. `git.go` shrinks to the exec adapter + `Detect` + `Repo.Commit` orchestration.
- [ ] Alternative: leave the file structure; add package-doc-level section comments demarcating the four regions. Cheaper but does not address the "one reason to change" concern.

## `Service.SettingsStore` exported → cached settings can desync

> [!WARNING]
>
> - [`docs/guidelines/domain_model.md`](../../../docs/guidelines/domain_model.md)

`internal/app/service.go:70-71` has `SettingsStore *settings.Store` (exported) alongside `settings settings.Settings` (unexported cache). Nothing prevents a caller from invoking `s.SettingsStore.Save(x)` directly, desyncing the cache. `Registry` and `Projects` are also exported but they are the sole owners of their aggregate state — no cached mirror to keep in lockstep.

- [ ] Unexport `settingsStore` (make it `settingsStore` like `committer`). `UpdateSettings` becomes the only write path.
- [x] Also add a mutex around the `settings` field: Bubble Tea executes `tea.Cmd` closures on goroutines; a `PlanProject` cmd calling `commitEnabled()` can race with an `UpdateSettings` cmd on another goroutine. Practical odds low, but race detector flags it. `sync.RWMutex` around read (`commitEnabled`, `Settings()`) + write (`UpdateSettings`).
