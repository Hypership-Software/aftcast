package insights

import (
	"reflect"
	"testing"
	"time"

	"github.com/Hypership-Software/aftcast/internal/analytics"
	"github.com/Hypership-Software/aftcast/internal/telemetry"
)

func TestGroupProjectsReconcilesHistoricalIdentitiesByRepositoryName(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	sessions := []telemetry.Session{
		{SessionID: "p1-old", ProjectID: "p1", ProjectName: "agent-gate", Started: now.Add(-2 * time.Hour).Format(time.RFC3339Nano), ToolCalls: 1},
		{SessionID: "p1-new", ProjectID: "p1", ProjectName: "agent-gate", Started: now.Add(-time.Hour).Format(time.RFC3339Nano), ToolCalls: 1},
		{SessionID: "p2", ProjectID: "p2", ProjectName: "agent-gate", Started: now.Add(-3 * time.Hour).Format(time.RFC3339Nano), ToolCalls: 1},
		{SessionID: "legacy-a", ProjectName: "kuper", Started: now.Add(-4 * time.Hour).Format(time.RFC3339Nano), ToolCalls: 1},
		{SessionID: "legacy-b", ProjectName: "kuper", Started: now.Add(-5 * time.Hour).Format(time.RFC3339Nano), ToolCalls: 1},
		{SessionID: "other-a", Started: now.Add(-6 * time.Hour).Format(time.RFC3339Nano), ToolCalls: 1},
		{SessionID: "other-b", Started: now.Add(-7 * time.Hour).Format(time.RFC3339Nano), ToolCalls: 1},
	}

	got := groupProjects(sessions, Scope{}, now)
	if len(got) != 3 {
		t.Fatalf("groups = %d, want 3: %+v", len(got), got)
	}
	if got[0].Key != "name:agent-gate" || got[1].Key != "name:kuper" || got[2].Key != "other" {
		t.Fatalf("order = %q, %q, %q", got[0].Key, got[1].Key, got[2].Key)
	}
	if len(got[0].Sessions) != 3 || got[0].Sessions[0].SessionID != "p1-new" {
		t.Fatalf("agent-gate sessions = %+v", got[0].Sessions)
	}
	if got[2].Name != "other project" || len(got[2].Sessions) != 2 {
		t.Fatalf("fallback = %+v", got[2])
	}
}

func TestGroupProjectsAggregatesDeliveryDurationChangesAndWork(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	sessions := []telemetry.Session{
		{
			SessionID: "a", ProjectID: "p1", ProjectName: "agent-gate", Started: now.Add(-2 * time.Hour).Format(time.RFC3339Nano),
			CaptureVersion: 2, FilesChanged: 2, Shipped: true, DurationMS: int64(time.Hour / time.Millisecond), ObservedToolMS: 600,
			ChangedFiles: []string{"a.go", "b.go"}, LinesAdded: 10, LinesRemoved: 4, ChangeStatsCovered: true,
			PlanMS: 100, BuildMS: 400, ReviewMS: 100, WorkMixCovered: true, Outcome: "success",
		},
		{
			SessionID: "b", ProjectID: "p1", ProjectName: "agent-gate", Started: now.Add(-time.Hour).Format(time.RFC3339Nano),
			CaptureVersion: 2, FilesChanged: 2, DurationMS: int64(2 * time.Hour / time.Millisecond), ObservedToolMS: 900,
			ChangedFiles: []string{"b.go", "c.go"}, LinesAdded: 20, LinesRemoved: 5, ChangeStatsCovered: true,
			PlanMS: 200, BuildMS: 500, ReviewMS: 200, WorkMixCovered: true, Outcome: "success",
		},
	}

	got := groupProjects(sessions, Scope{}, now)[0]
	if got.Shipping.Eligible != 2 || got.Shipping.Shipped != 1 || got.Shipping.Rate != .5 {
		t.Fatalf("shipping = %+v", got.Shipping)
	}
	if got.DurationMS != int64(3*time.Hour/time.Millisecond) || got.ObservedToolMS != 1500 {
		t.Fatalf("duration = %+v", got)
	}
	if !reflect.DeepEqual(got.ChangedFiles, []string{"a.go", "b.go", "c.go"}) || got.FilesChanged != 3 {
		t.Fatalf("files = %+v", got)
	}
	if !got.ChangeStatsCovered || got.LinesAdded != 30 || got.LinesRemoved != 9 {
		t.Fatalf("changes = %+v", got)
	}
	if !got.WorkMixCovered || got.PlanMS != 300 || got.BuildMS != 900 || got.ReviewMS != 300 {
		t.Fatalf("work mix = %+v", got)
	}
}

func TestProjectShippedCellUsesCompactPercentage(t *testing.T) {
	if got := projectShippedCell(projectSummary{Shipping: analytics.ShippedProfile{Shipped: 3, Eligible: 4, Rate: .75}}); got != "75%" {
		t.Fatalf("shipped cell = %q", got)
	}
	if got := projectShippedCell(projectSummary{}); got != "—" {
		t.Fatalf("empty shipped cell = %q", got)
	}
}
