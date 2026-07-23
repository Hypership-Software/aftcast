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

// promptCopyFixture exercises promptCoordinates' three branches directly:
// a single prompt id, several prompt ids, and none at all.
func promptCopyFixture(t *testing.T) *telemetry.Store {
	t.Helper()
	var events []schema.TelemetryEvent
	events = append(events, distillSession("solo-prompt", "2026-07-13T09:00:00Z", 1, false, "p3")...)
	events = append(events, distillSession("multi-prompt", "2026-07-14T09:00:00Z", 3, false, "p1", "p2", "p5")...)
	events = append(events, distillSession("no-prompt", "2026-07-15T09:00:00Z", 2, false)...)
	return coachStore(t, events)
}

// longIDDistillFixture uses a session id longer than shortID's 8-char cutoff
// so a regression to the truncated id in Coordinates is directly observable.
func longIDDistillFixture(t *testing.T) *telemetry.Store {
	t.Helper()
	return coachStore(t, distillSession("session-with-a-very-long-identifier", "2026-07-13T09:00:00Z", 1, false, "p9"))
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
		"2 failures at prompts p1, p2",
		"session s2",
		"1 failure at prompt p3",
		"no commands or content were captured",
		"## Drafting scaffold",
		"SKILL.md",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("bundle missing %q in:\n%s", want, got)
		}
	}
}

func TestCoachDistillCoordinatesUseFullSessionID(t *testing.T) {
	var out strings.Builder
	rep := audit.Report{OK: true, Count: 42}
	if err := CoachDistill(longIDDistillFixture(t), "cd-exit-1", rep, &out, distillNow); err != nil {
		t.Fatalf("CoachDistill: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "session session-with-a-very-long-identifier —") {
		t.Errorf("coordinates must carry the full session id (the reader substitutes it into <session-id>):\n%s", got)
	}
	if strings.Contains(got, "session session- —") {
		t.Errorf("coordinates must not truncate the session id to shortID's 8-char prefix:\n%s", got)
	}
}

func TestCoachDistillCoordinatesPromptCopy(t *testing.T) {
	var out strings.Builder
	rep := audit.Report{OK: true, Count: 42}
	if err := CoachDistill(promptCopyFixture(t), "cd-exit-1", rep, &out, distillNow); err != nil {
		t.Fatalf("CoachDistill: %v", err)
	}
	got := out.String()
	for _, want := range []string{
		"1 failure at prompt p3",
		"3 failures at prompts p1, p2, p5",
		"its failures carry no prompt references",
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
	if !strings.Contains(got, "cannot be distilled") {
		t.Errorf("broken chain must refuse in plain English, got:\n%s", got)
	}
	// The taint gate's own inputs come from the record that just failed
	// verification, so a broken chain must withhold coordinates and the
	// scaffold exactly like an all-tainted cluster does — not just move the
	// banner ahead of them.
	if strings.Contains(got, "## Coordinates") || strings.Contains(got, "session s1") || strings.Contains(got, "session s2") {
		t.Errorf("broken chain must not surface any session coordinates:\n%s", got)
	}
	if strings.Contains(got, "## Drafting scaffold") || strings.Contains(got, "SKILL.md") {
		t.Errorf("broken chain must not offer a drafting scaffold:\n%s", got)
	}
	failedIdx := strings.Index(got, "ATTESTATION FAILED")
	if failedIdx < 0 || failedIdx > strings.Index(got, "cannot be distilled") {
		t.Error("ATTESTATION FAILED must appear before the refusal sentence")
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
