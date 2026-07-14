package insights

import (
	"strings"
	"testing"

	"github.com/Hypership-Software/atlas/internal/schema"
	"github.com/Hypership-Software/atlas/internal/telemetry"
)

func ev(et schema.EventType, class schema.ToolClass) schema.TelemetryEvent {
	return schema.TelemetryEvent{EventType: et, ToolClass: class}
}

func TestBuildTraceCarriesShippedFromPairedPost(t *testing.T) {
	pre := ev(schema.EventPreTool, schema.ClassExec)
	pre.ToolUseID = "push1"
	pre.Verbs = []string{"git"}
	post := ev(schema.EventPostTool, schema.ClassExec)
	post.ToolUseID = "push1"
	post.ToolOK = schema.OutcomeOK
	post.DeliverySignal = schema.DeliveryGitPush
	rows := buildTrace([]schema.TelemetryEvent{pre, post})[0].Rows
	if len(rows) != 1 || !rows[0].Shipped {
		t.Fatalf("push rows = %+v", rows)
	}
}

func TestIsLowSignalExcludesAnnotatedRows(t *testing.T) {
	if !isLowSignal(traceRow{Verb: "read"}) {
		t.Fatal("a plain read row should be low-signal (collapsible)")
	}
	for name, r := range map[string]traceRow{
		"danger":    {Verb: "read", Danger: true},
		"failed":    {Verb: "read", Failed: true},
		"untrusted": {Verb: "read", Untrusted: true},
	} {
		if isLowSignal(r) {
			t.Errorf("%s row must not be low-signal — collapsing it would erase its ⚑/✗/⚠ annotation", name)
		}
	}
}

func TestRenderTraceKeepsFailedOrphanRow(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	// Outcome "" → verdict header shows "—", so any ✗ in the output comes from the
	// orphaned post row, proving the drop-empty-row rule doesn't discard a signal.
	sess := telemetry.Session{SessionID: "s", TaskType: "testing"}
	orphan := ev(schema.EventPostTool, schema.ClassExec)
	orphan.ToolUseID = "orphan"
	orphan.ToolOK = schema.OutcomeFailed
	out := renderTrace(sess, []schema.TelemetryEvent{orphan})
	if !strings.Contains(out, "✗") {
		t.Fatalf("a failed orphan post must still render (its ✗ is the signal), not be dropped:\n%s", out)
	}
}

func TestBuildTracePairsPrePostByToolUseID(t *testing.T) {
	pre := ev(schema.EventPreTool, schema.ClassExec)
	pre.ToolUseID = "t1"
	pre.Verbs = []string{"go"}
	post := ev(schema.EventPostTool, schema.ClassExec)
	post.ToolUseID = "t1"
	post.LatencyMS = 9109
	post.ToolOK = schema.OutcomeOK
	turns := buildTrace([]schema.TelemetryEvent{pre, post})
	if len(turns) != 1 || len(turns[0].Rows) != 1 {
		t.Fatalf("want 1 turn / 1 row, got %d turns", len(turns))
	}
	r := turns[0].Rows[0]
	if r.Verb != "ran" || r.Detail != "go" || r.DurMS != 9109 || r.Outcome != schema.OutcomeOK {
		t.Errorf("row = %+v", r)
	}
}

func TestBuildTraceCollapsesReadRuns(t *testing.T) {
	var evs []schema.TelemetryEvent
	for i := 0; i < 4; i++ {
		e := ev(schema.EventPreTool, schema.ClassFileRead)
		e.Files = []string{"f.go"}
		evs = append(evs, e)
	}
	turns := buildTrace(evs)
	if len(turns[0].Rows) != 1 || turns[0].Rows[0].CollapsedN != 4 {
		t.Fatalf("want 1 collapsed row of 4, got %+v", turns[0].Rows)
	}
}

