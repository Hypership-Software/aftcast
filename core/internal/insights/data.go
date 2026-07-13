// Package insights renders the telemetry read-model as an interactive terminal
// dashboard. It is a pure consumer of internal/telemetry and internal/analytics;
// nothing here observes or decides — it only shows what was captured.
package insights

import (
	"sort"
	"strings"

	"github.com/Hypership-Software/atlas/internal/analytics"
	"github.com/Hypership-Software/atlas/internal/telemetry"
)

type taskCount struct {
	task string
	n    int
}

type aggregates struct {
	profile analytics.Profile
	skills  analytics.SkillReport
	danger  int
	taskMix []taskCount
}

func aggregate(sessions []telemetry.Session) aggregates {
	stats := make([]analytics.SessionStat, len(sessions))
	counts := map[string]int{}
	var order []string
	danger := 0
	for i, s := range sessions {
		stats[i] = toStat(s)
		danger += s.DangerDetected
		tt := s.TaskType
		if tt == "" {
			tt = "unknown"
		}
		if _, ok := counts[tt]; !ok {
			order = append(order, tt)
		}
		counts[tt]++
	}
	sort.Strings(order)
	mix := make([]taskCount, len(order))
	for i, tt := range order {
		mix[i] = taskCount{task: tt, n: counts[tt]}
	}
	return aggregates{
		profile: analytics.Productivity(stats),
		skills:  analytics.SkillInsights(stats),
		danger:  danger,
		taskMix: mix,
	}
}

func toStat(s telemetry.Session) analytics.SessionStat {
	return analytics.SessionStat{
		Started:         s.Started,
		Outcome:         analytics.OutcomeClass(s.Outcome),
		CleanDelivery:   s.CleanDelivery,
		CorrectionTurns: s.CorrectionTurns,
		TurnCount:       s.TurnCount,
		ToolCalls:       s.ToolCalls,
		TaskType:        s.TaskType,
		Skills:          splitSkills(s.SkillsUsed),
		Tainted:         s.Taint,
	}
}

func splitSkills(csv string) []string {
	if csv == "" {
		return nil
	}
	return strings.Split(csv, ",")
}
