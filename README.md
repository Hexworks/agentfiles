# Agentfiles

Agentfiles is a CLI tool that enables you to centrally manage your AI coding assistant configurations across multiple projects. By setting up a personal profile, yor agent configurations are propagated from a single source, minimzing agent management overhead and drift over time.

- If you are new to Agentfiles, please read on to learn the basics.
- Go to [Prerequisites and installation](#prerequisites-and-installation) for first-time setup.
- Go to [Quickstart](#quickstart-your-first-end-to-end-run) for hands-on usage instructions.

---

## What is Agentfiles?

Modern AI coding assistants like Claude Code, Codex, Cursor or opencode all require individual configuration files added to each repository you work on:

- Claude Code reads files under `.claude/` and `.mcp.json`.
- Codex reads `AGENTS.md`, `.codex/config.toml`, and `.codex/skills/`.
- Cursor reads `.cursor/commands/` and `.cursor/config.json`.
- opencode reads `.opencode/`.

Across multiple repos, skill, setting, hook, and prompt assets get duplicated and eventually drift out of sync. Updates and deletions become complicated because copy-pasted or derivative assets are hard to distinguish from hand-written ones.

`agentfiles` solves this by making your personal **profile folder** the single source of truth, as well as planning, previewing, and writing **base assets** that you can selectively propagate to each of your project repositories.

You edit your skills, prompts, and settings (**base assets**) once, in the profile folder. Then you
`apply` them to as many project repos as you like, resulting in **propagated assets** placed in those projects.

---

## Workflow overview

```
┌─────────────────────────────────────────────────┐
│ ~/.config/.agentfiles/.profiles.json (registry) │   Global index. Lists your profiles.
│ ~/.config/.agentfiles/.projects.json (registry) │   Global index. Lists your projects.
└─────────────────────────────────────────────────┘
                     │
                     │ points to
                     ▼
┌─────────────────────────────────────────────────┐
│  ~/profiles/personal/   (one profile)           │   The source of truth.
│   ├── profile.json                              │   You edit your base assets here.
│   ├── assets/                                   │
│   │   ├── skill/review/SKILL.md                 │
│   │   ├── settings/…                            │
│   │   └── …                                     │
│   └── projects/                                 │
│       ├── app.json       (project spec)         │
│       └── website.json                          │
└─────────────────────────────────────────────────┘
                     │
                     │ render + apply
                     ▼
┌─────────────────────────────────────────────────┐
│  ~/src/app/   (a target repository)             │   Propagated assets land here.
│   ├── AGENTS.md                                 │
│   ├── .claude/skills/review/SKILL.md            │
│   ├── .codex/skills/review/SKILL.md             │
│   ├── .cursor/commands/review.md                │
│   └── .agentfiles/state.json                    │   Bookkeeping for drift detection.
└─────────────────────────────────────────────────┘
```

One profile can feed any number of target projects. Each project can declare which **assets** (skills, settings, prompts, etc.) it needs and for which **agents** (Claude Code, Codex, Cursor, opencode) it wants to render them.

---

## Key terms

Before you run any commands, it helps to know the vocabulary. These words mean something specific in `agentfiles` and are used throughout the TUI and docs.

| Term                 | What it is                                                                                                                                                                          |
| -------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Registry**         | A single file at `~/.config/.agentfiles/profiles.json` that lists every profile you have. Used purely for discovery.                                                                              |
| **Profile**          | A folder containing your reusable content. Holds `profile.json`, an `assets/` tree, and a `projects/` tree. This is the **source of truth**.                                        |
| **Asset**            | One reusable unit of content — a skill, a settings file, a hook, etc. Lives inside a profile and has its own `asset.json` manifest.                                                 |
| **Project**          | A target repository together with a list of enabled agents and selected assets. Defined by a JSON file inside the profile's `projects/` folder.                                     |
| **Enabled agent**    | An AI tool the project renders for. One of: `codex`, `claude-code`, `cursor`, `opencode`.                                                                                           |
| **Render plan**      | The set of files `agentfiles` wants to write into a project, computed from its selected assets.                                                                                     |
| **Preview**          | A render plan compared against what's already in the project repo. Shows creates, updates, drift, and delete candidates.                                                            |
| **Managed paths** | The only paths `agentfiles` is allowed to access inside a repo: `AGENTS.md`, `.claude/`, `.cursor/`, `.codex/`, `.opencode/`, `.mcp.json`. Anything outside this list is not read or written. |
| **Managed state**    | A bookkeeping file at `<repo>/.agentfiles/state.json` listing which files were written last time and what their hashes were. Used to detect drift.                                  |
| **Drift**            | A managed asset file that was edited locally after the last `apply` command, so its hash no longer matches what `agentfiles` wrote. Shown in previews so you don't lose that edit by accident.           |
| **Delete candidate** | A file inside a managed surface that `agentfiles` recognizes but no longer wants. Never deleted automatically — `Project → Apply` asks you to confirm removal when candidates exist. See [Safe deletion](#safe-deletion) for details of this workflow.|

A full glossary lives in [`docs/glossary.md`](./docs/glossary.md).

---

## Prerequisites and installation 

### Requirements

- **Go 1.26+** to build from source.
- A POSIX shell (Linux or macOS). The project builds for both.
- Optional: [`mise`](https://mise.jdx.dev/) to pin the Go toolchain (see
  `mise.toml`).

There is no database, no daemon, and no network service. Everything is plain JSON files on your disk.

---

### Install / build

Clone the repo and build with `make`:

```bash
git clone https://github.com/addamsson/agentfiles.git
cd agentfiles
make build
```

This produces a binary at `./bin/af`. Put it on your `PATH` (or alias it to `agentfiles` — that's the name the CLI uses internally and throughout this README).

```bash
# Example: put a symlink where your shell will find it
ln -s "$PWD/bin/af" ~/.local/bin/af
```

Other common `make` targets:

| Command             | What it does                                    |
| ------------------- | ----------------------------------------------- |
| `make build`        | Compile the binary into `./bin/af`.             |
| `make test`         | Run the Go test suite (`go test ./...`).        |
| `make lint`         | Run `go vet ./...`.                             |
| `make fmt`          | Format all Go files with `gofmt -w`.            |
| `make run ARGS="…"` | Build, then run the binary with the given args. |
| `make clean`        | Remove `./bin/` and clear Go's build cache.     |

---

## Run Agentfiles

`agentfiles` is fully TUI-driven. Run the `af` command and the top-level menu opens. From there you pick a category, then an action, and the matching form walks you through it:

[![demo](docs/demo.gif)](https://asciinema.org/a/61nrRvxqxMsCogpL)

Press `Esc` at any prompt to back out one level. `Ctrl+C` does the same.

### 1. Create a profile

> [!NOTE]
> You can create a git repository inside the profile folder. This way you can share a profile with other people working on the same project. You can also register an existing profile (for example if you cloned someone else's profile) by using `register` to add it to your `agentfiles` instance.

Pick `Profile → Create`. The form prompts for a display name and a path. After it completes, `~/{your-path}/{your-profile}/` exists with `profile.json` and empty `assets/` and `projects/` subfolders, and the profile is registered in `~/.config/.agentfiles/profiles.json`.

### 2. Add a reusable skill

A **skill** is one of the asset types. It is a markdown file (`SKILL.md`) plus optional supporting files that can be rendered for every agent.

(EDITORIAL NOTE: Instead of asking the first-time user to write their own SKILL.md, we could create an example project that the user can register to get to know some skill alternatives, profile layout, etc. Maybe as an optional on-boarding path. -> checkout example)

From the main menu, select `Profiles → Edit → Create Asset`. The Create Asset form asks for the id, name, and description of the new asset, and presents the supported asset types. The new asset directory (e.g. `~/profiles/personal/assets/skill/review/`) is printed when the form completes. You can now open `SKILL.md` in this directory and write your actual content.

### 3. Register a target project

From the main menu, select `Profiles → Edit → Create Project`. Tell `agentfiles` which repository your profile should propagate with assets and which AI agents it should render for.

The project manifest lands in the profile's `projects/` folder; nothing is written into the target repository yet.

### 4. Select assets to propagate

Once your project is created, it appears under `Profiles → Projects`. Select `Select Assets`. The form allows you to define which assets you want to propagate to the selected project. It should list the skill added in step 3. Select it. It shows up in the `Selected Assets` list.

### 5. Preview changes

Always run `Plan` before `Apply`. It is read-only and shows you exactly what files would be created, updated, or flagged.

Pick `Project → Plan`. The form picks the profile, then the project, then prints the preview:

```
Project: /home/you/src/app
- [create] .claude/skills/review/SKILL.md: file missing
- [create] .codex/skills/review/SKILL.md: file missing
```

### 6. Apply the changes

Pick `Project → Apply`. The flow runs `Plan`, shows the same summary, and then asks you to confirm before writing. If the project contains assets removed from active management (delete candidates), the form also asks whether to remove them.

After apply succeeds, `~/src/app/.agentfiles/state.json` records what was written. Next time you run `Plan`, `agentfiles` will compare against that state.

### 6. Iterate

Edit the skill in your profile (`~/profiles/personal/assets/skill/review/SKILL.md`), run `af` again and pick `Project → Apply`, and the changes propagate. The profile is the source of truth; the project's `.claude/` and `.codex/` are propagated assets.

---

## Menu reference

`af` takes no positional arguments. Running it opens the main menu; everything else is a submenu pick followed by a form. The only flag the binary accepts is `--registry <path>`, which overrides the default `~/.config/.agentfiles/profiles.json` location (useful for tests or isolated environments).

### Profile

Manage profiles and the global registry.

| Menu entry           | Flow                                                             |
| -------------------- | ---------------------------------------------------------------- |
| `Profile → Create`   | Form: display name + path. Scaffolds and registers the profile.  |
| `Profile → Register` | Form: path. Adopts an existing profile folder into the registry. |
| `Profile → List`     | Read-only list of every registered profile.                      |

### Asset

Scaffold reusable content inside a profile.

| Menu entry     | Flow                                                                                                |
| -------------- | --------------------------------------------------------------------------------------------------- |
| `Asset → Init` | Pick the profile from a list, pick the type from the supported set, then enter id/name/description. |

### Project

Manage the target-repository manifests and run the render/apply workflow.

| Menu entry        | Flow                                                                                                                   |
| ----------------- | ---------------------------------------------------------------------------------------------------------------------- |
| `Project → Add`   | Pick the profile, enter name + path, multi-select agents from the supported set, multi-select assets from the profile. |
| `Project → Plan`  | Pick the profile, then the project; previews changes (read-only).                                                      |
| `Project → Apply` | Pick the profile, then the project; previews changes; asks whether to delete candidates and confirm before writing.    |

---

## Asset types

Every asset has a `type` that controls how its files are rendered into each
agent's expected location.

| Type         | What it's for                                | Rendered as                                                                                                                                                                                                 |
| ------------ | -------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `skill`      | A reusable skill (SKILL.md + support files). | `.claude/skills/<id>/…`, `.codex/skills/<id>/…`, `.opencode/skills/<id>/…`, and `.cursor/commands/<id>.md`.                                                                                                 |
| `agents_doc` | The top-level `AGENTS.md` for Codex.         | `AGENTS.md` (Codex only).                                                                                                                                                                                   |
| `settings`   | Agent-specific settings files.               | `.claude/settings.local.json`, `.codex/config.toml`, `.cursor/config.json`, `.opencode/config.json` — whichever of `claude-code.json`, `codex.toml`, `cursor.json`, `opencode.json` exist inside the asset. |
| `mcp`        | MCP server configuration.                    | Per `projections` in the manifest.                                                                                                                                                                          |
| `rule`       | Agent rule files.                            | Per `projections`.                                                                                                                                                                                          |
| `hook`       | Shell hooks that the agent harness runs.     | Per `projections`.                                                                                                                                                                                          |

### Controlling which agents get an asset

Each `asset.json` can narrow which agents are allowed to use it:

```json
{
    "id": "review",
    "name": "review",
    "type": "skill",
    "compatible_agents": ["claude-code", "codex"],
    "exclusive_group": "review-style"
}
```

- `compatible_agents` — if set, only these agents render the asset. Empty
  means "all agents".
- `exclusive_group` — only one selected asset from this group may render for a
  given project. A second one causes a render error, which protects you from
  silently shipping conflicting content.

### Generic projections

For `mcp`, `rule`, `hook`, and any custom-shaped asset, you must describe the source-to-target mapping yourself in the manifest:

```json
{
    "projections": [
        {
            "agent": "claude-code",
            "source": "hook.sh",
            "target": ".claude/hooks/hook.sh"
        },
        {
            "agent": "codex",
            "source": "hook.sh",
            "target": ".codex/hooks/hook.sh"
        }
    ]
}
```

Targets **must** be located inside a managed surface (see next section). Anything outside is refused at render time.

---

## Managed surfaces and safety

`agentfiles` will only ever read from or write to these paths inside a target repo:

- `AGENTS.md`
- `.claude/`
- `.cursor/`
- `.codex/`
- `.opencode/`
- `.mcp.json`
- `.agentfiles/state.json` (its own bookkeeping)

Everything else in the repository is untouched. This is a deliberate safety fence: you can run `apply` without worrying that it will walk off into your source code.

### Drift detection

Each apply writes hashes of every managed file into `<repo>/.agentfiles/state.json`. The next `plan` compares the current on-disk file with that hash.

- **File unchanged** → skipped.
- **File changed by `agentfiles`** (i.e. the profile has new content) →
  reported as `update`.
- **File changed locally** (hash differs from state, state exists) → reported as `drift`. You will be warned that your edits will be overwritten if you run `apply`.

### Safe deletion

If a file was previously managed but is no longer part of the render plan (for example you removed an asset from the project's selection) it shows up as a **delete candidate**. `Project → Apply` does _not_ remove these automatically. When unmanaged files exist in your project, the flow pops an extra confirm prompt ("Delete recognized unmanaged files?") before the final apply confirmation; answer yes to remove them, or clean them up by hand.

---

## File layout on disk (TODO: review against current version)

### Profile folder

```
~/{profiles-path}/{profile-name}/
├── profile.json                    # Profile metadata.
├── assets/
│   ├── skill/
│   │   └── review/
│   │       ├── asset.json          # Asset manifest.
│   │       └── SKILL.md            # The skill content.
│   ├── agents_doc/
│   ├── settings/
│   ├── mcp/
│   ├── rule/
│   └── hook/
└── projects/
    └── app.json                    # One project manifest per target repo.
```

### Target repository (after running `apply`)

Only managed surfaces are modified:

```
~/app-dir/
├── AGENTS.md                       # From an agents_doc asset, if selected.
├── .claude/
│   ├── settings.local.json
│   └── skills/
│       └── review/SKILL.md
├── .codex/
│   ├── config.toml
│   └── skills/
│       └── review/SKILL.md
├── .cursor/
│   └── commands/review.md
├── .opencode/
│   └── skills/review/SKILL.md
└── .agentfiles/
    └── state.json                  # Managed-state bookkeeping.
```

### Global registry (TODO: review against current version)

```
~/.config/.agentfiles/profiles.json
```

A single JSON file listing every registered profile with its id, name, path, creation time, and last-opened time. It stores **only pointers** — no assets live here.

---

## Ownership rule

One target project path belongs to at most one profile. If profile A already owns `~/src/app`, profile B cannot `project add` the same path. This prevents two profiles from silently conflicting over the same generated files.

---

## Current features

This is an initial implementation. Current feature set:

- Profile creation, registration, and listing
- Asset scaffolding for all six types
- Project manifests with per-project agent/asset selection
- Render → preview → apply pipeline
- Managed-state tracking via `.agentfiles/state.json`
- Drift detection
- Delete-candidate detection for recognized LLM files
- TUI-driven menu and per-command forms for every operation

---

## License

Agentfiles is licensed under the **GNU Affero General Public License v3.0** (AGPL-3.0). See [`LICENSE`](./LICENSE) for the full text.

In short: you may use, modify, and distribute this software freely, but if you run a modified version as a network service you must make your source available to its users (AGPL §13).

### Commercial license

A separate commercial license — permitting use without the AGPL's copyleft and network-source obligations — is available for organizations that cannot comply with the AGPL. Contact **info@hexworks.org**.

### Contributing

By submitting a contribution you agree to the [Contributor License Agreement](./CLA.md), under which you assign copyright in your contribution to Hexworks. This lets Hexworks offer the project under both the AGPL and a commercial license. See [`CLA.md`](./CLA.md) for details.

---

## Further reading

More detailed documentation lives under [`docs/`](./docs/):

- [`docs/architecture/`](./docs/architecture/) — full arc42 architecture set
- [`docs/adr/`](./docs/adr/) — Architecture Decision Records
- [`docs/guidelines/`](./docs/guidelines/) — coding and design guidelines
- [`docs/glossary.md`](./docs/glossary.md) — canonical term definitions
