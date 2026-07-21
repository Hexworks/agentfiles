# License Under AGPL-3.0 With Copyright-Assignment CLA

## Status

accepted

## Context

Until this decision `agentfiles` shipped without an explicit open source
license, which under default copyright law forbids any downstream use
beyond viewing the source. That was tolerable while the project had a
single author and no external contributors, but it foreclosed three
things the project needs to grow:

1. Adoption by third-party developers who want to embed or fork the
   tool, since redistribution rights are unclear.
2. A commercial-license offering for organizations that cannot comply
   with copyleft, since dual-licensing requires a single owner
   authorized to relicense.
3. A predictable rule for accepting community pull requests, since each
   PR would otherwise transfer ambiguous rights to Hexworks.

The maintainer intends to keep the code publicly usable while retaining
the option to sell a separately-licensed commercial variant. That shape
maps onto the standard "GPL-style copyleft with commercial exception"
model already used by MySQL, Qt, GitLab EE, and others. Two knobs need
to be set:

- **Which copyleft license.** Plain GPL-3.0 leaves the SaaS loophole
  open: a downstream vendor can host a modified version as a service
  without distributing source. MIT/Apache-2.0 lose all copyleft
  reciprocity and would let a forked hosted service compete without any
  contribution back.
- **How contributions vest.** A permissive Developer Certificate of
  Origin or Apache-style Individual Contributor License Agreement keeps
  copyright with each contributor and only grants a license to the
  project. A dual-license offering requires the maintainer to relicense
  the entire codebase, which under a license-grant CLA needs every
  contributor's individual consent.

## Decision

- **License** — release the codebase under the **GNU Affero General
  Public License v3.0**, with the unmodified `LICENSE` text at the repo
  root so GitHub's license detector picks it up.
- **Contributor License Agreement** — require every contributor to sign
  a **copyright-assignment CLA**, drafted in `CLA.md` at the repo root,
  that transfers copyright in the contribution to Hexworks and grants a
  broad license-back so contributors may continue to use their own work
  under AGPL-compatible terms.
- **README disclosure** — surface both facts prominently in `README.md`
  (`## License` section), including the AGPL §13 network-source
  obligation in one sentence and the `info@hexworks.org` contact for the
  commercial license.
- **Commercial offering location** — the commercial license itself
  stays out of the repository; only the fact that one exists and the
  contact address are documented in `README.md`.

## Consequences

- **Positive** — anyone can use, modify, and redistribute the code so
  long as they respect AGPL §13; Hexworks retains the sole authority to
  offer a commercial-license variant without renegotiating with past
  contributors; new contributions carry a clear provenance chain.
- **Negative** — the copyright-assignment CLA is a higher friction
  onboarding step than a DCO, and some contributors object to
  assigning copyright on principle; contributions may slow accordingly.
- **Operational** — until PR volume warrants automation, CLA acceptance
  is handled via the PR template and a manual maintainer check; a bot
  like CLAassistant can be introduced later without changing the CLA
  text.
- **License-drift risk** — anyone updating the AGPL preamble or the CLA
  language must retain full text integrity (Free Software Foundation
  forbids modification of the AGPL body); guardrails live in a
  reviewer-facing note only.
- **Relation to other ADRs** — this decision does not affect the
  managed-surface fence (ADR 0002), the profile source-of-truth model
  (ADR 0001), or any code-level architecture. It is an ownership and
  distribution decision, orthogonal to the rest of the ADR set.
