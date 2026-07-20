package pathselector

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/hexworks/agentfiles/internal/errs"
)

// Options configures a path-selector modal at construction time. See the
// per-field doc comments for the exact semantics of each knob.
//
// FollowSymlinks intentionally has no "default true" behavior: the zero value
// is false. Callers set it to true explicitly to follow directory symlinks;
// see the field comment for the rationale.
type Options struct {
	// Caption shown in the modal's top border, e.g. "Selecting project root".
	// Empty defaults to "Select path" when the modal is opened through the
	// [modals.NewSelectPath] wrapper.
	Caption string

	// Constraint is the absolute directory the user cannot navigate above.
	// Empty means no constraint — the user may browse the entire filesystem.
	Constraint string

	// StartFolder is the folder shown when the modal opens. Empty defaults to
	// Constraint when set, otherwise to $HOME (falling back to "/" if HOME is
	// unset). Must resolve inside Constraint when Constraint is set.
	StartFolder string

	// ShowFiles: when false, files are hidden and only folders are selectable.
	// Not toggleable at runtime.
	ShowFiles bool

	// AllowedExtensions restricts visible files when ShowFiles is true. A nil
	// or empty slice allows all extensions. Match is case-insensitive and
	// leading-dot canonicalized ("md" and ".MD" both match ".md"). Silently
	// ignored when ShowFiles is false.
	AllowedExtensions []string

	// FollowSymlinks: when true, Enter follows directory symlinks and
	// constraint checks use filepath.EvalSymlinks so symlinks that resolve
	// outside Constraint are rejected. When false, symlinks are shown but
	// Enter on a symlink is a silent no-op.
	//
	// Zero value is false. Callers must set this to true explicitly to opt
	// into symlink following; a plain bool cannot encode a "true default"
	// distinguishable from an explicit false, and the safer behavior on a
	// forgotten field is "do not follow".
	FollowSymlinks bool

	// ShowHiddenInitially is the initial state of the runtime hidden-files
	// toggle (mnemonic `h`). Defaults to false (dotfiles hidden).
	ShowHiddenInitially bool
}

// resolvedOptions holds the post-normalize form of [Options]. Constraint and
// StartFolder are absolute + symlink-resolved; AllowedExtensions is a lookup
// map keyed by ".<lowercase-ext>".
type resolvedOptions struct {
	caption             string
	constraint          string // "" when no constraint set
	startFolder         string
	showFiles           bool
	allowedExt          map[string]struct{} // nil ⇒ allow all
	followSymlinks      bool
	showHiddenInitially bool
}

// normalize validates and materializes an Options value. It performs the
// filesystem probes required to reject unreadable constraints / start folders
// and out-of-constraint starts before the modal opens.
func (o Options) normalize() (resolvedOptions, errs.DomainError) {
	out := resolvedOptions{
		caption:             strings.TrimSpace(o.Caption),
		showFiles:           o.ShowFiles,
		followSymlinks:      o.FollowSymlinks,
		showHiddenInitially: o.ShowHiddenInitially,
	}

	constraint := strings.TrimSpace(o.Constraint)
	if constraint != "" {
		abs, err := filepath.Abs(constraint)
		if err != nil {
			return resolvedOptions{}, ConstraintUnreadableError{Path: constraint, Err: err}
		}
		if resolved, symErr := filepath.EvalSymlinks(abs); symErr == nil {
			abs = resolved
		}
		info, statErr := os.Stat(abs)
		if statErr != nil {
			return resolvedOptions{}, ConstraintUnreadableError{Path: constraint, Err: statErr}
		}
		if !info.IsDir() {
			return resolvedOptions{}, ConstraintUnreadableError{Path: constraint}
		}
		out.constraint = filepath.Clean(abs)
	}

	startInput := strings.TrimSpace(o.StartFolder)
	var start string
	if startInput == "" {
		switch {
		case out.constraint != "":
			start = out.constraint
		default:
			if home, err := os.UserHomeDir(); err == nil && home != "" {
				start = home
			} else {
				start = string(filepath.Separator)
			}
		}
	} else {
		abs, err := filepath.Abs(startInput)
		if err != nil {
			return resolvedOptions{}, StartUnreadableError{Path: startInput, Err: err}
		}
		if resolved, symErr := filepath.EvalSymlinks(abs); symErr == nil {
			abs = resolved
		}
		start = filepath.Clean(abs)
	}

	info, statErr := os.Stat(start)
	if statErr != nil {
		return resolvedOptions{}, StartUnreadableError{Path: firstNonEmpty(startInput, start), Err: statErr}
	}
	if !info.IsDir() {
		return resolvedOptions{}, StartUnreadableError{Path: firstNonEmpty(startInput, start)}
	}
	out.startFolder = filepath.Clean(start)

	if out.constraint != "" && !isUnderRoot(out.startFolder, out.constraint) {
		return resolvedOptions{}, StartOutsideConstraintError{
			Start:      firstNonEmpty(startInput, out.startFolder),
			Constraint: constraint,
		}
	}

	out.allowedExt = normalizeExtensions(o.AllowedExtensions)
	return out, nil
}

// normalizeExtensions lower-cases, leading-dot canonicalizes, and drops empty
// entries from the caller-supplied slice. Returns nil when the input carries
// no usable entries — nil means "allow all".
func normalizeExtensions(exts []string) map[string]struct{} {
	if len(exts) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(exts))
	for _, e := range exts {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		if !strings.HasPrefix(e, ".") {
			e = "." + e
		}
		out[strings.ToLower(e)] = struct{}{}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
