package analytics

import "github.com/Hypership-Software/atlas/internal/schema"

// CleanDelivery reports whether the agent reached a successful outcome without the
// human having to step back in to recover from a failure, and counts those
// corrective turns. A turn is corrective when the turn before it contained a tool
// failure — a failure surfaced and the human re-prompted to deal with it. Planning
// prompts therefore never count against delivery (a read/discussion turn does not
// fail): only failure-driven human corrections do, and a failure the agent recovers
// from on its own — within one prompt, including via subagents — is not one.
//
// Metadata-only per ADR-011: a purely semantic redirection ("no, wrong approach")
// with no tool failure is invisible to this signal, by design.
func CleanDelivery(evts []schema.TelemetryEvent) (clean bool, correctionTurns int) {
	segs := splitTurns(evts)
	for i := 1; i < len(segs); i++ {
		if turnFailed(segs[i-1]) {
			correctionTurns++
		}
	}
	clean = Outcome(evts) == Success && correctionTurns == 0
	return clean, correctionTurns
}
