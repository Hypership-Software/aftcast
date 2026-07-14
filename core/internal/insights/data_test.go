package insights

import (
	"fmt"
	"testing"
	"time"

	"github.com/Hypership-Software/atlas/internal/analytics"
	"github.com/Hypership-Software/atlas/internal/schema"
	"github.com/Hypership-Software/atlas/internal/telemetry"
)

func TestCoachWindowUsesLatestSixtyComparableSessions(t *testing.T) {
	var sessions []telemetry.Session
	for i := 0; i < 65; i++ {
		sessions = append(sessions, telemetry.Session{SessionID: fmt.Sprintf("s%02d", i), Started: fmt.Sprintf("2026-07-%02dT10:00:00Z", (i%28)+1),
			CaptureVersion: 2, FilesChanged: 1, TaskType: "feature", PlanStyle: "plan_first"})
	}
	sessions = append(sessions, telemetry.Session{SessionID: "unknown-style", CaptureVersion: 2, FilesChanged: 1})
	got := coachWindow(sessions)
	if len(got) != 60 {
		t.Fatalf("coachWindow = %d, want 60", len(got))
	}
	for _, s := range got {
		if s.PlanStyle == analytics.PlanUnknown {
			t.Fatalf("coach window included unclassifiable session: %+v", s)
		}
	}
}

func TestBuildKeepsFullHistoryButScopesOperationalRowsToSevenDays(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	sessions := []telemetry.Session{
		{SessionID: "recent", Started: now.Add(-time.Hour).Format(time.RFC3339Nano)},
		{SessionID: "history", Started: now.Add(-30 * 24 * time.Hour).Format(time.RFC3339Nano)},
	}
	m := build(sessions, Scope{}, func(string) ([]schema.TelemetryEvent, error) { return nil, nil }, now)
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
