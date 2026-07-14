package insights

import (
	"strings"
	"testing"
	"time"

	"github.com/Hypership-Software/atlas/internal/schema"
	"github.com/Hypership-Software/atlas/internal/telemetry"

	tea "github.com/charmbracelet/bubbletea"
)

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
