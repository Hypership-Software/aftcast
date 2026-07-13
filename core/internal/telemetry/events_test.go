package telemetry

import (
	"path/filepath"
	"testing"

	"github.com/Hypership-Software/atlas/internal/audit"
	"github.com/Hypership-Software/atlas/internal/schema"
)

func TestEventsForSession(t *testing.T) {
	dir := t.TempDir()
	key := []byte("0123456789abcdef0123456789abcdef")
	log, err := audit.NewLog(filepath.Join(dir, "log"), key)
	if err != nil {
		t.Fatalf("NewLog: %v", err)
	}
	defer log.Close()

	for _, e := range []schema.TelemetryEvent{
		{SessionID: "s1", EventType: schema.EventPreTool, Subagent: "researcher"},
		{SessionID: "s1", EventType: schema.EventPreTool},
		{SessionID: "s2", EventType: schema.EventPreTool},
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

	evs, err := store.EventsForSession("s1")
	if err != nil {
		t.Fatalf("EventsForSession: %v", err)
	}
	if len(evs) != 2 {
		t.Fatalf("want 2 events for s1, got %d", len(evs))
	}
	if evs[0].Seq > evs[1].Seq {
		t.Fatalf("events not seq-ordered: %d then %d", evs[0].Seq, evs[1].Seq)
	}
	if evs[0].Subagent != "researcher" {
		t.Fatalf("Subagent not preserved through raw round-trip: %q", evs[0].Subagent)
	}
}
