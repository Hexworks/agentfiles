// Package config holds the well-known filenames, directory names, and
// per-aggregate defaults shared across more than one domain package. Splitting
// these values out of their consumers means a behavior change is a single
// edit-point. The package has no internal dependencies and sits at the bottom
// of the import graph.
//
// Note: the managed-surface root list and its matcher live in the sibling
// `surfaces` package, not here, because the rule and the data belong together.
package config

// Filenames persisted to disk.

// RegistryFileName is the conventional name of the global profile registry
// file written under the user's home directory.
const RegistryFileName = ".agentprofiles.json"

// ProfileManifestFileName is the name of the per-profile manifest file
// scaffolded by profile.Init at the profile root.
const ProfileManifestFileName = "profile.json"

// AssetManifestFileName is the name of the per-asset manifest file scaffolded
// by asset.Init inside each asset directory.
const AssetManifestFileName = "asset.json"

// Directory names inside a profile root.

// AssetsDirName is the subdirectory under a profile root that contains
// per-type asset directories.
const AssetsDirName = "assets"

// ProjectsDirName is the subdirectory under a profile root that contains
// per-project manifest files.
const ProjectsDirName = "projects"

// Managed-state location inside a target repository. Stored as separate
// dir + filename so callers join with the OS separator at the use site.

// StateDirName is the directory under a target repository that holds the
// managed-state snapshot written by sync.Apply.
const StateDirName = ".agentfiles"

// StateFileName is the file inside StateDirName that holds the managed-state
// snapshot for one project.
const StateFileName = "state.json"

// Asset starter filenames written by asset.Init when scaffolding a new asset.
// Each name corresponds to one of the typed asset categories in
// internal/asset.

// SkillStarterFileName is the starter file written into a freshly scaffolded
// skill asset directory.
const SkillStarterFileName = "SKILL.md"

// AgentsDocStarterFileName is the starter file written into a freshly
// scaffolded agents_doc asset directory; it is also the well-known target
// filename rendered into a project for the codex agent.
const AgentsDocStarterFileName = "AGENTS.md"

// SettingsStarterFileName is the starter file written into a freshly
// scaffolded settings asset directory.
const SettingsStarterFileName = "codex.toml"
