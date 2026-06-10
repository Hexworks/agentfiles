# Sync And Safety Guidelines

Synchronization is the highest-risk part of `agentfiles` because it writes into
real repositories. The design should therefore prefer predictability and
explanation over silent convenience.

The core safety model is simple: plan first, show the user what will happen,
then apply only the desired changes inside known managed surfaces.

## Always Plan Before Apply

Write operations should be based on an explicit preview.

```text
Do:
- compute desired outputs first
- classify creates, updates, drift, and delete candidates
- show the preview before mutating the repository
```

```text
Don't:
- write files directly during selection or rendering
- hide destructive changes behind one-step commands
```

## Distinguish Update From Drift

The difference matters. An update means the desired output changed. Drift means
a previously managed file was edited locally after apply.

```text
Do:
- compare current file hashes with desired hashes
- compare current file hashes with managed-state hashes
```

```text
Don't:
- collapse drift into a generic update
- overwrite locally changed managed files without reporting why
```

## Require Explicit Deletion Opt-In

Files inside managed surfaces split into two classifications. Recognized
managed files (recorded in the previous `ManagedState` and missing from the
new desired output) auto-delete on apply because the user already saw them
in the preview. Unknown files (in a managed surface but never tracked)
require a per-file `ResolveDelete` resolution; the default is keep.

```text
Do:
- emit ChangeDelete for state-recorded paths missing from desired
- emit ChangeUnknown for surface files not in state and not in desired
- require an explicit ResolveDelete entry before removing an unknown file
```

```text
Don't:
- delete unknown files automatically during apply
- expand deletion to unrelated repository files
- emit ChangeUnknown on the first apply (no state yet to compare against)
```

## Keep Managed State Accurate

Managed state is the basis for drift detection and safe comparison.

```text
Do:
- rewrite .agentfiles/state.json after successful apply
- store hashes of the newly written managed files
```

```text
Don't:
- leave stale state after apply
- store unrelated project metadata in managed state
```

