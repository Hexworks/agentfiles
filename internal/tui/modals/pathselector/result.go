package pathselector

import "github.com/hexworks/agentfiles/internal/tui/components/modal"

// Result carries the selection back to the caller of the path-selector modal.
// Path is the absolute, cleaned filesystem path the user chose; IsDir reports
// whether Path names a directory (as observed at selection time — the modal
// resolves symlinks to their targets before classifying).
type Result struct {
	Path  string
	IsDir bool
}

// ResultFromMsg extracts a [Result] from a modal.ResolvedMsg. Returns
// (result, true) when the modal was confirmed and Value is a Result;
// (Result{}, false) otherwise (cancel, or Value is a different type).
func ResultFromMsg(m modal.ResolvedMsg) (Result, bool) {
	if !m.Confirmed {
		return Result{}, false
	}
	r, ok := m.Value.(Result)
	if !ok {
		return Result{}, false
	}
	return r, true
}
