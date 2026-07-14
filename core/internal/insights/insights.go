package insights

import (
	"fmt"
	"time"

	"github.com/Hypership-Software/atlas/internal/telemetry"

	tea "github.com/charmbracelet/bubbletea"
)

// Scope selects which project's sessions the TUI starts scoped to. An empty
// ProjectID or StartGlobal renders every project's sessions.
type Scope struct {
	ProjectID   string
	Name        string
	StartGlobal bool
}

// Run loads the read-model's sessions and drives the interactive dashboard until
// the user quits.
func Run(store *telemetry.Store, scope Scope) error {
	sessions, err := store.Sessions()
	if err != nil {
		return fmt.Errorf("insights: load sessions: %w", err)
	}
	now := time.Now()
	m := build(sessions, scope, store.EventsForSession, now)
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		return fmt.Errorf("insights: run tui: %w", err)
	}
	return nil
}
