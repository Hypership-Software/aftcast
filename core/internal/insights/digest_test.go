package insights

import (
	"strings"
	"testing"

	"github.com/Hypership-Software/atlas/internal/schema"
	"github.com/Hypership-Software/atlas/internal/telemetry"
)

func TestDigestHighlightsShippedWithoutCommandContent(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	session := telemetry.Session{SessionID: "ship1", TaskType: "feature", Shipped: true, ToolCalls: 1}
	pre := ev(schema.EventPreTool, schema.ClassExec)
	pre.ToolUseID = "push1"
	pre.Verbs = []string{"git", "push", "origin", "feature/coach"}
	pre.ToolRaw = "git push origin feature/coach"
	pre.Command = "git push origin feature/coach"
	post := ev(schema.EventPostTool, schema.ClassExec)
	post.ToolUseID = "push1"
	post.ToolOK = schema.OutcomeOK
	post.DeliverySignal = schema.DeliveryGitPush
	post.Command = "git push origin feature/coach"
	out := renderTrace(session, []schema.TelemetryEvent{pre, post})
	wantHighlight := highlightLine("↑", "shipped", "successful git push")
	foundHighlight := false
	for _, line := range strings.Split(out, "\n") {
		if line == wantHighlight {
			foundHighlight = true
		}
	}
	if !foundHighlight {
		t.Fatalf("digest did not render fixed generic highlight %q:\n%s", wantHighlight, out)
	}
	for _, banned := range []string{"origin", "feature/coach", "git push origin"} {
		if strings.Contains(out, banned) {
			t.Fatalf("digest leaked %q:\n%s", banned, out)
		}
	}
}

// The digest must surface the signal an observer cares about — which skill ran,
// where untrusted content entered, and which turn a flagged call happened in —
// without listing every call.
func TestDigestHighlightsSurfaceSignal(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	fetch := ev(schema.EventPreTool, schema.ClassNetFetch)
	fetch.Domain = "evil.example.com"
	danger := ev(schema.EventPreTool, schema.ClassExec)
	danger.Verbs = []string{"rm"}
	danger.Risk = schema.RiskDanger
	evs := []schema.TelemetryEvent{
		ev(schema.EventUserPrompt, ""),
		fetch,
		ev(schema.EventUserPrompt, ""),
		danger,
	}
	sess := telemetry.Session{SessionID: "sig123", TaskType: "feature", Outcome: "success",
		ToolCalls: 2, SkillsUsed: "superpowers:brainstorming"}
	out := renderTrace(sess, evs)

	for _, want := range []string{
		"Highlights",
		"brainstorming", // skill, namespace stripped
		"untrusted  entered turn 1 via evil.example.com",
		"⚑  flagged    1 — ran rm in turn 2",
		"Timeline",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("digest missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "superpowers:") {
		t.Errorf("skill namespace prefix should be stripped:\n%s", out)
	}
}

// A clean, unremarkable session shows no Highlights block — just the verdict and
// the timeline. The section must not appear as an empty "nothing to see here".
func TestDigestOmitsHighlightsWhenClean(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	evs := []schema.TelemetryEvent{
		ev(schema.EventUserPrompt, ""),
		ev(schema.EventPreTool, schema.ClassFileWrite),
	}
	sess := telemetry.Session{SessionID: "clean1", TaskType: "docs", Outcome: "success", ToolCalls: 1}
	out := renderTrace(sess, evs)
	if strings.Contains(out, "Highlights") {
		t.Errorf("a clean session should have no Highlights block:\n%s", out)
	}
	if !strings.Contains(out, "Timeline") {
		t.Errorf("timeline should always render for a session with turns:\n%s", out)
	}
}

func TestVerdictHeaderNamesRepositoryAndExplicitTimeUnits(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	sess := telemetry.Session{
		SessionID:    "c8aec3ff-x",
		ProjectName:  "agent-gate",
		TaskType:     "feature",
		Outcome:      "success",
		DurationMS:   6240000,
		ToolCalls:    119,
		FilesChanged: 17,
		FilesTouched: 30,
		SkillsUsed:   "strategic-review",
	}
	turns := []traceTurn{
		{Index: 1},
		{Index: 2, Calls: 109, DurMS: 103190},
		{Index: 3, Calls: 2, DurMS: 5},
		{Index: 4, Calls: 8, DurMS: 5000},
	}
	got := verdictHeader(sess, turns)
	for _, want := range []string{
		"agent-gate · feature · ✓ succeeded",
		"wall span 1h 44m",
		"observed tool time 1m 48s",
		"119 calls · 17 changed · 30 touched · ★ 1 skill",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q:\n%s", want, got)
		}
	}
}

