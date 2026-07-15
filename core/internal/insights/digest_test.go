package insights

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Hypership-Software/aftcast/internal/schema"
	"github.com/Hypership-Software/aftcast/internal/telemetry"
)

func observedCall(id string, class schema.ToolClass, operation schema.Operation, latencyMS int64) []schema.TelemetryEvent {
	return []schema.TelemetryEvent{
		{V: schema.ObservationVersion, EventType: schema.EventPreTool, ToolUseID: id, ToolClass: class, Operation: operation},
		{V: schema.ObservationVersion, EventType: schema.EventPostTool, ToolUseID: id, ToolOK: schema.OutcomeOK, LatencyMS: latencyMS},
	}
}

func TestSessionDetailHeaderUsesObservedDeveloperMetrics(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	sess := telemetry.Session{
		SessionID: "e4b91a20-rest", ProjectName: "agent-gate", TaskType: "feature", Outcome: "success", Shipped: true,
		DurationMS: 6240000, FilesChanged: 17, LinesAdded: 312, LinesRemoved: 84, ChangeStatsCovered: true,
		SkillsUsed: "strategic-review",
	}
	events := append(observedCall("plan", schema.ClassFileRead, schema.OperationRead, 47000),
		observedCall("build", schema.ClassFileWrite, schema.OperationEdit, 60000)...)
	out := renderTrace(sess, events)
	for _, want := range []string{
		"agent-gate · feature · succeeded · pushed",
		"Session e4b91a20 · wall span 1h 44m · observed tool time 1m 47s",
		"17 files changed · +312 / −84 observed · 1 invoked skill",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("session detail missing %q:\n%s", want, out)
		}
	}
}

func TestInvokedSkillUsesExplicitCaptureLanguageForLegacySession(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	sess := telemetry.Session{
		SessionID: "c8aec3ff", ProjectName: "agent-gate", TaskType: "feature", Outcome: "success",
		DurationMS: 6240000, FilesChanged: 17, FilesTouched: 30, SkillsUsed: "strategic-review",
	}
	out := renderTrace(sess, nil)
	for _, want := range []string{"17 files changed", "1 invoked skill", "invoked skill strategic-review"} {
		if !strings.Contains(out, want) {
			t.Fatalf("legacy detail missing %q:\n%s", want, out)
		}
	}
	for _, banned := range []string{"+0 / −0", "Plan 0%", "Build 0%", "Review 0%", "Timeline"} {
		if strings.Contains(out, banned) {
			t.Fatalf("legacy detail invented %q:\n%s", banned, out)
		}
	}
}

