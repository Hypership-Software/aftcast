package insights

import (
	"strings"
	"testing"
	"time"

	"github.com/Hypership-Software/atlas/internal/analytics"
	"github.com/Hypership-Software/atlas/internal/telemetry"
	"github.com/charmbracelet/x/ansi"
)

func TestProjectTableRendersCompactRepositoryRows(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	projects := []projectSummary{
		{
			Name: "agent-gate", LastStarted: now.Add(-24 * time.Hour).Format(time.RFC3339Nano),
			Sessions: make([]telemetry.Session, 4), Shipping: analytics.ShippedProfile{Shipped: 3, Eligible: 4, Rate: .75},
			DurationMS: int64(5*time.Hour/time.Millisecond + 12*time.Minute/time.Millisecond),
			LinesAdded: 1284, LinesRemoved: 436, ChangeStatsCovered: true,
		},
		{Name: "unshipped", Sessions: []telemetry.Session{{}}},
	}
	columns := buildProjectColumns(projects, now)
	if got := projectColumnTitles(columns); got != "Project|Active|Sessions|Shipped|Duration|Changes" {
		t.Fatalf("titles = %q", got)
	}
	table := renderSessionTable(columns, 0, 80, maxTableRows)
	for _, want := range []string{"agent-gate", "1d ago", "4", "75%", "5h 12m", "+1.3k/−436", "—"} {
		if !strings.Contains(table, want) {
			t.Fatalf("table missing %q:\n%s", want, table)
		}
	}
	for _, line := range strings.Split(table, "\n") {
		if ansi.StringWidth(line) > 80 {
			t.Fatalf("row width %d > 80: %q", ansi.StringWidth(line), line)
		}
	}
}

func TestRenderProjectsHasNoFlatSessionList(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	out := renderProjects(aggregates{}, analytics.PlanAssociation{}, nil, "PROJECT TABLE")
	for _, want := range []string{"Projects", "PROJECT TABLE", "What's moving your needle"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
	for _, banned := range []string{"Recent sessions", "calls"} {
		if strings.Contains(out, banned) {
			t.Fatalf("output retained %q:\n%s", banned, out)
		}
	}
}

func TestProjectWorkspaceRendersExactDeliveryAndDeveloperMetrics(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	project := projectSummary{
		Name: "agent-gate", Sessions: make([]telemetry.Session, 4),
		Shipping:   analytics.ShippedProfile{Shipped: 3, Eligible: 4, Rate: .75},
		DurationMS: int64(5*time.Hour/time.Millisecond + 12*time.Minute/time.Millisecond), ObservedToolMS: int64(9 * time.Minute / time.Millisecond),
		FilesChanged: 81, LinesAdded: 1284, LinesRemoved: 436, ChangeStatsCovered: true,
		PlanMS: 18, BuildMS: 64, ReviewMS: 18, WorkMixCovered: true,
		Aggregate: aggregates{profile: analytics.Profile{Completed: 4}, securitySessions: 1, danger: 2},
	}
	out := renderProjectWorkspace(project, "SESSION TABLE")
	for _, want := range []string{
		"agent-gate · last 7 days",
		"3 of 4 sessions (75%)",
		"5h 12m wall · 9m observed tool time",
		"81 files · +1,284 / −436 observed",
		"Plan 18% · Build 64% · Review 18%",
		"none across 4 completed sessions",
		"1 session · 2 flagged actions",
		"Recent work",
		"SESSION TABLE",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("workspace missing %q:\n%s", want, out)
		}
	}
}

func TestProjectWorkspaceFallsBackForLegacySessions(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	project := projectSummary{Name: "agent-gate", Sessions: []telemetry.Session{{FilesChanged: 17}}, FilesChanged: 17}
	out := renderProjectWorkspace(project, "TABLE")
	if !strings.Contains(out, "17 files") || !strings.Contains(out, "Starts with your next captured session") {
		t.Fatalf("legacy fallback missing:\n%s", out)
	}
	for _, banned := range []string{"+0 / −0", "0%", "3 of 4"} {
		if strings.Contains(out, banned) {
			t.Fatalf("legacy fallback invented %q:\n%s", banned, out)
		}
	}
}

func projectColumnTitles(columns []tableColumn) string {
	parts := make([]string, len(columns))
	for i, column := range columns {
		parts[i] = column.title
	}
	return strings.Join(parts, "|")
}
