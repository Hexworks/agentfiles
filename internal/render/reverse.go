package render

import (
	pathpkg "path"
	"path/filepath"
	"strings"

	"github.com/hexworks/agentfiles/internal/agent"
	"github.com/hexworks/agentfiles/internal/asset"
	"github.com/hexworks/agentfiles/internal/surfaces"
)

// ReverseLookup maps a managed repo path back to the asset and source
// path it was rendered from, so sync's Adopt flow (ADR 0020) can write a
// local edit into the owning profile asset. It is the single reverse
// entry point that replaced sync's own re-derivation, so forward render
// and reverse Adopt cannot drift.
//
// Two cases:
//
//   - repoPath is a rendered file: its recorded provenance is
//     authoritative and its strategy decides invertibility (a cursor
//     command reports ok=false).
//   - repoPath is an untracked sibling inside a rendered directory: the
//     owning directory's strategy re-derives the source by preserving the
//     tail below the rendered target root.
//
// ok is false when repoPath has no owning asset or the owning strategy is
// non-invertible.
func (p *ProjectPlan) ReverseLookup(repoPath string) (assetID, sourceRel string, ok bool) {
	idx := p.reverseIndex()

	if rf, hit := idx.exact[repoPath]; hit {
		strat, sok := strategyFor(rf.Agent, rf.Type)
		if !sok {
			return "", "", false
		}
		src, rok := strat.Reverse(ReverseContext{
			RepoPath:       repoPath,
			ExactSourceRel: rf.SourceRel,
			AssetID:        rf.AssetID,
		})
		if !rok {
			return "", "", false
		}
		return rf.AssetID, src, true
	}

	entry, found := idx.owner(repoPath)
	if !found {
		return "", "", false
	}
	strat, sok := strategyFor(entry.Agent, entry.Type)
	if !sok {
		return "", "", false
	}
	src, rok := strat.Reverse(ReverseContext{
		RepoPath:   repoPath,
		TargetRoot: entry.TargetRoot,
		SourceRoot: entry.SourceRoot,
		AssetID:    entry.AssetID,
	})
	if !rok {
		return "", "", false
	}
	return entry.AssetID, src, true
}

// reverseIndex is the memoized reverse-lookup structure derived from a
// plan's rendered files: an exact path→file map for rendered files and an
// owning-directory map for untracked siblings.
type reverseIndex struct {
	exact map[string]RenderedFile
	dirs  map[string]owningDir
}

// owningDir records one rendered directory together with the asset and
// (agent, type) strategy that produced it, plus the target/source roots
// that let the strategy re-derive an untracked sibling's source path.
type owningDir struct {
	AssetID    string
	SourceRoot string
	TargetRoot string
	Agent      agent.Agent
	Type       asset.Type
}

func (p *ProjectPlan) reverseIndex() *reverseIndex {
	if p.revIndex != nil {
		return p.revIndex
	}
	idx := &reverseIndex{
		exact: make(map[string]RenderedFile, len(p.Files)),
		dirs:  buildOwningDirs(p.Files),
	}
	for _, f := range p.Files {
		idx.exact[f.Path] = f
	}
	p.revIndex = idx
	return idx
}

// buildOwningDirs walks the rendered files and returns a map keyed by the
// rendered directory each file sits in, valued by its owning asset and
// strategy provenance. A directory hosting files from more than one asset
// (or with divergent source roots) is dropped as ambiguous, so Adopt is
// not offered for an untracked file whose owner cannot be resolved. The
// source root is the remainder after stripping the shared tail from the
// (target dir, source dir) pair, reproducing the projection root the
// forward render used.
func buildOwningDirs(files []RenderedFile) map[string]owningDir {
	type tally struct {
		entry     owningDir
		ambiguous bool
	}
	tallies := map[string]*tally{}
	for _, f := range files {
		if f.AssetID == "" || f.SourceRel == "" {
			continue
		}
		targetDir := pathpkg.Dir(f.Path)
		sourceDir := pathpkg.Dir(filepath.ToSlash(f.SourceRel))
		targetRoot, sourceRoot := stripCommonSuffix(targetDir, sourceDir)
		existing, ok := tallies[targetDir]
		if !ok {
			tallies[targetDir] = &tally{entry: owningDir{
				AssetID:    f.AssetID,
				SourceRoot: sourceRoot,
				TargetRoot: targetRoot,
				Agent:      f.Agent,
				Type:       f.Type,
			}}
			continue
		}
		if existing.entry.AssetID != f.AssetID || existing.entry.SourceRoot != sourceRoot {
			existing.ambiguous = true
		}
	}
	out := make(map[string]owningDir, len(tallies))
	for k, t := range tallies {
		if t.ambiguous {
			continue
		}
		out[k] = t.entry
	}
	return out
}

// owner returns the owning directory entry for repoPath by walking up
// from its parent directory until a rendered directory matches. The walk
// stops at an asset-container root (so an untracked file directly under
// .claude/skills, with no owning skill folder, resolves to no owner) and
// at the repo root.
func (idx *reverseIndex) owner(repoPath string) (owningDir, bool) {
	dir := pathpkg.Dir(repoPath)
	for dir != "." && dir != "/" {
		if entry, hit := idx.dirs[dir]; hit {
			return entry, true
		}
		if surfaces.IsAssetContainerRoot(dir) {
			return owningDir{}, false
		}
		next := pathpkg.Dir(dir)
		if next == dir {
			return owningDir{}, false
		}
		dir = next
	}
	return owningDir{}, false
}

// stripCommonSuffix walks target and source backwards while segments
// match and returns the two root prefixes that remain. Both inputs are
// forward-slash paths; empty strings map to ".".
func stripCommonSuffix(target, source string) (string, string) {
	tParts := splitSlash(target)
	sParts := splitSlash(source)
	i := len(tParts)
	j := len(sParts)
	for i > 0 && j > 0 && tParts[i-1] == sParts[j-1] {
		i--
		j--
	}
	return joinSlash(tParts[:i]), joinSlash(sParts[:j])
}

func splitSlash(p string) []string {
	if p == "" || p == "." {
		return nil
	}
	return strings.Split(p, "/")
}

func joinSlash(parts []string) string {
	if len(parts) == 0 {
		return "."
	}
	return strings.Join(parts, "/")
}
