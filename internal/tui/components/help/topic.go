package help

// Topic pairs the human-facing label rendered in the help dialog's tab
// with the file (relative to [ManualRoot]) that the modal loads. Topic
// owns both halves so a screen that exposes a per-screen help page only
// has to register a single record.
type Topic struct {
	// Label is the short topic name shown in the modal tab
	// (e.g. "Overview", "Profile").
	Label string
	// File is the manual file name relative to [ManualRoot]. Must end
	// in `.md` and must not escape the root via `..`; both checks are
	// enforced by the loader.
	File string
}

// Topical is the optional interface a Screen can satisfy to register a
// per-screen topic with the help modal. The help package does not
// import any screen package; the interface lives here so screens depend
// inward on the help vocabulary, not the other way round.
type Topical interface {
	Topic() Topic
}

// OverviewTopic is the default topic — the project overview page. Used
// when the focused screen does not implement [Topical].
var OverviewTopic = Topic{Label: "Overview", File: "overview.md"}

// TopicFor resolves the help topic for an arbitrary value. If v
// satisfies [Topical] the per-screen topic is returned; otherwise the
// caller gets [OverviewTopic]. Accepting `any` keeps this package free
// of any screen-type dependency.
func TopicFor(v any) Topic {
	if t, ok := v.(Topical); ok {
		return t.Topic()
	}
	return OverviewTopic
}
