package insights

import (
	"fmt"
	"time"

	"github.com/Hypership-Software/atlas/internal/telemetry"

	tea "github.com/charmbracelet/bubbletea"
)

// Run loads the read-model's sessions and drives the interactive dashboard until
// the user quits.
func Run(store *telemetry.Store) error {
	sessions, err := store.Sessions()
	if err != nil {
		return fmt.Errorf("insights: load sessions: %w", err)
	}
	now := time.Now()
	sessions = recentSessions(sessions, now)
	m := build(sessions, aggregate(sessions, now), store.EventsForSession)
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		return fmt.Errorf("insights: run tui: %w", err)
	}
	return nil
}
