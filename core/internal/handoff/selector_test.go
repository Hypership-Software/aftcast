package handoff

import (
	"testing"

	"github.com/Hypership-Software/aftcast/internal/schema"
	"github.com/Hypership-Software/aftcast/internal/telemetry"
)

type fakeSource struct {
	sessions []telemetry.Session
	events   map[string][]schema.TelemetryEvent
}

func (f *fakeSource) Sessions() ([]telemetry.Session, error) { return f.sessions, nil }
func (f *fakeSource) EventsForSession(id string) ([]schema.TelemetryEvent, error) {
	return f.events[id], nil
}

func TestSelectSessionsJoinsOnCommitSHA(t *testing.T) {
	src := &fakeSource{
		sessions: []telemetry.Session{{Key: "a", SessionID: "a"}, {Key: "b", SessionID: "b"}, {Key: "c", SessionID: "c"}},
		events: map[string][]schema.TelemetryEvent{
			"a": {{EventType: schema.EventPostTool, CommitSHA: "bb16536"}},
			"b": {{EventType: schema.EventPostTool, CommitSHA: "1234567"}},
			"c": {{EventType: schema.EventPostTool}},
		},
	}
	full := []string{"bb16536aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	got, err := SelectSessions(src, full)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Session.SessionID != "a" {
		t.Fatalf("selected %+v, want only session a", got)
	}
	if len(got[0].SHAs) != 1 || got[0].SHAs[0] != "bb16536" {
		t.Errorf("SHAs = %v, want [bb16536]", got[0].SHAs)
	}
	if len(got[0].Events) != 1 {
		t.Errorf("selected session should carry its events")
	}
}

func TestSelectSessionsDedupesSHAs(t *testing.T) {
	src := &fakeSource{
		sessions: []telemetry.Session{{Key: "a", SessionID: "a"}},
		events: map[string][]schema.TelemetryEvent{
			"a": {
				{EventType: schema.EventPostTool, CommitSHA: "bb16536"},
				{EventType: schema.EventPostTool, CommitSHA: "bb16536"},
			},
		},
	}
	got, err := SelectSessions(src, []string{"bb16536aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got[0].SHAs) != 1 {
		t.Errorf("SHAs = %v, want deduped single entry", got[0].SHAs)
	}
}
