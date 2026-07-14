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

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
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

// Column floors for the responsive layout: the fixed leading columns (When,
// Task, Outcome, Project) never shrink; Work and Flags give up width first when
// the terminal is too narrow. Flags keeps the larger floor so the risk glyphs it
// carries survive as long as possible.
const (
	workColFloor  = 12
	flagsColFloor = 18
)

type model struct {
	history     []telemetry.Session
	global      []telemetry.Session // full 7-day set (all projects)
	scope       Scope
	scopeGlobal bool

	// all is the active scope's (7-day-filtered) session set; it never
	// reorders or drops rows on its own — hide/sort only ever read from it.
	all         []telemetry.Session
	sessions    []telemetry.Session // visible+sorted; m.sessions[i] is table row i.
	agg         aggregates
	coach       analytics.PlanAssociation
	events      eventProvider
	now         time.Time
	showEmpty   bool
	sortMode    sortMode
	hiddenCount int

	cursor int // selected row into m.sessions
	width  int // last known terminal width; 0 until the first WindowSizeMsg
	height int

	mode       mode
	preHelp    mode // where ? was pressed from, so esc/? returns there
	detail     viewport.Model
	detailSess telemetry.Session
	detailEvs  []schema.TelemetryEvent
	showRaw    bool
}

func build(sessions []telemetry.Session, scope Scope, events eventProvider, now time.Time) model {
	m := model{
		history: sessions,
		global:  recentSessions(sessions, now),
		scope:   scope,
		events:  events,
		now:     now,
		mode:    modeList,
		detail:  viewport.New(80, 20),
	}
	m.coach = analytics.PlanFirstAssociation(coachWindow(m.history))
	m.scopeGlobal = scope.StartGlobal || scope.ProjectID == ""
	return m.applyScope()
}

// applyScope recomputes the active session set and its aggregates for the current
// scope, then rebuilds the visible rows.
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

// rebuildRows recomputes the visible+sorted session slice from m.all and resets
// the cursor to the top. m.sessions[m.cursor] is how "enter" resolves which
// session to open, so any hide/sort change rebuilds this slice and re-anchors the
// cursor together.
func (m model) rebuildRows() model {
	visible := visibleSessions(m.all, m.showEmpty)
	sortSessions(visible, m.sortMode)
	m.sessions = visible
	m.hiddenCount = len(m.all) - len(visible)
	m.cursor = 0
	return m
}

// buildColumns projects the visible sessions into the table's columns for the
// active scope: the global view gets a leading Project column; Work and Flags
// carry floors so they absorb (or give up) the terminal's spare width.
func (m model) buildColumns() []tableColumn {
	titles := []string{"When", "Task", "Outcome", "Work", "Flags"}
	floors := []int{0, 0, 0, workColFloor, flagsColFloor}
	if m.scopeGlobal {
		titles = append([]string{"Project"}, titles...)
		floors = append([]int{0}, floors...)
	}
	cols := make([]tableColumn, len(titles))
	for i := range titles {
		cols[i] = tableColumn{title: titles[i], floor: floors[i], cells: make([]string, len(m.sessions))}
	}
	for r, s := range m.sessions {
		row := sessionRow(s, m.now)
		if m.scopeGlobal {
			row = append([]string{m.projectCell(s)}, row...)
		}
		for i := range cols {
			cols[i].cells[r] = row[i]
		}
	}
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

func sessionRow(s telemetry.Session, now time.Time) []string {
	return []string{
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
	case s.Shipped:
		return ui.OK("↑ shipped")
	case s.CleanDelivery:
		return ui.OK("✓ no intervention")
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

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
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
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil
	case "down", "j":
		if m.cursor < len(m.sessions)-1 {
			m.cursor++
		}
		return m, nil
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
	case "h":
		m.showEmpty = !m.showEmpty
		return m.rebuildRows(), nil
	case "s":
		m.sortMode = m.sortMode.next()
		return m.rebuildRows(), nil
	case "enter":
		if len(m.sessions) == 0 {
			return m, nil
		}
		return m.openDetail(m.sessions[m.cursor])
	}
	return m, nil
}

func (m model) openDetail(sess telemetry.Session) (tea.Model, tea.Cmd) {
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
	tableView := renderSessionTable(m.buildColumns(), m.cursor, m.width, m.tableRowLimit())
	if note := hiddenNote(m.hiddenCount); note != "" {
		tableView += "\n" + note
	}
	return renderList(m.agg, m.coach, tableView)
}

func (m model) tableRowLimit() int {
	if m.height <= 0 {
		return maxTableRows
	}
	fixed := strings.Count(renderList(m.agg, m.coach, ""), "\n") + 1
	if m.hiddenCount > 0 {
		fixed++
	}
	budget := m.height - fixed
	if budget < 1 {
		return 1
	}
	limit := budget
	if limit > maxTableRows {
		limit = maxTableRows
	}
	for limit > 1 {
		start, end := windowBounds(m.cursor, len(m.sessions), limit)
		lines := end - start
		if start > 0 || end < len(m.sessions) {
			lines++
		}
		if lines <= budget {
			return limit
		}
		limit--
	}
	return 1
}
