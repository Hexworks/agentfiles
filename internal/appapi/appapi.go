// Package appapi holds the boundary value types the TUI, actions layer,
// and app service share. Splitting these out of `internal/app` keeps
// the documented `tui/shell → actions → app` edge honest: shell reads
// value types from this leaf package rather than importing `app`
// directly (see docs/architecture/05-building-block-view.md).
//
// The package is import-graph leaf-adjacent: it depends on the domain
// aggregates (`profile`, `project`) and the safety-fence rule package
// (`surfaces`) plus `errs`, but nothing above it. Both `app` and
// `actions` depend on it; no upstream package depends on either.
package appapi

import (
	"slices"
	"strings"
	"unicode"

	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/profile"
	"github.com/hexworks/agentfiles/internal/project"
	"github.com/hexworks/agentfiles/internal/surfaces"
)

// LoadedProfile pairs a profile aggregate with the per-user project
// selections that live in a separate aggregate on disk. Callers get one
// value that carries both boundaries so they do not have to compose
// (profile + projects) themselves; the composition is a read-time
// convenience — writes still route through the owning aggregate
// (profile assets in the profile folder, project manifests in the
// projects store). See ADR 0017.
type LoadedProfile struct {
	Profile  *profile.Profile
	Projects map[string]*project.Manifest
}

// ProjectList returns the loaded projects sorted by display name so
// callers get a stable order without re-implementing the rule.
func (l *LoadedProfile) ProjectList() []*project.Manifest {
	list := make([]*project.Manifest, 0, len(l.Projects))
	for _, p := range l.Projects {
		list = append(list, p)
	}
	slices.SortFunc(list, func(a, b *project.Manifest) int {
		return strings.Compare(a.Name, b.Name)
	})
	return list
}

// ChangeKind mirrors sync.ChangeKind. The TUI consumes this vocabulary
// so it never imports internal/sync directly.
type ChangeKind string

// Possible ChangeKind values mirror sync.ChangeKind.
const (
	ChangeCreate  ChangeKind = "create"
	ChangeUpdate  ChangeKind = "update"
	ChangeDrift   ChangeKind = "drift"
	ChangeDelete  ChangeKind = "delete"
	ChangeUnknown ChangeKind = "unknown"
)

// FileChange is the boundary mirror of sync.FileChange. Only the fields
// the TUI consumes are exposed; render leaves and managed-state
// metadata stay inside the domain.
type FileChange struct {
	Path string
	Kind ChangeKind
	// OwningAssetID gates the row-level Adopt action on ChangeUnknown
	// rows: the TUI offers UnknownAdopt only when this is non-empty,
	// matching sync's reverse-mapping table (ADR 0020).
	OwningAssetID string
	// AdoptProvenance carries the keys the Plan Project screen needs to
	// decide a ChangeDrift row's action buttons. Available() is true when
	// the row may offer the [Adopt] button; when false the screen renders a
	// bilean toggle instead so the user cannot pick a doomed Adopt. Mirrors
	// sync.FileChange.AdoptProvenance.
	AdoptProvenance AdoptProvenance
}

// AdoptProvenance is the boundary mirror of sync.AdoptProvenance: the keys
// that make Adopt a legal choice on a ChangeDrift row. A zero value means
// Adopt is unavailable for the row.
type AdoptProvenance struct {
	AssetID   string
	SourceRel string
}

// Available reports whether the drift row may offer the [Adopt] button.
func (p AdoptProvenance) Available() bool {
	return p.AssetID != "" && p.SourceRel != ""
}

// Preview is the boundary mirror of sync.Preview. It carries the change
// list the TUI renders plus the identifying ids; render leaves and
// managed-state stay inside the domain.
type Preview struct {
	ProfileID string
	ProjectID string
	Changes   []FileChange
	// IgnoredPaths mirrors the persisted ManagedState.IgnoredPaths: the folder
	// keys whose unknown subtree the plan suppressed. The Plan Project screen
	// surfaces these so the user can view and un-ignore them; nil when the
	// project has no managed state yet.
	IgnoredPaths []string
}

