package insights

import (
	"strings"
	"testing"
	"time"

	"github.com/Hypership-Software/atlas/internal/analytics"
	"github.com/Hypership-Software/atlas/internal/schema"
	"github.com/Hypership-Software/atlas/internal/telemetry"

	"github.com/charmbracelet/lipgloss"
)

func TestMetricLabelsAlignToFixedWidth(t *testing.T) {
	// Colour-safe alignment: the styled meter label must occupy a constant
	// display width regardless of length or ANSI styling, so the three meters
	// line up. lipgloss.Width strips ANSI, so this holds under colour too.
	for _, s := range []string{"Shipped", "Intervention", "Risk"} {
		if w := lipgloss.Width(metricLabel(s)); w != metricLabelWidth {
			t.Errorf("metricLabel(%q) display width = %d, want %d", s, w, metricLabelWidth)
		}
	}
}

func sampleAgg() aggregates {
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	sessions := []telemetry.Session{
		{
			SessionID:      "s1",
			User:           "kyled",
			TaskType:       "testing",
			Outcome:        "success",
			CleanDelivery:  true,
			CaptureVersion: 2,
			FilesChanged:   1,
			Shipped:        true,
			TurnCount:      3,
			ToolCalls:      10,
			Started:        now.Add(-2 * time.Hour).Format(time.RFC3339Nano),
		},
		{
			SessionID:       "s2",
			User:            "kyled",
			TaskType:        "testing",
			Outcome:         "success",
			CaptureVersion:  2,
			FilesChanged:    1,
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
	out := renderList(sampleAgg(), analytics.PlanAssociation{}, "TABLE")
	for _, banned := range []string{"corr/deliv", "clean_delivery", "taint", "danger ", "unknown", "Landed clean"} {
		if strings.Contains(out, banned) {
			t.Errorf("overview leaked code word %q:\n%s", banned, out)
		}
	}
	for _, want := range []string{"Shipped", "Intervention", "untrusted input"} {
		if !strings.Contains(out, want) {
			t.Errorf("overview missing plain-language %q:\n%s", want, out)
		}
	}
}

func TestRenderCoachStates(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	tests := []struct {
		name   string
		input  analytics.PlanAssociation
		want   []string
		banned []string
	}{
		{"zero", analytics.PlanAssociation{Status: analytics.CoachLearning},
			[]string{"What's moving your needle", "Atlas is learning", "no comparable delivery sessions yet"}, []string{"Try next"}},
		{"learning", analytics.PlanAssociation{Status: analytics.CoachLearning, Window: 12, TaskType: "feature", Total: 12, Planned: 4, Direct: 8},
			[]string{"latest 12 comparable sessions", "Atlas is learning", "12 comparable", "plan-first 4", "direct-to-edit 8"}, []string{"Try next"}},
		{"no pattern", analytics.PlanAssociation{Status: analytics.CoachNoPattern, Window: 20, TaskType: "feature", Total: 20, Planned: 10, Direct: 10, PlannedRate: .6, DirectRate: .5},
			[]string{"No reliable plan-first pattern yet", "60%", "50%"}, []string{"Try next"}},
		{"negative observation", analytics.PlanAssociation{Status: analytics.CoachNoPattern, Direction: analytics.AssociationNegative, Window: 20, TaskType: "feature", Total: 20, Planned: 10, Direct: 10, PlannedRate: .4, DirectRate: .7},
			[]string{"associated with fewer shipped sessions", "feature work", "40%", "70%", "n=20"}, []string{"No reliable plan-first pattern yet", "Try next", "Plan before editing"}},
		{"recommend", analytics.PlanAssociation{Status: analytics.CoachRecommend, Window: 24, TaskType: "feature", Total: 24, Planned: 10, Direct: 14, PlannedRate: .8, DirectRate: .55},
			[]string{"latest 24 comparable sessions", "associated with more shipped sessions", "80%", "55%", "Try next", "Plan before editing"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderCoach(tt.input)
			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Fatalf("missing %q:\n%s", want, got)
				}
			}
			for _, banned := range append(tt.banned, "cause", "led to", "resulted in", "results in") {
				if strings.Contains(strings.ToLower(got), banned) {
					t.Fatalf("render contained %q:\n%s", banned, got)
				}
			}
		})
	}
}

func TestHelpIncludesExactCoachDefinitions(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	lines := map[string]bool{}
	for _, line := range strings.Split(renderHelp(), "\n") {
		lines[line] = true
	}
	for _, want := range []string{
		"Shipped = a successful git push in a delivery session",
		"Delivery session = changed files or successfully pushed, captured with v2 telemetry",
		"Observed plan-first = explicit planning, or a completed preparatory prompt before editing",
	} {
		if !lines[want] {
			t.Fatalf("help missing exact definition line %q:\n%s", want, renderHelp())
		}
	}
}

func TestVerdictOutcomePrefersShippedOverContradictoryOutcome(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	got := verdictOutcome(telemetry.Session{Shipped: true, Outcome: "failure", CorrectionTurns: 3})
	if got != "↑ shipped" {
		t.Fatalf("verdictOutcome = %q, want shipped precedence", got)
	}
}

