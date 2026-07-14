package analytics

import (
	"fmt"
	"testing"

	"github.com/Hypership-Software/atlas/internal/schema"
)

func coachCohort(task string, planned, plannedShipped, direct, directShipped int) []SessionStat {
	var out []SessionStat
	for i := 0; i < planned; i++ {
		out = append(out, SessionStat{Started: fmt.Sprintf("2026-07-%02dT10:00:00Z", i+1), CaptureVersion: schema.DeliverySignalVersion,
			FilesChanged: 1, TaskType: task, PlanStyle: PlanFirst, Shipped: i < plannedShipped})
	}
	for i := 0; i < direct; i++ {
		out = append(out, SessionStat{Started: fmt.Sprintf("2026-06-%02dT10:00:00Z", i+1), CaptureVersion: schema.DeliverySignalVersion,
			FilesChanged: 1, TaskType: task, PlanStyle: PlanDirect, Shipped: i < directShipped})
	}
	return out
}

func TestPlanFirstAssociationLearningUntilEverySampleGatePasses(t *testing.T) {
	got := PlanFirstAssociation(coachCohort(TaskFeature, 5, 5, 5, 2))
	if got.Status != CoachLearning || got.Total != 10 {
		t.Fatalf("association = %+v, want learning at n=10", got)
	}
}

func TestPlanFirstAssociationRecommendsAtFifteenPointPositiveDifference(t *testing.T) {
	got := PlanFirstAssociation(coachCohort(TaskFeature, 20, 16, 20, 13))
	if got.Status != CoachRecommend || got.Difference < 0.15 || got.TaskType != TaskFeature {
		t.Fatalf("association = %+v, want positive recommendation", got)
	}
}

func TestPlanFirstAssociationDoesNotRecommendSmallOrNegativeDifference(t *testing.T) {
	for name, sessions := range map[string][]SessionStat{
		"small":    coachCohort(TaskFeature, 10, 6, 10, 5),
		"negative": coachCohort(TaskFeature, 10, 4, 10, 7),
	} {
		t.Run(name, func(t *testing.T) {
			if got := PlanFirstAssociation(sessions); got.Status != CoachNoPattern {
				t.Fatalf("association = %+v, want no_pattern", got)
			}
		})
	}
}

func TestPlanFirstAssociationSelectsLargestQualifyingDifference(t *testing.T) {
	sessions := append(coachCohort(TaskDocs, 10, 8, 10, 6), coachCohort(TaskFeature, 10, 9, 10, 5)...)
	got := PlanFirstAssociation(sessions)
	if got.TaskType != TaskFeature {
		t.Fatalf("selected %q, want feature: %+v", got.TaskType, got)
	}
}

func TestPlanFirstAssociationEmptyIsZero(t *testing.T) {
	if got := PlanFirstAssociation(nil); got != (PlanAssociation{}) {
		t.Fatalf("empty association = %+v, want zero value", got)
	}
}

func TestPlanFirstAssociationAppliesSampleGatesPerTask(t *testing.T) {
	sessions := append(coachCohort(TaskDocs, 5, 5, 5, 0), coachCohort(TaskFeature, 5, 5, 5, 0)...)
	got := PlanFirstAssociation(sessions)
	if got.Status != CoachLearning || got.Total != 10 || got.Window != 20 {
		t.Fatalf("association = %+v, want one learning cohort at n=10 in window 20", got)
	}
}

func TestPlanFirstAssociationLearningWhenEitherSideIsBelowMinimum(t *testing.T) {
	for name, sessions := range map[string][]SessionStat{
		"planned": coachCohort(TaskFeature, 4, 4, 16, 0),
		"direct":  coachCohort(TaskFeature, 16, 16, 4, 0),
	} {
		t.Run(name, func(t *testing.T) {
			if got := PlanFirstAssociation(sessions); got.Status != CoachLearning {
				t.Fatalf("association = %+v, want learning", got)
			}
		})
	}
}

func TestPlanFirstAssociationIgnoresIneligibleAndUnclassifiedSessions(t *testing.T) {
	sessions := coachCohort(TaskFeature, 10, 8, 10, 5)
	sessions = append(sessions,
		SessionStat{CaptureVersion: schema.DeliverySignalVersion - 1, FilesChanged: 1, TaskType: TaskDocs, PlanStyle: PlanFirst, Shipped: true},
		SessionStat{CaptureVersion: schema.DeliverySignalVersion, FilesChanged: 1, TaskType: TaskDocs, PlanStyle: PlanUnknown, Shipped: true},
	)
	got := PlanFirstAssociation(sessions)
	if got.Window != 20 || got.TaskType != TaskFeature || got.Total != 20 {
		t.Fatalf("association = %+v, want only 20 eligible classified feature sessions", got)
	}
}

func TestPlanFirstAssociationBreaksExactTiesByTaskType(t *testing.T) {
	sessions := append(coachCohort(TaskFeature, 10, 8, 10, 5), coachCohort(TaskDocs, 10, 8, 10, 5)...)
	got := PlanFirstAssociation(sessions)
	if got.TaskType != TaskDocs {
		t.Fatalf("selected %q, want lexicographically first docs: %+v", got.TaskType, got)
	}
}