// DriftDecision is the boundary mirror of sync.DriftDecision.
type DriftDecision string

// Possible DriftDecision values mirror sync.DriftDecision.
const (
	DriftKeep      DriftDecision = "keep"
	DriftOverwrite DriftDecision = "overwrite"
	// DriftAdopt promotes the local edit into the profile asset it came
	// from — the single sanctioned repo → profile flow (ADR 0020).
	DriftAdopt DriftDecision = "adopt"
)

// UnknownDecision is the boundary mirror of sync.UnknownDecision.
type UnknownDecision string

// Possible UnknownDecision values mirror sync.UnknownDecision.
const (
	UnknownKeep   UnknownDecision = "keep"
	UnknownDelete UnknownDecision = "delete"
	// UnknownAdopt promotes an untracked file that sits inside a known
	// asset projection dir into the owning asset (ADR 0020).
	UnknownAdopt UnknownDecision = "adopt"
)

// DriftResolution pairs a drifted path with the user's per-file
// decision. Service.Apply translates these into the corresponding
// sync types before invoking the engine.
type DriftResolution struct {
	Path     string
	Decision DriftDecision
}

// UnknownResolution pairs an unknown path with the user's per-file
// decision.
type UnknownResolution struct {
	Path     string
	Decision UnknownDecision
}

// Resolutions bundles per-file drift and unknown choices plus the
// folder keys to ignore for one Apply call.
type Resolutions struct {
	Drift        []DriftResolution
	Unknown      []UnknownResolution
	IgnoredPaths []string
}

// CommitOutcome is the discriminated result of the git-aware commit
// step. Callers pattern-match on the concrete type — the compiler
// keeps every consumer honest as new outcomes are added. Concrete
// types are Committed, Skipped, and Failed (see below). nil is treated
// the same as Skipped{Reason: SkipDisabled} by consumers so the
// zero-value never has to be constructed explicitly.
type CommitOutcome interface {
	isCommitOutcome()
}

// Committed reports a successful commit. SHA is the short hash git
// returned. Empty SHA is a wiring bug — Committed is never returned
// for an empty diff.
type Committed struct {
	SHA string
}

func (Committed) isCommitOutcome() {}

// SkipReason enumerates why a commit was not attempted or not
// recorded. The three named values match the glossary's "commit
// trigger" section: the feature is off, the folder is not a git repo,
// or the pathspec had no diff to commit.
type SkipReason int

// Possible SkipReason values.
const (
	// SkipDisabled means git integration is off in the settings store,
	// so no commit was attempted.
	SkipDisabled SkipReason = iota + 1
	// SkipNotARepo means the mutated folder is not a git work tree, so
	// no commit was attempted (silent skip per ADR 0019).
	SkipNotARepo
	// SkipEmptyDiff means the pathspec matched no differences at
	// commit time, so no commit was recorded.
	SkipEmptyDiff
)

// Skipped reports that no commit landed. Reason names the specific
// skip flavor so the TUI can render a targeted (or silent) message.
type Skipped struct {
	Reason SkipReason
}

func (Skipped) isCommitOutcome() {}

// Failed reports a hard commit error. The underlying domain error
// carries the specific typed error (HookFailedError, CommitError,
// UnrelatedStagedChangesError, etc.) so callers can `errors.As` for
// finer-grained UX.
type Failed struct {
	Err errs.DomainError
}

func (Failed) isCommitOutcome() {}

// ApplyOutcome bundles the results of one Apply call. Preview is the
// boundary preview the caller renders; Sync is the target-repo commit
// outcome; Adopt is the profile-repo commit outcome from the ADR 0020
// reverse-flow (Skipped{SkipDisabled} when no Adopt requests happened).
// Named fields eliminate the swap risk of two positional CommitOutcome
// return values (see task 0035 review).
type ApplyOutcome struct {
	Preview *Preview
	Sync    CommitOutcome
	Adopt   CommitOutcome
}

