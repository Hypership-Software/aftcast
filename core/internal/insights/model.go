package insights

import (
	"fmt"
	"strconv"

	"github.com/Hypership-Software/atlas/internal/schema"
	"github.com/Hypership-Software/atlas/internal/telemetry"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

type mode int

const (
	modeList mode = iota
	modeDetail
)

// eventProvider loads a session's events on drill-down. Injected so the model is
// testable without a store.
type eventProvider func(sessionID string) ([]schema.TelemetryEvent, error)

type model struct {
	sessions []telemetry.Session
	agg      aggregates
	events   eventProvider

	mode       mode
	table      table.Model
	detail     viewport.Model
	detailSess telemetry.Session
	detailEvs  []schema.TelemetryEvent
	showRaw    bool
}

func build(sessions []telemetry.Session, agg aggregates, events eventProvider) model {
	cols := []table.Column{
		{Title: "id", Width: 10},
		{Title: "task", Width: 11},
		{Title: "outcome", Width: 8},
		{Title: "clean/corr", Width: 11},
		{Title: "turns", Width: 6},
		{Title: "tools", Width: 6},
		{Title: "taint", Width: 5},
		{Title: "started", Width: 22},
	}
	rows := make([]table.Row, len(sessions))
	for i, s := range sessions {
		rows[i] = table.Row{
			shortID(s.SessionID), s.TaskType, s.Outcome, deliveryCell(s),
			strconv.Itoa(s.TurnCount), strconv.Itoa(s.ToolCalls), taintCell(s.Taint), s.Started,
		}
	}
	return model{
		sessions: sessions,
		agg:      agg,
		events:   events,
		mode:     modeList,
		table: table.New(
			table.WithColumns(cols),
			table.WithRows(rows),
			table.WithFocused(true),
			table.WithHeight(clampHeight(len(rows))),
		),
		detail: viewport.New(80, 20),
	}
}

func deliveryCell(s telemetry.Session) string {
	switch {
	case s.CleanDelivery:
		return "✓"
	case s.CorrectionTurns > 0:
		return fmt.Sprintf("%d corr", s.CorrectionTurns)
	default:
		return "-"
	}
}

func taintCell(t bool) string {
	if t {
		return "⚠"
	}
	return ""
}

func clampHeight(n int) int {
	switch {
	case n < 1:
		return 1
	case n > 15:
		return 15
	default:
		return n
	}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.detail.Width = msg.Width
		if h := msg.Height - 4; h > 0 {
			m.detail.Height = h
		}
		return m, nil
	case tea.KeyMsg:
		if m.mode == modeDetail {
			return m.updateDetail(msg)
		}
		return m.updateList(msg)
	}
	return m, nil
}

func (m model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "enter":
		if len(m.sessions) == 0 {
			return m, nil
		}
		sess := m.sessions[m.table.Cursor()]
		m.mode = modeDetail
		m.detailSess = sess
		m.showRaw = false
		evs, err := m.events(sess.SessionID)
		if err != nil {
			m.detailEvs = nil
			m.detail.SetContent("failed to load events: " + err.Error())
			m.detail.GotoTop()
			return m, nil
		}
		m.detailEvs = evs
		m.detail.SetContent(detailBody(sess, evs, false))
		m.detail.GotoTop()
		return m, nil
	}
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m model) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.mode = modeList
		return m, nil
	case "r":
		m.showRaw = !m.showRaw
		m.detail.SetContent(detailBody(m.detailSess, m.detailEvs, m.showRaw))
		m.detail.GotoTop()
		return m, nil
	}
	var cmd tea.Cmd
	m.detail, cmd = m.detail.Update(msg)
	return m, cmd
}

func (m model) View() string {
	if len(m.sessions) == 0 {
		return renderEmpty()
	}
	if m.mode == modeDetail {
		return renderDetail(m.detailSess.SessionID, m.detail.View())
	}
	return renderList(m.agg, m.table.View())
}
