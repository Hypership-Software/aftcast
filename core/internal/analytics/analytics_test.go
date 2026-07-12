package analytics

import (
	"testing"

	"github.com/Hypership-Software/atlas/internal/schema"
)

// --- event builders (post_tool carries tool_ok + verbs/files, matching the adapter) ---

func prompt() schema.TelemetryEvent { return schema.TelemetryEvent{EventType: schema.EventUserPrompt} }
func stop() schema.TelemetryEvent   { return schema.TelemetryEvent{EventType: schema.EventStop} }

func run(verb string, ok bool) schema.TelemetryEvent {
	o := schema.OutcomeOK
	if !ok {
		o = schema.OutcomeFailed
	}
	return schema.TelemetryEvent{EventType: schema.EventPostTool, ToolClass: schema.ClassExec, Verbs: []string{verb}, ToolOK: o}
}

func wrote(file string) schema.TelemetryEvent {
	return schema.TelemetryEvent{EventType: schema.EventPostTool, ToolClass: schema.ClassFileWrite, Files: []string{file}, ToolOK: schema.OutcomeOK}
}

func readf(file string) schema.TelemetryEvent {
	return schema.TelemetryEvent{EventType: schema.EventPostTool, ToolClass: schema.ClassFileRead, Files: []string{file}, ToolOK: schema.OutcomeOK}
}

func dangerEv(rule string, class schema.ToolClass) schema.TelemetryEvent {
	return schema.TelemetryEvent{EventType: schema.EventPreTool, Risk: schema.RiskDanger, RuleID: rule, ToolClass: class}
}

// --- Outcome ---

func TestOutcomeSuccess(t *testing.T) {
	evts := []schema.TelemetryEvent{prompt(), wrote("pkg/foo.go"), run("go", true), run("git", true), stop()}
	if got := Outcome(evts); got != Success {
		t.Errorf("Outcome = %v, want success", got)
	}
}

func TestOutcomeFailureEndsOnFailedTool(t *testing.T) {
	evts := []schema.TelemetryEvent{prompt(), wrote("pkg/foo.go"), run("go", false), stop()}
	if got := Outcome(evts); got != Failure {
		t.Errorf("Outcome = %v, want failure", got)
	}
}

func TestOutcomeUnknownNoCompletionSignal(t *testing.T) {
	evts := []schema.TelemetryEvent{prompt(), readf("a.go"), readf("b.go"), stop()}
	if got := Outcome(evts); got != Unknown {
		t.Errorf("Outcome = %v, want unknown", got)
	}
}

// --- OneShot / corrections ---

func TestOneShotSingleTurnSuccess(t *testing.T) {
	evts := []schema.TelemetryEvent{prompt(), wrote("pkg/foo.go"), run("go", true), stop()}
	oneShot, corrections := OneShot(evts)
	if !oneShot || corrections != 0 {
		t.Errorf("OneShot = (%v,%d), want (true,0)", oneShot, corrections)
	}
}

func TestOneShotAssistedAfterFailure(t *testing.T) {
	// turn 1 fails; turn 2 (after a new prompt) fixes it -> not one-shot, 1 correction.
	evts := []schema.TelemetryEvent{
		prompt(), wrote("pkg/foo.go"), run("go", false),
		prompt(), wrote("pkg/foo.go"), run("go", true), stop(),
	}
	oneShot, corrections := OneShot(evts)
	if oneShot || corrections != 1 {
		t.Errorf("OneShot = (%v,%d), want (false,1)", oneShot, corrections)
	}
}

// --- Taxonomy ---

func TestTaxonomyTesting(t *testing.T) {
	evts := []schema.TelemetryEvent{prompt(), wrote("internal/foo/foo_test.go"), run("go", true)}
	if tt, _ := Taxonomy(evts); tt != TaskTesting {
		t.Errorf("Taxonomy = %q, want testing", tt)
	}
}

func TestTaxonomyDocs(t *testing.T) {
	evts := []schema.TelemetryEvent{prompt(), wrote("README.md"), wrote("docs/guide.md")}
	if tt, _ := Taxonomy(evts); tt != TaskDocs {
		t.Errorf("Taxonomy = %q, want docs", tt)
	}
}

