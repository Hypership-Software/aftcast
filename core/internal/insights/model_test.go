package insights

import (
	"strings"
	"testing"

	"github.com/Hypership-Software/atlas/internal/schema"
	"github.com/Hypership-Software/atlas/internal/telemetry"

	tea "github.com/charmbracelet/bubbletea"
)

func sampleModel() model {
	sessions := []telemetry.Session{
		{SessionID: "aaaa1111", Harness: "claudecode", TaskType: "feature", Outcome: "success", CleanDelivery: true, TurnCount: 3, ToolCalls: 4},
		{SessionID: "bbbb2222", Harness: "claudecode", TaskType: "bugfix", Outcome: "failure", CorrectionTurns: 2, TurnCount: 6, ToolCalls: 11},
	}
	provider := func(id string) ([]schema.TelemetryEvent, error) {
		return []schema.TelemetryEvent{{SessionID: id, ToolRaw: "WebFetch", Subagent: "researcher"}}, nil
	}
	return build(sessions, aggregate(sessions), provider)
}

func TestListViewRendersSessions(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	v := sampleModel().View()
	if !strings.Contains(v, "aaaa1111") || !strings.Contains(v, "feature") {
		t.Fatalf("list view missing session row: %q", v)
	}
	if !strings.Contains(v, "clean") {
		t.Fatalf("list view missing header: %q", v)
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
	if !strings.Contains(m.View(), "WebFetch") {
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
	m := build(nil, aggregate(nil), func(string) ([]schema.TelemetryEvent, error) { return nil, nil })
	if !strings.Contains(m.View(), "No sessions") {
		t.Fatalf("empty model should show empty state: %q", m.View())
	}
}

// must unwraps an Update result back to the concrete model for assertions.
func must(mdl tea.Model, _ tea.Cmd) model { return mdl.(model) }
