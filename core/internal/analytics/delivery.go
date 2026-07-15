package analytics

import "github.com/Hypership-Software/aftcast/internal/schema"

// CleanDelivery reports whether the agent reached a successful outcome without the
// human having to step back in to recover from a failure, and counts those
// corrective turns. A turn is corrective when the turn before it ENDED on a failed
// tool call — the agent handed control back with something broken and the human
// re-prompted to deal with it. A failure the agent recovered from within the turn
// (a `cd` that failed, a read of a not-yet-created file, a red test that then went
// green — including via subagents) is not a correction, and planning prompts never
// count (a read/discussion turn does not end in failure).
//
// Metadata-only per ADR-011: a purely semantic redirection ("no, wrong approach")
// with no tool failure is invisible to this signal, by design.
func CleanDelivery(evts []schema.TelemetryEvent) (clean bool, correctionTurns int) {
	segs := splitTurns(evts)
	for i := 1; i < len(segs); i++ {
		if turnEndedInFailure(segs[i-1]) {
			correctionTurns++
		}
	}
	clean = Outcome(evts) == Success && correctionTurns == 0
	return clean, correctionTurns
}
