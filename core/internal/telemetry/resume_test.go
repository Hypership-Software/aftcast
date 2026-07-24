package telemetry

import (
	"path/filepath"
	"testing"

	"github.com/Hypership-Software/aftcast/internal/audit"
	"github.com/Hypership-Software/aftcast/internal/schema"
)

// A harness reuses one session id when a session is resumed, so a session_start
// for an id already seen begins a new session rather than extending the old one.
// Folded as one record, a session resumed over three days reports a single span
// covering all of them and dates its latest work to the first day.
func TestFoldSessionsSplitsResumedSessions(t *testing.T) {
	evs := []schema.TelemetryEvent{
		{SessionID: "r", EventType: schema.EventSessionStart, TS: "2026-07-22T09:00:00Z"},
		{SessionID: "r", EventType: schema.EventUserPrompt, TS: "2026-07-22T09:30:00Z"},
		{SessionID: "r", EventType: schema.EventSessionStart, TS: "2026-07-23T09:00:00Z"},
		{SessionID: "r", EventType: schema.EventUserPrompt, TS: "2026-07-23T09:30:00Z"},
		{SessionID: "r", EventType: schema.EventSessionStart, TS: "2026-07-24T09:00:00Z"},
		{SessionID: "r", EventType: schema.EventUserPrompt, TS: "2026-07-24T09:30:00Z"},
	}

	got := foldSessions(evs)
	if len(got) != 3 {
		t.Fatalf("sessions = %d, want 3: %+v", len(got), got)
	}

	wantKeys := []string{"r", "r#2", "r#3"}
	wantStarts := []string{"2026-07-22T09:00:00Z", "2026-07-23T09:00:00Z", "2026-07-24T09:00:00Z"}
	wantEnds := []string{"2026-07-22T09:30:00Z", "2026-07-23T09:30:00Z", "2026-07-24T09:30:00Z"}
	for i, session := range got {
		if session.Key != wantKeys[i] {
			t.Fatalf("session %d key = %q, want %q", i, session.Key, wantKeys[i])
		}
		if session.SessionID != "r" {
			t.Fatalf("session %d lost its raw id: %q", i, session.SessionID)
		}
		if session.Started != wantStarts[i] || session.Ended != wantEnds[i] {
			t.Fatalf("session %d span = %s..%s, want %s..%s",
				i, session.Started, session.Ended, wantStarts[i], wantEnds[i])
		}
		if session.TurnCount != 1 {
			t.Fatalf("session %d turn_count = %d, want 1", i, session.TurnCount)
		}
		if session.DurationMS != 1_800_000 {
			t.Fatalf("session %d duration = %dms, want 1800000", i, session.DurationMS)
		}
	}
}

func TestFoldSessionsKeepsASingleStartUnkeyed(t *testing.T) {
	evs := []schema.TelemetryEvent{
		{SessionID: "one", EventType: schema.EventSessionStart, TS: "2026-07-22T09:00:00Z"},
		{SessionID: "one", EventType: schema.EventUserPrompt, TS: "2026-07-22T09:30:00Z"},
	}
	got := foldSessions(evs)
	if len(got) != 1 || got[0].Key != "one" || got[0].SessionID != "one" {
		t.Fatalf("unresumed session = %+v", got)
	}
}

// Splitting a resumed session is only honest if its events split with it —
// otherwise every resume's detail view shows the whole id's history.
func TestEventsForSessionIsBoundedToOneResume(t *testing.T) {
	dir := t.TempDir()
	log, err := audit.NewLog(filepath.Join(dir, "log"), []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("NewLog: %v", err)
	}
	defer log.Close()

	for _, e := range []schema.TelemetryEvent{
		{SessionID: "r", EventType: schema.EventSessionStart, TS: "2026-07-22T09:00:00Z"},
		{SessionID: "r", EventType: schema.EventUserPrompt, TS: "2026-07-22T09:30:00Z"},
		{SessionID: "r", EventType: schema.EventSessionStart, TS: "2026-07-23T09:00:00Z"},
		{SessionID: "r", EventType: schema.EventUserPrompt, TS: "2026-07-23T09:30:00Z"},
		{SessionID: "r", EventType: schema.EventPreTool, TS: "2026-07-23T09:31:00Z"},
	} {
		if err := log.Record(e); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}

	store, err := OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()
	if err := store.Project(log); err != nil {
		t.Fatalf("Project: %v", err)
	}

	first, err := store.EventsForSession("r")
	if err != nil {
		t.Fatalf("EventsForSession(r): %v", err)
	}
	if len(first) != 2 {
		t.Fatalf("first resume events = %d, want 2", len(first))
	}

	second, err := store.EventsForSession("r#2")
	if err != nil {
		t.Fatalf("EventsForSession(r#2): %v", err)
	}
	if len(second) != 3 {
		t.Fatalf("second resume events = %d, want 3", len(second))
	}
	if second[0].TS != "2026-07-23T09:00:00Z" {
		t.Fatalf("second resume starts at %s", second[0].TS)
	}
}
