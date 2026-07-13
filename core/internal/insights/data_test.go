package insights

import (
	"testing"
	"time"

	"github.com/Hypership-Software/atlas/internal/telemetry"
)

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
