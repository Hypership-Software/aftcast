package analytics

import "github.com/Hypership-Software/atlas/internal/schema"

// OneShot reports whether the agent completed the task in a single turn with a
// successful outcome, and counts correction turns — turns whose preceding turn
// contained a failure (the user had to re-prompt to fix something).
func OneShot(evts []schema.TelemetryEvent) (oneShot bool, correctionTurns int) {
	segs := splitTurns(evts)
	for i := 1; i < len(segs); i++ {
		if turnFailed(segs[i-1]) {
			correctionTurns++
		}
	}
	oneShot = promptCount(evts) <= 1 && Outcome(evts) == Success
	return oneShot, correctionTurns
}
