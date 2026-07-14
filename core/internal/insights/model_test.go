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
			SessionID: fmt.Sprintf("session-%d", i),
			TaskType:  "feature",
			ToolCalls: 1,
			Started:   sampleNow.Add(-time.Duration(i) * time.Hour).Format(time.RFC3339Nano),
		}
	}
	sessions[len(sessions)-1].ToolCalls = 0
	m := build(sessions, Scope{}, func(string) ([]schema.TelemetryEvent, error) { return nil, nil }, sampleNow)
	m.coach = analytics.PlanAssociation{Status: analytics.CoachRecommend, TaskType: "feature", Total: 24,
		Planned: 10, Direct: 14, PlannedRate: .8, DirectRate: .55}
	m.cursor = 5
	m = must(m.Update(tea.WindowSizeMsg{Width: 100, Height: 24}))
	if lines := strings.Count(m.View(), "\n") + 1; lines > 24 {
		t.Fatalf("list with scroll notes rendered %d lines into a 24-line terminal:\n%s", lines, m.View())
	}
	if !strings.Contains(m.View(), "more sessions above") || !strings.Contains(m.View(), "more sessions below") {
		t.Fatalf("height-limited list omitted a scroll direction:\n%s", m.View())
	}
	if !strings.Contains(m.View(), "empty session hidden") {
		t.Fatalf("height-limited list omitted its hidden-session note:\n%s", m.View())
	}
}

