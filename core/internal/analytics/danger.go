package analytics

import (
	"sort"

	"github.com/Hypership-Software/aftcast/internal/schema"
)

// DangerItem is a count of high-risk actions observed for one rule. Observed, not
// blocked — Aftcast classifies and records; it never denies (ADR-015).
type DangerItem struct {
	RuleID string
	Class  string
	Count  int
}

// Danger aggregates danger-classified events by rule, most frequent first.
func Danger(evts []schema.TelemetryEvent) []DangerItem {
	byRule := map[string]*DangerItem{}
	for _, e := range evts {
		if e.Risk != schema.RiskDanger {
			continue
		}
		it := byRule[e.RuleID]
		if it == nil {
			it = &DangerItem{RuleID: e.RuleID, Class: string(e.ToolClass)}
			byRule[e.RuleID] = it
		}
		it.Count++
	}
	out := make([]DangerItem, 0, len(byRule))
	for _, it := range byRule {
		out = append(out, *it)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].RuleID < out[j].RuleID
	})
	return out
}
