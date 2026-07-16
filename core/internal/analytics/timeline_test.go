package analytics

import (
	"testing"

	"github.com/Hypership-Software/aftcast/internal/schema"
)

func tsEvent(eventType schema.EventType, ts string) schema.TelemetryEvent {
	return schema.TelemetryEvent{EventType: eventType, TS: ts}
}

func TestObservedTimelineSplitsActiveFromWaiting(t *testing.T) {
	events := []schema.TelemetryEvent{
		tsEvent(schema.EventUserPrompt, "2026-07-16T08:00:00Z"),
		tsEvent(schema.EventPreTool, "2026-07-16T08:00:30Z"),
		tsEvent(schema.EventPostTool, "2026-07-16T08:00:40Z"),
		tsEvent(schema.EventStop, "2026-07-16T08:01:00Z"),
		tsEvent(schema.EventUserPrompt, "2026-07-16T08:11:00Z"),
		tsEvent(schema.EventStop, "2026-07-16T08:11:30Z"),
	}
	tl := ObservedTimeline(events)
	if tl.ActiveMS != 90_000 {
		t.Fatalf("ActiveMS = %d, want 90000", tl.ActiveMS)
	}
	if tl.WaitingMS != 600_000 {
		t.Fatalf("WaitingMS = %d, want 600000", tl.WaitingMS)
	}
}

func TestObservedTimelineBackgroundResumeIsActive(t *testing.T) {
	events := []schema.TelemetryEvent{
		tsEvent(schema.EventStop, "2026-07-16T08:00:00Z"),
		tsEvent(schema.EventPreTool, "2026-07-16T08:05:00Z"),
	}
	tl := ObservedTimeline(events)
	if tl.ActiveMS != 300_000 || tl.WaitingMS != 0 {
		t.Fatalf("timeline = %+v, want 5m active after autonomous resume", tl)
	}
}

func TestObservedTimelineTrailingStopIsWaiting(t *testing.T) {
	events := []schema.TelemetryEvent{
		tsEvent(schema.EventStop, "2026-07-16T08:00:00Z"),
		tsEvent(schema.EventStop, "2026-07-16T08:15:00Z"),
	}
	tl := ObservedTimeline(events)
	if tl.WaitingMS != 900_000 || tl.ActiveMS != 0 {
		t.Fatalf("timeline = %+v, want 15m waiting between stops", tl)
	}
}

func TestObservedTimelineSkipsUnreadableTimestamps(t *testing.T) {
	events := []schema.TelemetryEvent{
		tsEvent(schema.EventUserPrompt, "2026-07-16T08:00:00Z"),
		tsEvent(schema.EventPreTool, ""),
		tsEvent(schema.EventStop, "2026-07-16T08:01:00Z"),
		tsEvent(schema.EventUserPrompt, "2026-07-16T07:59:00Z"),
	}
	tl := ObservedTimeline(events)
	if tl.ActiveMS != 60_000 || tl.WaitingMS != 0 {
		t.Fatalf("timeline = %+v, want 1m active and clock skew ignored", tl)
	}
}
