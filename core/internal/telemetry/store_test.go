package telemetry

import (
	"path/filepath"
	"testing"

	"github.com/Hypership-Software/atlas/internal/audit"
	"github.com/Hypership-Software/atlas/internal/schema"
)

var tKey = []byte("test-hmac-key-0123456789")

func ts(sec int) string {
	return "2026-07-10T12:00:" + string(rune('0'+sec/10)) + string(rune('0'+sec%10)) + "Z"
}

// twoSessions is the canonical fixture: one clean single-turn session (s1, three
// allowed tool calls, no blocks) and one two-turn session (s2, two allowed calls
// plus one blocked action, and a tainted event). Recording semantics match the
// daemon: a denied call is an EventBlock (not a pre_tool), turns are user_prompt
// events, and allowed calls emit pre_tool + post_tool.
func twoSessions() []schema.TelemetryEvent {
	pre := func(sess string, sec int) schema.TelemetryEvent {
		return schema.TelemetryEvent{SessionID: sess, User: "kyle", Harness: "claudecode",
			EventType: schema.EventPreTool, ToolClass: schema.ClassFileRead, Verdict: schema.VerdictAllow, TS: ts(sec)}
	}
	post := func(sess string, sec int) schema.TelemetryEvent {
		return schema.TelemetryEvent{SessionID: sess, User: "kyle", Harness: "claudecode",
			EventType: schema.EventPostTool, ToolOK: schema.OutcomeOK, TS: ts(sec)}
	}
	prompt := func(sess string, turn, sec int) schema.TelemetryEvent {
		return schema.TelemetryEvent{SessionID: sess, User: "kyle", Harness: "claudecode",
			EventType: schema.EventUserPrompt, TurnIndex: turn, TS: ts(sec)}
	}
	start := func(sess string, sec int) schema.TelemetryEvent {
		return schema.TelemetryEvent{SessionID: sess, User: "kyle", Harness: "claudecode",
			EventType: schema.EventSessionStart, TS: ts(sec)}
	}
	stop := func(sess string, sec int) schema.TelemetryEvent {
		return schema.TelemetryEvent{SessionID: sess, User: "kyle", Harness: "claudecode",
			EventType: schema.EventStop, TS: ts(sec)}
	}

	return []schema.TelemetryEvent{
		// s1 — clean one-turn session, 3 allowed tool calls, no block.
		start("s1", 0),
		prompt("s1", 1, 1),
		pre("s1", 2), post("s1", 3),
		pre("s1", 4), post("s1", 5),
		func() schema.TelemetryEvent { e := pre("s1", 6); e.Skill = "brainstorming"; return e }(), post("s1", 7),
		stop("s1", 8),
		// s2 — two turns, 2 allowed calls + 1 blocked action, second turn tainted.
		start("s2", 9),
		prompt("s2", 1, 10),
		pre("s2", 11), post("s2", 12),
		func() schema.TelemetryEvent {
			e := pre("s2", 13)
			e.EventType = schema.EventBlock
			e.Verdict = schema.VerdictDeny
			e.RuleID = "no-rm-rf"
			return e
		}(),
		prompt("s2", 2, 14),
		func() schema.TelemetryEvent { e := pre("s2", 15); e.Taint = true; return e }(), post("s2", 16),
		stop("s2", 17),
	}
}

func buildLog(t *testing.T, evs []schema.TelemetryEvent) *audit.Log {
	t.Helper()
	l, err := audit.NewLog(t.TempDir(), tKey)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range evs {
		if err := l.Record(e); err != nil {
			t.Fatal(err)
		}
	}
	return l
}

func openStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenStore(filepath.Join(t.TempDir(), "readmodel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func sessionsByID(t *testing.T, s *Store) map[string]Session {
	t.Helper()
	rows, err := s.Sessions()
	if err != nil {
		t.Fatal(err)
	}
	m := make(map[string]Session, len(rows))
	for _, r := range rows {
		m[r.SessionID] = r
	}
	return m
}

func TestProjectFoldsTwoSessions(t *testing.T) {
	log := buildLog(t, twoSessions())
	defer log.Close()
	s := openStore(t)

	if err := s.Project(log); err != nil {
		t.Fatal(err)
	}

	rows, err := s.Sessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("projected %d sessions, want 2", len(rows))
	}

	by := sessionsByID(t, s)
	s1, s2 := by["s1"], by["s2"]
	if s1.TurnCount != 1 || s1.ToolCalls != 3 || s1.BlockedCount != 0 {
		t.Errorf("s1 = {turns:%d tools:%d blocked:%d}, want {1 3 0}", s1.TurnCount, s1.ToolCalls, s1.BlockedCount)
	}
	if s2.TurnCount != 2 || s2.ToolCalls != 2 || s2.BlockedCount != 1 {
		t.Errorf("s2 = {turns:%d tools:%d blocked:%d}, want {2 2 1}", s2.TurnCount, s2.ToolCalls, s2.BlockedCount)
	}
	if s1.Harness != "claudecode" || s1.User != "kyle" {
		t.Errorf("s1 identity = {harness:%q user:%q}, want {claudecode kyle}", s1.Harness, s1.User)
	}
}

func TestProjectIsIdempotent(t *testing.T) {
	log := buildLog(t, twoSessions())
	defer log.Close()
	s := openStore(t)

	if err := s.Project(log); err != nil {
		t.Fatal(err)
	}
	if err := s.Project(log); err != nil {
		t.Fatal(err)
	}

	rows, err := s.Sessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("after re-projecting, %d sessions, want 2 (no dupes)", len(rows))
	}
	by := sessionsByID(t, s)
	if by["s1"].ToolCalls != 3 || by["s2"].BlockedCount != 1 {
		t.Errorf("re-projection changed aggregates: s1.tools=%d s2.blocked=%d", by["s1"].ToolCalls, by["s2"].BlockedCount)
	}
}

func TestProjectAggregatesTaintAndSkills(t *testing.T) {
	log := buildLog(t, twoSessions())
	defer log.Close()
	s := openStore(t)
	if err := s.Project(log); err != nil {
		t.Fatal(err)
	}

	by := sessionsByID(t, s)
	if by["s1"].Taint {
		t.Error("s1 should not be tainted")
	}
	if !by["s2"].Taint {
		t.Error("s2 has a tainted event and should be tainted")
	}
	if by["s1"].SkillsUsed != "brainstorming" {
		t.Errorf("s1 skills = %q, want %q", by["s1"].SkillsUsed, "brainstorming")
	}
}

func TestProjectEmptyLog(t *testing.T) {
	log := buildLog(t, nil)
	defer log.Close()
	s := openStore(t)
	if err := s.Project(log); err != nil {
		t.Fatal(err)
	}
	rows, err := s.Sessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("empty log projected %d sessions, want 0", len(rows))
	}
}
