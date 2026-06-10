# Sync ChangeUnknown + Resolutions review

The implementation lands the headline goals: `ChangeUnknown` is wired through `Plan`, `Apply`, `doctor`, and the preview renderer, the bool `deleteCandidates` flag is gone, `FileResolution` carries per-file choices, and the first-apply clean slate is in place. The ADR, architecture, guideline, and changelog updates accompany the code.

The review found one functional defect (state rewrite contradicts the plan and silently re-baselines kept drift), two real security exposures inherited and amplified by the refactor (path traversal via state keys for auto-applied `ChangeDelete`, and a TOCTOU/symlink window on `writeRendered`), and a cluster of contract gaps where the new `Resolution` enum encodes its rules only in prose: invalid `(Kind, Resolution)` pairs are silently no-op'd, `ResolveAuto` is unused, the int+iota default coincides with the missing-key zero value, and `FileResolution.Path` semantics are not pinned. Several drive-by hygiene issues round out the list: TUI now imports `internal/sync` directly, `doctor.convertKind` is a no-op identity switch the task extended instead of removing, `Plan` mixes too many abstraction levels, magic-string reasons are duplicated across the engine and the renderer test, and the glossary plus several docstrings still talk about "delete candidates" that no longer exist.

Test coverage matches most of plan step 10 but drops the planned `TestApply_StateRewritten`, never asserts the state-rewrite invariant for kept-drift, and uses no `errors.As` assertion despite the project's typed-error guideline. The `ChangeUnknown` glyph in `render_preview` is `?` — same as the default fallback — so the render preview test cannot tell the new branch from the silent default.

Issues below. Tick exactly one solution per block.

## 1. `Apply` rewrites `ManagedState.ManagedFiles` from `preview.Files` unconditionally, contradicting plan and breaking drift tracking after `ResolveKeep`

> [!WARNING]
>
> - [`docs/guidelines/sync_and_safety.md`](../../../docs/guidelines/sync_and_safety.md) — "store hashes of the newly written managed files" / "distinguish update from drift"
> - [`docs/guidelines/clean_code.md`](../../../docs/guidelines/clean_code.md) — Fragility: one change breaks distant behavior
> - [`docs/guidelines/solid.md`](../../../docs/guidelines/solid.md) — SRP, the state-rewrite responsibility cannot see which branch ran

Plan step 6 is explicit: "record hashes of every file actually written (Create + Update + Overwrite-Drift). Files that were kept (drift kept or unknown kept) are **not** added to managed state." The implementation does the opposite — at the bottom of `Apply` it loops over `preview.Files` (every desired file) and stamps the _rendered-body_ hash into state regardless of which branch of the switch ran. A `ChangeDrift` row that the user resolved to `ResolveKeep` therefore receives the desired hash in state even though the on-disk content is still the drifted version. On the next `Plan`, the comparison `state.ManagedFiles[file.Path] != currentHash` is true (state holds desired, disk holds drifted), so the row is reclassified as `ChangeDrift` again with reason "managed file changed locally" — but the _meaning_ of state is now wrong: it claims the managed baseline is the desired body, not what is on disk. Any future logic that trusts state as ground truth (drift detection, doctor reporting, conflict resolution) silently degrades. No test asserts this.

```go
// internal/sync/sync.go (Apply, tail)
for _, change := range preview.Changes {
    switch change.Kind {
    // ... drift may be kept, unknown may be kept ...
    }
}
state := &ManagedState{ /* ... */ ManagedFiles: map[string]string{} }
for _, file := range preview.Files {
    state.ManagedFiles[file.Path] = utils.HashBytes(file.Body) // unconditional — kept drift gets desired hash anyway
}
```

Pick one:

