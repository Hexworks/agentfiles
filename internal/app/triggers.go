package app

import (
	"fmt"

	"github.com/hexworks/agentfiles/internal/appapi"
)

// subjectMaxLen is the truncation ceiling for user-controlled substrings
// interpolated into a commit subject. Sized so a typical prefix plus
// suffix (`chore(agentfiles): update asset <id> manifest`) stays close
// to the Conventional-Commits 50-char norm even when both the project
// name and the asset id are long. Consumers pass this to
// appapi.SanitizeSubject.
const subjectMaxLen = 40

// commitTriggerCtx carries every value the three commit-trigger
// templates might need. Every trigger reads the fields it cares about
// and ignores the rest — a fixed shape lets the triggers live as
// package-level values rather than per-call closures.
type commitTriggerCtx struct {
	ProjectName string
	AssetID     string
	// MutatedFiles is the absolute-path pathspec entries for the
	// managed files a plan-apply wrote (excluding the state.json
	// snapshot, which the trigger appends itself).
	MutatedFiles []string
	// StatePath is the absolute path to `.agentfiles/state.json`
	// rewritten by the plan-apply. Empty for the asset triggers.
	StatePath string
	// AssetDir is the absolute path to the asset directory on disk
	// (the manifest / files pathspecs are derived from it).
	AssetDir string
	// ManifestPath is the absolute path to the asset's asset.json file.
	// Populated by the asset-manifest trigger only.
	ManifestPath string
}

// commitTrigger names the two functions a commit-trigger site
// contributes: the subject template and the pathspec derivation. Both
// take the same commitTriggerCtx so callers construct one value and
// dispatch, rather than juggling three ad-hoc closures.
type commitTrigger struct {
	Subject  func(commitTriggerCtx) string
	Pathspec func(commitTriggerCtx) []string
}

// triggerSyncProject is the plan-apply commit trigger. Subject counts
// managed-file mutations only (the state.json snapshot is bookkeeping
// and never contributes to the "N files" figure). Pathspec is the
// union of the mutated files sync recorded plus the state snapshot,
// exactly what needs to enter the single commit.
var triggerSyncProject = commitTrigger{
	Subject: func(c commitTriggerCtx) string {
		return fmt.Sprintf(
			"chore(agentfiles): sync project %s (%d files)",
			appapi.SanitizeSubject(c.ProjectName, subjectMaxLen),
			len(c.MutatedFiles),
		)
	},
	Pathspec: func(c commitTriggerCtx) []string {
		out := make([]string, 0, len(c.MutatedFiles)+1)
		out = append(out, c.MutatedFiles...)
		if c.StatePath != "" {
			out = append(out, c.StatePath)
		}
		return out
	},
}

// triggerAssetManifest is the manifest-save commit trigger emitted by
// the [Save] button on the Edit Asset screen.
var triggerAssetManifest = commitTrigger{
	Subject: func(c commitTriggerCtx) string {
		return fmt.Sprintf(
			"chore(agentfiles): update asset %s manifest",
			appapi.SanitizeSubject(c.AssetID, subjectMaxLen),
		)
	},
	Pathspec: func(c commitTriggerCtx) []string {
		return []string{c.ManifestPath}
	},
}

// triggerAssetFiles is the editor-return commit trigger emitted after
// the external editor finishes editing a file inside the asset
// directory. The `/**` suffix is the semantic wildcard the git
// wrapper's Covers rule understands; git itself sees the resolved
// literal directory (see toGitPathspec).
var triggerAssetFiles = commitTrigger{
	Subject: func(c commitTriggerCtx) string {
		return fmt.Sprintf(
			"chore(agentfiles): edit asset %s files",
			appapi.SanitizeSubject(c.AssetID, subjectMaxLen),
		)
	},
	Pathspec: func(c commitTriggerCtx) []string {
		return []string{c.AssetDir + "/**"}
	},
}

// triggerAdoptIntoProfile is the plan-apply reverse-flow commit
// trigger (ADR 0020). Fires on the profile repo when at least one
// Adopt request landed. Subject counts the adopted files; pathspec is
// the absolute paths of the profile-side asset files the service just
// wrote.
var triggerAdoptIntoProfile = commitTrigger{
	Subject: func(c commitTriggerCtx) string {
		return fmt.Sprintf(
			"chore(agentfiles): adopt %d file(s) into profile",
			len(c.MutatedFiles),
		)
	},
	Pathspec: func(c commitTriggerCtx) []string {
		return append([]string(nil), c.MutatedFiles...)
	},
}
