package insights

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Hypership-Software/atlas/internal/analytics"
	"github.com/Hypership-Software/atlas/internal/schema"
	"github.com/Hypership-Software/atlas/internal/telemetry"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestListFitsTwentyFourLineTerminalWithCoach(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	m := sampleModel()
	m.coach = analytics.PlanAssociation{Status: analytics.CoachRecommend, TaskType: "feature", Total: 24,
		Planned: 10, Direct: 14, PlannedRate: .8, DirectRate: .55}
	m = must(m.Update(tea.WindowSizeMsg{Width: 100, Height: 24}))
	if lines := strings.Count(m.View(), "\n") + 1; lines > 24 {
		t.Fatalf("list rendered %d lines into a 24-line terminal:\n%s", lines, m.View())
	}
}

func TestListFitsTwentyFourLineTerminalWithScrollNotes(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	sessions := make([]telemetry.Session, 11)
	for i := range sessions {
		sessions[i] = telemetry.Session{
			SessionID:   fmt.Sprintf("session-%d", i),
			ProjectID:   fmt.Sprintf("project-%d", i),
			ProjectName: fmt.Sprintf("repo-%d", i),
			TaskType:    "feature",
			ToolCalls:   1,
			Started:     sampleNow.Add(-time.Duration(i) * time.Hour).Format(time.RFC3339Nano),
		}
	}
	m := build(sessions, Scope{}, func(string) ([]schema.TelemetryEvent, error) { return nil, nil }, sampleNow)
	m.coach = analytics.PlanAssociation{Status: analytics.CoachRecommend, TaskType: "feature", Total: 24,
		Planned: 10, Direct: 14, PlannedRate: .8, DirectRate: .55}
	m.projectCursor = 5
	m = must(m.Update(tea.WindowSizeMsg{Width: 100, Height: 24}))
	if lines := strings.Count(m.View(), "\n") + 1; lines > 24 {
		t.Fatalf("list with scroll notes rendered %d lines into a 24-line terminal:\n%s", lines, m.View())
	}
	if !strings.Contains(m.View(), "▸") || !strings.Contains(m.View(), "repo-5") {
		t.Fatalf("height-limited list omitted the selected project:\n%s", m.View())
	}
}

func complexHeightModel() model {
	tasks := []string{"feature", "bugfix", "docs"}
	sessions := make([]telemetry.Session, 10)
	for i := range sessions {
		session := telemetry.Session{
			SessionID:    fmt.Sprintf("complex-%d", i),
			ProjectID:    fmt.Sprintf("project-%d", i),
			ProjectName:  tasks[i%len(tasks)] + fmt.Sprintf("-%d", i),
			TaskType:     tasks[i%len(tasks)],
			ToolCalls:    2,
			FilesTouched: 1,
			Started:      sampleNow.Add(-time.Duration(i) * time.Hour).Format(time.RFC3339Nano),
		}
		if i < 3 {
			session.Taint = true
			session.DangerDetected = 1
		}
		sessions[i] = session
	}
	m := build(sessions, Scope{}, func(string) ([]schema.TelemetryEvent, error) { return nil, nil }, sampleNow)
	m.coach = analytics.PlanAssociation{Status: analytics.CoachRecommend, Window: 24, TaskType: "feature", Total: 24,
		Planned: 10, Direct: 14, PlannedRate: .8, DirectRate: .55}
	m.projectCursor = 4
	return m
}

func visualRowCount(view string, width int) int {
	if view == "" {
		return 0
	}
	if width <= 0 {
		return 1
	}
	return strings.Count(ansi.Hardwrap(view, width, true), "\n") + 1
}

func TestComplexRecommendationListFitsTwentyFourLines(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	m := complexHeightModel()
	m = must(m.Update(tea.WindowSizeMsg{Width: 80, Height: 24}))
	view := m.View()
	if rows := visualRowCount(view, m.width); rows > m.height {
		t.Fatalf("complex list rendered %d visual rows into height %d at width %d:\n%s", rows, m.height, m.width, view)
	}
	for _, want := range []string{
		"Shipped", "Work observed", "Corrections", "Security", "feature", "bugfix", "docs",
		"What's moving your needle", "Try next", "Projects", "Project",
		"q quit",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("complex height-constrained list missing %q:\n%s", want, view)
		}
	}
}