func TestBuildTraceSegmentsTurnsAtUserPrompt(t *testing.T) {
	evs := []schema.TelemetryEvent{
		ev(schema.EventUserPrompt, ""),
		ev(schema.EventPreTool, schema.ClassFileWrite),
		ev(schema.EventUserPrompt, ""),
		ev(schema.EventPreTool, schema.ClassExec),
	}
	if got := len(buildTrace(evs)); got != 2 {
		t.Fatalf("turns = %d, want 2", got)
	}
}

func TestBuildTraceCollapseKeepsDangerAndFailed(t *testing.T) {
	read := func() schema.TelemetryEvent {
		e := ev(schema.EventPreTool, schema.ClassFileRead)
		e.Files = []string{"clean.go"}
		return e
	}
	danger := read()
	danger.Risk = schema.RiskDanger
	danger.Files = []string{".env"}
	failPre := read()
	failPre.ToolUseID = "fail1"
	failPre.Files = []string{"secrets.go"}
	failPost := ev(schema.EventPostTool, schema.ClassFileRead)
	failPost.ToolUseID = "fail1"
	failPost.ToolOK = schema.OutcomeFailed

	evs := []schema.TelemetryEvent{
		read(), read(), read(),
		danger,
		read(), read(), read(),
		failPre, failPost,
	}
	rows := buildTrace(evs)[0].Rows
	if len(rows) != 4 {
		t.Fatalf("want 4 rows (×3, danger, ×3, failed), got %d: %+v", len(rows), rows)
	}
	if rows[0].CollapsedN != 3 || rows[2].CollapsedN != 3 {
		t.Errorf("collapsed runs missing: rows[0]=%+v rows[2]=%+v", rows[0], rows[2])
	}
	if !rows[1].Danger || rows[1].CollapsedN != 0 || rows[1].Detail != ".env" {
		t.Errorf("danger read must survive as its own row, got %+v", rows[1])
	}
	if !rows[3].Failed || rows[3].Outcome != schema.OutcomeFailed || rows[3].CollapsedN != 0 {
		t.Errorf("failed read must survive as its own row, got %+v", rows[3])
	}
}

func TestBuildTraceCollapsesGrepRunsCaseInsensitive(t *testing.T) {
	var evs []schema.TelemetryEvent
	for i := 0; i < 3; i++ {
		e := ev(schema.EventPreTool, schema.ClassOther)
		e.ToolRaw = "Grep"
		evs = append(evs, e)
	}
	rows := buildTrace(evs)[0].Rows
	if len(rows) != 1 || rows[0].CollapsedN < 3 || rows[0].Verb != "Grep" {
		t.Fatalf("want 1 collapsed Grep row (CollapsedN>=3, Verb=Grep), got %+v", rows)
	}

	t.Setenv("NO_COLOR", "1")
	out := renderTrace(telemetry.Session{SessionID: "s"}, evs)
	if !strings.Contains(out, "3 grep") {
		t.Errorf("collapsed Grep run must surface under its own verb in the timeline breakdown:\n%s", out)
	}
	if strings.Contains(out, "read") {
		t.Errorf("collapsed Grep run mislabeled as read:\n%s", out)
	}
}

func TestBuildTraceMarksFirstUntrustedAndSkill(t *testing.T) {
	skill := ev(schema.EventPreTool, schema.ClassSkill)
	skill.Skill = "superpowers:brainstorming"
	f1 := ev(schema.EventPreTool, schema.ClassNetFetch)
	f1.Domain = "evil.example.com"
	f2 := ev(schema.EventPreTool, schema.ClassNetFetch)
	f2.Domain = "also.example.com"
	turns := buildTrace([]schema.TelemetryEvent{skill, f1, f2})
	rows := turns[0].Rows
	if !rows[0].Skill || rows[0].Detail != "superpowers:brainstorming" {
		t.Errorf("skill row = %+v", rows[0])
	}
	if !rows[1].Untrusted {
		t.Error("first net_fetch should be marked untrusted")
	}
	if rows[2].Untrusted {
		t.Error("only the FIRST untrusted event is the entry marker")
	}
}
