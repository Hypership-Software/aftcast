package insights

import (
	"strings"
	"testing"
	"time"

	"github.com/Hypership-Software/aftcast/internal/audit"
	"github.com/Hypership-Software/aftcast/internal/schema"
)

var evidenceNow = time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)

// evidenceSession builds one session that lights up every RepoEvidence field
// this document renders: two turns, the first ending in a failed exec (one
// correction), a danger-flagged tool call, a taint marker, and a two-file
// write (two files changed) — plus an optional delivery signal.
func evidenceSession(session, ts string, shipped bool) []schema.TelemetryEvent {
	events := []schema.TelemetryEvent{
		{SessionID: session, TS: ts, EventType: schema.EventUserPrompt, TurnIndex: 0},
		{SessionID: session, TS: ts, EventType: schema.EventPreTool, TurnIndex: 0,
			ToolClass: schema.ClassExec, Risk: schema.RiskDanger, Taint: true},
		{SessionID: session, TS: ts, EventType: schema.EventPostTool, TurnIndex: 0,
			ToolClass: schema.ClassExec, ToolOK: schema.OutcomeFailed},
		{SessionID: session, TS: ts, EventType: schema.EventUserPrompt, TurnIndex: 1},
		{SessionID: session, TS: ts, EventType: schema.EventPreTool, TurnIndex: 1,
			ToolClass: schema.ClassFileWrite, Files: []string{"a.txt", "b.txt"}},
		{SessionID: session, TS: ts, EventType: schema.EventPostTool, TurnIndex: 1,
			ToolClass: schema.ClassFileWrite, ToolOK: schema.OutcomeOK},
	}
	if shipped {
		events[len(events)-1].DeliverySignal = schema.DeliveryGitPush
	}
	return events
}

// plainSession is evidenceSession's clean twin: one turn, no failure, no
// danger, no taint, no file writes — the axes evidenceSession exercises.
func plainSession(session, ts string, shipped bool) []schema.TelemetryEvent {
	events := []schema.TelemetryEvent{
		{SessionID: session, TS: ts, EventType: schema.EventUserPrompt, TurnIndex: 0},
		{SessionID: session, TS: ts, EventType: schema.EventPreTool, TurnIndex: 0,
			ToolClass: schema.ClassExec},
		{SessionID: session, TS: ts, EventType: schema.EventPostTool, TurnIndex: 0,
			ToolClass: schema.ClassExec, ToolOK: schema.OutcomeOK},
	}
	if shipped {
		events[len(events)-1].DeliverySignal = schema.DeliveryGitPush
	}
	return events
}

func TestEvidenceReportRendersRowsAndScaffold(t *testing.T) {
	since := evidenceNow.AddDate(0, 0, -14)
	var events []schema.TelemetryEvent
	events = append(events, evidenceSession("session-shipped-dirty", "2026-07-20T09:00:00Z", true)...)
	events = append(events, plainSession("session-plain", "2026-07-21T09:00:00Z", false)...)
	store := coachStore(t, events)
	rep := audit.Report{OK: true, Count: 42}

	var out strings.Builder
	if err := EvidenceReport(store, since, rep, &out, evidenceNow); err != nil {
		t.Fatalf("EvidenceReport: %v", err)
	}
	got := out.String()
	for _, want := range []string{
		"# Evidence of agent work",
		"chain verified across 42 records",
		"counts, dates, rates, repository names, and session references",
		"no prompts, code, or command content",
		"## other work",
		"shipped in 1 of 2 sessions",
		"1 correction",
		"2 files changed",
		"1 flagged operation was recorded",
		"1 session carried taint from an untrusted source",
		"## Narrative scaffold",
		"session-shipped-dirty",
		"session-plain",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("report missing %q in:\n%s", want, got)
		}
	}
}

func TestEvidenceReportEmptyWindowIsHonest(t *testing.T) {
	store := coachStore(t, plainSession("too-old", "2026-06-01T09:00:00Z", true))
	since := evidenceNow.AddDate(0, 0, -14)
	rep := audit.Report{OK: true, Count: 5}

	var out strings.Builder
	if err := EvidenceReport(store, since, rep, &out, evidenceNow); err != nil {
		t.Fatalf("EvidenceReport: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "No captured sessions started in this period.") {
		t.Fatalf("report should be honest about an empty window:\n%s", got)
	}
	if strings.Contains(got, "## Narrative scaffold") {
		t.Errorf("empty window must not offer a scaffold:\n%s", got)
	}
}

func TestEvidenceReportBrokenChainRefuses(t *testing.T) {
	store := coachStore(t, evidenceSession("session-shipped-dirty", "2026-07-20T09:00:00Z", true))
	since := evidenceNow.AddDate(0, 0, -14)
	rep := audit.Report{OK: false, Count: 6, BadSeq: 7, Detail: "hash mismatch (record was altered)"}

	var out strings.Builder
	if err := EvidenceReport(store, since, rep, &out, evidenceNow); err != nil {
		t.Fatalf("EvidenceReport: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "ATTESTATION FAILED") || !strings.Contains(got, "record 7") {
		t.Fatalf("broken chain must be loud, got:\n%s", got)
	}
	if strings.Contains(got, "chain verified") {
		t.Error("broken chain must not read as verified")
	}
	if strings.Contains(got, "## other work") || strings.Contains(got, "session-shipped-dirty") {
		t.Errorf("broken chain must not surface any repository rows:\n%s", got)
	}
	if strings.Contains(got, "## Narrative scaffold") {
		t.Errorf("broken chain must not offer a narrative scaffold:\n%s", got)
	}
	failedIdx := strings.Index(got, "ATTESTATION FAILED")
	refusalIdx := strings.Index(got, "cannot be produced")
	if failedIdx < 0 || refusalIdx < 0 || failedIdx > refusalIdx {
		t.Error("ATTESTATION FAILED must appear before the refusal sentence")
	}
}

func TestEvidenceReportDangerAndTaintOnlyWhenPresent(t *testing.T) {
	since := evidenceNow.AddDate(0, 0, -14)
	rep := audit.Report{OK: true, Count: 3}

	cleanStore := coachStore(t, plainSession("session-clean", "2026-07-20T09:00:00Z", true))
	var cleanOut strings.Builder
	if err := EvidenceReport(cleanStore, since, rep, &cleanOut, evidenceNow); err != nil {
		t.Fatalf("EvidenceReport: %v", err)
	}
	clean := cleanOut.String()
	for _, banned := range []string{"flagged operation", "carried taint"} {
		if strings.Contains(clean, banned) {
			t.Errorf("clean fixture must not mention %q:\n%s", banned, clean)
		}
	}

	dirtyStore := coachStore(t, evidenceSession("session-dirty", "2026-07-20T09:00:00Z", true))
	var dirtyOut strings.Builder
	if err := EvidenceReport(dirtyStore, since, rep, &dirtyOut, evidenceNow); err != nil {
		t.Fatalf("EvidenceReport: %v", err)
	}
	dirty := dirtyOut.String()
	for _, want := range []string{"flagged operation", "carried taint"} {
		if !strings.Contains(dirty, want) {
			t.Errorf("dirty fixture missing %q:\n%s", want, dirty)
		}
	}
}