func TestKnownWidthsAndTinyHeightsNeverOverflow(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	for _, width := range []int{80, 48, 19, 1} {
		for _, height := range []int{0, 1, 2, 3, 8, 16, 23, 24} {
			t.Run(fmt.Sprintf("width_%d_height_%d", width, height), func(t *testing.T) {
				m := complexHeightModel()
				m = must(m.Update(tea.WindowSizeMsg{Width: width, Height: height}))
				if rows := visualRowCount(m.View(), width); rows > height {
					t.Fatalf("rendered %d visual rows into height %d at width %d:\n%s", rows, height, width, m.View())
				}
			})
		}
	}
}

func TestKnownNonPositiveWidthFailsClosed(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	for _, width := range []int{0, -1} {
		m := complexHeightModel()
		m = must(m.Update(tea.WindowSizeMsg{Width: width, Height: 24}))
		if got := m.View(); got != "" {
			t.Fatalf("known width %d returned terminal content: %q", width, got)
		}
	}
}

func TestHeightFallbackWrapsUnicodeAndANSIWithoutOverflow(t *testing.T) {
	view := "\x1b[31mstatus 界界界界界\x1b[0m\n\x1b[1mcoach recommendation is deliberately long\x1b[0m\n? help · q quit"
	got := fitViewHeight(view, 8, 3)
	if rows := visualRowCount(got, 8); rows > 3 {
		t.Fatalf("fallback rendered %d visual rows:\n%q", rows, got)
	}
	for _, line := range strings.Split(got, "\n") {
		if width := ansi.StringWidth(line); width > 8 {
			t.Fatalf("fallback line width = %d, want <= 8: %q", width, line)
		}
	}
	if strings.Contains(got, "\x1b") {
		t.Fatalf("fallback retained ANSI after a visual-row crop: %q", got)
	}
}

func TestSingleLineTableBudgetKeepsCombinedStatus(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	m := complexHeightModel()
	m = must(m.Update(tea.WindowSizeMsg{Width: 100, Height: 19}))
	view := m.View()
	if rows := visualRowCount(view, m.width); rows > m.height {
		t.Fatalf("single-line table budget rendered %d visual rows into height %d:\n%s", rows, m.height, view)
	}
	for _, want := range []string{"▸", "4h ago", "q quit"} {
		if !strings.Contains(view, want) {
			t.Fatalf("single-line table budget omitted %q:\n%s", want, view)
		}
	}
}

func TestZeroRowStatusTracksKeyboardNavigation(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	m := complexHeightModel()
	m.projectCursor = 0
	m = must(m.Update(tea.WindowSizeMsg{Width: 100, Height: 19}))
	assertSelected := func(want string) {
		t.Helper()
		for _, line := range strings.Split(m.View(), "\n") {
			if strings.Contains(line, "▸") && strings.Contains(line, want) {
				return
			}
		}
		t.Fatalf("selected row missing %q at cursor %d:\n%s", want, m.projectCursor, m.View())
	}
	assertSelected("just now")
	for range 4 {
		m = must(m.Update(key("j")))
	}
	assertSelected("4h ago")
	for range 4 {
		m = must(m.Update(key("down")))
	}
	assertSelected("8h ago")
	m = must(m.Update(key("k")))
	assertSelected("7h ago")
}

