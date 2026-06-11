package help

import "testing"

type fixedTopic struct{ topic Topic }

func (f fixedTopic) Topic() Topic { return f.topic }

func TestTopicFor_NonTopicalReturnsOverview(t *testing.T) {
	got := TopicFor(struct{}{})
	if got != OverviewTopic {
		t.Errorf("TopicFor(non-Topical) = %+v, want %+v", got, OverviewTopic)
	}
	if got := TopicFor(nil); got != OverviewTopic {
		t.Errorf("TopicFor(nil) = %+v, want %+v", got, OverviewTopic)
	}
}

func TestTopicFor_TopicalReturnsItsTopic(t *testing.T) {
	want := Topic{Label: "Profile", File: "profile.md"}
	got := TopicFor(fixedTopic{topic: want})
	if got != want {
		t.Errorf("TopicFor(Topical) = %+v, want %+v", got, want)
	}
}
