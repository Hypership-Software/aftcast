package insights

import (
	"strings"
	"testing"
	"time"

	"github.com/Hypership-Software/aftcast/internal/audit"
	"github.com/Hypership-Software/aftcast/internal/schema"
	"github.com/Hypership-Software/aftcast/internal/telemetry"
)

var distillNow = time.Date(2026, 7, 15, 18, 0, 0, 0, time.UTC)

// distillSession builds one session's worth of failing "cd" calls, mirroring
// cdFailureSession but adding taint and per-failure prompt ids — the two axes
// this bundle gates and threads through that cdFailureSession doesn't need.
func distillSession(session, ts string, failures int, tainted bool, promptIDs ...string) []schema.TelemetryEvent {
	events := []schema.TelemetryEvent{{
		SessionID: session, TS: ts, EventType: schema.EventPreTool,
		ToolClass: schema.ClassExec, ToolRaw: "Bash", Verbs: []string{"cd"}, Taint: tainted,
	}}
	for i := 0; i < failures; i++ {
		e := schema.TelemetryEvent{
			SessionID: session, TS: ts, EventType: schema.EventPostTool,
			ToolClass: schema.ClassExec, ToolRaw: "Bash", Verbs: []string{"cd"},
			ToolOK: schema.OutcomeFailed, BashExitCode: 1,
		}
		if i < len(promptIDs) {
			e.PromptID = promptIDs[i]
		}
		events = append(events, e)
	}
	return events
}

func cleanDistillFixture(t *testing.T) *telemetry.Store {
	t.Helper()
	var events []schema.TelemetryEvent
	events = append(events, distillSession("s1", "2026-07-13T09:00:00Z", 2, false, "p1", "p2")...)
	events = append(events, distillSession("s2", "2026-07-14T10:00:00Z", 1, false, "p3")...)
	return coachStore(t, events)
}

func oneTaintedDistillFixture(t *testing.T) *telemetry.Store {
	t.Helper()
	var events []schema.TelemetryEvent
	events = append(events, distillSession("s1", "2026-07-13T09:00:00Z", 2, false, "p1", "p2")...)
	events = append(events, distillSession("s2", "2026-07-14T10:00:00Z", 1, true, "p3")...)
	return coachStore(t, events)
}

func allTaintedDistillFixture(t *testing.T) *telemetry.Store {
	t.Helper()
	var events []schema.TelemetryEvent
	events = append(events, distillSession("s1", "2026-07-13T09:00:00Z", 2, true, "p1", "p2")...)
	events = append(events, distillSession("s2", "2026-07-14T10:00:00Z", 1, true, "p3")...)
	return coachStore(t, events)
}

func TestCoachDistillBundleCoordinatesAndScaffold(t *testing.T) {
	var out strings.Builder
	rep := audit.Report{OK: true, Count: 42}
	if err := CoachDistill(cleanDistillFixture(t), "cd-exit-1", rep, &out, distillNow); err != nil {
		t.Fatalf("CoachDistill: %v", err)
	}
	got := out.String()
	for _, want := range []string{
		"# Distill a skill from a recurring failure",
		"chain verified",
		"session s1",
		"p1, p2",
		"session s2",
		"p3",
		"no commands or content were captured",
		"## Drafting scaffold",
		"SKILL.md",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("bundle missing %q in:\n%s", want, got)
		}
	}
}

func TestCoachDistillExcludesTaintedSessions(t *testing.T) {
	var out strings.Builder
	rep := audit.Report{OK: true, Count: 42}
	if err := CoachDistill(oneTaintedDistillFixture(t), "cd-exit-1", rep, &out, distillNow); err != nil {
		t.Fatalf("CoachDistill: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "session s2") || !strings.Contains(got, "touched untrusted input") {
		t.Errorf("tainted session s2 should be named with an exclusion reason:\n%s", got)
	}
	if strings.Contains(got, "p3") {
		t.Errorf("tainted session's prompt ids must not be offered for distillation:\n%s", got)
	}
	if !strings.Contains(got, "## Drafting scaffold") {
		t.Errorf("bundle with at least one clean session must still scaffold:\n%s", got)
	}
}

func TestCoachDistillAllTaintedRefusesScaffold(t *testing.T) {
	var out strings.Builder
	rep := audit.Report{OK: true, Count: 42}
	if err := CoachDistill(allTaintedDistillFixture(t), "cd-exit-1", rep, &out, distillNow); err != nil {
		t.Fatalf("CoachDistill: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "cannot be distilled") {
		t.Errorf("all-tainted cluster must refuse distillation in plain English:\n%s", got)
	}
	if strings.Contains(got, "Drafting") {
		t.Errorf("all-tainted cluster must not offer a drafting scaffold:\n%s", got)
	}
}

func TestCoachDistillBrokenChainIsLoud(t *testing.T) {
	var out strings.Builder
	rep := audit.Report{OK: false, Count: 10, BadSeq: 7, Detail: "hash mismatch (record was altered)"}
	if err := CoachDistill(cleanDistillFixture(t), "cd-exit-1", rep, &out, distillNow); err != nil {
		t.Fatalf("CoachDistill: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "ATTESTATION FAILED") || !strings.Contains(got, "record 7") {
		t.Fatalf("broken chain must be loud, got:\n%s", got)
	}
	if strings.Contains(got, "chain verified") {
		t.Error("broken chain must not read as verified")
	}
	failedIdx := strings.Index(got, "ATTESTATION FAILED")
	sessionIdx := strings.Index(got, "session s1")
	if sessionIdx < 0 {
		t.Fatal("fixture should still surface session s1's coordinates")
	}
	if failedIdx > sessionIdx {
		t.Error("ATTESTATION FAILED must appear before any session id")
	}
}

func TestCoachDistillUnknownSlug(t *testing.T) {
	var out strings.Builder
	rep := audit.Report{OK: true, Count: 42}
	err := CoachDistill(cleanDistillFixture(t), "nope", rep, &out, distillNow)
	if err == nil {
		t.Fatalf("want error for unknown fingerprint")
	}
	if !strings.Contains(err.Error(), "nope") || !strings.Contains(err.Error(), "aftcast coach") {
		t.Fatalf("error should name the id and point at aftcast coach: %v", err)
	}
}
