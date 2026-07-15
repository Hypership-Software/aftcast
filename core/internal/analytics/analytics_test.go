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

// --- CleanDelivery / corrections ---

func TestCleanDeliverySingleTurnSuccess(t *testing.T) {
	evts := []schema.TelemetryEvent{prompt(), wrote("pkg/foo.go"), run("go", true), stop()}
	clean, corrections := CleanDelivery(evts)
	if !clean || corrections != 0 {
		t.Errorf("CleanDelivery = (%v,%d), want (true,0)", clean, corrections)
	}
}

func TestCleanDeliveryMultiPromptNoCorrections(t *testing.T) {
	// Heavy planning (three prompts, reads/discussion) then a clean successful
	// execution with no failures. Prompt count must NOT disqualify it — the fix.
	evts := []schema.TelemetryEvent{
		prompt(), readf("a.go"),
		prompt(), readf("b.go"),
		prompt(), wrote("pkg/foo.go"), run("go", true), stop(),
	}
	clean, corrections := CleanDelivery(evts)
	if !clean || corrections != 0 {
		t.Errorf("CleanDelivery = (%v,%d), want (true,0) — planning prompts must not disqualify a clean delivery", clean, corrections)
	}
}

func TestCleanDeliveryAssistedAfterFailure(t *testing.T) {
	// turn 1 fails; turn 2 (after a new human prompt) fixes it -> not clean, 1 correction.
	evts := []schema.TelemetryEvent{
		prompt(), wrote("pkg/foo.go"), run("go", false),
		prompt(), wrote("pkg/foo.go"), run("go", true), stop(),
	}
	clean, corrections := CleanDelivery(evts)
	if clean || corrections != 1 {
		t.Errorf("CleanDelivery = (%v,%d), want (false,1)", clean, corrections)
	}
}

func TestCleanDeliveryRecoveredFailureNotCounted(t *testing.T) {
	// A failure the agent recovers from within the same turn (the turn ends OK) is
	// not a human correction: an incidental cd/read failure mid-turn must not
	// disqualify an otherwise clean, autonomous delivery. Only an unrecovered
	// failure at hand-off — the prior turn's LAST tool call failed — counts.
	evts := []schema.TelemetryEvent{
		prompt(), run("cd", false), wrote("pkg/foo.go"), run("go", true),
		prompt(), wrote("pkg/foo.go"), run("go", true), stop(),
	}
	clean, corrections := CleanDelivery(evts)
	if !clean || corrections != 0 {
		t.Errorf("CleanDelivery = (%v,%d), want (true,0) — a mid-turn failure the agent recovered from is not a correction", clean, corrections)
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

func TestTaxonomyFeatureWithTests(t *testing.T) {
	// TDD: a feature session writes implementation AND its tests in the same session.
	// Writing test files must not, by itself, make it a "testing" task — when
	// implementation source is present, the session is feature work.
	evts := []schema.TelemetryEvent{
		prompt(), wrote("internal/foo/foo.go"), wrote("internal/foo/foo_test.go"), run("go", true),
	}
	if tt, _ := Taxonomy(evts); tt != TaskFeature {
		t.Errorf("Taxonomy = %q, want feature (tests alongside implementation are feature work)", tt)
	}
}

func TestTaxonomyTestingOnlyWhenNoImplementation(t *testing.T) {
	// A pure test-writing session (no implementation source) is still testing.
	evts := []schema.TelemetryEvent{
		prompt(), wrote("internal/foo/foo_test.go"), wrote("internal/foo/bar_test.go"), run("go", true),
	}
	if tt, _ := Taxonomy(evts); tt != TaskTesting {
		t.Errorf("Taxonomy = %q, want testing (tests only, no implementation)", tt)
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
		{Outcome: Success, CleanDelivery: true, TurnCount: 1, ToolCalls: 4, Started: "2026-07-10T09:00:00Z"},
		{Outcome: Success, CleanDelivery: false, TurnCount: 3, CorrectionTurns: 1, ToolCalls: 8, Started: "2026-07-10T15:00:00Z"},
		{Outcome: Unknown, CleanDelivery: false, TurnCount: 2, ToolCalls: 2, Started: "2026-07-11T09:00:00Z"},
	}
	p := Productivity(sessions)
	if p.Sessions != 3 {
		t.Errorf("Sessions = %d, want 3", p.Sessions)
	}
	// clean-delivery rate over determinate (Success/Failure) sessions: 1 of 2.
	if p.CleanDeliveryRate != 0.5 {
		t.Errorf("CleanDeliveryRate = %v, want 0.5", p.CleanDeliveryRate)
	}
	// correction load = corrections (1) over determinate sessions (2).
	if p.CorrectionLoad != 0.5 {
		t.Errorf("CorrectionLoad = %v, want 0.5", p.CorrectionLoad)
	}
	// CleanCount is the exact clean-delivery tally, not derived from the rate.
	// Here round(rate*Sessions) = round(0.5*3) = 2, but only 1 session landed
	// clean — the indeterminate session must not inflate the count.
	if p.CleanCount != 1 {
		t.Errorf("CleanCount = %d, want 1", p.CleanCount)
	}
	if p.Completed != 2 || p.TotalCorrections != 1 {
		t.Errorf("Completed/TotalCorrections = %d/%d, want 2/1", p.Completed, p.TotalCorrections)
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
