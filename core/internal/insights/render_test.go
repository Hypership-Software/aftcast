package insights

import (
	"strings"
	"testing"

	"github.com/Hypership-Software/atlas/internal/analytics"
	"github.com/Hypership-Software/atlas/internal/schema"
	"github.com/Hypership-Software/atlas/internal/telemetry"
)

func TestToStatSplitsSkills(t *testing.T) {
	st := toStat(telemetry.Session{Outcome: "success", SkillsUsed: "a,b", OneShot: true})
	if len(st.Skills) != 2 || st.Skills[0] != "a" {
		t.Fatalf("skills not split: %v", st.Skills)
	}
	if st.Outcome != analytics.Success {
		t.Fatalf("outcome not mapped: %v", st.Outcome)
	}
}

func TestAggregateMatchesProductivity(t *testing.T) {
	sessions := []telemetry.Session{
		{SessionID: "s1", Outcome: "success", OneShot: true, TaskType: "feature", DangerDetected: 1},
		{SessionID: "s2", Outcome: "failure", CorrectionTurns: 2, TaskType: "bugfix", DangerDetected: 2},
	}
	agg := aggregate(sessions)
	stats := []analytics.SessionStat{toStat(sessions[0]), toStat(sessions[1])}
	if agg.profile.OneShotRate != analytics.Productivity(stats).OneShotRate {
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
	h := renderHeader(aggregate([]telemetry.Session{{Outcome: "success", OneShot: true}}))
	if !strings.Contains(h, "one-shot") {
		t.Fatalf("header missing one-shot: %q", h)
	}
	if !strings.Contains(renderEmpty(), "No sessions") {
		t.Fatalf("empty state missing copy")
	}
}

func TestDetailBodyRawShowsSubagent(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	sess := telemetry.Session{SessionID: "s1", Harness: "claudecode", TaskType: "feature"}
	events := []schema.TelemetryEvent{{SessionID: "s1", ToolRaw: "WebFetch", Subagent: "researcher"}}
	if !strings.Contains(detailBody(sess, events, false), "WebFetch") {
		t.Fatalf("summary missing tool")
	}
	raw := detailBody(sess, events, true)
	if !strings.Contains(raw, "researcher") || !strings.Contains(raw, "subagent") {
		t.Fatalf("raw JSON missing subagent field: %q", raw)
	}
}
