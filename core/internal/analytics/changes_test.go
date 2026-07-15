package analytics

import (
	"reflect"
	"testing"

	"github.com/Hypership-Software/aftcast/internal/schema"
)

func TestObservedChangesPairsSuccessfulWritesAndTracksCoverage(t *testing.T) {
	events := []schema.TelemetryEvent{
		changePre("p1", "a.go", 5, 2),
		changePost("p1", schema.OutcomeOK, 10),
		changePre("p2", "a.go", 2, 1),
		changePost("p2", schema.OutcomeOK, 20),
		changePre("p3", "failed.go", 9, 9),
		changePost("p3", schema.OutcomeFailed, 30),
		changePre("p4", "orphan.go", 1, 0),
		{V: schema.ObservationVersion, EventType: schema.EventPreTool, ToolUseID: "p5", ToolClass: schema.ClassFileWrite, Files: []string{"b.go"}},
		changePost("p5", schema.OutcomeOK, 40),
	}

	got := ObservedChanges(events)
	if got.LinesAdded != 7 || got.LinesRemoved != 3 || got.Covered {
		t.Fatalf("changes = %+v", got)
	}
	if paths := got.Paths(); !reflect.DeepEqual(paths, []string{"a.go", "b.go"}) {
		t.Fatalf("paths = %v", paths)
	}
	if len(got.Files) != 2 || got.Files[0].LinesAdded != 7 || got.Files[0].LinesRemoved != 3 {
		t.Fatalf("files = %+v", got.Files)
	}
}

func TestObservedChangesCoveredForCompleteV3Session(t *testing.T) {
	events := []schema.TelemetryEvent{
		{V: schema.ObservationVersion, EventType: schema.EventSessionStart},
		changePre("p1", "a.go", 0, 0),
		changePost("p1", schema.OutcomeOK, 0),
	}
	got := ObservedChanges(events)
	if !got.Covered || len(got.Files) != 1 || got.LinesAdded != 0 || got.LinesRemoved != 0 {
		t.Fatalf("changes = %+v", got)
	}
}

func changePre(id, file string, added, removed int) schema.TelemetryEvent {
	return schema.TelemetryEvent{
		V: schema.ObservationVersion, EventType: schema.EventPreTool, ToolUseID: id,
		ToolClass: schema.ClassFileWrite, Files: []string{file}, Operation: schema.OperationEdit,
		ChangeStats: &schema.ChangeStats{LinesAdded: added, LinesRemoved: removed},
	}
}

func changePost(id string, outcome schema.ToolOutcome, latency int64) schema.TelemetryEvent {
	return schema.TelemetryEvent{
		V: schema.ObservationVersion, EventType: schema.EventPostTool, ToolUseID: id,
		ToolOK: outcome, LatencyMS: latency,
	}
}
