---
id: 0049
type: bug
status: pending
topics: tui, sync_and_safety, asset_authoring
---

# Register/Ignore offered for folders whose asset id already exists

On the **Plan Project** screen a folder that maps to an asset the profile
already manages is still offered the **Register** and **Ignore** actions.
Pressing **Register** fails with `asset already exists: <id>`. The plan view
and the profile's asset set are out of sync: the view treats the folder as
unmanaged/unknown, while the profile already owns an asset under that id.

## Acceptance Criteria

- [ ] TBD (filled during grilling)

## Out of scope

- TBD (filled during grilling)

## Verification

- Baseline: `make build && make test && make lint` pass.
- TBD (filled during grilling)
