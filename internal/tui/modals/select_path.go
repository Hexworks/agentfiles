package modals

import (
	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/tui/components/modal"
	"github.com/hexworks/agentfiles/internal/tui/modals/pathselector"
)

// NewSelectPath opens the reusable path-picker modal defined by
// [pathselector.Content]. A validation failure (unreadable constraint / start
// folder, start outside constraint) is returned as a typed [errs.DomainError]
// so the caller can surface it through the standard notification path.
//
// The returned [modal.Modal] is passed to the shell exactly like any other
// modal. Read the selection with [pathselector.ResultFromMsg] on the
// [modal.ResolvedMsg] the runtime dispatches when the user confirms.
func NewSelectPath(opts pathselector.Options) (*modal.Modal, errs.DomainError) {
	content, err := pathselector.New(opts)
	if err != nil {
		return nil, err
	}
	caption := opts.Caption
	if caption == "" {
		caption = "Select path"
	}
	return modal.New("select-path", content, modal.WithCaption(caption)), nil
}