- [ ] Track written paths inside the switch (`writtenPaths := map[string]bool{}`, set on Create/Update/Overwrite-Drift, dropped on auto-delete). After the switch, build `state.ManagedFiles` by intersecting `preview.Files` with `writtenPaths` and preserving the previous state hash for kept-drift entries (or omitting them entirely, matching the plan literally).
- [ ] Split `Apply` into `apply(preview, resolutions) (writtenPaths []string, err errs.DomainError)` and `recordState(preview, writtenPaths) errs.DomainError`, called sequentially by `Service.Apply`. The state writer then takes explicit input and the responsibilities separate cleanly.
- [ ] Update plan.md + changelog to describe the _current_ implementation (state always reflects the rendered plan) and add a test that pins it. This codifies that drift never escapes back to a managed baseline through `ResolveKeep`; pick only if the team agrees that's intended.
- [x] **Adopt-current-as-baseline on `ResolveKeep`.** Inside the switch's drift branch, when the resolution is `ResolveKeep`, hash the on-disk file (`utils.HashFile(abs)`) and record _that_ hash in `state.ManagedFiles[path]` — not the rendered-body hash. Skip the unconditional `for _, file := range preview.Files` rewrite at the tail (use the per-branch tracking from option 1 to drive it). Effect: next `Plan` sees `state[path] == currentHash` so the file is no longer drift; it falls through to `ChangeUpdate` because `currentHash != desired[path]`, which the user then chooses to apply (overwrite kept content) or skip again. Add a test `TestApply_ResolveKeep_AdoptsCurrentAsBaseline` that asserts (a) state hash equals on-disk hash, (b) the next `Plan` no longer reports the path as `ChangeDrift`. Note: this does not make the kept content permanently win against the rendered desired output — that would require a separate "user-preferred" override field on `ManagedState`; flag as a follow-up if you want that stronger semantics.

## 2. `Apply` silently ignores invalid `(ChangeKind, Resolution)` combinations

> [!WARNING]
>
> - [`docs/guidelines/domain_model.md`](../../../docs/guidelines/domain_model.md) — "model constraints explicitly", domain rules in domain code
> - [`docs/guidelines/clean_code.md`](../../../docs/guidelines/clean_code.md) — Understandability: hidden invariants
> - [`docs/guidelines/solid.md`](../../../docs/guidelines/solid.md) — LSP: `Resolution` values are not substitutable across kinds

The docstrings on `Resolution` make a hard claim: `ResolveOverwrite` is "drift only", `ResolveDelete` is "unknown only", `ResolveAuto` applies "only valid for create/update/delete". `Apply` enforces none of it. The drift branch only checks `!= ResolveOverwrite`, so `{Path: "AGENTS.md (drift)", Resolution: ResolveDelete}` is silently coerced to keep. The unknown branch only checks `!= ResolveDelete`, so `{Path: "x (unknown)", Resolution: ResolveOverwrite}` is silently coerced to keep. There is no typed error for this and no test exercises it. Until the Plan Project Screen (task 0029) becomes the only producer, that producer's bugs (or any future API consumer's) will silently disappear into a no-op — exactly the failure mode "model constraints explicitly" exists to prevent.

```go
// internal/sync/sync.go (Apply)
case ChangeDrift:
    if resolutionsByPath[change.Path] != ResolveOverwrite { continue } // ResolveDelete here → silently kept
case ChangeUnknown:
    if resolutionsByPath[change.Path] != ResolveDelete { continue }    // ResolveOverwrite here → silently kept
```

Pick one:

- [ ] Add `InvalidResolutionError{Path, Kind, Resolution}` in `internal/sync/errors.go`. Validate the resolutions slice up front against `preview.Changes` (one walk, accumulate via `[]errs.DomainError`), return `errs.Errors(...)` before doing any I/O. Apply then assumes a validated input.
- [x] Make invalid pairings unrepresentable by splitting `Apply`'s parameter into two typed slices: `driftDecisions []DriftDecision` (with `DriftOverwrite | DriftKeep`) and `unknownDecisions []UnknownDecision` (with `UnknownDelete | UnknownKeep`). The type system enforces the contract; no runtime validator needed.
- [ ] Document the silent-fallback policy explicitly in `Apply`'s doc comment and in ADR 0010's "Considered but not done" section. Lock it in by adding `TestApply_InvalidResolutionForKind_IsIgnored`. Pick only if silent absorption is genuinely the desired contract.

## 3. Path traversal via tampered `ManagedState.ManagedFiles` keys causes auto-delete outside the project root

> [!WARNING]
>
> - [`docs/guidelines/security.md`](../../../docs/guidelines/security.md) — keep file access inside intended roots; reject unsafe path forms
> - [`docs/guidelines/sync_and_safety.md`](../../../docs/guidelines/sync_and_safety.md) — managed state is the basis for safe comparison; never expand deletion to unrelated repository files

