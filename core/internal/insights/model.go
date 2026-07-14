package insights

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Hypership-Software/atlas/internal/analytics"
	"github.com/Hypership-Software/atlas/internal/schema"
	"github.com/Hypership-Software/atlas/internal/telemetry"
	"github.com/Hypership-Software/atlas/internal/ui"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type mode int

const (
	modeList mode = iota
	modeDetail
	modeHelp
)

// eventProvider loads a session's events on drill-down. Injected so the model is
// testable without a store.
type eventProvider func(sessionID string) ([]schema.TelemetryEvent, error)

type sortMode int

const (
	sortRecent sortMode = iota
	sortCalls
	sortRisk
)

func (s sortMode) next() sortMode { return (s + 1) % 3 }

var sessionColumns = []table.Column{
	{Title: "When", Width: 9},
	{Title: "Task", Width: 12},
	{Title: "Outcome", Width: 10},
	{Title: "Work", Width: 26},
	{Title: "Flags", Width: 32},
}

const flagsColMaxWidth = 48

// flagsColumnWidth sizes the Flags column to the widest flags cell in the data
// (measured the same ANSI/runewidth-aware way the table truncates), so a session
// carrying all of ⚠/⚑/★ isn't clipped. Capped so a pathological count can't blow
// the layout. Computed over the full set (not just visible) so it stays stable
// across hide-empty/sort toggles.
func flagsColumnWidth(sessions []telemetry.Session) int {
	w := lipgloss.Width("Flags")
	for _, s := range sessions {
		if cw := lipgloss.Width(flagsCell(s)); cw > w {
			w = cw
		}
	}
	if w > flagsColMaxWidth {
		w = flagsColMaxWidth
	}
	return w
}

type model struct {
	global      []telemetry.Session // full 7-day set (all projects)
	scope       Scope
	scopeGlobal bool

	// all is the active scope's (7-day-filtered) session set; it never
	// reorders or drops rows on its own — hide/sort only ever read from it.
	all         []telemetry.Session
	sessions    []telemetry.Session // visible+sorted, in lockstep with table rows: m.sessions[i] is table row i.
	agg         aggregates
	events      eventProvider
	now         time.Time
	showEmpty   bool
	sortMode    sortMode
	hiddenCount int

	mode       mode
	preHelp    mode // where ? was pressed from, so esc/? returns there
	table      table.Model
	detail     viewport.Model
	detailSess telemetry.Session
	detailEvs  []schema.TelemetryEvent
	showRaw    bool
}

func build(sessions []telemetry.Session, scope Scope, events eventProvider, now time.Time) model {
	m := model{
		global: sessions,
		scope:  scope,
		events: events,
		now:    now,
		mode:   modeList,
		table: table.New(
			table.WithColumns(sessionColumns),
			table.WithFocused(true),
		),
		detail: viewport.New(80, 20),
	}
	m.scopeGlobal = scope.StartGlobal || scope.ProjectID == ""
	return m.applyScope()
}

// applyScope recomputes the active session set and its aggregates for the current
// scope, then rebuilds the table rows.
func (m model) applyScope() model {
	m.all = scopeSessions(m.global, m.scope.ProjectID, m.scopeGlobal)
	m.agg = aggregate(m.all, m.now)
	m.agg.scopeLabel = scopeLabel(m.scope, m.scopeGlobal)
	return m.rebuildRows()
}

func scopeSessions(all []telemetry.Session, projectID string, global bool) []telemetry.Session {
	if global || projectID == "" {
		return all
	}
	out := make([]telemetry.Session, 0, len(all))
	for _, s := range all {
		if s.ProjectID == projectID {
			out = append(out, s)
		}
	}
	return out
}

func scopeLabel(scope Scope, global bool) string {
	if global || scope.Name == "" {
		return "all projects"
	}
	return scope.Name
}

// rebuildRows recomputes the visible+sorted session slice from m.all and pushes
// matching rows into the table, resetting the cursor to the top. Sessions and
// rows must always be rebuilt together: m.sessions[m.table.Cursor()] is how
// "enter" resolves which session to open, so an out-of-step rebuild opens the
// wrong session's detail.
func (m model) rebuildRows() model {
	visible := visibleSessions(m.all, m.showEmpty)
	sortSessions(visible, m.sortMode)

	rows := make([]table.Row, len(visible))
	for i, s := range visible {
		r := sessionRow(s, m.now)
		if m.scopeGlobal {
			r = append(table.Row{m.projectCell(s)}, r...)
		}
		rows[i] = r
	}

	cols := m.columns()

	m.sessions = visible
	m.hiddenCount = len(m.all) - len(visible)
	// bubbles/table's SetColumns/SetRows each re-render the viewport immediately
	// against whatever the OTHER field currently holds. Since the Project column
	// makes the row width vary by scope, a stale wider/narrower pairing during
	// this swap indexes out of range — clear rows first so neither setter ever
	// renders mismatched cols/rows.
	m.table.SetRows(nil)
	m.table.SetColumns(cols)
	m.table.SetRows(rows)
	// SetHeight reserves one line for the header internally, so +1 here is what
	// makes clampHeight's return value the number of visible DATA rows.
	m.table.SetHeight(clampHeight(len(rows)) + 1)
	m.table.SetCursor(0)
	return m
}

