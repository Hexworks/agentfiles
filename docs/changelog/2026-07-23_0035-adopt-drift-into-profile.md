# 0035 changes

Introduces **Adopt**, the single sanctioned repo → profile flow. A drift
row can now be resolved as `DriftAdopt` (promote the local edit into
the owning profile asset) and an unknown row nested inside a known
asset's projection dir can be resolved as `UnknownAdopt`. `sync.Apply`
classifies these rows and returns an `AdoptRequests` slice; the
profile-side write and its git commit live in `app.Service.Apply`, so
`sync` itself stays repo-only. `state.json` bumps to schema v3 whose
`managed_files` entries carry `{hash, asset_id, source_rel}`; the
loader stays backwards-compatible with v2 (bare hash) entries but
disables Adopt for them until the next re-apply repopulates the
provenance keys. The TUI drift toggle cycles Keep → Overwrite →
Adopt → Keep; the unknown toggle grows an Adopt step only when the
change carries an `OwningAssetID`.

## Decisions

- Adopt is per-file, single-agent — **Why:** sibling agent projections
  (e.g. the `.codex` mirror of a `.claude` skill) surface as ordinary
  `update` rows on the next plan, keeping the flow narrow and the
  invariant break well-scoped.
- Reverse-mapping keys are persisted per-entry in `state.json`
  rather than re-derived at Apply time — **Why:** the source-of-truth
  information (`AssetID`, `SourceRel`) is only reliable at Apply time
  when the render pipeline just produced it; a later Apply may run
  after the profile has changed shape.
- The v2 → v3 schema break is presence-based, not versioned via a new
  `schema_version` field — **Why:** the existing
  `GeneratorVersion` is the only version signal today; a parallel
  field would double-source it. Loader detects a bare-string entry
  and treats it as v2.
- Adopt writes only the primary agent's projection; siblings catch up
  in the next plan/apply cycle — **Why:** avoids fan-out complexity
  and makes the reverse write reviewable one commit at a time.
- Adopt mnemonic is `t` (`adopT`) — **Why:** `a` collides with the
  screen-level `[Apply]` button that already binds `a`.
- Adopt gets its own commit trigger + subject template on the profile
  repo (`chore(agentfiles): adopt N file(s) into profile`) — **Why:**
  the target-repo sync commit already covers state.json + managed
  files, so the profile-side commit needs its own scoped pathspec
  and message to make the reverse-flow visible in `git log`.

Considered but not done:

- Auto-propagating an Adopt to sibling agent projections in the same
  apply — deferred: users can Apply once more when they want the
  siblings updated; a broadcast Adopt would multiply the surface
  each single reverse-write can affect.
- Adopting a fully-unknown file that is not inside a known asset dir —
  remains the Register-as-Asset flow.
- Backfilling `asset_id`/`source_rel` on legacy v2 state entries —
  disabled Adopt is a small enough friction that the next clean apply
  fixes it for free.

## Assumptions

- `RenderedFile.SourceRel` matches the shape used by
  `asset.ResolveRelative` (forward-slash asset-relative) — **Why:** it
  matches the value the render pipeline reads from `asset.RelativeFiles`
  and the projection sources on `Manifest.Projections`.
- Two Committed outcomes are always OK to merge into one info toast —
  **Why:** the sync commit and the adopt commit are independent by
  intent (target repo vs. profile repo); users read them as one
  bookkeeping event.

## Other Notes

- `docs/adr/0020-adopt-as-sanctioned-reverse-flow.md` — new ADR.
- `docs/adr/0015-drift-keep-preserves-baseline.md` — Adopt moved from
  "future" to "shipped"; the forward-reference to task 0035 is now a
  link to ADR 0020.
- `docs/architecture/05-building-block-view.md` — sync + app + render
  blocks describe the new adopt request return and the profile-write
  responsibility.
- `docs/architecture/06-runtime-view.md` — adds an Adopt sequence and
  a `Drift --> Clean` state edge.
- `docs/architecture/08-concepts.md` — drift decision section is now
  three-way; a Source-Of-Truth Exception subsection explains the
  invariant carve-out.
- `docs/architecture/09-architecture-decisions.md`, `docs/adr/README.md`
  — index updated.