`loadState` JSON-decodes `state.json` into a `map[string]string` without validating the keys. `detectExtraneous` iterates `state.ManagedFiles`, marks any key missing from `desired` as a `ChangeDelete`, and the entry flows straight into `Apply`'s `removeFile(projectPath, change.Path)`. Because `ChangeDelete` is auto-applied with no user opt-in (per ADR 0010), a tampered or hostile `state.json` with a key like `"../../home/user/.ssh/authorized_keys"` causes `os.Remove` to delete files outside the project root the next time `Apply` runs. The user never sees the `..` segment translated to its absolute target in the preview, and `removeFile` does not check whether the resolved absolute path stays under `projectPath`.

```go
// internal/sync/sync.go — detectExtraneous → Apply → removeFile
for path := range state.ManagedFiles {
    if desired[path] == "" { deleteSet[path] = true }   // path comes from JSON, untrusted
}
case ChangeDelete:
    if err := removeFile(preview.ProjectPath, change.Path); err != nil { ... }
// removeFile:
abs := filepath.Join(projectPath, filepath.FromSlash(path))
if err := os.Remove(abs); err != nil && !os.IsNotExist(err) { ... }
```

Pick one:

- [ ] Validate every key in `ManagedFiles` inside `loadState`: reject absolute paths and any path whose `filepath.Clean` form contains a leading `..` segment, returning a typed `StateCorruptError`. Mirror the same check inside `removeFile` and `writeRendered` as a defense in depth — recompute `filepath.Rel(projectPath, abs)` and bail if the result begins with `..` or is absolute.
- [ ] Cross-check every change path against `surfaces.IsAllowed(change.Path)` before writing or deleting; the managed-surface fence is already a domain concept, extend it to operate at apply time too. Stronger than path validation because it also catches future bugs that route a non-surface path into `Changes`.
- [x] Both: validate in `loadState` and gate writes/deletes through `surfaces.IsAllowed`. Strongest option; pick if you want defense in depth for a high-blast-radius operation.

## 4. TOCTOU symlink follow on `writeRendered` and `os.Stat` surface walk

> [!WARNING]
>
> - [`docs/guidelines/security.md`](../../../docs/guidelines/security.md) — symlinks are safety-sensitive during sync

Two related issues. (a) `writeRendered` calls `utils.WriteFile` which delegates to `os.WriteFile` — symlink-following. Between `Plan` (which classifies based on `utils.Exists` + `HashFile`) and the TUI confirmation prompt in `RunProjectApply`, an attacker with write access to the project tree can replace a managed file (e.g. `AGENTS.md`) with a symlink to `/etc/passwd` or another sensitive path; the subsequent write through `os.WriteFile` clobbers the link target with the rendered body. The new `Resolution` API does not gate `ChangeCreate`/`ChangeUpdate` — they are auto-applied. (b) `detectExtraneous` uses `os.Stat(abs)` on each surface root (`.claude`, `.cursor`, …); `Stat` follows symlinks. If `.claude` is a symlink to an unrelated directory, `info.IsDir()` reports true and `filepath.WalkDir` is invoked on the link target. Files discovered that way become `ChangeUnknown` entries the user can `ResolveDelete` — without seeing they are cross-boundary.

```go
// (a) internal/sync/sync.go (Apply → writeRendered → utils.WriteFile → os.WriteFile)
case ChangeCreate, ChangeUpdate:
    if err := writeRendered(preview.ProjectPath, bodiesByPath[change.Path]); err != nil { ... }

// (b) internal/sync/sync.go (detectExtraneous)
info, err := os.Stat(abs)   // follows symlinks
if !info.IsDir() { ... }
walkErr := filepath.WalkDir(abs, ...)
```

Pick one:

- [x] Harden both sites. In `writeRendered`: `os.Lstat` the target first; if it exists and is a symlink, return a typed `UnsafeSymlinkError` rather than writing through. Use `os.OpenFile(..., O_NOFOLLOW, ...)` on Linux for the actual write so a final-segment race is refused by the kernel. In `detectExtraneous`: replace `os.Stat` with `os.Lstat` and skip (or emit a typed error for) symlinked surface roots; also `Lstat` entries inside the walk so symlinks never enter `unknownSet`.
- [ ] Only address (a) now since it is the write path; punt (b) to a follow-up task with the existing security guideline. Pick if the team wants a narrower change that still closes the highest-blast-radius hole.
- [ ] Treat both as out of scope for 0017 and file a follow-up task plus an ADR change that explicitly accepts symlink-follow as the current behavior, documenting the trust assumption (user controls the project tree). Pick only if you can accept that on the record.

## 5. `safe()` does not strip bidi overrides, zero-width formatters, or C1 control bytes from attacker-controlled filenames

