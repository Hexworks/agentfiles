package shell

// ManualOverview is the default manual page shown by the Info modal
// when no per-screen override is registered. Per-screen entries land as
// concrete screens arrive (tasks 0024+); until then the overview is the
// only topic mapping.
const ManualOverview = "docs/manual/overview.md"

// topicFor returns the manual file the Info modal should render for the
// currently focused screen. Today every screen resolves to the
// overview; the parameter is retained so future screens can register
// per-topic overrides without churning the call site in keys.go.
func topicFor(_ Screen) string { return ManualOverview }