// RegisterableDirs returns the set of directory keys in changes that
// are eligible for asset registration. Thin adapter over
// surfaces.RegisterableFolders: copies the change-kind classification
// into surfaces.Leaf so the eligibility rule and its container-root
// data live together in surfaces, while the boundary keeps its
// FileChange/ChangeKind vocabulary intact.
func RegisterableDirs(changes []FileChange) map[string]bool {
	return surfaces.RegisterableFolders(leavesFromChanges(changes))
}

// LeavesFromChanges converts a FileChange slice into the surfaces.Leaf
// vocabulary. Exported so the app service can reuse the mapping when
// re-asserting registerability against a freshly computed plan
// (assertIgnoredRegisterable) without redefining the loop.
func LeavesFromChanges(changes []FileChange) []surfaces.Leaf {
	return leavesFromChanges(changes)
}

func leavesFromChanges(changes []FileChange) []surfaces.Leaf {
	leaves := make([]surfaces.Leaf, len(changes))
	for i, ch := range changes {
		leaves[i] = surfaces.Leaf{Path: ch.Path, IsUnknown: ch.Kind == ChangeUnknown}
	}
	return leaves
}

// DesiredIgnored computes the complete persisted ignored set a project should
// have after the user's in-session edits on the Plan Project screen:
// (persisted − unignored) ∪ newlyIgnored, deduplicated and sorted. The TUI
// sends the result on Apply and sync writes it verbatim (replace semantics), so
// dropping a key here un-ignores that folder on the next plan. The rule lives
// here rather than inside a Bubble Tea screen so it is testable without the
// TUI. Returns nil when the desired set is empty.
func DesiredIgnored(persisted, unignored, newlyIgnored []string) []string {
	drop := make(map[string]bool, len(unignored))
	for _, p := range unignored {
		drop[p] = true
	}
	desired := make(map[string]bool, len(persisted)+len(newlyIgnored))
	for _, p := range persisted {
		if !drop[p] {
			desired[p] = true
		}
	}
	for _, p := range newlyIgnored {
		desired[p] = true
	}
	if len(desired) == 0 {
		return nil
	}
	out := make([]string, 0, len(desired))
	for p := range desired {
		out = append(out, p)
	}
	slices.Sort(out)
	return out
}

// DriftResolutionsFromMap encodes the ADR 0015 / ADR 0020 emission
// contract: a drift row emits a resolution only when the user picked
// DriftOverwrite or DriftAdopt; DriftKeep (and "no choice") stays absent
// so sync preserves the prior baseline. Callers assembling the Apply
// resolutions from a change list and a path→decision map use this
// instead of open-coding the rule so the domain contract lives one hop
// from sync rather than in each UI. Returns nil when no row would emit.
func DriftResolutionsFromMap(changes []FileChange, decisions map[string]DriftDecision) []DriftResolution {
	var out []DriftResolution
	for _, ch := range changes {
		if ch.Kind != ChangeDrift {
			continue
		}
		decision := decisions[ch.Path]
		if decision != DriftOverwrite && decision != DriftAdopt {
			continue
		}
		out = append(out, DriftResolution{Path: ch.Path, Decision: decision})
	}
	return out
}

// SanitizeSubject prepares a free-form user value (project name, asset
// id) for interpolation into a git commit subject. Control bytes are
// dropped (a stray newline would silently split the subject into a
// body; a NUL would confuse downstream tooling); runs of internal
// whitespace collapse to a single space; the result is trimmed and
// truncated to max runes so a paste-accident value cannot blow past
// the Conventional-Commits 50-char norm. max ≤ 0 disables the truncation.
func SanitizeSubject(s string, max int) string {
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := true
	for _, r := range s {
		if r == '\t' {
			r = ' '
		}
		if unicode.IsControl(r) {
			continue
		}
		if unicode.IsSpace(r) {
			if prevSpace {
				continue
			}
			b.WriteRune(' ')
			prevSpace = true
			continue
		}
		b.WriteRune(r)
		prevSpace = false
	}
	out := strings.TrimSpace(b.String())
	if max > 0 {
		runes := []rune(out)
		if len(runes) > max {
			out = string(runes[:max])
		}
	}
	return out
}
