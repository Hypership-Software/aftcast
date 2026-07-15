package telemetry

import (
	"path/filepath"
	"testing"

	"github.com/Hypership-Software/atlas/internal/analytics"
	"github.com/Hypership-Software/atlas/internal/audit"
	"github.com/Hypership-Software/atlas/internal/schema"
)

var tKey = []byte("test-hmac-key-0123456789")

func ts(sec int) string {
	return "2026-07-10T12:00:" + string(rune('0'+sec/10)) + string(rune('0'+sec%10)) + "Z"
}

// twoSessions is the canonical fixture: a clean single-turn session (s1) and a
// two-turn session (s2, one tool call classified dangerous, one tainted event).
// Recording semantics match the daemon — every tool call is a pre_tool carrying
// its risk, turns are user_prompt events.
func twoSessions() []schema.TelemetryEvent {
	pre := func(sess string, sec int) schema.TelemetryEvent {
		return schema.TelemetryEvent{SessionID: sess, User: "kyle", Harness: "claudecode",
			EventType: schema.EventPreTool, ToolClass: schema.ClassFileRead, Risk: schema.RiskSafe, TS: ts(sec)}
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
		// s2 — two turns, 3 tool calls (one classified dangerous), second turn tainted.
		start("s2", 9),
		prompt("s2", 1, 10),
		pre("s2", 11), post("s2", 12),
		func() schema.TelemetryEvent {
			e := pre("s2", 13)
			e.Risk = schema.RiskDanger
			e.RuleID = "danger-rm-rf"
			return e
		}(), post("s2", 14),
		prompt("s2", 2, 15),
		func() schema.TelemetryEvent { e := pre("s2", 16); e.Taint = true; return e }(), post("s2", 17),
		stop("s2", 18),
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

func TestProjectCountsMultipleFilesPerEvent(t *testing.T) {
	multi := schema.TelemetryEvent{SessionID: "fx", User: "kyle", Harness: "claudecode",
		EventType: schema.EventPreTool, ToolClass: schema.ClassFileRead, TS: ts(1),
		Files: []string{"a.go", "b.go", "a.go"}}
	evs := []schema.TelemetryEvent{
		{SessionID: "fx", User: "kyle", Harness: "claudecode", EventType: schema.EventUserPrompt, TS: ts(0)},
		multi,
	}
	log := buildLog(t, evs)
	defer log.Close()
	s := openStore(t)
	if err := s.Project(log); err != nil {
		t.Fatal(err)
	}
	if got := sessionsByID(t, s)["fx"].FilesTouched; got != 2 {
		t.Errorf("files_touched = %d, want 2 (a.go, b.go across one event) — foldSessions must range every entry of e.Files, not just index 0", got)
	}
}

func TestProjectFoldsDeliveryFacts(t *testing.T) {
	evs := []schema.TelemetryEvent{
		{V: 1, SessionID: "old", EventType: schema.EventUserPrompt, TS: ts(0)},
		{V: 1, SessionID: "old", EventType: schema.EventPreTool, ToolClass: schema.ClassFileWrite, Files: []string{"old.go"}, TS: ts(1)},
		{V: 2, SessionID: "new", EventType: schema.EventUserPrompt, TS: ts(2)},
		{V: 2, SessionID: "new", EventType: schema.EventPreTool, ToolClass: schema.ClassFileWrite, Files: []string{"a.go", "a.go"}, TS: ts(3)},
		{V: 2, SessionID: "new", EventType: schema.EventPostTool, ToolClass: schema.ClassFileWrite, Files: []string{"a.go"}, ToolOK: schema.OutcomeOK, TS: ts(4)},
		{V: 2, SessionID: "new", EventType: schema.EventPostTool, ToolClass: schema.ClassExec, ToolOK: schema.OutcomeOK, DeliverySignal: schema.DeliveryGitPush, TS: ts(5)},
	}

	log := buildLog(t, evs)
	defer log.Close()
	store := openStore(t)
	if err := store.Project(log); err != nil {
		t.Fatal(err)
	}

	got := sessionsByID(t, store)
	if got["old"].CaptureVersion != 1 || got["old"].Shipped {
		t.Fatalf("old session = %+v", got["old"])
	}
	if got["new"].CaptureVersion != 2 || got["new"].FilesChanged != 1 || !got["new"].Shipped {
		t.Fatalf("new session = %+v", got["new"])
	}
}

func TestProjectFoldsObservedPlanStyle(t *testing.T) {
	evs := []schema.TelemetryEvent{
		{V: 2, SessionID: "planned", PromptID: "p1", EventType: schema.EventPreTool, ToolClass: schema.ClassFileRead, TS: ts(0)},
		{V: 2, SessionID: "planned", PromptID: "p1", EventType: schema.EventPreTool, ToolClass: schema.ClassAgentSpawn, TS: ts(1)},
		{V: 2, SessionID: "planned", PromptID: "p2", EventType: schema.EventPreTool, ToolClass: schema.ClassFileWrite, Files: []string{"x.go"}, TS: ts(2)},
	}
	log := buildLog(t, evs)
	defer log.Close()
	store := openStore(t)
	if err := store.Project(log); err != nil {
		t.Fatal(err)
	}
	if got := sessionsByID(t, store)["planned"].PlanStyle; got != string(analytics.PlanFirst) {
		t.Fatalf("plan_style = %q, want plan_first", got)
	}
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
	if s1.TurnCount != 1 || s1.ToolCalls != 3 || s1.DangerDetected != 0 {
		t.Errorf("s1 = {turns:%d tools:%d danger:%d}, want {1 3 0}", s1.TurnCount, s1.ToolCalls, s1.DangerDetected)
	}
	if s2.TurnCount != 2 || s2.ToolCalls != 3 || s2.DangerDetected != 1 {
		t.Errorf("s2 = {turns:%d tools:%d danger:%d}, want {2 3 1}", s2.TurnCount, s2.ToolCalls, s2.DangerDetected)
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
	if by["s1"].ToolCalls != 3 || by["s2"].DangerDetected != 1 {
		t.Errorf("re-projection changed aggregates: s1.tools=%d s2.danger=%d", by["s1"].ToolCalls, by["s2"].DangerDetected)
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

// A skill stays attributed to its session even if a post_tool event arrives
// without the name (an older-CLI artifact): the pre event supplies it, and the
// two events are the same invocation by tool_use_id. Guards the projection
// against regressing to depend on the post carrying the name.
func TestProjectSkillAttributedWhenPostLacksName(t *testing.T) {
	mk := func(et schema.EventType, skill string, sec int) schema.TelemetryEvent {
		return schema.TelemetryEvent{SessionID: "sk", User: "kyle", Harness: "claudecode",
			EventType: et, ToolClass: schema.ClassSkill, Skill: skill, ToolUseID: "toolu_pair1", TS: ts(sec)}
	}
	evs := []schema.TelemetryEvent{
		{SessionID: "sk", User: "kyle", Harness: "claudecode", EventType: schema.EventUserPrompt, TS: ts(0)},
		mk(schema.EventPreTool, "superpowers:writing-plans", 1),
		mk(schema.EventPostTool, "", 2),
		{SessionID: "sk", User: "kyle", Harness: "claudecode", EventType: schema.EventStop, TS: ts(3)},
	}
	log := buildLog(t, evs)
	defer log.Close()
	s := openStore(t)
	if err := s.Project(log); err != nil {
		t.Fatal(err)
	}
	if got := sessionsByID(t, s)["sk"].SkillsUsed; got != "superpowers:writing-plans" {
		t.Errorf("skills = %q, want superpowers:writing-plans", got)
	}
}

func TestProjectComputesAnalyticalColumns(t *testing.T) {
	mk := func(et schema.EventType, class schema.ToolClass, verbs, files []string, ok schema.ToolOutcome, sec int) schema.TelemetryEvent {
		return schema.TelemetryEvent{SessionID: "s3", User: "kyle", Harness: "claudecode",
			EventType: et, ToolClass: class, Verbs: verbs, Files: files, ToolOK: ok, TS: ts(sec)}
	}
	// One-turn testing session: writes a _test.go file, runs go, ends clean.
	evs := []schema.TelemetryEvent{
		{SessionID: "s3", User: "kyle", Harness: "claudecode", EventType: schema.EventSessionStart, TS: ts(0)},
		{SessionID: "s3", User: "kyle", Harness: "claudecode", EventType: schema.EventUserPrompt, TS: ts(1)},
		mk(schema.EventPreTool, schema.ClassFileWrite, nil, []string{"internal/foo/foo_test.go"}, "", 2),
		mk(schema.EventPostTool, schema.ClassFileWrite, nil, []string{"internal/foo/foo_test.go"}, schema.OutcomeOK, 3),
		mk(schema.EventPreTool, schema.ClassExec, []string{"go"}, nil, "", 4),
		mk(schema.EventPostTool, schema.ClassExec, []string{"go"}, nil, schema.OutcomeOK, 5),
		{SessionID: "s3", User: "kyle", Harness: "claudecode", EventType: schema.EventStop, TS: ts(6)},
	}
	log := buildLog(t, evs)
	defer log.Close()
	s := openStore(t)
	if err := s.Project(log); err != nil {
		t.Fatal(err)
	}

	got := sessionsByID(t, s)["s3"]
	if got.Outcome != "success" {
		t.Errorf("outcome = %q, want success", got.Outcome)
	}
	if !got.CleanDelivery {
		t.Error("clean_delivery = false, want true")
	}
	if got.CorrectionTurns != 0 {
		t.Errorf("correction_turns = %d, want 0", got.CorrectionTurns)
	}
	if got.TaskType != "testing" {
		t.Errorf("task_type = %q, want testing", got.TaskType)
	}
}

func TestProjectCountsDistinctFilesTouched(t *testing.T) {
	mk := func(et schema.EventType, class schema.ToolClass, file string, sec int) schema.TelemetryEvent {
		e := schema.TelemetryEvent{SessionID: "fx", User: "kyle", Harness: "claudecode",
			EventType: et, ToolClass: class, TS: ts(sec)}
		if file != "" {
			e.Files = []string{file}
		}
		return e
	}
	evs := []schema.TelemetryEvent{
		{SessionID: "fx", User: "kyle", Harness: "claudecode", EventType: schema.EventUserPrompt, TS: ts(0)},
		mk(schema.EventPreTool, schema.ClassFileWrite, "a.go", 1),
		mk(schema.EventPostTool, schema.ClassFileWrite, "a.go", 2),
		mk(schema.EventPreTool, schema.ClassFileRead, "a.go", 3), // same file, not double-counted
		mk(schema.EventPreTool, schema.ClassFileWrite, "b.go", 4),
		mk(schema.EventPreTool, schema.ClassExec, "", 5), // exec, no file
	}
	log := buildLog(t, evs)
	defer log.Close()
	s := openStore(t)
	if err := s.Project(log); err != nil {
		t.Fatal(err)
	}
	if got := sessionsByID(t, s)["fx"].FilesTouched; got != 2 {
		t.Errorf("files_touched = %d, want 2 (a.go, b.go)", got)
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

func TestSessions_ScansProjectID(t *testing.T) {
	s, err := OpenStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.db.Exec(`INSERT INTO sessions
		(session_id, user, org, harness, started, ended, exit_reason,
		 turn_count, tool_calls, danger_detected, taint, outcome, clean_delivery,
		 correction_turns, task_type, skills_used, duration_ms, files_touched,
		 files_changed, shipped, capture_version, plan_style, project_id, project_name)
		VALUES ('s1','','','','','','',0,5,0,0,'',0,0,'','',0,0,0,0,0,'','proj123','my-repo')`); err != nil {
		t.Fatal(err)
	}
	got, err := s.Sessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ProjectID != "proj123" || got[0].ProjectName != "my-repo" {
		t.Fatalf("project identity = (%q, %q), want (proj123, my-repo)", got[0].ProjectID, got[0].ProjectName)
	}
}
