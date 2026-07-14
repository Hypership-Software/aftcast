package telemetry

import (
	"path/filepath"
	"testing"

	"github.com/Hypership-Software/atlas/internal/audit"
	"github.com/Hypership-Software/atlas/internal/schema"
)

func TestProjectExcludesInternalSessions(t *testing.T) {
	dir := t.TempDir()
	key := []byte("0123456789abcdef0123456789abcdef")
	log, err := audit.NewLog(filepath.Join(dir, "log"), key)
	if err != nil {
		t.Fatalf("NewLog: %v", err)
	}
	defer log.Close()

	// A real session, then the init self-check marker as the LAST (highest-seq)
	// event — so the watermark must still advance past it even though it is filtered.
	for _, e := range []schema.TelemetryEvent{
		{SessionID: "real-1", EventType: schema.EventUserPrompt},
		{SessionID: "real-1", EventType: schema.EventPreTool},
		{SessionID: schema.SelfCheckSessionID, EventType: schema.EventPreTool},
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

	sessions, err := store.Sessions()
	if err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	if len(sessions) != 1 || sessions[0].SessionID != "real-1" {
		t.Fatalf("read-model should hold only the real session, got %+v", sessions)
	}

	// The marker's events must not be queryable either.
	if evs, _ := store.EventsForSession(schema.SelfCheckSessionID); len(evs) != 0 {
		t.Fatalf("self-check events leaked into the read-model: %d", len(evs))
	}

	// Watermark advanced past the filtered marker: a re-project is a no-op and the
	// real session is unchanged.
	if err := store.Project(log); err != nil {
		t.Fatalf("re-Project: %v", err)
	}
	sessions2, _ := store.Sessions()
	if len(sessions2) != 1 {
		t.Fatalf("re-project changed the read-model: %+v", sessions2)
	}
}

func TestFoldSessions_ProjectID(t *testing.T) {
	evs := []schema.TelemetryEvent{
		{SessionID: "s1", EventType: schema.EventSessionStart, TS: "2026-07-14T00:00:00Z"},
		{SessionID: "s1", EventType: schema.EventPreTool, Project: "proj123", TS: "2026-07-14T00:00:01Z"},
	}
	got := foldSessions(evs)
	if len(got) != 1 || got[0].ProjectID != "proj123" {
		t.Fatalf("ProjectID = %q, want proj123", got[0].ProjectID)
	}
}
