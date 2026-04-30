# 11. Technical Risks

The project is young and several important areas are intentionally thin. The
table below records the architectural risks worth tracking, with impact,
likelihood, and the mitigation or follow-up plan.

| Risk                                | Impact | Likelihood | Mitigation / Follow-up                                                                                                         |
| ----------------------------------- | ------ | ---------- | ------------------------------------------------------------------------------------------------------------------------------ |
| No persisted active profile         | Low    | High       | Resolve a profile per command for now. Revisit if user feedback shows the per-command selection is friction in real workflows. |
| Generic asset semantics for `mcp`, `rule`, `hook` | Medium | High | These types currently render through generic projections. Promote to specialised workflows + validations once usage patterns stabilise. |
| TUI is the only surface             | Medium | Medium     | ADR 0005 records the decision. Re-evaluate if scripted automation or CI usage becomes a real requirement.                      |
