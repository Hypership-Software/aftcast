package approval

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Hypership-Software/atlas/internal/schema"
)

// Controller is what the TUI drives. *Queue satisfies it in-process; a future
// IPC-backed client (Task 23) satisfies it when `gated approvals` runs as a
// separate process from the daemon.
type Controller interface {
	Pending() []Pending
	Resolve(id string, v schema.Verdict, makeRule bool) error
}

// Run starts the interactive approvals TUI over the given controller.
func Run(c Controller) error {
	_, err := tea.NewProgram(newModel(c)).Run()
	return err
}

var (
	titleStyle  = lipgloss.NewStyle().Bold(true)
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	cursorStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	okStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	denyStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
)

type refreshMsg struct{}

type model struct {
	ctrl    Controller
	pending []Pending
	cursor  int
	status  string
}

func newModel(c Controller) model {
	return model{ctrl: c, pending: c.Pending()}
}

func (m model) Init() tea.Cmd { return tick() }

func tick() tea.Cmd {
	return tea.Tick(400*time.Millisecond, func(time.Time) tea.Msg { return refreshMsg{} })
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case refreshMsg:
		m.pending = m.ctrl.Pending()
		if m.cursor >= len(m.pending) {
			m.cursor = max(0, len(m.pending)-1)
		}
		return m, tick()
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.pending)-1 {
			m.cursor++
		}
	case "a":
		m.resolveSelected(schema.VerdictAllow, false, "approved")
	case "A":
		m.resolveSelected(schema.VerdictAllow, true, "approved + drafted standing rule")
	case "d":
		m.resolveSelected(schema.VerdictDeny, false, "denied")
	}
	return m, nil
}

func (m *model) resolveSelected(v schema.Verdict, makeRule bool, label string) {
	if m.cursor < 0 || m.cursor >= len(m.pending) {
		return
	}
	sel := m.pending[m.cursor]
	if err := m.ctrl.Resolve(sel.ID, v, makeRule); err != nil {
		m.status = "error: " + err.Error()
		return
	}
	m.status = fmt.Sprintf("%s %s", label, describe(sel.Desc))
	m.pending = m.ctrl.Pending()
	if m.cursor >= len(m.pending) {
		m.cursor = max(0, len(m.pending)-1)
	}
}

func (m model) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("gated · approvals"))
	b.WriteByte('\n')

	if len(m.pending) == 0 {
		b.WriteString(dimStyle.Render("\n  No approvals waiting. The gate is quiet.\n"))
	} else {
		b.WriteByte('\n')
		for i, p := range m.pending {
			cursor := "  "
			line := fmt.Sprintf("%-4s  %-11s  %s  %s",
				p.ID, p.Desc.ToolClass, describe(p.Desc), dimStyle.Render(age(p.At)))
			if i == m.cursor {
				cursor = cursorStyle.Render("▸ ")
				line = cursorStyle.Render(line)
			}
			b.WriteString(cursor + line + "\n")
		}
	}

	b.WriteByte('\n')
	b.WriteString(dimStyle.Render("  ↑/↓ move · "))
	b.WriteString(okStyle.Render("a") + dimStyle.Render(" approve · "))
	b.WriteString(okStyle.Render("A") + dimStyle.Render(" always · "))
	b.WriteString(denyStyle.Render("d") + dimStyle.Render(" deny · q quit"))
	if m.status != "" {
		b.WriteString("\n\n  " + m.status)
	}
	b.WriteByte('\n')
	return b.String()
}

// describe renders the salient detail of an action for the operator.
func describe(d schema.Descriptor) string {
	switch d.ToolClass {
	case schema.ClassExec:
		return strings.Join(d.Argv, " ")
	case schema.ClassFileRead, schema.ClassFileWrite:
		if len(d.Files) > 0 {
			return d.Files[0]
		}
	case schema.ClassNetFetch, schema.ClassNetSearch:
		return d.Domain
	case schema.ClassMCP:
		return d.MCPServer + "/" + d.MCPTool
	}
	return d.ToolRaw
}

func age(at time.Time) string {
	if at.IsZero() {
		return ""
	}
	d := time.Since(at).Round(time.Second)
	return d.String() + " ago"
}
