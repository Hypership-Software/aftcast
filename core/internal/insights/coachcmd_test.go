package insights

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Hypership-Software/aftcast/internal/audit"
	"github.com/Hypership-Software/aftcast/internal/schema"
	"github.com/Hypership-Software/aftcast/internal/telemetry"
)

var coachNow = time.Date(2026, 7, 15, 18, 0, 0, 0, time.UTC)

func coachStore(t *testing.T, events []schema.TelemetryEvent) *telemetry.Store {
	t.Helper()
	dir := t.TempDir()
	log, err := audit.NewLog(filepath.Join(dir, "log"), []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("NewLog: %v", err)
	}
	defer log.Close()
	for _, e := range events {
		if err := log.Record(e); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}
	store, err := telemetry.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	if err := store.Project(log); err != nil {
		t.Fatalf("Project: %v", err)
	}
	return store
}

func cdFailureSession(session, ts string, failures int) []schema.TelemetryEvent {
	events := []schema.TelemetryEvent{{
		SessionID: session, TS: ts, EventType: schema.EventPreTool,
		ToolClass: schema.ClassExec, ToolRaw: "Bash", Verbs: []string{"cd"},
	}}
	for i := 0; i < failures; i++ {
		events = append(events, schema.TelemetryEvent{
			SessionID: session, TS: ts, EventType: schema.EventPostTool,
			ToolClass: schema.ClassExec, ToolRaw: "Bash", Verbs: []string{"cd"},
			ToolOK: schema.OutcomeFailed, BashExitCode: 1,
		})
	}
	return events
}

func worthFixingFixture(t *testing.T) *telemetry.Store {
	t.Helper()
	var events []schema.TelemetryEvent
	events = append(events, cdFailureSession("s1", "2026-07-13T09:00:00Z", 2)...)
	events = append(events, cdFailureSession("s2", "2026-07-14T10:00:00Z", 1)...)
	events = append(events, cdFailureSession("s3", "2026-07-15T11:00:00Z", 1)...)
	return coachStore(t, events)
}

func TestCoachReportListsWorthFixing(t *testing.T) {
	var out strings.Builder
	if err := CoachReport(worthFixingFixture(t), &out, coachNow); err != nil {
		t.Fatalf("CoachReport: %v", err)
	}
	for _, want := range []string{
		"Worth a permanent fix",
		"Your agents failed to change directory 4 times across 3 sessions on 3 days this week.",
		"gated coach export cd-exit-1",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("report missing %q in:\n%s", want, out.String())
		}
	}
}

func TestCoachReportNothingWorthFixing(t *testing.T) {
	store := coachStore(t, cdFailureSession("s1", "2026-07-13T09:00:00Z", 2))
	var out strings.Builder
	if err := CoachReport(store, &out, coachNow); err != nil {
		t.Fatalf("CoachReport: %v", err)
	}
	if !strings.Contains(out.String(), "Nothing has crossed the worth-a-permanent-fix line this week.") {
		t.Fatalf("report = %q", out.String())
	}
	if !strings.Contains(out.String(), "3+ sessions on 2+ days") {
		t.Fatalf("report should explain the gate:\n%s", out.String())
	}
}

func TestCoachExportBundle(t *testing.T) {
	var out strings.Builder
	if err := CoachExport(worthFixingFixture(t), "cd-exit-1", &out, coachNow); err != nil {
		t.Fatalf("CoachExport: %v", err)
	}
	for _, want := range []string{
		"# Worth a permanent fix",
		"Your agents failed to change directory 4 times across 3 sessions between July 13 and July 15.",
		"No commands were captured.",
		"## The sessions",
		"- July 13 — session s1, 2 failures",
		"- July 15 — session s3, 1 failure",
		"## What to do with this",
		"1. Fix the root cause — a product or environment change",
		"5. A rule in CLAUDE.md",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("bundle missing %q in:\n%s", want, out.String())
		}
	}
	for _, banned := range []string{"post_tool", "tool_class", "exit code", "exec ·"} {
		if strings.Contains(out.String(), banned) {
			t.Errorf("bundle leaks internal register %q", banned)
		}
	}
}

func TestCoachExportUnknownSlug(t *testing.T) {
	var out strings.Builder
	err := CoachExport(worthFixingFixture(t), "nope", &out, coachNow)
	if err == nil {
		t.Fatalf("want error for unknown fingerprint")
	}
	if !strings.Contains(err.Error(), "nope") || !strings.Contains(err.Error(), "gated coach") {
		t.Fatalf("error should name the id and point at gated coach: %v", err)
	}
}
