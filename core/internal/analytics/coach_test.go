package analytics

import (
	"fmt"
	"math"
	"testing"

	"github.com/Hypership-Software/aftcast/internal/schema"
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

func TestPlanFirstAssociationRecommendsAtExactRationalThreshold(t *testing.T) {
	got := PlanFirstAssociation(coachCohort(TaskFeature, 10, 6, 20, 9))
	if got.Status != CoachRecommend || got.Direction != AssociationPositive {
		t.Fatalf("association = %+v, want recommendation at exact 6/10 - 9/20 threshold", got)
	}
}

func TestPlanFirstAssociationSurfacesExactNegativeThresholdAsObservation(t *testing.T) {
	got := PlanFirstAssociation(coachCohort(TaskFeature, 20, 9, 10, 6))
	if got.Status != CoachNoPattern || got.Direction != AssociationNegative {
		t.Fatalf("association = %+v, want negative observation at exact 9/20 - 6/10 threshold", got)
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

func TestPlanFirstAssociationSelectsLargestAbsoluteQualifyingDifference(t *testing.T) {
	tests := []struct {
		name     string
		sessions []SessionStat
		wantTask string
		want     AssociationDirection
	}{
		{
			name:     "stronger negative over weaker positive",
			sessions: append(coachCohort(TaskDocs, 10, 6, 20, 9), coachCohort(TaskFeature, 10, 2, 10, 7)...),
			wantTask: TaskFeature,
			want:     AssociationNegative,
		},
		{
			name:     "stronger positive over weaker negative",
			sessions: append(coachCohort(TaskDocs, 20, 9, 10, 6), coachCohort(TaskFeature, 10, 8, 10, 3)...),
			wantTask: TaskFeature,
			want:     AssociationPositive,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PlanFirstAssociation(tt.sessions)
			if got.TaskType != tt.wantTask || got.Direction != tt.want {
				t.Fatalf("association = %+v, want %s %s", got, tt.wantTask, tt.want)
			}
		})
	}
}

func TestPlanFirstAssociationBreaksAbsoluteDifferenceTiesDeterministically(t *testing.T) {
	t.Run("total", func(t *testing.T) {
		sessions := append(coachCohort(TaskDocs, 20, 8, 20, 14), coachCohort(TaskFeature, 10, 7, 10, 4)...)
		got := PlanFirstAssociation(sessions)
		if got.TaskType != TaskDocs || got.Direction != AssociationNegative {
			t.Fatalf("association = %+v, want larger negative docs cohort", got)
		}
	})

	t.Run("task_type", func(t *testing.T) {
		sessions := append(coachCohort(TaskFeature, 10, 7, 10, 4), coachCohort(TaskDocs, 10, 4, 10, 7)...)
		got := PlanFirstAssociation(sessions)
		if got.TaskType != TaskDocs || got.Direction != AssociationNegative {
			t.Fatalf("association = %+v, want lexicographically first negative docs cohort", got)
		}
	})
}

func TestPlanFirstAssociationFilteredEmptyIsLearning(t *testing.T) {
	tests := []struct {
		name     string
		sessions []SessionStat
	}{
		{name: "nil"},
		{name: "ineligible", sessions: []SessionStat{
			{CaptureVersion: schema.DeliverySignalVersion - 1, FilesChanged: 1, TaskType: TaskFeature, PlanStyle: PlanFirst},
		}},
		{name: "unknown_plan", sessions: []SessionStat{
			{CaptureVersion: schema.DeliverySignalVersion, FilesChanged: 1, TaskType: TaskFeature, PlanStyle: PlanUnknown},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := PlanAssociation{Status: CoachLearning}
			if got := PlanFirstAssociation(tt.sessions); got != want {
				t.Fatalf("association = %+v, want %+v", got, want)
			}
		})
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
	tests := []struct {
		name     string
		sessions []SessionStat
	}{
		{name: "planned_below", sessions: coachCohort(TaskFeature, 4, 4, 16, 0)},
		{name: "direct_below", sessions: coachCohort(TaskFeature, 16, 16, 4, 0)},
		{name: "planned_only", sessions: coachCohort(TaskFeature, 20, 20, 0, 0)},
		{name: "direct_only", sessions: coachCohort(TaskFeature, 0, 0, 20, 0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PlanFirstAssociation(tt.sessions); got.Status != CoachLearning {
				t.Fatalf("association = %+v, want learning", got)
			}
		})
	}
}

func TestPlanFirstAssociationSampleBoundariesAreInclusive(t *testing.T) {
	tests := []struct {
		name     string
		sessions []SessionStat
	}{
		{name: "five_planned", sessions: coachCohort(TaskFeature, 5, 5, 15, 0)},
		{name: "five_direct", sessions: coachCohort(TaskFeature, 15, 15, 5, 0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PlanFirstAssociation(tt.sessions)
			if got.Status != CoachRecommend || got.Total != 20 {
				t.Fatalf("association = %+v, want recommendation at inclusive sample boundaries", got)
			}
		})
	}
}

func TestPlanFirstAssociationReportsCohortMetrics(t *testing.T) {
	got := PlanFirstAssociation(coachCohort(TaskFeature, 10, 6, 20, 9))
	if got.Total != 30 || got.Planned != 10 || got.Direct != 20 || got.PlannedShipped != 6 || got.DirectShipped != 9 {
		t.Fatalf("association counts = %+v, want total=30 planned=10 direct=20 planned_shipped=6 direct_shipped=9", got)
	}
	if got.PlannedRate != 0.6 || got.DirectRate != 0.45 || math.Abs(got.Difference-0.15) > 1e-12 {
		t.Fatalf("association rates = %+v, want planned=0.6 direct=0.45 difference=0.15", got)
	}
}

func TestPlanFirstAssociationNormalizesEmptyTaskType(t *testing.T) {
	got := PlanFirstAssociation(coachCohort("", 10, 8, 10, 5))
	if got.TaskType != "other" {
		t.Fatalf("task type = %q, want other: %+v", got.TaskType, got)
	}
}

func TestPlanFirstAssociationMatureNoPatternOutranksImmatureLearning(t *testing.T) {
	sessions := append(coachCohort(TaskDocs, 4, 4, 30, 0), coachCohort(TaskFeature, 10, 6, 10, 5)...)
	got := PlanFirstAssociation(sessions)
	if got.Status != CoachNoPattern || got.TaskType != TaskFeature {
		t.Fatalf("association = %+v, want mature feature no_pattern", got)
	}
}

func TestPlanFirstAssociationBreaksEqualDifferenceTiesDeterministically(t *testing.T) {
	t.Run("total", func(t *testing.T) {
		sessions := append(coachCohort(TaskDocs, 10, 6, 20, 9), coachCohort(TaskFeature, 20, 12, 40, 18)...)
		got := PlanFirstAssociation(sessions)
		if got.TaskType != TaskFeature {
			t.Fatalf("selected %q, want larger feature cohort: %+v", got.TaskType, got)
		}
	})

	t.Run("task_type", func(t *testing.T) {
		sessions := append(coachCohort(TaskFeature, 10, 8, 20, 13), coachCohort(TaskDocs, 10, 6, 20, 9)...)
		got := PlanFirstAssociation(sessions)
		if got.TaskType != TaskDocs {
			t.Fatalf("selected %q, want lexicographically first docs: %+v", got.TaskType, got)
		}
	})
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
