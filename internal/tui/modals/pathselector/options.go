package pathselector

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/utils"
)

// Options configures a path-selector modal at construction time. See the
// per-field doc comments for the exact semantics of each knob.
//
// FollowSymlinks intentionally has no "default true" behavior: the zero
// value is false. Callers set it to true explicitly to follow directory
// symlinks; see the field comment for the rationale.
type Options struct {
	// Caption shown in the modal's top border, e.g. "Selecting project root".
	// Empty defaults to "Select path" when the modal is opened through the
	// [modals.NewSelectPath] wrapper.
	Caption string

	// ConstraintRoot is the absolute directory the user cannot navigate
	// above — the modal's ubiquitous "Constraint Root" (see the glossary).
	// Empty means no constraint root — the user may browse the entire
	// filesystem.
	ConstraintRoot string

	// StartFolder is the folder shown when the modal opens. Empty defaults to
	// ConstraintRoot when set, otherwise to $HOME (falling back to "/" if
	// HOME is unset). Must resolve inside ConstraintRoot when set.
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
	// outside ConstraintRoot are rejected. When false, symlinks are shown
	// but Enter on a symlink is a silent no-op.
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

// canonOptions is the pure, syscall-free canonicalization of [Options].
// Trimmed strings, lower-cased extension keys, and the un-probed
// constraint / start-folder inputs — everything the filesystem probe
// step needs without having touched the disk yet. Keeping this
// intermediate form testable in isolation is the point of the split
// (see docs/guidelines/clean_architecture.md — "I/O at the edges").
type canonOptions struct {
	caption             string
	constraint          string
	startInput          string
	showFiles           bool
	allowedExt          map[string]struct{}
	followSymlinks      bool
	showHiddenInitially bool
}

// resolvedOptions holds the post-probe form of [Options]. constraint and
// startFolder are absolute + symlink-resolved; allowedExt is a lookup
// map keyed by ".<lowercase-ext>".
type resolvedOptions struct {
	constraint          string // "" when no constraint set
	startFolder         string
	showFiles           bool
	allowedExt          map[string]struct{} // nil ⇒ allow all
	followSymlinks      bool
	showHiddenInitially bool
}

// canonicalize is the pure input-normalization pass: trim, lowercase
// extensions, dedupe empties. It performs no I/O and never fails so
// probe() can operate on a known-clean value.
func (o Options) canonicalize() canonOptions {
	return canonOptions{
		caption:             strings.TrimSpace(o.Caption),
		constraint:          strings.TrimSpace(o.ConstraintRoot),
		startInput:          strings.TrimSpace(o.StartFolder),
		showFiles:           o.ShowFiles,
		allowedExt:          normalizeExtensions(o.AllowedExtensions),
		followSymlinks:      o.FollowSymlinks,
		showHiddenInitially: o.ShowHiddenInitially,
	}
}

// probe performs every filesystem check the modal needs before it opens:
// resolve the constraint, resolve (or default) the start folder, and
// verify the start still sits under the constraint. All syscalls live
// here so canonicalize() stays pure.
func probe(canon canonOptions) (resolvedOptions, errs.DomainError) {
	out := resolvedOptions{
		showFiles:           canon.showFiles,
		allowedExt:          canon.allowedExt,
		followSymlinks:      canon.followSymlinks,
		showHiddenInitially: canon.showHiddenInitially,
	}

	if canon.constraint != "" {
		resolved, err := probeDirectory(canon.constraint)
		if err != nil {
			return resolvedOptions{}, ConstraintUnreadableError{Path: canon.constraint, Err: err}
		}
		out.constraint = resolved
	}

	start, startErr := resolveStart(canon.startInput, out.constraint)
	if startErr != nil {
		return resolvedOptions{}, startErr
	}
	out.startFolder = start

	if out.constraint != "" && !utils.IsUnderRoot(out.startFolder, out.constraint) {
		return resolvedOptions{}, StartOutsideConstraintError{
			Start:      userFacingPath(canon.startInput, out.startFolder),
			Constraint: canon.constraint,
		}
	}

	return out, nil
}

// probeDirectory resolves path to an absolute, symlink-expanded form
// and confirms it names a readable directory. Any failure surfaces the
// underlying error so the caller can wrap it in the right typed
// DomainError.
func probeDirectory(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if resolved, symErr := filepath.EvalSymlinks(abs); symErr == nil {
		abs = resolved
	}
	info, statErr := os.Stat(abs)
	if statErr != nil {
		return "", statErr
	}
	if !info.IsDir() {
		return "", errors.New("not a directory")
	}
	return filepath.Clean(abs), nil
}

// resolveStart returns the resolved absolute path of the folder the
// modal should open in. When startInput is empty the resolved constraint
// is used (or $HOME / "/" when unconstrained). When startInput is set it
// is probed for readability. Failures surface as StartUnreadableError
// carrying the caller-supplied form of the path so the message stays
// meaningful even after Abs+Clean.
func resolveStart(startInput, constraint string) (string, errs.DomainError) {
	if startInput == "" {
		if constraint != "" {
			return constraint, nil
		}
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			return probeStart(home, "")
		}
		return probeStart(string(filepath.Separator), "")
	}
	return probeStart(startInput, startInput)
}

// probeStart is the shared tail for the resolveStart branches: run the
// same probe, wrap failures in StartUnreadableError so callers get one
// error shape regardless of how the input arrived.
func probeStart(path, displayInput string) (string, errs.DomainError) {
	resolved, err := probeDirectory(path)
	if err != nil {
		return "", StartUnreadableError{Path: userFacingPath(displayInput, path), Err: err}
	}
	return resolved, nil
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

// userFacingPath prefers the caller-supplied form of a path over the
// post-resolution form when composing an error message: the input string
// is what the caller typed and can recognize, whereas the resolved path
// may look unrelated after Abs+EvalSymlinks. Falls back to resolved when
// input is empty.
func userFacingPath(input, resolved string) string {
	if input != "" {
		return input
	}
	return resolved
}
