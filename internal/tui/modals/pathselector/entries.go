package pathselector

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hexworks/agentfiles/internal/errs"
)

// entryKind classifies a single row in the modal's listing. Sort order and
// per-row actions dispatch on this classification.
type entryKind int

const (
	// entryHeader is the always-present root row that shows the current
	// folder. Cursor may land on it; Enter is a no-op and no [Select] button
	// is offered. Selecting via `s` while cursor is on the header is treated
	// as [Select current] to keep the interaction predictable.
	entryHeader entryKind = iota
	// entryParent is the `..` navigation row. Present as the first Child of
	// the tree root unless the current folder equals the constraint root.
	entryParent
	// entryDir is a real directory row.
	entryDir
	// entryDirSymlink is a symlink whose target resolves to a directory. When
	// FollowSymlinks is true it navigates like a regular directory; when
	// false, Enter on it is a silent no-op.
	entryDirSymlink
	// entryFile is a regular file row.
	entryFile
	// entryFileSymlink is a symlink whose target resolves to a file (or does
	// not resolve at all — best-effort classification).
	entryFileSymlink
	// entryEmpty is the italic "<empty>" placeholder shown when a folder has
	// no visible children after filtering. Non-selectable.
	entryEmpty
)

// entry is one row's model. Name carries the display string (including
// trailing '/' or '@' suffix when applicable). Abs is the absolute path the
// row points at — the value that would be returned in [Result.Path] or fed to
// navigate() on Enter. Hidden marks dotfile-prefixed names for the runtime
// visibility toggle.
type entry struct {
	Name   string
	Abs    string
	Kind   entryKind
	Hidden bool
}

// isDir reports whether the entry's target is a directory the modal can
// navigate into or select as a folder.
func (e entry) isDir() bool {
	switch e.Kind {
	case entryHeader, entryParent, entryDir, entryDirSymlink:
		return true
	default:
		return false
	}
}

// isSelectable reports whether the modal should offer a per-row [Select]
// button when the cursor is on this entry. The header row shares [Select
// current] semantics, so we suppress the per-row button on it to keep the
// mnemonic surface uniform.
func (e entry) isSelectable() bool {
	return e.Kind != entryHeader && e.Kind != entryEmpty
}

// buildEntries reads dir, filters, and sorts the visible rows. The returned
// slice never contains an [entryHeader] row — that is produced by the caller
// (it depends on the constraint state and is used as the treetable root).
// A read failure returns a typed [ReadDirError].
func buildEntries(dir string, opts resolvedOptions, showHidden bool) ([]entry, errs.DomainError) {
	dirents, err := os.ReadDir(dir)
	if err != nil {
		return nil, ReadDirError{Path: dir, Err: err}
	}

	var dirs, files []entry
	for _, d := range dirents {
		name := d.Name()
		hidden := strings.HasPrefix(name, ".")
		if hidden && !showHidden {
			continue
		}
		abs := filepath.Join(dir, name)

		kind := classify(d, abs)
		switch kind {
		case entryDir, entryDirSymlink:
			dirs = append(dirs, entry{
				Name:   displayName(name, kind),
				Abs:    abs,
				Kind:   kind,
				Hidden: hidden,
			})
		case entryFile, entryFileSymlink:
			if !opts.showFiles {
				continue
			}
			if !extAllowed(name, opts.allowedExt) {
				continue
			}
			files = append(files, entry{
				Name:   displayName(name, kind),
				Abs:    abs,
				Kind:   kind,
				Hidden: hidden,
			})
		}
	}

	sortByNameFold(dirs)
	sortByNameFold(files)

	out := make([]entry, 0, len(dirs)+len(files)+1)
	if dir != opts.constraint {
		out = append(out, entry{
			Name: "..",
			Abs:  filepath.Dir(filepath.Clean(dir)),
			Kind: entryParent,
		})
	}
	out = append(out, dirs...)
	out = append(out, files...)
	if len(out) == 0 {
		out = append(out, entry{Name: "<empty>", Abs: dir, Kind: entryEmpty})
	}
	return out, nil
}

// classify identifies the entry kind for a dirent. Directory symlinks are
// stat-followed so a broken symlink degrades to a file-symlink classification
// rather than being surfaced as a real directory the user can enter.
func classify(d os.DirEntry, abs string) entryKind {
	if d.Type()&os.ModeSymlink != 0 {
		info, err := os.Stat(abs)
		if err != nil {
			return entryFileSymlink
		}
		if info.IsDir() {
			return entryDirSymlink
		}
		return entryFileSymlink
	}
	if d.IsDir() {
		return entryDir
	}
	return entryFile
}

// displayName decorates the file name for the treetable label. Directories get
// a trailing '/', symlinks get a trailing '@' (matching the ls -F convention);
// regular files show the bare name.
func displayName(name string, kind entryKind) string {
	switch kind {
	case entryDir:
		return name + "/"
	case entryDirSymlink, entryFileSymlink:
		return name + "@"
	default:
		return name
	}
}

// extAllowed reports whether name passes the extension filter. A nil allow-set
// means "allow all". The comparison uses the file's lower-case extension so
// "NOTES.MD" matches ".md".
func extAllowed(name string, allowed map[string]struct{}) bool {
	if allowed == nil {
		return true
	}
	ext := strings.ToLower(filepath.Ext(name))
	_, ok := allowed[ext]
	return ok
}

// sortByNameFold sorts entries A→Z case-insensitive on the raw file name (the
// bare name without decoration), with the display Name as a stable tie-breaker
// so directories and their symlink twins do not reorder unpredictably.
func sortByNameFold(es []entry) {
	sort.SliceStable(es, func(i, j int) bool {
		a := strings.ToLower(strings.TrimRight(es[i].Name, "/@"))
		b := strings.ToLower(strings.TrimRight(es[j].Name, "/@"))
		if a != b {
			return a < b
		}
		return es[i].Name < es[j].Name
	})
}