> [!WARNING]
>
> - [`docs/guidelines/security.md`](../../../docs/guidelines/security.md) — reject unknown or unsafe path forms early; do not let hostile profile/asset names rewrite the terminal

`ChangeUnknown` paths come straight from a filesystem walk inside a managed surface; anyone who can drop a file into `.claude/` of the target repo controls the rendered path. `RenderPreview` calls `safe()` on the path, but `safe()` only filters byte-level `< 0x20` and `0x7F` ASCII controls. UTF-8-encoded C1 controls (e.g. CSI as `0xC2 0x9B`), Unicode bidi overrides (`U+202A..U+202E`, `U+2066..U+2069`), and zero-width formatters (`U+200B..U+200D`, `U+FEFF`) pass through unchanged. A filename like `.claude/<U+202E>dlrow.txt` flips the visible direction of the previewed path, so the user about to tick `ResolveDelete` on what they think is one row is actually approving the deletion of another.

```go
// internal/tui/styles.go — safe()
case c == '\n' || c == '\t':
    b = append(b, c)
case c < 0x20 || c == 0x7f:
    // drop ASCII control
default:
    b = append(b, c)   // bidi overrides and zero-width runes pass through
```

Pick one:

- [ ] Extend `safe()` to iterate runes (not bytes), drop C1 controls (`U+0080..U+009F`), bidi overrides (`U+202A..U+202E`, `U+2066..U+2069`), and zero-width formatters (`U+200B..U+200D`, `U+FEFF`). Add a test fixture with a known hostile filename to lock the behavior.
- [x] Render path strings via `strconv.Quote` whenever they contain any non-printable Unicode rune, so the user sees the escaped form rather than the rendered glyph. Less surgical, more deterministic.

## 6. TUI imports `internal/sync` directly, breaching the documented `tui → app` dependency edge

> [!WARNING]
>
> - [`docs/guidelines/clean_architecture.md`](../../../docs/guidelines/clean_architecture.md) — dependency direction; Common Reuse Principle
> - [`docs/architecture/05-building-block-view.md`](../../../docs/architecture/05-building-block-view.md) — the package overview shows `tui → app`, not `tui → sync`
> - [`docs/guidelines/tui.md`](../../../docs/guidelines/tui.md) — TUI calls `app.Service` or domain functions for use cases; sync vocabulary must not leak

`internal/tui/forms.go` now imports `internal/sync` (as `llmsync`) only to materialize `[]llmsync.FileResolution{}` for `service.Apply`. The TUI currently builds zero resolutions — it always passes an empty slice — yet the import drags a sync-owned type past the orchestration boundary the building-block view promises. The same alias drift is now present in `render_preview.go`, while sibling files (`render_report.go`) still import the package as plain `sync`. The Plan Project Screen in task 0029 will eventually need _some_ representation of resolutions, but that screen is in the TUI layer and should consume an app- or screen-owned type, not a `sync.FileResolution`.

```go
// internal/tui/forms.go
llmsync "github.com/hexworks/agentfiles/internal/sync"
// ...
if _, err := service.Apply(profileID, projectID, []llmsync.FileResolution{}); err != nil { ... }
```

Pick one:

- [ ] Add `Service.ApplyDefaults(profileRef, projectID)` that wraps `Apply(..., nil)`. The existing TUI calls that; the Plan Project Screen in 0029 introduces a richer `Service.ApplyWithResolutions(profileRef, projectID, app.FileResolution{...})` when it actually has data. Removes the `llmsync` import from the TUI now.
- [x] Introduce `app.FileResolution` + `app.Resolution` enum; `Service.Apply` translates app-level types into `[]llmsync.FileResolution` before calling the engine. The TUI never sees sync types; the architecture diagram becomes true again.
- [ ] Standardise on `llmsync` everywhere in `internal/tui` and `internal/app` and document the boundary as "TUI may import sync types but must only construct them via `Service`-supplied factories". Weaker; pick only if the team wants the path of least change.

## 7. `doctor.convertKind` is an identity switch; the task extends it instead of fixing the boundary

> [!WARNING]
>
> - [`docs/guidelines/clean_code.md`](../../../docs/guidelines/clean_code.md) — Code Smells: needless complexity, dead abstractions
> - [`docs/guidelines/clean_architecture.md`](../../../docs/guidelines/clean_architecture.md) — stable abstractions, hide volatile detail

