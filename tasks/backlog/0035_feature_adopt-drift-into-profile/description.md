---
id: 0035
type: feature
status: Pending
topics: sync_and_safety, domain_model, tui
depends_on: 0033
---

# Adopt: promote a local drift edit into the profile

A third drift resolution alongside `DriftKeep` (leave alone) and
`DriftOverwrite` (replace local with rendered): **Adopt** takes the user's
**local edit** and makes it the *canonical* representation by writing it back
into the **profile** (source of truth), then propagating it to every sibling
agent so no drift or update remains on the next plan.

## Use case

1. User edits a managed file locally in a project to try something out.
2. User decides the change is good and wants it to become canonical.
3. User picks **Adopt** on that drift row.
4. The local content is copied into the profile asset it came from.
5. Because the profile is now the source of truth, the change re-renders to
   **all** compatible agents. Example: editing a `claude` skill in a project
   that has both `claude` and `codex` agents — after adopting the `claude`
   edit, the profile asset updates and the change syncs to `codex` too.
6. On the next plan the path (and its siblings) show **neither drift nor
   update** — everything has converged.

## Key constraint — reverses the source-of-truth invariant

Normal flow is profile → repo; render never reads the repo as input
(CLAUDE.md invariant #6, ADR 0001). Adopt is the **one deliberate reverse
flow**: repo → profile. This needs explicit design, not an ad-hoc write:

- Which profile asset + which file inside it does a given rendered path map
  back to? (reverse of the render projection.)
- Adopt must render+apply to **all** sibling agents in the **same** operation,
  or the user sees `update` rows on siblings until a second apply — which
  contradicts requirement #6 above.
- Plan/preview should show what Adopt will touch (the profile asset + every
  sibling projection) before it writes, consistent with invariant #1
  (plan before apply).

## Notes

- Builds on 0033, which fixes `DriftKeep` to mean "leave alone, preserve
  baseline" and removes the accidental adopt-on-keep behavior. Adopt is the
  *intentional* successor capability, with its own enum value and UI button.
- Needs a new `DriftDecision` value (e.g. `DriftAdopt`) and a TUI affordance
  on the drift row (today the row toggles only Keep ↔ Overwrite).
- Likely warrants an ADR: it is the single sanctioned exception to the
  profile-is-authoritative rule.

## Acceptance Criteria

- [ ] Placeholder — fill via grilling before implementation.

## Out of scope

- Placeholder — fill via grilling before implementation.

## Verification

```
make build && make test && make lint
```
