# 11. Technical Risks

The project is young and several important areas are intentionally thin.

## No Persisted Active Profile

The README mentions one active profile at a time, but the current implementation
resolves a profile per command and does not persist a global active-profile
selection.

## Generic Asset Semantics

`mcp`, `rule`, and `hook` exist as asset types, but they are currently defined
through generic projections rather than specialized workflows and validations.

## Documentation Drift Risk

The README mentions prompts as a concept, but the current code does not expose a
`prompt` asset type. That mismatch needs to be managed carefully as the domain
evolves.

## TUI Is The Only Surface

Every command is collected through a TUI form. There is no non-interactive
flag-based path, which means scripted automation or CI usage would need a new
entry point. The risk is intentional for now: ADR 0005 records the decision
and the followup it implies.

