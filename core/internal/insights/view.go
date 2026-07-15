package insights

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Hypership-Software/aftcast/internal/analytics"
	"github.com/Hypership-Software/aftcast/internal/schema"
	"github.com/Hypership-Software/aftcast/internal/telemetry"
	"github.com/Hypership-Software/aftcast/internal/ui"
)

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

const metricLabelWidth = 16

// metricLabel pads the plain label to a fixed width BEFORE styling. fmt's %-Ns
// counts a style's ANSI escape bytes, so padding a coloured label would misalign
// the meters in a real terminal — pad first, then colour the padded string.
func metricLabel(s string) string {
	return ui.Bold(fmt.Sprintf("%-*s", metricLabelWidth, s))
}

func renderHeader(agg aggregates) string {
	return strings.Join([]string{
		surfaceContext(agg, surfaceOverview),
		"",
		renderShipped(agg.shipping),
		renderWorkObserved(agg),
		renderCorrections(agg.profile),
		renderSecurity(agg),
	}, "\n")
}

func surfaceContext(agg aggregates, surface listSurface) string {
	overview, security := "[Projects]", "Security"
	if surface == surfaceSecurity {
		overview, security = "Projects", "[Security]"
	}
	return headerContext(agg) + "    " + overview + " | " + security
}

func headerContext(agg aggregates) string {
	label := agg.scopeLabel
	if label == "" {
		label = "all projects"
	}
	parts := []string{}
	if agg.user != "" {
		parts = append(parts, agg.user)
	}
	parts = append(parts, label, "last 7 days")
	return "Aftcast — " + strings.Join(parts, " · ")
}

func renderShipped(p analytics.ShippedProfile) string {
	if p.Eligible > 0 {
		return fmt.Sprintf("%s %d of %d delivery sessions · %.0f%%", metricLabel("Shipped"), p.Shipped, p.Eligible, p.Rate*100)
	}
	if !p.TrackingSince.IsZero() {
		return fmt.Sprintf("%s Tracking since %s · waiting for first delivery session", metricLabel("Shipped"), p.TrackingSince.Format("2 Jan"))
	}
	return fmt.Sprintf("%s Starts with your next captured session", metricLabel("Shipped"))
}

func renderWorkObserved(agg aggregates) string {
	return fmt.Sprintf("%s %s across %s", metricLabel("Work observed"),
		countNoun(agg.profile.Sessions, "session", "sessions"), countNoun(agg.projects, "project", "projects"))
}

func renderCorrections(p analytics.Profile) string {
	if p.Completed == 0 {
		return fmt.Sprintf("%s no completed sessions yet", metricLabel("Corrections"))
	}
	prefix := "none"
	if p.TotalCorrections > 0 {
		prefix = countNoun(p.TotalCorrections, "human correction", "human corrections")
	}
	return fmt.Sprintf("%s %s across %s", metricLabel("Corrections"), prefix,
		countNoun(p.Completed, "completed session", "completed sessions"))
}

func renderSecurity(agg aggregates) string {
	if agg.securitySessions == 0 && agg.danger == 0 {
		return fmt.Sprintf("%s nothing flagged", metricLabel("Security"))
	}
	verb := "need"
	if agg.securitySessions == 1 {
		verb = "needs"
	}
	return fmt.Sprintf("%s %s %s review · %s   Tab to review", metricLabel("Security"),
		countNoun(agg.securitySessions, "session", "sessions"), verb, countNoun(agg.danger, "flagged action", "flagged actions"))
}

func renderScopedEmpty(global, hasHistory bool) string {
	if !global && hasHistory {
		return "No Aftcast activity for this project in the last 7 days.\n" +
			ui.Hint("Press g to view all projects · q to quit")
	}
	if !global {
		return "No Aftcast activity for this project yet.\n" +
			ui.Hint("Press g to view all projects · q to quit")
	}
	if global && hasHistory {
		return "No Aftcast activity in the last 7 days.\n" +
			ui.Hint("? help · q quit")
	}
	return "Nothing captured yet — start a Claude Code session in a wired project.\n" +
		ui.Hint("Check `gated status` if you expected data.")
}

func renderEmptyList(coach analytics.PlanAssociation, empty string) string {
	return renderCoach(coach) + "\n\n" + empty
}