func TestToStatSplitsSkills(t *testing.T) {
	st := toStat(telemetry.Session{
		Outcome:        "success",
		SkillsUsed:     "a,b",
		CleanDelivery:  true,
		CaptureVersion: 2,
		FilesChanged:   1,
		Shipped:        true,
		PlanStyle:      "plan_first",
	})
	if len(st.Skills) != 2 || st.Skills[0] != "a" {
		t.Fatalf("skills not split: %v", st.Skills)
	}
	if st.Outcome != analytics.Success {
		t.Fatalf("outcome not mapped: %v", st.Outcome)
	}
	if st.CaptureVersion != 2 || st.FilesChanged != 1 || !st.Shipped || st.PlanStyle != analytics.PlanFirst {
		t.Fatalf("delivery fields not mapped: %+v", st)
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

func TestRenderHeaderLeadsWithShippedAndIntervention(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	sessions := []telemetry.Session{
		{CaptureVersion: 2, FilesChanged: 2, Shipped: true, Outcome: "success", CleanDelivery: true},
		{CaptureVersion: 2, FilesChanged: 1, Outcome: "success", CorrectionTurns: 1},
	}
	h := renderHeader(aggregate(sessions, time.Now()))
	for _, want := range []string{"last 7 days", "Shipped", "1 of 2 delivery sessions", "50%", "Intervention", "0.5 human fixes / completed session", "Risk"} {
		if !strings.Contains(h, want) {
			t.Fatalf("header missing %q:\n%s", want, h)
		}
	}
	for _, banned := range []string{"Landed clean", "no rework needed"} {
		if strings.Contains(h, banned) {
			t.Fatalf("header retained %q:\n%s", banned, h)
		}
	}
}

func TestRenderScopedEmpty(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	tests := []struct {
		name       string
		global     bool
		hasHistory bool
		want       []string
	}{
		{name: "project has history", hasHistory: true, want: []string{"No Atlas activity for this project in the last 7 days.", "Press g to view all projects"}},
		{name: "project has no history", want: []string{"No Atlas activity for this project yet.", "Press g to view all projects"}},
		{name: "global has history", global: true, hasHistory: true, want: []string{"No Atlas activity in the last 7 days.", "? help"}},
		{name: "global has no history", global: true, want: []string{"Nothing captured yet", "gated status"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderScopedEmpty(tt.global, tt.hasHistory)
			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Fatalf("empty copy missing %q: %q", want, got)
				}
			}
		})
	}
}

func TestRenderNeedsAttentionCapAndOrder(t *testing.T) {
	now := time.Date(2026, 7, 13, 15, 0, 0, 0, time.UTC)
	mk := func(id string, hoursAgo int) telemetry.Session {
		return telemetry.Session{SessionID: id, ToolCalls: 5, Taint: true,
			Started: now.Add(time.Duration(-hoursAgo) * time.Hour).Format(time.RFC3339Nano)}
	}
	sessions := []telemetry.Session{mk("old", 5), mk("newest", 1), mk("mid2", 3), mk("mid1", 2)}
	lines := renderNeedsAttention(sessions, aggregates{}, now)
	if len(lines) != 3 {
		t.Fatalf("want 3 lines (capped at 3), got %d: %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], "1h ago") {
		t.Errorf("most-recent-first: line[0] = %q, want the 1h-ago session", lines[0])
	}
	for _, l := range lines {
		if strings.Contains(l, "5h ago") {
			t.Errorf("oldest flagged session should be dropped by the cap, but appears: %q", l)
		}
	}
}

func TestAttentionBlockFallbackWhenEmpty(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	if got := renderAttentionBlock(nil); !strings.Contains(got, "nothing needs attention") {
		t.Fatalf("empty attention block should show fallback, got %q", got)
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
	sess := telemetry.Session{SessionID: "s", ProjectName: "atlas", TaskType: "testing", Outcome: "success",
		DurationMS: 1080000, ToolCalls: 165, FilesChanged: 7, FilesTouched: 12}
	pre := schema.TelemetryEvent{EventType: schema.EventPreTool, ToolClass: schema.ClassExec, ToolUseID: "t1", Verbs: []string{"go"}}
	post := schema.TelemetryEvent{EventType: schema.EventPostTool, ToolUseID: "t1", LatencyMS: 9109, ToolOK: schema.OutcomeOK}
	out := renderTrace(sess, []schema.TelemetryEvent{pre, post})
	for _, want := range []string{"atlas · testing · ✓ succeeded", "wall span 18m", "observed tool time 9s", "7 changed", "12 touched"} {
		if !strings.Contains(out, want) {
			t.Errorf("verdict header missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "risk=") || strings.Contains(out, "sub=") || strings.Contains(out, "[t0]") {
		t.Errorf("trace leaked raw debug fields:\n%s", out)
	}
	if !strings.Contains(out, "ran") || !strings.Contains(out, "9s") {
		t.Errorf("trace missing paired call / duration:\n%s", out)
	}
}

func TestRenderTraceOmitsEmptyDurationSeparators(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	sess := telemetry.Session{SessionID: "abc12345", TaskType: "testing", Outcome: "success",
		DurationMS: 0, ToolCalls: 1, FilesTouched: 0}
	unpairedPre := schema.TelemetryEvent{EventType: schema.EventPreTool, ToolClass: schema.ClassExec, ToolUseID: "p1", Verbs: []string{"go"}}
	orphanPost := schema.TelemetryEvent{EventType: schema.EventPostTool, ToolUseID: "orphan", ToolOK: schema.OutcomeOK}
	out := renderTrace(sess, []schema.TelemetryEvent{unpairedPre, orphanPost})

	if strings.Contains(out, "·  ·") {
		t.Errorf("doubled separator from empty field:\n%s", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasSuffix(line, "· ") || strings.HasSuffix(line, "·") {
			t.Errorf("line ends in a dangling separator: %q\nfull:\n%s", line, out)
		}
		if g := strings.TrimSpace(line); g == "✓" || g == "✗" {
			t.Errorf("row rendered as bare glyph only: %q\nfull:\n%s", line, out)
		}
	}
}
