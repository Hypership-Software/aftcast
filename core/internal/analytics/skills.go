package analytics

import "sort"

// SkillStat is per-skill usage and how sessions using it fared.
type SkillStat struct {
	Skill       string
	Sessions    int
	OneShots    int
	Corrections int
}

// SkillReport is the skill view: usage stats, task types that look like skill
// opportunities, and skills seen in tainted sessions.
type SkillReport struct {
	Skills        []SkillStat
	Opportunities []string
	RiskFlags     []string
}

// SkillInsights correlates skills with outcomes. Opportunities are task types that
// needed corrections without any skill in play and that no skill currently covers;
// RiskFlags are skills that appeared in a tainted session.
func SkillInsights(sessions []SessionStat) SkillReport {
	agg := map[string]*SkillStat{}
	var order []string
	coveredType := map[string]bool{}
	oppType := map[string]bool{}
	risk := map[string]bool{}

	for _, s := range sessions {
		for _, sk := range s.Skills {
			a := agg[sk]
			if a == nil {
				a = &SkillStat{Skill: sk}
				agg[sk] = a
				order = append(order, sk)
			}
			a.Sessions++
			if s.OneShot {
				a.OneShots++
			}
			a.Corrections += s.CorrectionTurns
			if s.TaskType != "" {
				coveredType[s.TaskType] = true
			}
			if s.Tainted {
				risk[sk] = true
			}
		}
		if len(s.Skills) == 0 && s.CorrectionTurns > 0 && s.TaskType != "" {
			oppType[s.TaskType] = true
		}
	}

	report := SkillReport{}
	for _, sk := range order {
		report.Skills = append(report.Skills, *agg[sk])
	}
	for tt := range oppType {
		if !coveredType[tt] {
			report.Opportunities = append(report.Opportunities, tt)
		}
	}
	for sk := range risk {
		report.RiskFlags = append(report.RiskFlags, sk)
	}
	sort.Strings(report.Opportunities)
	sort.Strings(report.RiskFlags)
	return report
}