// columns returns the table columns for the active scope: the global view gets a
// leading Project column; the last column (Flags) is auto-widened either way.
func (m model) columns() []table.Column {
	cols := make([]table.Column, 0, len(sessionColumns)+1)
	if m.scopeGlobal {
		cols = append(cols, table.Column{Title: "Project", Width: 14})
	}
	cols = append(cols, sessionColumns...)
	cols[len(cols)-1].Width = flagsColumnWidth(m.all)
	return cols
}

// projectCell labels a session's project in the global view: the current project
// shows its real (live-derived) name, other projects their short id, and
// pre-field sessions "unknown". Never a path — only the opaque id or the live name.
func (m model) projectCell(s telemetry.Session) string {
	switch {
	case s.ProjectID == "":
		return "unknown"
	case s.ProjectID == m.scope.ProjectID && m.scope.Name != "":
		return m.scope.Name
	default:
		return shortID(s.ProjectID)
	}
}

// visibleSessions filters out 0-call sessions unless showEmpty is set. It
// always returns a fresh slice so callers (rebuildRows' in-place sort) never
// mutate the caller's backing array.
func visibleSessions(sessions []telemetry.Session, showEmpty bool) []telemetry.Session {
	out := make([]telemetry.Session, 0, len(sessions))
	for _, s := range sessions {
		if showEmpty || s.ToolCalls != 0 {
			out = append(out, s)
		}
	}
	return out
}

func sortSessions(sessions []telemetry.Session, mode sortMode) {
	switch mode {
	case sortCalls:
		sort.SliceStable(sessions, func(i, j int) bool { return sessions[i].ToolCalls > sessions[j].ToolCalls })
	case sortRisk:
		sort.SliceStable(sessions, func(i, j int) bool { return moreRisky(sessions[i], sessions[j]) })
	default:
		sort.SliceStable(sessions, func(i, j int) bool { return startedUnixNano(sessions[i]) > startedUnixNano(sessions[j]) })
	}
}

func moreRisky(a, b telemetry.Session) bool {
	if a.Taint != b.Taint {
		return a.Taint
	}
	return a.DangerDetected > b.DangerDetected
}

func startedUnixNano(s telemetry.Session) int64 {
	t, err := time.Parse(time.RFC3339Nano, s.Started)
	if err != nil {
		return 0
	}
	return t.UnixNano()
}

func sessionRow(s telemetry.Session, now time.Time) table.Row {
	return table.Row{
		humanize(s.Started, now),
		taskCell(s.TaskType),
		outcomeCell(s),
		workCell(s),
		flagsCell(s),
	}
}

func taskCell(t string) string {
	if t == "" || t == "unknown" {
		return "other"
	}
	return t
}

func outcomeCell(s telemetry.Session) string {
	class := analytics.OutcomeClass(s.Outcome)
	switch {
	case s.CleanDelivery:
		return ui.OK("✓ clean")
	case class == analytics.Success && s.CorrectionTurns > 0:
		return ui.Warn(fmt.Sprintf("✓ %d fix", s.CorrectionTurns))
	case class == analytics.Failure:
		return ui.Bad("✗ failed")
	default:
		return ui.Hint("—")
	}
}

func workCell(s telemetry.Session) string {
	return fmt.Sprintf("%d calls · %d files", s.ToolCalls, s.FilesTouched)
}

func flagsCell(s telemetry.Session) string {
	var parts []string
	if s.Taint {
		parts = append(parts, ui.Warn("⚠ untrusted input"))
	}
	if s.DangerDetected > 0 {
		parts = append(parts, ui.Bad(fmt.Sprintf("⚑ %d flagged", s.DangerDetected)))
	}
	if n := len(splitSkills(s.SkillsUsed)); n > 0 {
		parts = append(parts, fmt.Sprintf("★ %d skills", n))
	}
	return strings.Join(parts, " ")
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
		switch m.mode {
		case modeHelp:
			return m.updateHelp(msg)
		case modeDetail:
			return m.updateDetail(msg)
		default:
			return m.updateList(msg)
		}
	}
	return m, nil
}

func (m model) updateHelp(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc", "?":
		m.mode = m.preHelp
		return m, nil
	}
	return m, nil
}

func (m model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "?":
		m.preHelp = modeList
		m.mode = modeHelp
		return m, nil
	case "h":
		m.showEmpty = !m.showEmpty
		return m.rebuildRows(), nil
	case "s":
		m.sortMode = m.sortMode.next()
		return m.rebuildRows(), nil
	case "g":
		if !m.scopeGlobal {
			m.scopeGlobal = true
			return m.applyScope(), nil
		}
		return m, nil
	case "p":
		if m.scopeGlobal && m.scope.ProjectID != "" {
			m.scopeGlobal = false
			return m.applyScope(), nil
		}
		return m, nil
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
	case "?":
		m.preHelp = modeDetail
		m.mode = modeHelp
		return m, nil
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
	if m.mode == modeHelp {
		return renderHelp()
	}
	if len(m.all) == 0 {
		return renderScopedEmpty(m.scopeGlobal, len(m.global) > 0)
	}
	if m.mode == modeDetail {
		return renderDetail(m.detail.View())
	}
	tableView := m.table.View()
	if note := hiddenNote(m.hiddenCount); note != "" {
		tableView += "\n" + note
	}
	return renderList(m.agg, tableView)
}
