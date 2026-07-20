package pathselector

// ConstraintViolationMsg is emitted when a navigation or selection would
// leave [Options.ConstraintRoot]. The modal stays on its current folder
// and expects its host to translate this into whatever error-reporting
// mechanism the host uses (a toast notification, a status-bar line, a
// modal-in-a-modal, …). Constraint carries the resolved constraint root
// so the translation layer can include it in the user-facing message.
type ConstraintViolationMsg struct {
	Constraint string
}

// ReadDirErrorMsg is emitted when a runtime os.ReadDir on the target
// folder fails — permission denied, folder disappeared, partial read.
// The modal stays on the previous folder and expects its host to
// translate this into the host's error-reporting mechanism. Path is the
// folder that failed to read; Err is the underlying I/O error (usually
// an *os.PathError) preserved for host-side formatting.
type ReadDirErrorMsg struct {
	Path string
	Err  error
}