func TestSessionWorkMixRendersDurationPercentageAndOperations(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	events := append(observedCall("read", schema.ClassFileRead, schema.OperationRead, 1000),
		observedCall("edit", schema.ClassFileWrite, schema.OperationEdit, 3000)...)
	events = append(events, observedCall("test", schema.ClassExec, schema.OperationTest, 1000)...)
	out := renderTrace(telemetry.Session{SessionID: "mix", Outcome: "success"}, events)
	for _, want := range []string{
		"Work mix",
		"Plan", "1s · 20% · read",
		"Build", "3s · 60% · edit",
		"Review", "1s · 20% · test",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("work mix missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Timeline") {
		t.Fatalf("developer detail retained turn timeline:\n%s", out)
	}
}

func TestSessionFilesAggregateSuccessfulWritesAndUseRepositoryRelativePaths(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	a := filepath.Join(root, "core", "a.go")
	b := filepath.Join(root, "docs", "b.md")
	for _, path := range []string{a, b} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("observed"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write := func(id, path string, added, removed int, outcome schema.ToolOutcome) []schema.TelemetryEvent {
		return []schema.TelemetryEvent{
			{V: schema.ObservationVersion, EventType: schema.EventPreTool, ToolUseID: id, ToolClass: schema.ClassFileWrite,
				Operation: schema.OperationEdit, Files: []string{path}, ChangeStats: &schema.ChangeStats{LinesAdded: added, LinesRemoved: removed}},
			{V: schema.ObservationVersion, EventType: schema.EventPostTool, ToolUseID: id, ToolOK: outcome, LatencyMS: 1},
		}
	}
	events := write("a1", a, 10, 2, schema.OutcomeOK)
	events = append(events, write("a2", a, 3, 1, schema.OutcomeOK)...)
	events = append(events, write("failed", a, 90, 90, schema.OutcomeFailed)...)
	events = append(events, write("b", b, 2, 4, schema.OutcomeOK)...)
	out := renderTrace(telemetry.Session{SessionID: "files", Outcome: "success", FilesChanged: 2}, events)

	for _, want := range []string{"Files changed", "core/a.go", "+13 / −3", "docs/b.md", "+2 / −4"} {
		if !strings.Contains(out, want) {
			t.Fatalf("file detail missing %q:\n%s", want, out)
		}
	}
	if strings.Index(out, "core/a.go") > strings.Index(out, "docs/b.md") {
		t.Fatalf("files not sorted by observed change magnitude:\n%s", out)
	}
	for _, banned := range []string{root, "+103", "−93"} {
		if strings.Contains(out, banned) {
			t.Fatalf("file detail leaked or counted %q:\n%s", banned, out)
		}
	}
}

func TestSessionFilesShortenMissingAbsolutePathsAndKeepEveryFile(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := t.TempDir()
	var events []schema.TelemetryEvent
	for i := 0; i < 35; i++ {
		path := filepath.Join(root, "private", "repo", fmt.Sprintf("file-%02d.go", i))
		events = append(events,
			schema.TelemetryEvent{V: schema.ObservationVersion, EventType: schema.EventPreTool, ToolUseID: fmt.Sprintf("w-%d", i),
				ToolClass: schema.ClassFileWrite, Operation: schema.OperationEdit, Files: []string{path},
				ChangeStats: &schema.ChangeStats{LinesAdded: 1}},
			schema.TelemetryEvent{V: schema.ObservationVersion, EventType: schema.EventPostTool, ToolUseID: fmt.Sprintf("w-%d", i), ToolOK: schema.OutcomeOK},
		)
	}
	out := renderChangedFiles(telemetry.Session{}, events)
	for _, want := range []string{"…/private/repo/file-00.go", "…/private/repo/file-34.go"} {
		if !strings.Contains(out, want) {
			t.Fatalf("file list dropped %q:\n%s", want, out)
		}
	}
	count := strings.Count(out, "file-")
	if strings.Contains(out, root) || count != 35 {
		t.Fatalf("file list leaked its root or dropped rows (count=%d):\n%s", count, out)
	}
}

func TestSessionHighlightsPrecedeFilesChanged(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	events := []schema.TelemetryEvent{
		{
			V: schema.ObservationVersion, EventType: schema.EventPreTool, ToolUseID: "write", ToolClass: schema.ClassFileWrite,
			Operation: schema.OperationEdit, Files: []string{"core/main.go"}, ChangeStats: &schema.ChangeStats{LinesAdded: 3, LinesRemoved: 1},
		},
		{V: schema.ObservationVersion, EventType: schema.EventPostTool, ToolUseID: "write", ToolOK: schema.OutcomeOK},
	}
	out := renderTrace(telemetry.Session{SessionID: "ordered", Outcome: "success", FilesChanged: 1, SkillsUsed: "strategic-review"}, events)
	highlights := strings.Index(out, "Highlights")
	files := strings.Index(out, "Files changed")
	if highlights < 0 || files < 0 {
		t.Fatalf("detail missing required sections:\n%s", out)
	}
	if highlights > files {
		t.Fatalf("Highlights must precede Files changed:\n%s", out)
	}
}

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
	wantHighlight := highlightLine("↑", "pushed", "successful git push")
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

// The digest surfaces notable signals without pulling developers down into turn
// mechanics. Turn-level evidence remains available in raw JSON.
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
		"untrusted  entered via evil.example.com",
		"⚑  flagged    1 — ran rm",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("digest missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "superpowers:") {
		t.Errorf("skill namespace prefix should be stripped:\n%s", out)
	}
	if strings.Contains(out, "turn 1") || strings.Contains(out, "turn 2") || strings.Contains(out, "Timeline") {
		t.Errorf("default detail retained turn mechanics:\n%s", out)
	}
}

// A clean, unremarkable session shows no empty Highlights block.
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
	if !strings.Contains(out, "Work mix") || strings.Contains(out, "Timeline") {
		t.Errorf("developer summary should replace the timeline:\n%s", out)
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
		"agent-gate · feature · succeeded",
		"wall span 1h 44m",
		"observed tool time 1m 48s",
		"17 files changed · 1 invoked skill",
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
