# 10. Quality Requirements

The four quality goals from [section 1](./01-introduction-and-goals.md) are
expanded here as measurable scenarios. Each scenario follows the standard
*context / stimulus / response / response measure* form, plus a usability
scenario that the goals do not directly cover.

| Goal             | Context                                                                | Stimulus                                                                | Response                                                                | Response measure                                                                                       |
| ---------------- | ---------------------------------------------------------------------- | ----------------------------------------------------------------------- | ----------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------ |
| Safety           | A managed file in the target repository was edited locally after apply. | The user runs `Project → Plan` for that project.                        | The preview reports the file as drift, distinct from create/update.     | The drift entry appears in the preview produced by the same command, before any write occurs.         |
| Traceability     | A user has just applied a project successfully.                         | The user inspects `<repo>/.agentfiles/state.json`.                      | The state file lists every managed file with its hash and source asset. | One entry per managed file is present immediately after `Apply` returns; no out-of-band update needed. |
| Maintainability  | A new asset type or projection rule is being added.                     | The contributor implements the change.                                  | The change fits inside the existing domain packages without merging registry, profile, asset, project, render, or sync. | No new cross-package import is introduced between the previously separable packages.                  |
| Git-Friendly Storage | A profile directory exists on disk.                                | The user commits the profile to git and reviews the diff.               | All profile state is plain text (JSON manifests + asset files).         | `git diff` produces human-readable hunks; no binary blobs or lock files are required.                  |
| Usability        | A user wants to inspect pending changes for a project quickly.          | The user reaches `Project → Plan` and selects a profile and project.    | The TUI shows a concise preview of creates, updates, drift, and delete candidates. | The preview is rendered on the same screen step, without an intermediate processing screen.            |
