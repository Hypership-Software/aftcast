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

// The per-turn breakdown counts calls by activity, folds collapsed runs into
// their real count, and leaves skills to Highlights rather than the breakdown.
func TestTurnBreakdownCountsAndExcludesSkills(t *testing.T) {
	got := turnBreakdown(traceTurn{Rows: []traceRow{
		{Verb: "edited"},
		{Verb: "edited"},
		{Verb: "ran"},
		{Verb: "read", CollapsedN: 8}, // a folded run of 8 reads
		{Verb: "skill", Skill: true},  // excluded — lives in Highlights
		{Verb: ""},                    // orphan post — not a call
	}})
	if !strings.Contains(got, "8 read") || !strings.Contains(got, "2 edited") || !strings.Contains(got, "1 ran") {
		t.Errorf("breakdown miscounted: %q", got)
	}
	if strings.Contains(got, "skill") {
		t.Errorf("skills must not appear in the breakdown: %q", got)
	}
}