func complexHeightModel() model {
	tasks := []string{"feature", "bugfix", "docs"}
	sessions := make([]telemetry.Session, 10)
	for i := range sessions {
		session := telemetry.Session{
			SessionID:    fmt.Sprintf("complex-%d", i),
			ProjectID:    "project-two",
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
	sessions[len(sessions)-1].ToolCalls = 0
	m := build(sessions, Scope{}, func(string) ([]schema.TelemetryEvent, error) { return nil, nil }, sampleNow)
	m.coach = analytics.PlanAssociation{Status: analytics.CoachRecommend, Window: 24, TaskType: "feature", Total: 24,
		Planned: 10, Direct: 14, PlannedRate: .8, DirectRate: .55}
	m.cursor = 4
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
		"What the AI worked on", "feature", "bugfix", "docs", "Needs attention", "flagged command",
		"What's moving your needle", "Try next", "Project", "more sessions above", "more sessions below",
		"empty session hidden", "q quit",
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
	for _, want := range []string{"selected 5 of 9", "4 above", "4 below", "empty session hidden", "q quit"} {
		if !strings.Contains(view, want) {
			t.Fatalf("single-line table budget omitted %q:\n%s", want, view)
		}
	}
}

func TestZeroRowStatusTracksKeyboardNavigation(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	m := complexHeightModel()
	m.cursor = 0
	m = must(m.Update(tea.WindowSizeMsg{Width: 100, Height: 19}))
	assertStatus := func(want string) {
		t.Helper()
		if view := m.View(); !strings.Contains(view, want) {
			t.Fatalf("status missing %q at cursor %d:\n%s", want, m.cursor, view)
		}
	}
	assertStatus("selected 1 of 9 · 0 above · 8 below")
	for range 4 {
		m = must(m.Update(key("j")))
	}
	assertStatus("selected 5 of 9 · 4 above · 4 below")
	for range 4 {
		m = must(m.Update(key("down")))
	}
	assertStatus("selected 9 of 9 · 8 above · 0 below")
	m = must(m.Update(key("k")))
	assertStatus("selected 8 of 9 · 7 above · 1 below")
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
	for _, want := range []string{"What the AI worked on", "Try next", "4h ago", "q quit"} {
		if !strings.Contains(view, want) {
			t.Fatalf("unknown-height list missing %q:\n%s", want, view)
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
			if view := m.View(); !strings.Contains(view, tt.want) || !strings.Contains(view, wantCoach) {
				t.Fatalf("project empty view missing honest copy or coach:\n%s", view)
			}
			m = must(m.Update(key("g")))
			if view := m.View(); !strings.Contains(view, tt.wantGlobal) || !strings.Contains(view, wantCoach) {
				t.Fatalf("global empty view changed copy or coach:\n%s", view)
			}
			m = must(m.Update(key("p")))
			if view := m.View(); !strings.Contains(view, tt.want) || !strings.Contains(view, wantCoach) {
				t.Fatalf("project empty view after p changed copy or coach:\n%s", view)
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
	if !strings.Contains(m.View(), wantCoach) {
		t.Fatalf("empty project view omitted coach:\n%s", m.View())
	}
	m = must(m.Update(key("g")))
	if !strings.Contains(m.View(), wantCoach) || !strings.Contains(m.View(), "docs") {
		t.Fatalf("populated global view changed or omitted coach:\n%s", m.View())
	}
	m = must(m.Update(key("p")))
	if !strings.Contains(m.View(), wantCoach) || !strings.Contains(m.View(), "No Atlas activity for this project in the last 7 days.") {
		t.Fatalf("empty project view after p changed or omitted coach:\n%s", m.View())
	}
}

var sampleNow = time.Date(2026, 7, 13, 15, 0, 0, 0, time.UTC)

func sampleModel() model {
	sessions := []telemetry.Session{
		{SessionID: "aaaa1111", Harness: "claudecode", TaskType: "feature", Outcome: "success", CleanDelivery: true, TurnCount: 3, ToolCalls: 4, FilesTouched: 2, Started: sampleNow.Add(-2 * time.Hour).Format(time.RFC3339Nano)},
		{SessionID: "bbbb2222", Harness: "claudecode", TaskType: "bugfix", Outcome: "failure", CorrectionTurns: 2, TurnCount: 6, ToolCalls: 11, FilesTouched: 5, Started: sampleNow.Add(-3 * time.Hour).Format(time.RFC3339Nano)},
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

func TestFlagsColumnFitsAllFlags(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	s := telemetry.Session{SessionID: "cccc3333", Harness: "claudecode", TaskType: "testing",
		Outcome: "success", CorrectionTurns: 1, ToolCalls: 179, FilesTouched: 44,
		Taint: true, DangerDetected: 11, SkillsUsed: "a,b,c,d",
		Started: sampleNow.Add(-20 * time.Hour).Format(time.RFC3339Nano)}
	provider := func(string) ([]schema.TelemetryEvent, error) { return nil, nil }
	m := build([]telemetry.Session{s}, Scope{}, provider, sampleNow)
	v := m.View()
	// All three flags co-occur here; the Flags column must widen to fit rather
	// than truncate the trailing "★ 4 skills".
	for _, want := range []string{"⚠ untrusted input", "⚑ 11 flagged", "★ 4 skills"} {
		if !strings.Contains(v, want) {
			t.Fatalf("flags column truncated %q:\n%s", want, v)
		}
	}
}

func TestListViewRendersSessions(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	v := sampleModel().View()
	if !strings.Contains(v, "feature") || !strings.Contains(v, "2h ago") {
		t.Fatalf("list view missing session row: %q", v)
	}
	if !strings.Contains(v, "4 calls") || !strings.Contains(v, "2 files") {
		t.Fatalf("list view missing work cell: %q", v)
	}
	if !strings.Contains(v, "no intervention") {
		t.Fatalf("list view missing header: %q", v)
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
		CleanDelivery: true, ToolCalls: 165, FilesTouched: 12, Started: "2026-07-13T13:00:00Z"}
	row := sessionRow(s, now)
	joined := strings.Join(row, " | ")
	for _, want := range []string{"2h ago", "testing", "no intervention", "165 calls", "12 files"} {
		if !strings.Contains(joined, want) {
			t.Errorf("row %q missing %q", joined, want)
		}
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
	if m.mode != modeDetail {
		t.Fatalf("enter did not switch to detail mode")
	}
	if !strings.Contains(m.View(), "fetched") {
		t.Fatalf("detail view missing tool: %q", m.View())
	}
	m = must(m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}}))
	if !m.showRaw {
		t.Fatalf("r did not toggle raw")
	}
	if !strings.Contains(m.View(), "subagent") {
		t.Fatalf("raw detail view missing subagent field: %q", m.View())
	}
	m = must(m.Update(tea.KeyMsg{Type: tea.KeyEsc}))
	if m.mode != modeList {
		t.Fatalf("esc did not return to list")
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
	if got := m.projectCell(telemetry.Session{ProjectID: "p1abcdef"}); got != "myproj" {
		t.Errorf("current project cell = %q, want myproj", got)
	}
	if got := m.projectCell(telemetry.Session{ProjectID: "otherhash1234"}); got != shortID("otherhash1234") {
		t.Errorf("other project cell = %q, want short hash", got)
	}
	if got := m.projectCell(telemetry.Session{ProjectID: ""}); got != "unknown" {
		t.Errorf("empty project cell = %q, want unknown", got)
	}
}