- `docs/glossary.md` — new **Adopt** entry, **Drift** entry now
  mentions all three decisions, **Resolution** entry updated.
- `CLAUDE.md` — invariant #6 carries the Adopt carve-out.
- Manual smoke pending: run through `./bin/af` and confirm the drift
  toggle now cycles Keep → Overwrite → Adopt → Keep, and that a
  git-enabled Adopt records the profile-repo commit.

## `render.RenderedFile.SourceRel`

Every renderer branch (`addSkillOutputs`, agents_doc, settings, generic
file projection, generic dir projection) now carries the
asset-relative source path on the emitted file, in the same
forward-slash form the rest of the domain uses.

```go
// before
type RenderedFile struct {
    Path string
    Body []byte
    Mode os.FileMode
    AssetID string
}
```

```go
// after — SourceRel is the reverse-mapping key Adopt persists
type RenderedFile struct {
    Path      string
    Body      []byte
    Mode      os.FileMode
    AssetID   string
    SourceRel string
}
```

## `sync` — schema v3 + Adopt classification

`ManagedState.ManagedFiles` is now `map[string]ManagedFileEntry`
(`{hash, asset_id, source_rel}`) with a custom `UnmarshalJSON` that
accepts either the v3 object shape or a legacy v2 hash string. Apply
gains a `DriftAdopt` / `UnknownAdopt` branch that appends to a new
`ApplyResult.AdoptRequests` slice and leaves the repo file untouched.

```go
// before — schema v2 map, no reverse-mapping keys
type ManagedState struct {
    ManagedFiles map[string]string `json:"managed_files"`
    // ...
}
```

```go
// after — schema v3 value type with typed reverse-mapping fields
type ManagedFileEntry struct {
    Hash      string `json:"hash"`
    AssetID   string `json:"asset_id,omitempty"`
    SourceRel string `json:"source_rel,omitempty"`
}

type ManagedState struct {
    ManagedFiles map[string]ManagedFileEntry `json:"managed_files"`
    // ...
}
```

## `app.Service.Apply` — two commit outcomes, adopt writes

`Service.Apply` grows a third return value — the profile-repo commit
outcome from `executeAdoptRequests`. When git integration is enabled
and any AdoptRequest lands, the service reads the local body, writes
it into `<asset.Dir>/<source_rel>` via a new `asset.WriteFile`
helper, and records a scoped commit on the profile repo.

```go
// before
func (s *Service) Apply(profileRef, projectID string, r appapi.Resolutions) (
    *appapi.Preview, appapi.CommitOutcome, errs.DomainError,
) { ... }
```

```go
// after — third outcome carries the profile-repo Adopt commit result
func (s *Service) Apply(profileRef, projectID string, r appapi.Resolutions) (
    *appapi.Preview,
    appapi.CommitOutcome,       // target repo (sync)
    appapi.CommitOutcome,       // profile repo (adopt), Skipped{SkipDisabled} when unused
    errs.DomainError,
) { ... }
```

## TUI — three-way drift cycle, gated unknown Adopt

The drift toggle now cycles Keep → Overwrite → Adopt → Keep. The
unknown toggle grows an Adopt step only when the change carries an
`OwningAssetID` (populated at Plan time for unknowns nested inside a
known asset projection dir); otherwise the row stays a 2-way
Keep ↔ Delete cycle. Adopt mnemonic is `t` (avoiding the `a`
collision with `[Apply]`).

```go
// before — 2-way drift toggle
func (s *planProjectScreen) driftToggleBtn(path string) *mnemonic.Button {
    if s.driftResolutions[path] == appapi.DriftOverwrite {
        return mnemonic.New("Keep", 'p', ...)
    }
    return mnemonic.New("Overwrite", 'w', ...)
}
```

```go
// after — 3-way cycle, next-state labels
func (s *planProjectScreen) driftToggleBtn(path string) *mnemonic.Button {
    switch s.driftResolutions[path] {
    case appapi.DriftOverwrite:
        return mnemonic.New("Adopt", 't', ...)
    case appapi.DriftAdopt:
        return mnemonic.New("Keep", 'p', ...)
    }
    return mnemonic.New("Overwrite", 'w', ...)
}
```
