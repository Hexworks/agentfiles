# 0039 changes

Adopted the GNU Affero General Public License v3.0 as the project's open
source license and introduced a copyright-assignment Contributor License
Agreement. The pairing lets Hexworks distribute the same codebase under
both AGPL-3.0 and a separately-negotiated commercial license, keeping the
door open for a dual-license business model without ambiguity about who
owns downstream contributions.

## Decisions

- **AGPL-3.0, not MIT/Apache/GPL-3.0.** — **Why:** the network-copyleft
  clause (AGPL §13) closes the SaaS loophole a plain GPL leaves open, so
  a hosted fork must also make source available. MIT/Apache would allow
  a proprietary re-host without reciprocity, which is incompatible with
  the "profile-as-source-of-truth" tool being adoptable inside internal
  SaaS pipelines without eroding the commons.
- **Copyright assignment, not a permissive DCO / CLA-style license grant
  (Apache ICLA).** — **Why:** a dual-license strategy requires a single
  copyright holder authorized to relicense; a license-grant CLA leaves
  copyright with each contributor and would require every contributor's
  consent to re-issue the commercial license. Assigning to Hexworks
  concentrates that authority in one entity while still granting the
  contributor a broad license-back through the AGPL distribution.
- **Physical file placement in the repo root, not under `docs/`.** —
  **Why:** GitHub's license auto-detection scans `LICENSE` at the root
  and surfaces the SPDX identifier on the repo landing page. Putting
  the CLA next to it (`CLA.md`) mirrors the discoverability pattern
  contributors already expect.

Considered but rejected:

- Adopting AGPL-3.0 **without** a CLA (keeps copyright distributed).
- Adopting a permissive license (MIT/Apache-2.0) and monetizing solely
  through hosted services.
- Deferring the licensing decision to a later release — rejected because
  every commit that lands before the license is chosen contributes
  ambiguously-owned code.

## Assumptions

- Contributor volume stays low enough that per-PR CLA acceptance can be
  handled by the PR template + a manual maintainer check, rather than
  requiring a bot like CLAassistant.
- The commercial license offering itself remains out of the repo. Only
  the fact that one exists and the contact address (`info@hexworks.org`)
  live in `README.md`.

## Other Notes

- New files at repo root: `LICENSE` (unmodified AGPL-3.0 v3 text) and
  `CLA.md` (copyright-assignment agreement drafted for Hexworks).
- `README.md` gained a `## License` section pointing at both files and
  describing the "network-source" obligation in one sentence for readers
  who have not encountered AGPL before.
- No code paths change. Contributor onboarding docs may add a "you agree
  to the CLA on PR submission" note in a later pass.

## Add `LICENSE` at repo root

The full unmodified AGPL-3.0 v3 text was placed at the repo root so
GitHub's license-detector picks it up and displays the SPDX tag.

## Add `CLA.md` at repo root

Introduced the copyright-assignment Contributor License Agreement. Key
mechanic:

```markdown
## 2. Assignment of copyright

You hereby irrevocably assign, transfer, and convey to Hexworks all right,
title, and interest worldwide in and to the copyright in Your Contributions,
including all rights to reproduce, prepare derivative works of, publicly
display, publicly perform, sublicense, distribute, and relicense them under
any terms — including both open source and proprietary or commercial
licenses.
```

The assignment is paired with a licence-back so the contributor may
continue to use their own contribution under any AGPL-compatible terms.

## Extend `README.md` with a License section

Added a top-level `## License` block naming AGPL-3.0, an inline sentence
on the network-source clause for readers new to AGPL, a `### Commercial
license` sub-section pointing at `info@hexworks.org`, and a
`### Contributing` sub-section pointing at `CLA.md`. The change is
additive; no existing sections were removed or renamed.
