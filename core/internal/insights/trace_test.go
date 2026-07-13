package insights

import (
	"testing"

	"github.com/Hypership-Software/atlas/internal/schema"
)

func ev(et schema.EventType, class schema.ToolClass) schema.TelemetryEvent {
	return schema.TelemetryEvent{EventType: et, ToolClass: class}
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
