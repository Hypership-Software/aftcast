package insights

import (
	"fmt"
	"strings"
	"time"

	"github.com/Hypership-Software/atlas/internal/analytics"
	"github.com/Hypership-Software/atlas/internal/schema"
	"github.com/Hypership-Software/atlas/internal/ui"
)

const frictionCardLimit = 2

// frictionWindow keeps failures that provably happened in the last 7 days —
// the card says "this week", so an undatable failure can't be claimed by it
// (unlike recentSessions, where keeping undatable rows is the honest default).
func frictionWindow(events []schema.TelemetryEvent, now time.Time) []schema.TelemetryEvent {
	out := make([]schema.TelemetryEvent, 0, len(events))
	for _, e := range events {
		t, err := time.Parse(time.RFC3339Nano, e.TS)
		if err != nil {
			continue
		}
		if now.Sub(t) <= recentWindow {
			out = append(out, e)
		}
	}
	return out
}

func describeFriction(c analytics.FrictionCluster) string {
	switch {
	case c.ToolClass == schema.ClassExec && len(c.Verbs) > 0 && c.Verbs[0] == "cd":
		return "failed to change directory"
	case c.ToolClass == schema.ClassExec && len(c.Verbs) > 0:
		return "had " + strings.Join(c.Verbs, " ") + " commands fail"
	case c.ToolClass == schema.ClassFileRead:
		return "had file reads fail"
	case c.ToolClass == schema.ClassFileWrite:
		return "had file edits fail"
	case c.ToolClass == schema.ClassNetFetch:
		return "had web fetches fail"
	case c.ToolClass == schema.ClassNetSearch:
		return "had web searches fail"
	case c.ToolClass == schema.ClassMCP:
		return "had " + mcpServer(c.ToolName) + " connector calls fail"
	}
	name := c.ToolName
	if name == "" {
		name = "tool"
	}
	return "had " + name + " calls fail"
}

func mcpServer(tool string) string {
	parts := strings.Split(tool, "__")
	if len(parts) >= 2 && parts[1] != "" {
		return parts[1]
	}
	return "connector"
}

func frictionLine(c analytics.FrictionCluster) string {
	return fmt.Sprintf("Your agents %s %d times across %s on %s this week.",
		describeFriction(c), c.Failures,
		countNoun(len(c.Sessions), "session", "sessions"),
		countNoun(c.Days, "day", "days"))
}

func renderFriction(clusters []analytics.FrictionCluster) string {
	if len(clusters) == 0 {
		return ""
	}
	lines := []string{"Worth a permanent fix · across your projects"}
	shown := clusters
	if len(shown) > frictionCardLimit {
		shown = shown[:frictionCardLimit]
	}
	for _, c := range shown {
		lines = append(lines,
			"  "+frictionLine(c),
			"    "+ui.Hint("→ gated coach export "+c.Slug()))
	}
	if rest := len(clusters) - len(shown); rest > 0 {
		lines = append(lines, "  "+ui.Hint(fmt.Sprintf("…%d more worth a look · gated coach", rest)))
	}
	return strings.Join(lines, "\n")
}
