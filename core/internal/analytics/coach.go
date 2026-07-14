package analytics

import (
	"math/big"
	"sort"
)

const (
	coachMinTotal                 = 20
	coachMinPerSide               = 5
	coachMinDifferenceNumerator   = 3
	coachMinDifferenceDenominator = 20
)

type CoachStatus string

const (
	CoachLearning  CoachStatus = "learning"
	CoachNoPattern CoachStatus = "no_pattern"
	CoachRecommend CoachStatus = "recommend"
)

type PlanAssociation struct {
	Status         CoachStatus
	Window         int
	TaskType       string
	Total          int
	Planned        int
	Direct         int
	PlannedShipped int
	DirectShipped  int
	PlannedRate    float64
	DirectRate     float64
	Difference     float64
}

type coachCounts struct {
	planned, direct               int
	plannedShipped, directShipped int
}

func PlanFirstAssociation(sessions []SessionStat) PlanAssociation {
	byTask := map[string]*coachCounts{}
	window := 0
	for _, s := range sessions {
		if !DeliveryEligible(s) || (s.PlanStyle != PlanFirst && s.PlanStyle != PlanDirect) {
			continue
		}
		window++
		task := s.TaskType
		if task == "" {
			task = "other"
		}
		counts := byTask[task]
		if counts == nil {
			counts = &coachCounts{}
			byTask[task] = counts
		}
		if s.PlanStyle == PlanFirst {
			counts.planned++
			if s.Shipped {
				counts.plannedShipped++
			}
		} else {
			counts.direct++
			if s.Shipped {
				counts.directShipped++
			}
		}
	}

	keys := make([]string, 0, len(byTask))
	for task := range byTask {
		keys = append(keys, task)
	}
	sort.Strings(keys)
	var best PlanAssociation
	set := false
	for _, task := range keys {
		candidate := associationFor(task, byTask[task])
		if !set || betterAssociation(candidate, best) {
			best, set = candidate, true
		}
	}
	if !set {
		return PlanAssociation{Status: CoachLearning, Window: window}
	}
	best.Window = window
	return best
}

func associationFor(task string, c *coachCounts) PlanAssociation {
	a := PlanAssociation{TaskType: task, Planned: c.planned, Direct: c.direct,
		PlannedShipped: c.plannedShipped, DirectShipped: c.directShipped}
	a.Total = a.Planned + a.Direct
	if a.Planned > 0 {
		a.PlannedRate = float64(a.PlannedShipped) / float64(a.Planned)
	}
	if a.Direct > 0 {
		a.DirectRate = float64(a.DirectShipped) / float64(a.Direct)
	}
	a.Difference = a.PlannedRate - a.DirectRate
	switch {
	case a.Total < coachMinTotal || a.Planned < coachMinPerSide || a.Direct < coachMinPerSide:
		a.Status = CoachLearning
	case associationDifferenceAtLeast(a, coachMinDifferenceNumerator, coachMinDifferenceDenominator):
		a.Status = CoachRecommend
	default:
		a.Status = CoachNoPattern
	}
	return a
}

func associationDifferenceAtLeast(a PlanAssociation, numerator, denominator int64) bool {
	return associationDifference(a).Cmp(big.NewRat(numerator, denominator)) >= 0
}

func associationDifference(a PlanAssociation) *big.Rat {
	planned := new(big.Rat).SetFrac64(int64(a.PlannedShipped), int64(a.Planned))
	direct := new(big.Rat).SetFrac64(int64(a.DirectShipped), int64(a.Direct))
	return planned.Sub(planned, direct)
}

func betterAssociation(a, b PlanAssociation) bool {
	if coachRank(a.Status) != coachRank(b.Status) {
		return coachRank(a.Status) > coachRank(b.Status)
	}
	if a.Status == CoachRecommend {
		if differenceOrder := compareAssociationDifferences(a, b); differenceOrder != 0 {
			return differenceOrder > 0
		}
	}
	if a.Total != b.Total {
		return a.Total > b.Total
	}
	return a.TaskType < b.TaskType
}

func compareAssociationDifferences(a, b PlanAssociation) int {
	return associationDifference(a).Cmp(associationDifference(b))
}

func coachRank(status CoachStatus) int {
	switch status {
	case CoachRecommend:
		return 2
	case CoachNoPattern:
		return 1
	default:
		return 0
	}
}