func TestTaxonomyMigration(t *testing.T) {
	evts := []schema.TelemetryEvent{prompt(), wrote("db/migrations/0007_add_users.sql")}
	if tt, _ := Taxonomy(evts); tt != TaskMigration {
		t.Errorf("Taxonomy = %q, want migration", tt)
	}
}

func TestTaxonomyConfig(t *testing.T) {
	evts := []schema.TelemetryEvent{prompt(), wrote("package.json")}
	if tt, _ := Taxonomy(evts); tt != TaskConfig {
		t.Errorf("Taxonomy = %q, want config", tt)
	}
}

func TestTaxonomyFeature(t *testing.T) {
	evts := []schema.TelemetryEvent{prompt(), wrote("internal/foo/foo.go"), wrote("internal/foo/bar.go")}
	if tt, _ := Taxonomy(evts); tt != TaskFeature {
		t.Errorf("Taxonomy = %q, want feature", tt)
	}
}

func TestTaxonomyExploration(t *testing.T) {
	evts := []schema.TelemetryEvent{prompt(), readf("a.go"), readf("b.go"), readf("c.go")}
	if tt, _ := Taxonomy(evts); tt != TaskExploration {
		t.Errorf("Taxonomy = %q, want exploration", tt)
	}
}

// --- Danger (reframed: observed, not blocked) ---

func TestDangerAggregatesByRule(t *testing.T) {
	evts := []schema.TelemetryEvent{
		dangerEv("danger-rm-rf", schema.ClassExec),
		dangerEv("danger-rm-rf", schema.ClassExec),
		dangerEv("danger-secret-file-access", schema.ClassFileRead),
		{EventType: schema.EventPreTool, Risk: schema.RiskSafe, RuleID: "safe-file-read"},
	}
	got := Danger(evts)
	if len(got) != 2 {
		t.Fatalf("Danger returned %d groups, want 2: %+v", len(got), got)
	}
	if got[0].RuleID != "danger-rm-rf" || got[0].Count != 2 {
		t.Errorf("top danger = %+v, want danger-rm-rf x2", got[0])
	}
	if got[1].RuleID != "danger-secret-file-access" || got[1].Count != 1 {
		t.Errorf("second danger = %+v, want danger-secret-file-access x1", got[1])
	}
}

// --- Productivity ---

func TestProductivityAggregates(t *testing.T) {
	sessions := []SessionStat{
		{Outcome: Success, OneShot: true, TurnCount: 1, ToolCalls: 4, Started: "2026-07-10T09:00:00Z"},
		{Outcome: Success, OneShot: false, TurnCount: 3, CorrectionTurns: 1, ToolCalls: 8, Started: "2026-07-10T15:00:00Z"},
		{Outcome: Unknown, OneShot: false, TurnCount: 2, ToolCalls: 2, Started: "2026-07-11T09:00:00Z"},
	}
	p := Productivity(sessions)
	if p.Sessions != 3 {
		t.Errorf("Sessions = %d, want 3", p.Sessions)
	}
	// one-shot rate over determinate (Success/Failure) sessions: 1 of 2.
	if p.OneShotRate != 0.5 {
		t.Errorf("OneShotRate = %v, want 0.5", p.OneShotRate)
	}
	// corrections 1 / turns 6.
	if p.CorrectionRatio < 0.16 || p.CorrectionRatio > 0.17 {
		t.Errorf("CorrectionRatio = %v, want ~0.1667", p.CorrectionRatio)
	}
}

// --- SkillInsights ---

func TestSkillInsightsOpportunityAndRisk(t *testing.T) {
	sessions := []SessionStat{
		// bugfix task, needed corrections, no skill used -> opportunity.
		{TaskType: TaskBugfix, CorrectionTurns: 2, TurnCount: 3, Outcome: Success},
		// a skill used in a tainted session -> risk flag.
		{TaskType: TaskFeature, Skills: []string{"superpowers:brainstorming"}, Tainted: true, Outcome: Success},
	}
	r := SkillInsights(sessions)
	if !contains(r.Opportunities, TaskBugfix) {
		t.Errorf("Opportunities = %v, want to include bugfix", r.Opportunities)
	}
	if !contains(r.RiskFlags, "superpowers:brainstorming") {
		t.Errorf("RiskFlags = %v, want to include the tainted-session skill", r.RiskFlags)
	}
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
