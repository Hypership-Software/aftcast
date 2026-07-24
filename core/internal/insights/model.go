package insights

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Hypership-Software/aftcast/internal/analytics"
	"github.com/Hypership-Software/aftcast/internal/schema"
	"github.com/Hypership-Software/aftcast/internal/telemetry"
	"github.com/Hypership-Software/aftcast/internal/ui"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type mode int

const (
	modeList mode = iota
	modeProject
	modeDetail
	modeHelp
)

type listSurface int

const (
	surfaceOverview listSurface = iota
	surfaceSecurity
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
	all             []telemetry.Session
	sessions        []telemetry.Session // visible+sorted; m.sessions[i] is table row i.
	agg             aggregates
	coach           analytics.PlanAssociation
	friction        []analytics.FrictionCluster
	events          eventProvider
	now             time.Time
	showEmpty       bool
	sortMode        sortMode
	hiddenCount     int
	surface         listSurface
	projects        []projectSummary
	projectCursor   int
	selectedProject projectSummary

	cursor      int // selected row into m.sessions
	width       int // last known terminal width; 0 until the first WindowSizeMsg
	height      int
	heightKnown bool

	mode             mode
	preHelp          mode // where ? was pressed from, so esc/? returns there
	detail           viewport.Model
	detailSess       telemetry.Session
	detailEvs        []schema.TelemetryEvent
	showRaw          bool
	detailReturnMode mode
}

func build(sessions []telemetry.Session, scope Scope, events eventProvider, failures []schema.TelemetryEvent, now time.Time) model {
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
	m.friction = analytics.WorthFixing(analytics.FrictionClusters(frictionWindow(failures, now)))
	m.scopeGlobal = scope.StartGlobal || scope.ProjectID == ""
	return m.applyScope()
}

// applyScope recomputes the active session set and its aggregates for the current
// scope, then rebuilds the visible rows.
func (m model) applyScope() model {
	m.all = scopeSessions(m.global, m.scope.ProjectID, m.scopeGlobal)
	m.agg = aggregate(m.all, m.now)
	m.agg.scopeLabel = scopeLabel(m.scope, m.scopeGlobal)
	m.projects = groupProjects(m.all, m.scope, m.now)
	if !m.scopeGlobal {
		m.mode = modeProject
		m.surface = surfaceOverview
		m.selectedProject = m.scopedProject()
	}
	return m.rebuildRows()
}

// A scope is one project by definition — scopeSessions has already established that
// every session in it shares the scoped id — so the card summarizes all of them.
// Rendering one name-group instead split the card whenever a session edited files
// outside the repository it started in, and showed only whichever part held the
// most recently started session.
func (m model) scopedProject() projectSummary {
	placeholder := projectSummary{Name: m.scope.Name, ID: m.scope.ProjectID, Key: "id:" + m.scope.ProjectID, Aggregate: m.agg}
	if len(m.all) == 0 {
		return placeholder
	}
	out := summarizeProject(placeholder.Key, m.all, m.scope, m.now)
	if m.scope.Name != "" {
		out.Name = m.scope.Name
	}
	return out
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
	if m.surface == surfaceSecurity {
		visible := securitySessions(m.all)
		sortSessions(visible, sortRecent)
		m.sessions = visible
		m.hiddenCount = 0
		m.cursor = 0
		return m
	}
	if m.mode == modeProject {
		m.sessions = append([]telemetry.Session(nil), m.selectedProject.Sessions...)
		m.hiddenCount = 0
		if m.cursor >= len(m.sessions) {
			m.cursor = max(0, len(m.sessions)-1)
		}
		return m
	}
	m.projects = groupProjects(m.all, m.scope, m.now)
	m.sessions = nil
	m.hiddenCount = 0
	if m.projectCursor >= len(m.projects) {
		m.projectCursor = max(0, len(m.projects)-1)
	}
	return m
}

// buildColumns projects the visible sessions into the table's columns for the
// active scope: the global view gets a leading Project column; Work and Flags
// carry floors so they absorb (or give up) the terminal's spare width.
func (m model) buildColumns() []tableColumn {
	if m.surface == surfaceSecurity {
		return m.buildSecurityColumns()
	}
	if m.mode == modeProject {
		return buildProjectSessionColumns(m.sessions, m.now)
	}
	return buildProjectColumns(m.projects, m.now)
}