func TestKnownAmpleHeightPreservesNormalView(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	m := complexHeightModel()
	m.width = 100
	want := m.View()
	m = must(m.Update(tea.WindowSizeMsg{Width: 100, Height: 100}))
	if got := m.View(); got != want {
		t.Fatalf("ample known height changed normal view\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func TestUnknownHeightRetainsNormalList(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	view := complexHeightModel().View()
	for _, want := range []string{"Projects", "Try next", "4h ago", "q quit"} {
		if !strings.Contains(view, want) {
			t.Fatalf("unknown-height list missing %q:\n%s", want, view)
		}
	}
}

func TestOverviewColumnsKeepSecurityOutOfTheMainTable(t *testing.T) {
	m := sampleModel()
	var titles []string
	for _, col := range m.buildColumns() {
		titles = append(titles, col.title)
	}
	if got := strings.Join(titles, "|"); got != "Project|Active|Sessions|Shipped|Duration|Changes" {
		t.Fatalf("overview columns = %q", got)
	}
}

func TestSecuritySurfaceSelectsOnlyFlaggedAndReturnsFromDetail(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	clean := telemetry.Session{SessionID: "clean", ProjectName: "atlas", ProjectID: "p1", Outcome: "success", ToolCalls: 3,
		Started: sampleNow.Add(-2 * time.Hour).Format(time.RFC3339Nano)}
	flagged := telemetry.Session{SessionID: "flagged", ProjectName: "agent-gate", ProjectID: "p2", Outcome: "success", ToolCalls: 7, FilesChanged: 2,
		Taint: true, DangerDetected: 2, Started: sampleNow.Add(-time.Hour).Format(time.RFC3339Nano)}
	provider := func(id string) ([]schema.TelemetryEvent, error) {
		return []schema.TelemetryEvent{{SessionID: id, EventType: schema.EventUserPrompt}}, nil
	}
	m := build([]telemetry.Session{clean, flagged}, Scope{}, provider, sampleNow)
	m = must(m.Update(tea.KeyMsg{Type: tea.KeyTab}))
	if m.surface != surfaceSecurity || len(m.sessions) != 1 || m.sessions[0].SessionID != "flagged" {
		t.Fatalf("security surface selected %#v", m.sessions)
	}
	for _, want := range []string{"[Security]", "Security review", "agent-gate", "untrusted input + 2 flagged actions", "2 changed · 7 calls"} {
		if !strings.Contains(m.View(), want) {
			t.Fatalf("security view missing %q:\n%s", want, m.View())
		}
	}

	m = must(m.Update(tea.KeyMsg{Type: tea.KeyEnter}))
	if m.mode != modeDetail || m.detailSess.SessionID != "flagged" {
		t.Fatalf("security Enter opened %q in mode %v", m.detailSess.SessionID, m.mode)
	}
	m = must(m.Update(tea.KeyMsg{Type: tea.KeyEsc}))
	if m.mode != modeList || m.surface != surfaceSecurity {
		t.Fatalf("Esc lost originating surface: mode=%v surface=%v", m.mode, m.surface)
	}
	m = must(m.Update(tea.KeyMsg{Type: tea.KeyTab}))
	if m.surface != surfaceOverview || len(m.projects) != 2 || len(m.sessions) != 0 {
		t.Fatalf("Tab did not restore Projects: surface=%v projects=%d sessions=%d", m.surface, len(m.projects), len(m.sessions))
	}
}

func TestSecuritySurfaceHonoursScopeAndHasHonestEmptyState(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	sessions := []telemetry.Session{
		{SessionID: "p1-clean", ProjectID: "p1", ToolCalls: 2},
		{SessionID: "p1-danger", ProjectID: "p1", ToolCalls: 2, DangerDetected: 1},
		{SessionID: "p2-taint", ProjectID: "p2", ToolCalls: 2, Taint: true},
	}
	provider := func(string) ([]schema.TelemetryEvent, error) { return nil, nil }
	m := build(sessions, Scope{ProjectID: "p1", Name: "atlas"}, provider, sampleNow)
	m = must(m.Update(tea.KeyMsg{Type: tea.KeyTab}))
	if len(m.sessions) != 1 || m.sessions[0].SessionID != "p1-danger" {
		t.Fatalf("scoped security rows = %#v", m.sessions)
	}
	if !strings.Contains(m.View(), "1 flagged action") || strings.Contains(m.View(), "untrusted input") {
		t.Fatalf("danger-only signal copy is wrong:\n%s", m.View())
	}

	empty := build([]telemetry.Session{{SessionID: "clean", ToolCalls: 1}}, Scope{}, provider, sampleNow)
	empty = must(empty.Update(tea.KeyMsg{Type: tea.KeyTab}))
	if got := empty.View(); !strings.Contains(got, "Nothing needs review in this scope.") {
		t.Fatalf("security empty state = %q", got)
	}

	noHistory := build(nil, Scope{}, provider, sampleNow)
	noHistory = must(noHistory.Update(tea.KeyMsg{Type: tea.KeyTab}))
	if got := noHistory.View(); !strings.Contains(got, "Nothing needs review in this scope.") {
		t.Fatalf("no-history security empty state = %q", got)
	}
}

func TestSecurityColumnsAndRecentSortAreStable(t *testing.T) {
	sessions := []telemetry.Session{
		{SessionID: "older", ToolCalls: 2, Taint: true, Started: sampleNow.Add(-2 * time.Hour).Format(time.RFC3339Nano)},
		{SessionID: "newer", ToolCalls: 2, DangerDetected: 1, Started: sampleNow.Add(-time.Hour).Format(time.RFC3339Nano)},
	}
	m := build(sessions, Scope{}, func(string) ([]schema.TelemetryEvent, error) { return nil, nil }, sampleNow)
	m = must(m.Update(tea.KeyMsg{Type: tea.KeyTab}))
	var titles []string
	for _, col := range m.buildColumns() {
		titles = append(titles, col.title)
	}
	if got := strings.Join(titles, "|"); got != "Project|When|Result|Signal|Work" {
		t.Fatalf("security columns = %q", got)
	}
	if m.sessions[0].SessionID != "newer" {
		t.Fatalf("security rows not recent-first: %#v", m.sessions)
	}
}

func TestSecuritySurfaceFitsEightyByTwentyFour(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var sessions []telemetry.Session
	for i := 0; i < 20; i++ {
		sessions = append(sessions, telemetry.Session{
			SessionID: fmt.Sprintf("flagged-%02d", i), ProjectName: "agent-gate", Outcome: "success",
			ToolCalls: 10 + i, FilesChanged: i, Taint: i%2 == 0, DangerDetected: 1,
			Started: sampleNow.Add(-time.Duration(i) * time.Hour).Format(time.RFC3339Nano),
		})
	}
	m := build(sessions, Scope{}, func(string) ([]schema.TelemetryEvent, error) { return nil, nil }, sampleNow)
	m = must(m.Update(tea.KeyMsg{Type: tea.KeyTab}))
	m = must(m.Update(tea.WindowSizeMsg{Width: 80, Height: 24}))
	if rows := visualRowCount(m.View(), 80); rows > 24 {
		t.Fatalf("security rendered %d rows:\n%s", rows, m.View())
	}
	for _, want := range []string{"Security review", "Signal", "more sessions below", "tab projects"} {
		if !strings.Contains(strings.ToLower(m.View()), strings.ToLower(want)) {
			t.Fatalf("security view missing %q:\n%s", want, m.View())
		}
	}
}

func historicalCoachSessions(now time.Time) []telemetry.Session {
	var sessions []telemetry.Session
	for i := 0; i < 20; i++ {
		style := "direct_to_edit"
		shipped := false
		if i < 10 {
			style = "plan_first"
			shipped = true
		}
		sessions = append(sessions, telemetry.Session{
			SessionID: fmt.Sprintf("history-%d", i), ProjectID: "project-one", TaskType: "feature",
			CaptureVersion: 2, FilesChanged: 1, ToolCalls: 2, PlanStyle: style, Shipped: shipped,
			Started: now.Add(-8*24*time.Hour - time.Duration(i)*time.Hour).Format(time.RFC3339Nano),
		})
	}
	return sessions
}

func TestHistoricalCoachRendersWhenGlobalOperationalScopeIsEmpty(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	m := build(historicalCoachSessions(sampleNow), Scope{}, func(string) ([]schema.TelemetryEvent, error) { return nil, nil }, sampleNow)
	view := m.View()
	if !strings.Contains(view, renderCoach(m.coach)) {
		t.Fatalf("empty operational view omitted full-history coach:\n%s", view)
	}
	if !strings.Contains(view, "No Atlas activity in the last 7 days") || strings.Contains(view, "Nothing captured") {
		t.Fatalf("historical empty view used dishonest onboarding copy:\n%s", view)
	}
}

func TestHistoricalProjectEmptyCopyDistinguishesSameProjectHistory(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	tests := []struct {
		name       string
		projectID  string
		want       string
		wantGlobal string
	}{
		{name: "same project", projectID: "project-one", want: "No Atlas activity for this project in the last 7 days.", wantGlobal: "No Atlas activity in the last 7 days."},
		{name: "other project only", projectID: "project-two", want: "No Atlas activity for this project yet.", wantGlobal: "No Atlas activity in the last 7 days."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := build(historicalCoachSessions(sampleNow), Scope{ProjectID: tt.projectID, Name: tt.projectID}, func(string) ([]schema.TelemetryEvent, error) { return nil, nil }, sampleNow)
			wantCoach := renderCoach(m.coach)
			if view := m.View(); !strings.Contains(view, tt.projectID+" · last 7 days") || !strings.Contains(view, "Starts with your next captured session") || strings.Contains(view, wantCoach) {
				t.Fatalf("project empty workspace is wrong:\n%s", view)
			}
			m = must(m.Update(key("g")))
			if view := m.View(); !strings.Contains(view, tt.wantGlobal) || !strings.Contains(view, wantCoach) {
				t.Fatalf("global empty view changed copy or coach:\n%s", view)
			}
			m = must(m.Update(key("p")))
			if view := m.View(); !strings.Contains(view, tt.projectID+" · last 7 days") || strings.Contains(view, wantCoach) {
				t.Fatalf("project empty workspace after p is wrong:\n%s", view)
			}
		})
	}
}

func TestCoachRemainsVisibleAcrossPopulatedAndEmptyScopeToggles(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	sessions := historicalCoachSessions(sampleNow)
	sessions = append(sessions, telemetry.Session{SessionID: "recent-other", ProjectID: "project-two", TaskType: "docs",
		ToolCalls: 2, Started: sampleNow.Add(-time.Hour).Format(time.RFC3339Nano)})
	m := build(sessions, Scope{ProjectID: "project-one", Name: "project-one"}, func(string) ([]schema.TelemetryEvent, error) { return nil, nil }, sampleNow)
	wantCoach := renderCoach(m.coach)
	if strings.Contains(m.View(), wantCoach) || !strings.Contains(m.View(), "project-one · last 7 days") {
		t.Fatalf("project workspace should not repeat global coach:\n%s", m.View())
	}
	m = must(m.Update(key("g")))
	if !strings.Contains(m.View(), wantCoach) || !strings.Contains(m.View(), "other project") {
		t.Fatalf("populated global view changed or omitted coach:\n%s", m.View())
	}
	m = must(m.Update(key("p")))
	if strings.Contains(m.View(), wantCoach) || !strings.Contains(m.View(), "project-one · last 7 days") {
		t.Fatalf("project workspace after p is wrong:\n%s", m.View())
	}
}

var sampleNow = time.Date(2026, 7, 13, 15, 0, 0, 0, time.UTC)

func sampleModel() model {
	sessions := []telemetry.Session{
		{SessionID: "aaaa1111", Harness: "claudecode", TaskType: "feature", Outcome: "success", CleanDelivery: true, TurnCount: 3, ToolCalls: 4, FilesChanged: 2, FilesTouched: 2, Started: sampleNow.Add(-2 * time.Hour).Format(time.RFC3339Nano)},
		{SessionID: "bbbb2222", Harness: "claudecode", TaskType: "bugfix", Outcome: "failure", CorrectionTurns: 2, TurnCount: 6, ToolCalls: 11, FilesChanged: 5, FilesTouched: 5, Started: sampleNow.Add(-3 * time.Hour).Format(time.RFC3339Nano)},
	}
	provider := func(id string) ([]schema.TelemetryEvent, error) {
		return []schema.TelemetryEvent{
			{SessionID: id, EventType: schema.EventPreTool, ToolClass: schema.ClassNetFetch,
				ToolUseID: "t1", Domain: "example.com", Subagent: "researcher"},
			{SessionID: id, EventType: schema.EventPostTool, ToolUseID: "t1", ToolOK: schema.OutcomeOK},
		}, nil
	}
	return build(sessions, Scope{}, provider, sampleNow)
}

func TestOverviewSummarizesSecurityWithoutCrowdingSessionRows(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	s := telemetry.Session{SessionID: "cccc3333", Harness: "claudecode", TaskType: "testing",
		Outcome: "success", CorrectionTurns: 1, ToolCalls: 179, FilesTouched: 44,
		Taint: true, DangerDetected: 11, SkillsUsed: "a,b,c,d",
		Started: sampleNow.Add(-20 * time.Hour).Format(time.RFC3339Nano)}
	provider := func(string) ([]schema.TelemetryEvent, error) { return nil, nil }
	m := build([]telemetry.Session{s}, Scope{}, provider, sampleNow)
	v := m.View()
	for _, want := range []string{"Security", "1 session needs review", "11 flagged actions"} {
		if !strings.Contains(v, want) {
			t.Fatalf("overview omitted security summary %q:\n%s", want, v)
		}
	}
	for _, banned := range []string{"Flags", "⚠ untrusted input", "★ 4 skills"} {
		if strings.Contains(v, banned) {
			t.Fatalf("overview row retained security detail %q:\n%s", banned, v)
		}
	}
}

func TestListViewRendersSessions(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	m := sampleModel()
	if v := m.View(); !strings.Contains(v, "Projects") || !strings.Contains(v, "other project") || !strings.Contains(v, "2") {
		t.Fatalf("projects view missing repository summary: %q", v)
	}
	m = must(m.Update(key("enter")))
	v := m.View()
	if !strings.Contains(v, "feature") || !strings.Contains(v, "2h ago") {
		t.Fatalf("project view missing session row: %q", v)
	}
	if !strings.Contains(v, "2 files") || strings.Contains(v, "4 calls") {
		t.Fatalf("project view has wrong change cell: %q", v)
	}
	if !strings.Contains(v, "succeeded") {
		t.Fatalf("list view missing result: %q", v)
	}
}

func TestVisibleSessionsHidesEmptyByDefault(t *testing.T) {
	ss := []telemetry.Session{{SessionID: "a", ToolCalls: 5}, {SessionID: "b", ToolCalls: 0}}
	if got := visibleSessions(ss, false); len(got) != 1 || got[0].SessionID != "a" {
		t.Errorf("hide-empty got %v", got)
	}
	if got := visibleSessions(ss, true); len(got) != 2 {
		t.Errorf("show-empty got %d", len(got))
	}
}

func TestSessionRowIsHumanReadable(t *testing.T) {
	now := time.Date(2026, 7, 13, 15, 0, 0, 0, time.UTC)
	s := telemetry.Session{SessionID: "32b1a075x", TaskType: "testing", Outcome: "success",
		CleanDelivery: true, ToolCalls: 119, FilesChanged: 17, FilesTouched: 30, Started: "2026-07-13T13:00:00Z"}
	row := sessionRow(s, now)
	joined := strings.Join(row, " | ")
	for _, want := range []string{"2h ago", "testing", "succeeded", "17 changed · 119 calls"} {
		if !strings.Contains(joined, want) {
			t.Errorf("row %q missing %q", joined, want)
		}
	}
}

func TestFlagsCellUsesSingularSkill(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	if got := flagsCell(telemetry.Session{SkillsUsed: "strategic-review"}); got != "★ 1 skill" {
		t.Fatalf("flagsCell = %q, want singular skill", got)
	}
}

func TestOutcomeCellPrefersObservedShipment(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	got := outcomeCell(telemetry.Session{Shipped: true, Outcome: "unknown"})
	if !strings.Contains(got, "shipped") {
		t.Fatalf("outcome cell = %q", got)
	}
}

func TestEnterOpensDetailRawTogglesEscReturns(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	m := sampleModel()
	m = must(m.Update(tea.WindowSizeMsg{Width: 100, Height: 40}))
	m = must(m.Update(tea.KeyMsg{Type: tea.KeyEnter}))
	if m.mode != modeProject {
		t.Fatalf("first enter did not switch to project mode")
	}
	m = must(m.Update(tea.KeyMsg{Type: tea.KeyEnter}))
	if m.mode != modeDetail {
		t.Fatalf("second enter did not switch to detail mode")
	}
	if !strings.Contains(m.View(), "untrusted") || strings.Contains(m.View(), "Timeline") {
		t.Fatalf("detail view missing summarized signal: %q", m.View())
	}
	m = must(m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}}))
	if !m.showRaw {
		t.Fatalf("r did not toggle raw")
	}
	if !strings.Contains(m.View(), "subagent") {
		t.Fatalf("raw detail view missing subagent field: %q", m.View())
	}
	m = must(m.Update(tea.KeyMsg{Type: tea.KeyEsc}))
	if m.mode != modeProject {
		t.Fatalf("esc did not return to project")
	}
	m = must(m.Update(tea.KeyMsg{Type: tea.KeyEsc}))
	if m.mode != modeList {
		t.Fatalf("second esc did not return to projects")
	}
}

