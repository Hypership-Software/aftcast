// Package analytics computes Atlas's insight signals as pure functions over
// telemetry — input is events (per session) or session summaries (cross-session),
// output is values, no I/O. The correctness of the product's value proposition
// lives here (ADR-011), so every function is exhaustively golden-testable and its
// heuristics are transparent. Atlas observes; nothing here is a decision.
package analytics

import "github.com/Hypership-Software/atlas/internal/schema"

// OutcomeClass is a session's derived result. Success/Failure are only asserted
// from evidence; Unknown is a correct answer, never a fabricated success.
type OutcomeClass string

const (
	Success OutcomeClass = "success"
	Failure OutcomeClass = "failure"
	Unknown OutcomeClass = "unknown"
)

// Task types returned by Taxonomy.
const (
	TaskTesting     = "testing"
	TaskFeature     = "feature"
	TaskBugfix      = "bugfix"
	TaskConfig      = "config"
	TaskMigration   = "migration"
	TaskDocs        = "docs"
	TaskInfra       = "infra"
	TaskExploration = "exploration"
)

// SessionStat is the per-session summary the cross-session functions
// (Productivity, SkillInsights) consume. The read-model maps its rows onto this;
// keeping it here leaves analytics dependent on schema alone (no import cycle).
type SessionStat struct {
	Started         string
	Outcome         OutcomeClass
	CleanDelivery   bool
	CaptureVersion  int
	PlanStyle       PlanStyle
	FilesChanged    int
	Shipped         bool
	CorrectionTurns int
	TurnCount       int
	ToolCalls       int
	TaskType        string
	Skills          []string
	Tainted         bool
}

// splitTurns groups a session's seq-ordered events into turns, each beginning at a
// user_prompt. Events before the first prompt attach to the first turn. A stream
// with no prompt is one turn.
func splitTurns(evts []schema.TelemetryEvent) [][]schema.TelemetryEvent {
	var segs [][]schema.TelemetryEvent
	var cur []schema.TelemetryEvent
	seen := false
	for _, e := range evts {
		if e.EventType == schema.EventUserPrompt {
			if seen {
				segs = append(segs, cur)
				cur = nil
			}
			seen = true
		}
		cur = append(cur, e)
	}
	if len(cur) > 0 {
		segs = append(segs, cur)
	}
	return segs
}

func promptCount(evts []schema.TelemetryEvent) int {
	n := 0
	for _, e := range evts {
		if e.EventType == schema.EventUserPrompt {
			n++
		}
	}
	return n
}

// turnEndedInFailure reports whether the turn's LAST completed tool call failed —
// i.e. the agent handed control back to the human with something still broken. A
// failure the agent recovered from mid-turn (a later call in the same turn
// succeeded) is not counted: it was never the human's to fix.
func turnEndedInFailure(seg []schema.TelemetryEvent) bool {
	var last schema.ToolOutcome
	for _, e := range seg {
		if e.EventType == schema.EventPostTool && e.ToolOK != "" {
			last = e.ToolOK
		}
	}
	return last == schema.OutcomeFailed
}