func (m model) buildOverviewColumns() []tableColumn {
	titles := []string{"When", "Task", "Result", "Work"}
	floors := []int{0, 0, 0, workColFloor}
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

func (m model) buildSecurityColumns() []tableColumn {
	titles := []string{"Project", "When", "Result", "Signal", "Work"}
	floors := []int{0, 0, 0, flagsColFloor, workColFloor}
	cols := make([]tableColumn, len(titles))
	for i := range titles {
		cols[i] = tableColumn{title: titles[i], floor: floors[i], cells: make([]string, len(m.sessions))}
	}
	for r, session := range m.sessions {
		row := []string{
			m.projectCell(session),
			humanize(session.Started, m.now),
			outcomeCell(session),
			securitySignalCell(session),
			workCell(session),
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
	if s.ProjectName != "" {
		return s.ProjectName
	}
	if s.ProjectID == m.scope.ProjectID && m.scope.Name != "" {
		return m.scope.Name
	}
	return "other project"
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

func securitySessions(sessions []telemetry.Session) []telemetry.Session {
	out := make([]telemetry.Session, 0, len(sessions))
	for _, session := range sessions {
		if session.Taint || session.DangerDetected > 0 {
			out = append(out, session)
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
	case class == analytics.Success:
		return ui.OK("✓ succeeded")
	case class == analytics.Failure:
		return ui.Bad("✗ failed")
	default:
		return ui.Hint("—")
	}
}

func workCell(s telemetry.Session) string {
	return fmt.Sprintf("%d changed · %s", s.FilesChanged, countNoun(s.ToolCalls, "call", "calls"))
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
		parts = append(parts, "★ "+countNoun(n, "skill", "skills"))
	}
	return strings.Join(parts, " ")
}

func securitySignalCell(s telemetry.Session) string {
	actions := countNoun(s.DangerDetected, "flagged action", "flagged actions")
	switch {
	case s.Taint && s.DangerDetected > 0:
		return "untrusted input + " + actions
	case s.Taint:
		return "untrusted input"
	case s.DangerDetected > 0:
		return actions
	default:
		return ""
	}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.heightKnown = true
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
		case modeProject:
			return m.updateProject(msg)
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
		if m.surface == surfaceSecurity {
			if m.cursor > 0 {
				m.cursor--
			}
		} else if m.projectCursor > 0 {
			m.projectCursor--
		}
		return m, nil
	case "down", "j":
		if m.surface == surfaceSecurity {
			if m.cursor < len(m.sessions)-1 {
				m.cursor++
			}
		} else if m.projectCursor < len(m.projects)-1 {
			m.projectCursor++
		}
		return m, nil
	case "tab":
		if m.surface == surfaceOverview {
			m.surface = surfaceSecurity
		} else {
			m.surface = surfaceOverview
			if !m.scopeGlobal {
				m.mode = modeProject
			}
		}
		return m.rebuildRows(), nil
	case "g":
		if !m.scopeGlobal {
			m.scopeGlobal = true
			m.mode = modeList
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
		if m.surface == surfaceSecurity {
			if len(m.sessions) == 0 {
				return m, nil
			}
			m.detailReturnMode = modeList
			return m.openDetail(m.sessions[m.cursor])
		}
		if len(m.projects) == 0 {
			return m, nil
		}
		return m.openProject(m.projects[m.projectCursor]), nil
	}
	return m, nil
}

func (m model) updateProject(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "?":
		m.preHelp = modeProject
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
	case "enter":
		if len(m.sessions) == 0 {
			return m, nil
		}
		m.detailReturnMode = modeProject
		return m.openDetail(m.sessions[m.cursor])
	case "tab":
		m.mode = modeList
		m.surface = surfaceSecurity
		return m.rebuildRows(), nil
	case "esc", "g":
		m.scopeGlobal = true
		m.mode = modeList
		m.surface = surfaceOverview
		return m.applyScope(), nil
	}
	return m, nil
}

func (m model) openProject(project projectSummary) model {
	m.mode = modeProject
	m.selectedProject = project
	m.sessions = append([]telemetry.Session(nil), project.Sessions...)
	m.cursor = 0
	return m
}

func (m model) openDetail(sess telemetry.Session) (tea.Model, tea.Cmd) {
	m.mode = modeDetail
	m.detailSess = sess
	m.showRaw = false
	evs, err := m.events(sess.Key)
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
		m.mode = m.detailReturnMode
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
	var view string
	if m.mode == modeHelp {
		view = renderHelp()
	} else if m.mode == modeDetail {
		view = renderDetail(m.detail.View())
	} else if m.mode == modeProject {
		view = m.renderProjectView()
	} else if m.surface == surfaceSecurity {
		view = m.renderListView()
	} else if len(m.all) == 0 {
		view = renderEmptyList(m.coach, renderScopedEmpty(m.scopeGlobal, m.hasScopedHistory()))
	} else {
		view = m.renderListView()
	}
	if !m.heightKnown {
		return view
	}
	return fitViewHeight(view, m.width, m.height)
}

func (m model) hasScopedHistory() bool {
	if m.scopeGlobal {
		return len(m.history) > 0
	}
	for _, session := range m.history {
		if session.ProjectID == m.scope.ProjectID {
			return true
		}
	}
	return false
}

func (m model) renderListView() string {
	if m.surface == surfaceSecurity {
		return m.renderResponsiveTable(m.buildSecurityColumns(), m.cursor, len(m.sessions), 0, func(table string) string {
			return renderSecurityList(m.agg, table, len(m.sessions))
		})
	}
	return m.renderResponsiveTable(buildProjectColumns(m.projects, m.now), m.projectCursor, len(m.projects), 0, func(table string) string {
		return renderProjects(m.agg, m.coach, m.friction, table)
	})
}

func (m model) renderProjectView() string {
	return m.renderResponsiveTable(buildProjectSessionColumns(m.sessions, m.now), m.cursor, len(m.sessions), 0, func(table string) string {
		return renderProjectWorkspace(m.selectedProject, table)
	})
}

func (m model) renderResponsiveTable(cols []tableColumn, cursor, rowCount, hidden int, wrap func(string) string) string {
	fullTable := renderSessionTable(cols, cursor, m.width, maxTableRows)
	if note := hiddenNote(hidden); note != "" {
		fullTable += "\n" + note
	}
	full := wrap(fullTable)
	if !m.heightKnown || viewFits(full, m.width, m.height) {
		return full
	}
	maxRows := rowCount
	if maxRows > maxTableRows {
		maxRows = maxTableRows
	}
	var smallest string
	for rows := maxRows; rows >= 0; rows-- {
		tableView := renderCompactSessionTable(cols, cursor, m.width, rows, hidden)
		candidate := compactView(wrap(tableView))
		smallest = candidate
		if viewFits(candidate, m.width, m.height) {
			return candidate
		}
	}
	if status := renderCompactSessionStatus(cols, cursor, m.width, hidden); status != "" {
		candidate := compactView(wrap(status))
		smallest = candidate
		if viewFits(candidate, m.width, m.height) {
			return candidate
		}
	}
	candidate := compactView(wrap(""))
	smallest = candidate
	if viewFits(candidate, m.width, m.height) {
		return candidate
	}
	return smallest
}

func visualRows(view string, width int) int {
	if view == "" {
		return 0
	}
	if width <= 0 {
		return 0
	}
	return strings.Count(ansi.Hardwrap(view, width, true), "\n") + 1
}

func viewFits(view string, width, height int) bool {
	if view == "" {
		return true
	}
	if width <= 0 || height <= 0 || visualRows(view, width) > height {
		return false
	}
	for _, row := range strings.Split(ansi.Hardwrap(view, width, true), "\n") {
		if ansi.StringWidth(row) > width {
			return false
		}
	}
	return true
}

func compactView(view string) string {
	lines := strings.Split(view, "\n")
	compact := lines[:0]
	for _, line := range lines {
		if line != "" {
			compact = append(compact, line)
		}
	}
	return strings.Join(compact, "\n")
}

func fitViewHeight(view string, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	if viewFits(view, width, height) {
		return view
	}
	view = compactView(view)
	if viewFits(view, width, height) {
		return view
	}

	// The emergency fallback favours terminal safety over styling. Stripping
	// styles before splitting visual rows prevents a crop from separating an
	// opening escape sequence from its reset.
	wrapped := ansi.Hardwrap(ansi.Strip(view), width, true)
	lines := strings.Split(wrapped, "\n")
	safeLines := lines[:0]
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, width, "")
		if lines[i] != "" {
			safeLines = append(safeLines, lines[i])
		}
	}
	lines = safeLines
	if len(lines) == 0 {
		return ""
	}
	if len(lines) <= height {
		return strings.Join(lines, "\n")
	}
	if height == 1 {
		return lines[0]
	}
	return strings.Join(append(lines[:height-1], lines[len(lines)-1]), "\n")
}