func TestQuitKey(t *testing.T) {
	_, cmd := sampleModel().Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatalf("q produced no command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("q did not produce QuitMsg")
	}
}

func TestEmptyState(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	m := build(nil, Scope{}, func(string) ([]schema.TelemetryEvent, error) { return nil, nil }, sampleNow)
	if !strings.Contains(m.View(), "Nothing captured") {
		t.Fatalf("empty model should show empty state: %q", m.View())
	}
}

func TestHelpOverlayToggles(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	m := build(nil, Scope{}, func(string) ([]schema.TelemetryEvent, error) { return nil, nil }, sampleNow)
	m = must(m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}}))
	if !strings.Contains(m.View(), "help") {
		t.Error("? did not open help overlay")
	}
}

func TestHelpOverlayClosesBackToPreviousMode(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	m := sampleModel()
	m = must(m.Update(tea.WindowSizeMsg{Width: 100, Height: 40}))
	m = must(m.Update(tea.KeyMsg{Type: tea.KeyEnter}))
	m = must(m.Update(tea.KeyMsg{Type: tea.KeyEnter}))
	if m.mode != modeDetail {
		t.Fatalf("enter did not switch to detail mode")
	}
	m = must(m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}}))
	if m.mode != modeHelp {
		t.Fatalf("? from detail did not open help, got mode %v", m.mode)
	}
	m = must(m.Update(tea.KeyMsg{Type: tea.KeyEsc}))
	if m.mode != modeDetail {
		t.Fatalf("esc from help should return to detail, got mode %v", m.mode)
	}

	m = must(m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}}))
	if m.mode != modeHelp {
		t.Fatalf("? from detail (second time) did not open help, got mode %v", m.mode)
	}
	m = must(m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}}))
	if m.mode != modeDetail {
		t.Fatalf("? from help should return to detail, got mode %v", m.mode)
	}

	m.mode = modeList
	m = must(m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}}))
	if m.mode != modeHelp {
		t.Fatalf("? from list did not open help, got mode %v", m.mode)
	}
	m = must(m.Update(tea.KeyMsg{Type: tea.KeyEsc}))
	if m.mode != modeList {
		t.Fatalf("esc from help should return to list, got mode %v", m.mode)
	}
}