`doctor.ProjectChange.Kind` is typed as `llmsync.ChangeKind` despite the package comment claiming doctor exists "without dragging the sync vocabulary across doctor's API boundary". `convertKind` then enumerates every `ChangeKind` and returns it unchanged, with a fallback that re-wraps the string into the same type. The task added a `case llmsync.ChangeUnknown: return llmsync.ChangeUnknown` arm — five lines of code that do nothing. Every future `ChangeKind` will require the same no-op edit. Either commit to a doctor-owned vocabulary or admit there isn't one.

```go
// internal/doctor/doctor.go
func convertKind(k llmsync.ChangeKind) llmsync.ChangeKind {
    switch k {
    case llmsync.ChangeCreate:  return llmsync.ChangeCreate
    case llmsync.ChangeUpdate:  return llmsync.ChangeUpdate
    case llmsync.ChangeDrift:   return llmsync.ChangeDrift
    case llmsync.ChangeDelete:  return llmsync.ChangeDelete
    case llmsync.ChangeUnknown: return llmsync.ChangeUnknown   // added by this task
    }
    return llmsync.ChangeKind(string(k))
}
```

Pick one:

- [x] Delete `convertKind`. Inline `Kind: c.Kind` in `convertChanges`. Update the `ProjectChange` package comment to drop the "doctor owns its vocabulary" claim — the type re-exports `llmsync.ChangeKind` and that is fine. Smallest, most honest fix.
- [ ] Declare `doctor.ChangeKind` (its own string-typed enum mirroring sync's). Rewrite `convertKind` as a real mapping. TUI and other consumers of `doctor.Report` then stop transitively importing `internal/sync`. Pick if the package comment's promise is worth keeping.

## 8. `ResolveAuto` is unused and the `int` + `iota` zero-value coincidence is fragile

> [!WARNING]
>
> - [`docs/guidelines/clean_code.md`](../../../docs/guidelines/clean_code.md) — needless complexity; opacity (zero-value semantics hidden behind iota order)
> - [`docs/guidelines/go.md`](../../../docs/guidelines/go.md) — explicit types over zero-value coincidences

`ResolveAuto` is declared but never compared against, never constructed by any test, and never produced by the TUI — the changelog admits it "exists but is implicit". Worse, because `Resolution` is `int` with `iota`, `ResolveAuto = 0` is the zero value of the type. `Apply` relies on that: `resolutionsByPath[change.Path]` returns `ResolveAuto` on missing key, and the drift/unknown branches happen to be safe under that default because they only check `!= ResolveOverwrite` / `!= ResolveDelete`. Reordering the `iota` block — alphabetising the constants, moving `ResolveKeep` first — silently flips every drift/unknown default. The compiler cannot catch it; the existing explicit-resolution tests do not either.

```go
type Resolution int
const (
    ResolveAuto      Resolution = iota // 0 — zero value of the type, never asserted
    ResolveOverwrite                   // 1
    ResolveKeep                        // 2
    ResolveDelete                      // 3
)
// in Apply, missing-key lookup returns ResolveAuto (== 0); logic depends on that.
if resolutionsByPath[change.Path] != ResolveOverwrite { continue }
```

Pick one:

- [x] Switch to `type Resolution string` (matching `ChangeKind`'s precedent). Constants become `ResolveAuto = "auto"`, etc. Stringer comes for free; the missing-key default is `""` which no branch ever matches, so the drift/unknown defaults are explicit (`if r, ok := resolutionsByPath[path]; !ok || r != ResolveOverwrite { continue }`). Reordering becomes safe.
- [ ] Keep `int`+`iota` but remove `ResolveAuto` entirely and check missing-key with `r, ok := resolutionsByPath[path]; if !ok { r = ResolveKeep }`. Documents the default in code and removes the unused constant.
- [ ] Keep `ResolveAuto` and actually use it: include `case ResolveAuto:` in every relevant switch (`Apply`, future validators) and explicitly reject `ResolveAuto` if supplied for drift/unknown. Pick only if option 2 of issue #2 (typed validation) is also taken — they reinforce each other.

## 9. `sync.Plan` interleaves render, state load, per-file classification, and extraneous detection at three levels of abstraction

> [!WARNING]
>
> - [`docs/guidelines/clean_code.md`](../../../docs/guidelines/clean_code.md) — Functions: one coherent job at one level of abstraction
> - [`docs/guidelines/solid.md`](../../../docs/guidelines/solid.md) — SRP

`Plan` is ~65 lines. The body holds at least three distinct concerns inline: pipeline orchestration (render, state load, sort, return), per-desired-file decision tree (`state == nil` first-apply branch, `utils.Exists` check, hash equal short-circuit, drift vs update), and extraneous-bucket extraction (the `if state != nil { detectExtraneous … }` postscript). The per-file loop alone is a small classifier; the task even already extracted a symmetric `classifyExtraneous` helper for the surface walk, leaving the asymmetry visible.

```go
for _, file := range rendered.Files {
    desired[file.Path] = utils.HashBytes(file.Body)
    if state == nil { /* first-apply */ }
    abs := filepath.Join(...)
    if !utils.Exists(abs) { /* create */ }
    currentHash, hashErr := utils.HashFile(abs)
    if currentHash == desired[file.Path] { continue }
    if state.ManagedFiles[file.Path] != "" && state.ManagedFiles[file.Path] != currentHash { /* drift */ }
    /* update */
}
```

Pick one:

- [x] Extract `classifyDesired(file render.RenderedFile, projectPath string, state *ManagedState) (FileChange, bool, errs.DomainError)` (returning `(change, skip, err)`). `Plan` becomes orchestration only; the per-file decision tree gets a name and a test surface.
- [ ] Introduce a `classifier` struct capturing `state`, `projectPath`, and `desired`; expose `ClassifyDesired` and `ClassifyExtraneous` methods. `Plan` is then ~15 lines of pipeline. Pick if you anticipate more classification logic in 0021/0029.
- [ ] Leave it. The function is at the edge of the threshold and tests cover the branches; pick only if you accept the trade-off and document it in a code comment.

## 10. `errors.go` docstrings still describe "delete-candidate" detection, which no longer exists

> [!WARNING]
>
> - [`docs/guidelines/clean_code.md`](../../../docs/guidelines/clean_code.md) — Comments: do not let comments lie; leave touched code clearer than you found it

The task renamed `detectDeleteCandidates` to `detectExtraneous`, removed `Preview.DeleteCandidates`, and rewrote the `Apply` switch so `removeFile` is invoked both for `ChangeDelete` and for unknowns resolved with `ResolveDelete`. The three error types still describe the old world:

```go
// StatError reports a failure to stat a managed-surface root while
// detecting delete candidates.
type StatError struct { Path string; Err error }

// SurfaceWalkError reports a failure encountered while walking one of
// the managed-surface roots for delete-candidate detection.
type SurfaceWalkError struct { Root string; Err error }

// DeleteError reports a failure to remove a delete-candidate file
// during Apply.
type DeleteError struct { Path string; Err error }
```

Pick one:

- [x] Update all three docstrings to refer to `detectExtraneous` / extraneous-file detection and note that `DeleteError` covers both auto-deletes and `ResolveDelete`-driven unknown removals.

## 11. Magic-string reasons emitted at the source; "unrecognized" reason text mismatched with `detectExtraneous` and `ChangeUnknown`

> [!WARNING]
>
> - [`docs/guidelines/clean_code.md`](../../../docs/guidelines/clean_code.md) — Naming: replace magic strings with named constants
> - [`docs/guidelines/domain_model.md`](../../../docs/guidelines/domain_model.md) — use the project language consistently

`Plan` emits six reason strings as inline literals: `"first apply"`, `"file missing"`, `"content differs"`, `"managed file changed locally"`, `"recognized llm file not selected"`, `"unrecognized file in managed surface"`. The sync test asserts on `"first apply"`; the render-preview test asserts on `"unrecognized file in managed surface"`. They are domain vocabulary, not local English. Worse, three different words describe the same concept in the same code path: the kind is _unknown_ (`ChangeUnknown`), the detection helper is _extraneous_ (`detectExtraneous`), and the user-facing reason is _unrecognized_. The domain-model guideline opens with "prefer the canonical vocabulary from the glossary and existing packages."

```go
changes = append(changes, FileChange{Path: file.Path, Kind: ChangeCreate, Reason: "first apply"})
// ...
changes = append(changes, FileChange{Path: path, Kind: ChangeUnknown, Reason: "unrecognized file in managed surface"})
// helper named:
func detectExtraneous(projectPath string, desired map[string]string, state *ManagedState) ([]string, []string, []errs.DomainError) { ... }
```

Pick one:

- [ ] Promote the six reasons to package-level constants (`ReasonFirstApply`, `ReasonFileMissing`, `ReasonContentDiffers`, `ReasonDriftDetected`, `ReasonStateRecordedDelete`, `ReasonUnknown`) and reference them from `Plan` and tests. Pick _unknown_ as the canonical word: rewrite the reason text to "unknown file in managed surface" and rename `detectExtraneous` → `detectDeletesAndUnknowns` (or similar) so the helper, the kind, and the reason all share the same word.
- [x] Make `Reason` a typed `ReasonKind` enum and let the TUI translate to display text; tests assert against the enum value. Strongest decoupling between domain and presentation; pick if you expect i18n or restyled reasons soon.

## 12. Glossary not updated for the new ubiquitous-language terms; `Delete Candidate` entry is now stale

> [!WARNING]
>
> - [`docs/guidelines/domain_model.md`](../../../docs/guidelines/domain_model.md) — update the glossary when a new durable domain term appears
> - [`docs/guidelines/documentation.md`](../../../docs/guidelines/documentation.md) — domain vocabulary lives in the glossary

The task introduces seven first-class terms — `Resolution`, `FileResolution`, `ResolveAuto/Overwrite/Keep/Delete`, `ChangeUnknown` — plus two compound concepts ("first-apply clean slate", "extraneous file"). None of them appear in `docs/glossary.md`. The existing `Delete Candidate` entry still describes the old model (`Preview.DeleteCandidates`, single bucket) and the `Change Kind` entry still lists only `create / update / drift / delete`.

Pick one:

- [x] Update `docs/glossary.md`: extend `Change Kind` to include `unknown`; rewrite or split the `Delete Candidate` entry into `ChangeDelete` (state-recorded) and `ChangeUnknown` (never-tracked); add entries for `Resolution`, `File Resolution`, and `First-Apply Clean Slate`. Cross-link each new entry to ADR 0010.
- [ ] Mark the glossary update as a follow-up task and add `docs/glossary.md` to the file checklist in `docs/guidelines/documentation.md` so a future task cannot land a new `ChangeKind` constant without touching it. Pick only if the consolidated update is genuinely better deferred.

## 13. "Resolution" overloaded between `sync.Resolution` and `tui/components/modal.ResolutionState`

> [!WARNING]
>
> - [`docs/guidelines/domain_model.md`](../../../docs/guidelines/domain_model.md) — one precise meaning per term in the bounded context; don't invent parallel names

The changelog acknowledges that `modal.ResolutionState` (modal lifecycle: Active/Confirmed/Cancelled) and `sync.Resolution` (per-file apply choice) live in different packages, so they "coexist without renaming". That is true today, but the Plan Project Screen (task 0029) will display modal confirmations _and_ per-file resolutions on the same screen, in the same `internal/tui` package. A developer reading "resolution" in TUI code will not know which one is meant without inspecting imports.

Pick one:

- [x] Rename the TUI modal type to a lifecycle-specific noun (`modal.LifecycleState`, `modal.Outcome`, or `modal.Status`). Reserves "resolution" for the dominant domain meaning (the sync per-file decision). Touches modal callers — small blast radius today, larger after 0029.
- [ ] Keep both names but add an entry to `docs/glossary.md` that documents the collision and tells readers which package owns which meaning. Pick if rename churn is undesirable; ambiguity is at least made explicit.

## 14. Test coverage gaps from plan step 10 and from the errors guideline

> [!WARNING]
>
> - [`docs/guidelines/testing.md`](../../../docs/guidelines/testing.md) — pin documented behavior; assert behavior, not implementation
> - [`docs/guidelines/errors.md`](../../../docs/guidelines/errors.md) — assert on typed errors via `errors.As`

Several specific gaps:

1. Plan step 10 lists `TestApply_StateRewritten` ("after Apply with mix of changes, state.json contains only files actually written"). Not present.
2. The `ResolveKeep` test only asserts file content unchanged; nothing asserts state semantics. Combined with issue #1, the behaviour is unpinned.
3. No symmetric `TestApply_DefaultDrift_LeavesAlone` — only the explicit `ResolveKeep` path is covered.
4. `Apply`'s docstring documents "duplicate paths: last entry wins" — no test asserts it.
5. Changelog explicitly says "resolution path not in `Changes`" is silently ignored — no test asserts it.
6. No test in `sync_test.go` or `doctor_test.go` uses `errors.As` on a typed error. The errors guideline mandates the pattern.

Pick one:

- [x] Add all six tests: `TestApply_StateRewritten`, `TestApply_ResolveKeep_StateSemantics`, `TestApply_DefaultDrift_LeavesAlone`, `TestApply_DuplicateResolutions_LastWins`, `TestApply_UnknownResolutionPath_IsIgnored`, and one `errors.As`-based negative test (e.g. corrupt `state.json` → `errors.As` on the typed read error).
- [ ] Add the first four (state, default drift, duplicate, unknown-path) and defer the `errors.As` coverage to a follow-up. Pick if the team agrees the typed-error pattern is better addressed across the package in a focused pass.

## 15. `RenderPreview` test cannot tell the `ChangeUnknown` branch from the default `?` fallback

> [!WARNING]
>
> - [`docs/guidelines/testing.md`](../../../docs/guidelines/testing.md) — assertion must pin the behavior the test name promises

`changeStyle` returns `"?"` for `ChangeUnknown` and `"?"` for the default fallback. The new test asserts the rendered line contains `"? [unknown] .cursor/stray.md: ..."`. If the `case llmsync.ChangeUnknown:` branch were deleted entirely, the fallback would still produce `"?"` and the kind string `[unknown]` would still be interpolated — the test would still pass.

```go
case llmsync.ChangeUnknown:
    return "?", driftStyle
}
return "?", mutedStyle   // identical glyph
```

Pick one:

- [ ] Pick a distinct glyph for `ChangeUnknown` (e.g. `"u"` or `"⁇"`) — the test then differentiates by character. Bonus: users can tell unknown from "internal error fallback" at a glance.
- [x] Keep the glyph but assert on the styled output (`mutedStyle` vs `driftStyle` produce visibly different ANSI prefixes). Render without stripping ANSI for this single assertion. Strongest pinning; pick if changing the glyph is undesirable.

## 16. "First-apply clean slate" is a domain decision that lives only as an `if state == nil` branch with a magic reason string

> [!WARNING]
>
> - [`docs/guidelines/domain_model.md`](../../../docs/guidelines/domain_model.md) — introduce value objects for concepts with rules; domain rules in domain code

The clean-slate policy gets a dedicated ADR section, an architecture subsection, and three "Why" bullets in the changelog. In code it is one branch in `Plan` and one literal `"first apply"` reason string. There is no `Preview.FirstApply` boolean, no `ManagedState` flag, no helper to ask "is this a clean-slate preview". The doctor tests already had to seed an empty `ManagedState` just to opt _out_ of clean-slate, demonstrating the absence of the concept already leaked into a test fixture.

```go
// internal/sync/sync.go
if state == nil {
    changes = append(changes, FileChange{Path: file.Path, Kind: ChangeCreate, Reason: "first apply"})
    continue
}
```

Pick one:

- [x] Surface the policy on `Preview`: add `Preview.FirstApply bool`, set by `Plan` when `state == nil`. TUI and doctor can render the policy explicitly (e.g. a banner "First apply — every file will be created"); the magic reason string becomes a constant `ReasonFirstApply` that callers can switch on.
- [ ] Add only the `ReasonFirstApply` constant for now (cheapest fix). Pick if the Preview-level surface is genuinely out of scope for 0017.

## 17. `FileResolution.Path` semantics are implicit (slash-key vs absolute) and unenforced

> [!WARNING]
>
> - [`docs/guidelines/domain_model.md`](../../../docs/guidelines/domain_model.md) — stable identifiers; make domain rules easier to see
> - [`docs/guidelines/clean_code.md`](../../../docs/guidelines/clean_code.md) — naming: meaningful distinctions

`FileResolution.Path` is documented as "a target path" but the convention is "slash-separated key matching `FileChange.Path`, not an absolute path and not OS-native". The convention is implicit — the existing test uses `".codex/stray.txt"` and the convention works only because the lookup is keyed by `change.Path`. The same field name `Path` already means several things in this codebase (absolute project path, slash-key, OS-native), so a reader cannot infer the right shape from the type.

Pick one:

- [ ] Tighten the field doc: explicitly state "slash-separated key matching `FileChange.Path`; absolute or OS-separated values will not match any change". Cheapest fix.
- [ ] Rename to `ChangePath` (mirroring `FileChange.Path`) and update the doc. Self-documenting; small breaking change since the public type is new in this task and only the TUI constructs it (empty slice).
- [x] Add a validator inside `Apply` that rejects entries whose `Path` is absolute (`filepath.IsAbs`) or contains an OS path separator different from `/`, returning a typed error. Loud failure for malformed input; combine with issue #2 if both are taken.