func TestFailureTextExplainsRecoveryAndHumanCorrections(t *testing.T) {
	fails := []rowRef{{}, {}, {}}
	tests := []struct {
		name string
		sess telemetry.Session
		want string
	}{
		{"agent recovered", telemetry.Session{Outcome: "success"}, "3 failed attempts · agent recovered"},
		{"human corrected", telemetry.Session{Outcome: "success", CorrectionTurns: 1}, "3 failed attempts · 1 human correction"},
		{"session failed", telemetry.Session{Outcome: "failure"}, "3 failed attempts · session failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := failureText(fails, tt.sess); got != tt.want {
				t.Fatalf("failureText = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTurnBreakdownAccountsForEveryCallAndTranslatesTools(t *testing.T) {
	var rows []traceRow
	add := func(n int, verb string, skill bool) {
		for i := 0; i < n; i++ {
			rows = append(rows, traceRow{Verb: verb, Skill: skill})
		}
	}
	add(55, "ran", false)
	add(25, "edited", false)
	add(22, "read", false)
	add(4, "Glob", false)
	add(2, "Grep", false)
	add(1, "AskUserQuestion", false)
	got := turnBreakdown(traceTurn{Calls: 109, Rows: rows})
	if got != "55 ran · 25 edited · 22 read · 4 searched · +3 other" {
		t.Fatalf("breakdown = %q", got)
	}

	got = turnBreakdown(traceTurn{Calls: 2, Rows: []traceRow{{Verb: "skill", Skill: true}, {Verb: "AskUserQuestion"}}})
	if got != "1 asked · 1 skill" {
		t.Fatalf("translated breakdown = %q", got)
	}
}

func TestTimelineHasNoSpeculativePhaseAndNamesZeroCalls(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	got := strings.Join(timelineLines([]traceTurn{{Index: 1}, {Index: 2, Calls: 1, DurMS: 5}}), "\n")
	for _, banned := range []string{"planning", "execution", "wrap-up", "0 calls", "0.0s"} {
		if strings.Contains(got, banned) {
			t.Fatalf("timeline retained %q:\n%s", banned, got)
		}
	}
	for _, want := range []string{"turn 1", "no tool calls", "1 call", "<1s"} {
		if !strings.Contains(got, want) {
			t.Fatalf("timeline missing %q:\n%s", want, got)
		}
	}
}

func TestCountNounUsesSingularAndPlural(t *testing.T) {
	for _, tc := range []struct {
		n               int
		one, many, want string
	}{
		{1, "skill", "skills", "1 skill"},
		{2, "skill", "skills", "2 skills"},
		{1, "human correction", "human corrections", "1 human correction"},
	} {
		if got := countNoun(tc.n, tc.one, tc.many); got != tc.want {
			t.Errorf("countNoun(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

// The per-turn breakdown counts calls by activity and folds collapsed runs into
// their real count. Skills remain visible because every call must be accounted for.
func TestTurnBreakdownCountsCollapsedRunsAndSkills(t *testing.T) {
	got := turnBreakdown(traceTurn{Rows: []traceRow{
		{Verb: "edited"},
		{Verb: "edited"},
		{Verb: "ran"},
		{Verb: "read", CollapsedN: 8}, // a folded run of 8 reads
		{Verb: "skill", Skill: true},
		{Verb: ""}, // orphan post — not a call
	}})
	if !strings.Contains(got, "8 read") || !strings.Contains(got, "2 edited") || !strings.Contains(got, "1 ran") {
		t.Errorf("breakdown miscounted: %q", got)
	}
	if !strings.Contains(got, "1 skill") {
		t.Errorf("skills must appear in the breakdown: %q", got)
	}
}