func renderCoach(a analytics.PlanAssociation) string {
	title := "What's moving your needle"
	if a.Window > 0 {
		title += fmt.Sprintf(" · latest %d comparable sessions", a.Window)
	}
	if a.Direction == analytics.AssociationNegative {
		return strings.Join([]string{
			title,
			fmt.Sprintf("  Plan-first was associated with fewer shipped sessions · %s work", taskCell(a.TaskType)),
			fmt.Sprintf("  %.0f%% planned vs %.0f%% direct-to-edit · n=%d", a.PlannedRate*100, a.DirectRate*100, a.Total),
		}, "\n")
	}
	switch a.Status {
	case analytics.CoachRecommend:
		return strings.Join([]string{
			title,
			fmt.Sprintf("  Plan-first was associated with more shipped sessions · %s work", taskCell(a.TaskType)),
			fmt.Sprintf("  %.0f%% planned vs %.0f%% direct-to-edit · n=%d", a.PlannedRate*100, a.DirectRate*100, a.Total),
			"",
			"Try next",
			"  → Plan before editing on your next " + taskCell(a.TaskType) + ".",
		}, "\n")
	case analytics.CoachNoPattern:
		return strings.Join([]string{
			title,
			"  No reliable plan-first pattern yet.",
			fmt.Sprintf("  %.0f%% planned vs %.0f%% direct-to-edit · %s work · n=%d", a.PlannedRate*100, a.DirectRate*100, taskCell(a.TaskType), a.Total),
		}, "\n")
	default:
		if a.Total == 0 {
			return title + "\n  Learning your baseline · 0 of 20 comparable delivery sessions"
		}
		return strings.Join([]string{
			title,
			fmt.Sprintf("  Learning your baseline · %d of 20 comparable delivery sessions", a.Total),
			fmt.Sprintf("  plan-first %d · direct-to-edit %d · need 20 total and 5 each way", a.Planned, a.Direct),
		}, "\n")
	}
}

func renderList(agg aggregates, coach analytics.PlanAssociation, tableView string) string {
	return strings.Join([]string{
		renderHeader(agg),
		"",
		renderCoach(coach),
		"",
		ui.Bold("Recent sessions"),
		tableView,
		ui.Hint("↑↓ move · ↵ open · tab security · s sort · h empty · g/p scope · ? help · q quit"),
	}, "\n")
}

func renderSecurityList(agg aggregates, tableView string, findings int) string {
	body := tableView
	if findings == 0 {
		body = "Nothing needs review in this scope."
	}
	return strings.Join([]string{
		surfaceContext(agg, surfaceSecurity),
		"",
		ui.Bold("Security review"),
		fmt.Sprintf("%s with security signals · %s", countNoun(agg.securitySessions, "session", "sessions"),
			countNoun(agg.danger, "flagged action", "flagged actions")),
		"",
		body,
		ui.Hint("↑↓ move · ↵ open · tab projects · g/p scope · ? help · q quit"),
	}, "\n")
}

// renderHelp is the full keybinding overlay, opened with ? from either the
// list or detail screen. It only lists keys the model actually handles —
// no aspirational bindings (e.g. no "/" filter, which isn't implemented yet).
func renderHelp() string {
	return strings.Join([]string{
		ui.Bold("Keybindings — help"),
		"",
		"↑↓ (k/j) move · ↵ open · esc back · tab projects/security · g/p scope · r raw (detail) · ? help · q quit",
		"",
		"⚠ untrusted input · ⚑ flagged actions · ★ invoked skills",
		"",
		"Shipped = a successful git push in a delivery session",
		"Delivery session = changed files or successfully pushed, captured with v2 telemetry",
		"Observed plan-first = explicit planning, or a completed preparatory prompt before editing",
		"Worth a permanent fix = the same kind of failure in 3+ sessions on 2+ days this week",
		"",
		ui.Hint("esc or ? to close"),
	}, "\n")
}

func hiddenNote(n int) string {
	if n <= 0 {
		return ""
	}
	return ui.Hint("… " + hiddenSummary(n) + " (press h to show)")
}

func hiddenSummary(n int) string {
	word := "sessions"
	if n == 1 {
		word = "session"
	}
	return fmt.Sprintf("%d empty %s hidden", n, word)
}

func verdictOutcome(sess telemetry.Session) string {
	class := analytics.OutcomeClass(sess.Outcome)
	switch {
	case sess.Shipped:
		return ui.OK("↑ shipped")
	case class == analytics.Success:
		return ui.OK("✓ succeeded")
	case class == analytics.Failure:
		return ui.Bad("✗ failed")
	default:
		return ui.Hint("—")
	}
}

func detailRawJSON(events []schema.TelemetryEvent) (string, error) {
	b, err := json.MarshalIndent(events, "", "  ")
	if err != nil {
		return "", fmt.Errorf("insights: encode events: %w", err)
	}
	return string(b), nil
}

func detailBody(sess telemetry.Session, events []schema.TelemetryEvent, raw bool) string {
	if raw {
		s, err := detailRawJSON(events)
		if err != nil {
			return "raw encode error: " + err.Error()
		}
		return s
	}
	return renderTrace(sess, events)
}

func renderDetail(body string) string {
	return strings.Join([]string{
		body,
		ui.Hint("↑↓ scroll · r raw · esc back · ? help · q quit"),
	}, "\n")
}
