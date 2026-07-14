package insights

import (
	"strings"
	"testing"
	"time"

	"github.com/Hypership-Software/atlas/internal/analytics"
	"github.com/Hypership-Software/atlas/internal/schema"
	"github.com/Hypership-Software/atlas/internal/telemetry"
)

func sampleAgg() aggregates {
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	sessions := []telemetry.Session{
		{
			SessionID:     "s1",
			User:          "dev",
			TaskType:      "testing",
			Outcome:       "success",
			CleanDelivery: true,
			TurnCount:     3,
			ToolCalls:     10,
			Started:       now.Add(-2 * time.Hour).Format(time.RFC3339Nano),
		},
		{
			SessionID:       "s2",
			User:            "dev",
			TaskType:        "testing",
			Outcome:         "success",
			CorrectionTurns: 2,
			DangerDetected:  3,
			Taint:           true,
			TurnCount:       5,
			ToolCalls:       20,
			Started:         now.Add(-3 * time.Hour).Format(time.RFC3339Nano),
		},
	}
	return aggregate(sessions, now)
}

func TestOverviewIsPlainLanguage(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	out := renderList(sampleAgg(), "TABLE")
	for _, banned := range []string{"corr/deliv", "clean_delivery", "taint", "danger ", "unknown"} {
		if strings.Contains(out, banned) {
			t.Errorf("overview leaked code word %q:\n%s", banned, out)
		}
	}
	for _, want := range []string{"Landed clean", "fixes / session", "untrusted input"} {
		if !strings.Contains(out, want) {
			t.Errorf("overview missing plain-language %q:\n%s", want, out)
		}
	}
}

func TestToStatSplitsSkills(t *testing.T) {
	st := toStat(telemetry.Session{Outcome: "success", SkillsUsed: "a,b", CleanDelivery: true})
	if len(st.Skills) != 2 || st.Skills[0] != "a" {
		t.Fatalf("skills not split: %v", st.Skills)
	}
	if st.Outcome != analytics.Success {
		t.Fatalf("outcome not mapped: %v", st.Outcome)
	}
}

func TestAggregateMatchesProductivity(t *testing.T) {
	sessions := []telemetry.Session{
		{SessionID: "s1", Outcome: "success", CleanDelivery: true, TaskType: "feature", DangerDetected: 1},
		{SessionID: "s2", Outcome: "failure", CorrectionTurns: 2, TaskType: "bugfix", DangerDetected: 2},
	}
	agg := aggregate(sessions, time.Now())
	stats := []analytics.SessionStat{toStat(sessions[0]), toStat(sessions[1])}
	if agg.profile.CleanDeliveryRate != analytics.Productivity(stats).CleanDeliveryRate {
		t.Fatalf("aggregate profile disagrees with Productivity")
	}
	if agg.danger != 3 {
		t.Fatalf("danger tally = %d, want 3", agg.danger)
	}
	if len(agg.taskMix) != 2 {
		t.Fatalf("task mix len = %d, want 2", len(agg.taskMix))
	}
}

func TestRenderHeaderAndEmpty(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	h := renderHeader(aggregate([]telemetry.Session{{Outcome: "success", CleanDelivery: true}}, time.Now()))
	if !strings.Contains(h, "clean") {
		t.Fatalf("header missing clean-delivery rate: %q", h)
	}
	if !strings.Contains(renderEmpty(), "No sessions") {
		t.Fatalf("empty state missing copy")
	}
}

func TestDetailBodyRawShowsSubagent(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	sess := telemetry.Session{SessionID: "s1", Harness: "claudecode", TaskType: "feature"}
	pre := schema.TelemetryEvent{SessionID: "s1", EventType: schema.EventPreTool, ToolClass: schema.ClassNetFetch,
		ToolUseID: "t1", Domain: "example.com", Subagent: "researcher"}
	post := schema.TelemetryEvent{SessionID: "s1", EventType: schema.EventPostTool, ToolUseID: "t1", ToolOK: schema.OutcomeOK}
	events := []schema.TelemetryEvent{pre, post}
	if !strings.Contains(detailBody(sess, events, false), "fetched") {
		t.Fatalf("trace missing rendered verb")
	}
	raw := detailBody(sess, events, true)
	if !strings.Contains(raw, "researcher") || !strings.Contains(raw, "subagent") {
		t.Fatalf("raw JSON missing subagent field: %q", raw)
	}
}

func TestRenderTraceHasVerdictAndNoEmptyFields(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	sess := telemetry.Session{SessionID: "s", TaskType: "testing", Outcome: "success",
		DurationMS: 1080000, ToolCalls: 165, FilesTouched: 12, Taint: true}
	pre := schema.TelemetryEvent{EventType: schema.EventPreTool, ToolClass: schema.ClassExec, ToolUseID: "t1", Verbs: []string{"go"}}
	post := schema.TelemetryEvent{EventType: schema.EventPostTool, ToolUseID: "t1", LatencyMS: 9109, ToolOK: schema.OutcomeOK}
	out := renderTrace(sess, []schema.TelemetryEvent{pre, post})
	if !strings.Contains(out, "untrusted input") { // taint flag in header
		t.Error("verdict header missing untrusted-input flag")
	}
	if strings.Contains(out, "risk=") || strings.Contains(out, "sub=") || strings.Contains(out, "[t0]") {
		t.Errorf("trace leaked raw debug fields:\n%s", out)
	}
	if !strings.Contains(out, "ran") || !strings.Contains(out, "9.1s") {
		t.Errorf("trace missing paired call / duration:\n%s", out)
	}
}
