package analytics

import (
	"time"

	"github.com/Hypership-Software/aftcast/internal/schema"
)

type Timeline struct {
	ActiveMS  int64
	WaitingMS int64
}

// ObservedTimeline attributes the wall-clock span between recorded events.
// Time after a stop belongs to the user — the turn was over — unless the next
// event is a tool call, which means the harness resumed on its own and the
// agent was still working. Everything inside a turn is agent-active time.
// Unreadable timestamps and clock skew contribute nothing.
func ObservedTimeline(events []schema.TelemetryEvent) Timeline {
	var out Timeline
	var prevType schema.EventType
	var prevTime time.Time
	for _, event := range events {
		t, err := time.Parse(time.RFC3339Nano, event.TS)
		if err != nil {
			continue
		}
		if !prevTime.IsZero() {
			if gap := t.Sub(prevTime).Milliseconds(); gap > 0 {
				if waitingGap(prevType, event.EventType) {
					out.WaitingMS += gap
				} else {
					out.ActiveMS += gap
				}
			}
		}
		prevType = event.EventType
		prevTime = t
	}
	return out
}

func waitingGap(before, after schema.EventType) bool {
	if before != schema.EventStop {
		return false
	}
	return after != schema.EventPreTool && after != schema.EventPostTool
}
