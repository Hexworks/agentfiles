package pathselector

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/utils"
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

// selectsAsDir reports whether selecting or navigating into this row
// should be treated as producing a directory. Named after the rule the
// method actually encodes: entryHeader and entryParent are synthetic UI
// rows, not filesystem directories, but selecting them yields
// Result{IsDir: true}. A caller that needs to know "is this row backed
// by a real directory on disk" should compare Kind against the concrete
// entryDir / entryDirSymlink values instead.
func (e entry) selectsAsDir() bool {
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
//
// When root is non-nil, the directory is read through the pinned
// *os.Root so a swap of the constraint root or a symlink escape between
// construction and this call cannot redirect the read (see ADR-worthy
// discussion in the task's review notes; boils down to closing the
// TOCTOU window between construction-time validation and runtime read).
func buildEntries(dir string, opts resolvedOptions, root *os.Root, showHidden bool) ([]entry, errs.DomainError) {
	dirents, err := readDirents(dir, opts.constraint, root)
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
		if parentAbs, parentErr := utils.ResolveAbs(filepath.Dir(dir), true); parentErr == nil {
			out = append(out, entry{
				Name: "..",
				Abs:  parentAbs,
				Kind: entryParent,
			})
		}
	}
	out = append(out, dirs...)
	out = append(out, files...)
	if len(out) == 0 {
		out = append(out, entry{Name: "<empty>", Abs: dir, Kind: entryEmpty})
	}
	return out, nil
}

// readDirents chooses between the pinned *os.Root (constrained mode) and
// a plain os.ReadDir (unconstrained mode) so the caller does not have to
// branch. When root is set the read is anchored to the fd opened at
// construction; when root is nil the modal is running without a
// constraint root and the caller has opted out of the fence.
func readDirents(dir, constraint string, root *os.Root) ([]os.DirEntry, error) {
	if root == nil {
		return os.ReadDir(dir)
	}
	rel, err := relTo(constraint, dir)
	if err != nil {
		return nil, err
	}
	f, err := root.Open(rel)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return f.ReadDir(-1)
}

// relTo returns the root-relative form of dir. Root.Open expects a name
// relative to the fd it was opened at; filepath.Rel converts the
// modal's absolute-path bookkeeping into that form. "." is Root's own
// spelling for the root directory itself.
func relTo(root, dir string) (string, error) {
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return "", err
	}
	if rel == "" || rel == "." {
		return ".", nil
	}
	return rel, nil
}

// classify identifies the entry kind for a dirent. Directory symlinks are
// stat-followed so a broken symlink degrades to a file-symlink classification
// rather than being surfaced as a real directory the user can enter.
//
// Classification uses a plain os.Stat (not Root.Stat): symlinks whose
// targets sit outside the constraint should still appear in the listing
// so the user can see the escape attempt, and the [Content.navigate] gate
// is what refuses to follow them. Hiding them here would surface them as
// silent no-ops instead of an "Cannot leave …" message.
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