func TestHelpOverlayQuitKey(t *testing.T) {
	quitKeys := map[string]tea.KeyMsg{
		"q":      {Type: tea.KeyRunes, Runes: []rune{'q'}},
		"ctrl+c": {Type: tea.KeyCtrlC},
	}
	for name, key := range quitKeys {
		m := sampleModel()
		m = must(m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}}))
		_, cmd := m.Update(key)
		if cmd == nil {
			t.Fatalf("%s from help produced no command", name)
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatalf("%s from help did not produce QuitMsg", name)
		}
	}
}

// must unwraps an Update result back to the concrete model for assertions.
func must(mdl tea.Model, _ tea.Cmd) model { return mdl.(model) }

func key(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func TestProjectScopeToggle(t *testing.T) {
	sessions := []telemetry.Session{
		{SessionID: "a", ProjectID: "p1", ToolCalls: 3},
		{SessionID: "b", ProjectID: "p2", ToolCalls: 4},
	}
	provider := func(string) ([]schema.TelemetryEvent, error) { return nil, nil }
	m := build(sessions, Scope{ProjectID: "p1", Name: "proj-one"}, provider, sampleNow)
	if len(m.all) != 1 || m.all[0].SessionID != "a" {
		t.Fatalf("project scope should show only p1, got %d", len(m.all))
	}
	gm, _ := m.updateList(key("g"))
	if len(gm.(model).all) != 2 {
		t.Fatalf("global should show all, got %d", len(gm.(model).all))
	}
	if gm.(model).agg.scopeLabel != "all projects" {
		t.Errorf("global agg.scopeLabel = %q, want \"all projects\"", gm.(model).agg.scopeLabel)
	}
	pm, _ := gm.(model).updateList(key("p"))
	if len(pm.(model).all) != 1 {
		t.Fatalf("back to project should show 1, got %d", len(pm.(model).all))
	}
	if pm.(model).agg.scopeLabel != "proj-one" {
		t.Errorf("project agg.scopeLabel = %q, want \"proj-one\"", pm.(model).agg.scopeLabel)
	}
}

func TestProjectCell(t *testing.T) {
	provider := func(string) ([]schema.TelemetryEvent, error) { return nil, nil }
	m := build(nil, Scope{ProjectID: "p1abcdef", Name: "myproj"}, provider, sampleNow)
	if got := m.projectCell(telemetry.Session{ProjectID: "otherhash1234", ProjectName: "agent-gate"}); got != "agent-gate" {
		t.Errorf("resolved project cell = %q, want agent-gate", got)
	}
	if got := m.projectCell(telemetry.Session{ProjectID: "p1abcdef"}); got != "myproj" {
		t.Errorf("current project cell = %q, want myproj", got)
	}
	if got := m.projectCell(telemetry.Session{ProjectID: "otherhash1234"}); got != "other project" {
		t.Errorf("other project cell = %q, want other project", got)
	}
	if got := m.projectCell(telemetry.Session{ProjectID: ""}); got != "other project" {
		t.Errorf("empty project cell = %q, want other project", got)
	}
}

func TestProjectNavigationOpensRepositoryBeforeSessionAndRestoresSelection(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	sessions := []telemetry.Session{
		{SessionID: "a1", ProjectID: "p1", ProjectName: "agent-gate", ToolCalls: 2, Started: sampleNow.Add(-time.Hour).Format(time.RFC3339Nano)},
		{SessionID: "a2", ProjectID: "p1", ProjectName: "agent-gate", ToolCalls: 2, Started: sampleNow.Add(-2 * time.Hour).Format(time.RFC3339Nano)},
		{SessionID: "k1", ProjectID: "p2", ProjectName: "kuper", ToolCalls: 2, Started: sampleNow.Add(-3 * time.Hour).Format(time.RFC3339Nano)},
	}
	provider := func(string) ([]schema.TelemetryEvent, error) { return nil, nil }
	m := build(sessions, Scope{ProjectID: "p1", Name: "agent-gate", StartGlobal: true}, provider, sampleNow)
	if m.mode != modeList || len(m.projects) != 2 {
		t.Fatalf("global start = mode %v projects %d", m.mode, len(m.projects))
	}
	m = must(m.Update(key("enter")))
	if m.mode != modeProject || m.selectedProject.Name != "agent-gate" || len(m.sessions) != 2 {
		t.Fatalf("opened project = mode %v project %+v sessions %d", m.mode, m.selectedProject, len(m.sessions))
	}
	m = must(m.Update(key("down")))
	m = must(m.Update(key("enter")))
	if m.mode != modeDetail || m.detailSess.SessionID != "a2" {
		t.Fatalf("opened session = mode %v session %q", m.mode, m.detailSess.SessionID)
	}
	m = must(m.Update(key("esc")))
	if m.mode != modeProject || m.cursor != 1 {
		t.Fatalf("detail return = mode %v cursor %d", m.mode, m.cursor)
	}
	m = must(m.Update(key("esc")))
	if m.mode != modeList || m.projectCursor != 0 {
		t.Fatalf("project return = mode %v cursor %d", m.mode, m.projectCursor)
	}
}

func TestCurrentProjectStartsInWorkspaceAndGReturnsToProjects(t *testing.T) {
	sessions := []telemetry.Session{
		{SessionID: "a", ProjectID: "p1", ProjectName: "agent-gate", ToolCalls: 2, Started: sampleNow.Format(time.RFC3339Nano)},
		{SessionID: "k", ProjectID: "p2", ProjectName: "kuper", ToolCalls: 2, Started: sampleNow.Add(-time.Hour).Format(time.RFC3339Nano)},
	}
	m := build(sessions, Scope{ProjectID: "p1", Name: "agent-gate"}, func(string) ([]schema.TelemetryEvent, error) { return nil, nil }, sampleNow)
	if m.mode != modeProject || m.selectedProject.Name != "agent-gate" || len(m.sessions) != 1 {
		t.Fatalf("project start = mode %v project %+v sessions %d", m.mode, m.selectedProject, len(m.sessions))
	}
	m = must(m.Update(key("g")))
	if m.mode != modeList || !m.scopeGlobal || len(m.projects) != 2 {
		t.Fatalf("global = mode %v scopeGlobal %v projects %d", m.mode, m.scopeGlobal, len(m.projects))
	}
	m = must(m.Update(key("p")))
	if m.mode != modeProject || m.scopeGlobal || m.selectedProject.Name != "agent-gate" {
		t.Fatalf("project = mode %v scopeGlobal %v project %+v", m.mode, m.scopeGlobal, m.selectedProject)
	}
}

func TestSecurityDetailReturnsToSecuritySurface(t *testing.T) {
	sessions := []telemetry.Session{{SessionID: "risk", ProjectID: "p1", ProjectName: "agent-gate", ToolCalls: 2, Taint: true}}
	m := build(sessions, Scope{StartGlobal: true}, func(string) ([]schema.TelemetryEvent, error) { return nil, nil }, sampleNow)
	m = must(m.Update(key("tab")))
	if m.surface != surfaceSecurity || len(m.sessions) != 1 {
		t.Fatalf("security = surface %v sessions %d", m.surface, len(m.sessions))
	}
	m = must(m.Update(key("enter")))
	m = must(m.Update(key("esc")))
	if m.mode != modeList || m.surface != surfaceSecurity {
		t.Fatalf("security return = mode %v surface %v", m.mode, m.surface)
	}
}
