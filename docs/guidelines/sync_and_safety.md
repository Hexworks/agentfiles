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

Recognized unmanaged files should be surfaced, not removed automatically.

```text
Do:
- report delete candidates in the preview
- require explicit confirmation or a delete flag before removal
```

```text
Don't:
- delete recognized files automatically during apply
- expand deletion to unrelated repository files
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

