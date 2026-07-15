package analytics

import (
	"sort"
	"time"
)

// Profile is the cross-session productivity summary behind the insight surfaces.
type Profile struct {
	Sessions            int
	Completed           int
	TotalCorrections    int
	CleanCount          int     // clean deliveries (exact tally, not derived from the rate)
	CleanDeliveryRate   float64 // clean deliveries / determinate-outcome sessions
	CorrectionLoad      float64 // mean correction turns per determinate-outcome session
	ToolCallsPerSession float64
	SessionsPerDay      float64
	Trend               float64 // clean-delivery rate, later half minus earlier half
}

func Productivity(sessions []SessionStat) Profile {
	p := Profile{Sessions: len(sessions)}
	if len(sessions) == 0 {
		return p
	}
	var determinate, clean, determinateCorrections, calls int
	for _, s := range sessions {
		if s.Outcome == Success || s.Outcome == Failure {
			determinate++
			determinateCorrections += s.CorrectionTurns
			if s.CleanDelivery {
				clean++
			}
		}
		calls += s.ToolCalls
	}
	p.CleanCount = clean
	p.Completed = determinate
	p.TotalCorrections = determinateCorrections
	if determinate > 0 {
		p.CleanDeliveryRate = float64(clean) / float64(determinate)
		p.CorrectionLoad = float64(determinateCorrections) / float64(determinate)
	}
	p.ToolCallsPerSession = float64(calls) / float64(len(sessions))
	p.SessionsPerDay = sessionsPerDay(sessions)
	p.Trend = trend(sessions)
	return p
}

func sessionsPerDay(sessions []SessionStat) float64 {
	days := map[string]struct{}{}
	for _, s := range sessions {
		if d := dayOf(s.Started); d != "" {
			days[d] = struct{}{}
		}
	}
	if len(days) == 0 {
		return 0
	}
	return float64(len(sessions)) / float64(len(days))
}

// trend is the clean-delivery rate of the later half of time-ordered sessions minus
// the earlier half — a coarse "getting better/worse" signal. 0 when too few sessions.
func trend(sessions []SessionStat) float64 {
	if len(sessions) < 2 {
		return 0
	}
	ordered := append([]SessionStat(nil), sessions...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Started < ordered[j].Started })
	mid := len(ordered) / 2
	return cleanRate(ordered[mid:]) - cleanRate(ordered[:mid])
}

func cleanRate(sessions []SessionStat) float64 {
	det, clean := 0, 0
	for _, s := range sessions {
		if s.Outcome == Success || s.Outcome == Failure {
			det++
			if s.CleanDelivery {
				clean++
			}
		}
	}
	if det == 0 {
		return 0
	}
	return float64(clean) / float64(det)
}

func dayOf(ts string) string {
	if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
		return t.UTC().Format("2006-01-02")
	}
	return ""
}
