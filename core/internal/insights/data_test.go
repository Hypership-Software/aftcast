package insights

import (
	"testing"
	"time"

	"github.com/Hypership-Software/atlas/internal/analytics"
	"github.com/Hypership-Software/atlas/internal/schema"
	"github.com/Hypership-Software/atlas/internal/telemetry"
)

func TestCoachWindowUsesLatestSixtyComparableSessions(t *testing.T) {
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	sessions := make([]telemetry.Session, 0, 65)
	for i := 0; i < 65; i++ {
		sessions = append(sessions, telemetry.Session{Started: base.Add(time.Duration(i) * time.Hour).Format(time.RFC3339Nano),
			CaptureVersion: 2, FilesChanged: 1, TaskType: "feature", PlanStyle: "plan_first"})
	}
	got := coachWindow(sessions)
	if len(got) != 60 {
		t.Fatalf("coachWindow = %d, want 60", len(got))
	}
	for i, stat := range got {
		want := base.Add(time.Duration(64-i) * time.Hour).Format(time.RFC3339Nano)
		if stat.Started != want {
			t.Fatalf("coachWindow[%d].Started = %q, want %q", i, stat.Started, want)
		}
	}
}

func TestCoachWindowFiltersBeforeCapAndOnlyAdmitsKnownStyles(t *testing.T) {
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	var sessions []telemetry.Session
	for i := 0; i < 60; i++ {
		style := "plan_first"
		if i%2 == 1 {
			style = "direct_to_edit"
		}
		sessions = append(sessions, telemetry.Session{Started: base.Add(time.Duration(i) * time.Hour).Format(time.RFC3339Nano),
			CaptureVersion: 2, FilesChanged: 1, TaskType: "feature", PlanStyle: style})
	}
	for i := 0; i < 70; i++ {
		style := "invented"
		if i%2 == 1 {
			style = ""
		}
		sessions = append(sessions, telemetry.Session{Started: base.Add(time.Duration(100+i) * time.Hour).Format(time.RFC3339Nano),
			CaptureVersion: 2, FilesChanged: 1, TaskType: "invalid", PlanStyle: style})
	}
	got := coachWindow(sessions)
	if len(got) != 60 {
		t.Fatalf("coachWindow = %d, want 60 valid sessions after filtering", len(got))
	}
	for _, stat := range got {
		if stat.PlanStyle != analytics.PlanFirst && stat.PlanStyle != analytics.PlanDirect {
			t.Fatalf("coach window admitted invalid plan style: %+v", stat)
		}
		if stat.TaskType == "invalid" {
			t.Fatalf("invalid newer session displaced a comparable session: %+v", stat)
		}
	}
}

func TestCoachWindowKeepsStableDuplicateAndInvalidTimestampOrder(t *testing.T) {
	const same = "2026-07-14T10:00:00Z"
	sessions := []telemetry.Session{
		{Started: "invalid-a", CaptureVersion: 2, FilesChanged: 1, TaskType: "invalid-a", PlanStyle: "plan_first"},
		{Started: same, CaptureVersion: 2, FilesChanged: 1, TaskType: "first", PlanStyle: "plan_first"},
		{Started: same, CaptureVersion: 2, FilesChanged: 1, TaskType: "second", PlanStyle: "direct_to_edit"},
		{Started: same, CaptureVersion: 2, FilesChanged: 1, TaskType: "third", PlanStyle: "plan_first"},
		{Started: "2026-07-15T10:00:00Z", CaptureVersion: 2, FilesChanged: 1, TaskType: "newest", PlanStyle: "direct_to_edit"},
		{Started: "invalid-b", CaptureVersion: 2, FilesChanged: 1, TaskType: "invalid-b", PlanStyle: "plan_first"},
	}
	got := coachWindow(sessions)
	want := []string{"newest", "first", "second", "third", "invalid-a", "invalid-b"}
	if len(got) != len(want) {
		t.Fatalf("coachWindow = %d, want %d", len(got), len(want))
	}
	for i, task := range want {
		if got[i].TaskType != task {
			t.Fatalf("coachWindow task order = %+v, want %v", got, want)
		}
	}
}

func TestBuildKeepsFullHistoryButScopesOperationalRowsToSevenDays(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	sessions := []telemetry.Session{
		{SessionID: "recent", Started: now.Add(-time.Hour).Format(time.RFC3339Nano)},
		{SessionID: "history", Started: now.Add(-30 * 24 * time.Hour).Format(time.RFC3339Nano)},
	}
	m := build(sessions, Scope{}, func(string) ([]schema.TelemetryEvent, error) { return nil, nil }, nil, now)
	if len(m.history) != 2 || len(m.global) != 1 || m.global[0].SessionID != "recent" {
		t.Fatalf("history=%d operational=%v", len(m.history), m.global)
	}
}

func TestRecentSessionsFiltersToSevenDayWindow(t *testing.T) {
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	sessions := []telemetry.Session{
		{SessionID: "in-window", Started: now.Add(-2 * 24 * time.Hour).Format(time.RFC3339Nano)},
		{SessionID: "too-old", Started: now.Add(-10 * 24 * time.Hour).Format(time.RFC3339Nano)},
		{SessionID: "unparseable", Started: "not-a-timestamp"},
		{SessionID: "empty-started", Started: ""},
	}
	got := recentSessions(sessions, now)

	ids := map[string]bool{}
	for _, s := range got {
		ids[s.SessionID] = true
	}
	if !ids["in-window"] {
		t.Errorf("recentSessions dropped an in-window session: %v", got)
	}
	if ids["too-old"] {
		t.Errorf("recentSessions kept a session older than 7 days: %v", got)
	}
	if !ids["unparseable"] {
		t.Errorf("recentSessions dropped an unparseable-Started session: %v", got)
	}
	if !ids["empty-started"] {
		t.Errorf("recentSessions dropped an empty-Started session: %v", got)
	}
	if len(got) != 3 {
		t.Fatalf("recentSessions returned %d sessions, want 3: %v", len(got), got)
	}
}
