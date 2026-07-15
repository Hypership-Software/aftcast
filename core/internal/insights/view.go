package insights

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Hypership-Software/atlas/internal/analytics"
	"github.com/Hypership-Software/atlas/internal/schema"
	"github.com/Hypership-Software/atlas/internal/telemetry"
	"github.com/Hypership-Software/atlas/internal/ui"
)

const maxBar = 40

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
		headerContext(agg),
		"",
		renderShipped(agg.shipping),
		renderIntervention(agg.profile),
		renderRisk(agg),
	}, "\n")
}

func headerContext(agg aggregates) string {
	label := agg.scopeLabel
	if label == "" {
		label = "all projects"
	}
	line := fmt.Sprintf("Atlas — %s · last 7 days · %d sessions", label, agg.profile.Sessions)
	if agg.user != "" {
		line += " · " + agg.user
	}
	return line
}

func renderShipped(p analytics.ShippedProfile) string {
	if p.Eligible == 0 {
		return fmt.Sprintf("%s no observable delivery sessions yet", metricLabel("Shipped"))
	}
	return fmt.Sprintf("%s %d of %d delivery sessions  %.0f%%", metricLabel("Shipped"), p.Shipped, p.Eligible, p.Rate*100)
}

func renderIntervention(p analytics.Profile) string {
	return fmt.Sprintf("%s %.1f human fixes / completed session", metricLabel("Intervention"), p.CorrectionLoad)
}

func renderRisk(agg aggregates) string {
	sessionWord := "sessions"
	if agg.tainted == 1 {
		sessionWord = "session"
	}
	base := fmt.Sprintf("%d %s on untrusted input · %d flagged actions", agg.tainted, sessionWord, agg.danger)
	suffix := ui.OK("✓ nothing flagged")
	if agg.tainted > 0 || agg.danger > 0 {
		suffix = ui.Warn("⚠ review")
	}
	return fmt.Sprintf("%s %s   %s", metricLabel("Risk"), base, suffix)
}

func renderTaskMix(mix []taskCount) string {
	var b strings.Builder
	for _, tc := range mix {
		label := tc.task
		if label == "unknown" || label == "" {
			label = "other"
		}
		n := tc.n
		if n > maxBar {
			n = maxBar
		}
		fmt.Fprintf(&b, "%-12s %s %d\n", label, strings.Repeat("█", n), tc.n)
	}
	return strings.TrimRight(b.String(), "\n")
}

// renderNeedsAttention builds the 0-3 actionable lines for the overview's
// "Needs attention" section: tainted/danger sessions (most recent first),
// then the existing skill-opportunity signal if room remains.
func renderNeedsAttention(sessions []telemetry.Session, agg aggregates, now time.Time) []string {
	var flagged []telemetry.Session
	for _, s := range sessions {
		if s.Taint || s.DangerDetected > 0 {
			flagged = append(flagged, s)
		}
	}
	sort.SliceStable(flagged, func(i, j int) bool { return flagged[i].Started > flagged[j].Started })

	var lines []string
	for _, s := range flagged {
		if len(lines) >= 3 {
			break
		}
		lines = append(lines, riskAttentionLine(s, now))
	}
	if len(lines) < 3 {
		if line, ok := skillOpportunityLine(sessions, agg.skills); ok {
			lines = append(lines, line)
		}
	}
	return lines
}

func riskAttentionLine(s telemetry.Session, now time.Time) string {
	var reason string
	switch {
	case s.Taint && s.DangerDetected > 0:
		reason = "untrusted input + a flagged command"
	case s.Taint:
		reason = "untrusted input"
	default:
		reason = pluralActions(s.DangerDetected)
	}
	when := humanize(s.Started, now)
	if when == "" {
		when = "recently"
	}
	return fmt.Sprintf("%s  session %s — %s      → ↵ to inspect", ui.Warn("⚠"), when, reason)
}

func pluralActions(n int) string {
	if n == 1 {
		return "1 flagged action"
	}
	return fmt.Sprintf("%d flagged actions", n)
}

func skillOpportunityLine(sessions []telemetry.Session, sk analytics.SkillReport) (string, bool) {
	if len(sk.Opportunities) == 0 {
		return "", false
	}
	oppTypes := map[string]bool{}
	for _, t := range sk.Opportunities {
		oppTypes[t] = true
	}
	n := 0
	for _, s := range sessions {
		if s.CorrectionTurns > 0 && s.SkillsUsed == "" && oppTypes[s.TaskType] {
			n++
		}
	}
	if n == 0 {
		return "", false
	}
	plural := ""
	if n != 1 {
		plural = "s"
	}
	return fmt.Sprintf("○  %d rework-heavy session%s ran with no skill in play         → skill opportunity", n, plural), true
}

func renderAttentionBlock(lines []string) string {
	if len(lines) == 0 {
		return ui.OK("✓ nothing needs attention")
	}
	return strings.Join(lines, "\n")
}

func renderScopedEmpty(global, hasHistory bool) string {
	if !global && hasHistory {
		return "No Atlas activity for this project in the last 7 days.\n" +
			ui.Hint("Press g to view all projects · q to quit")
	}
	if !global {
		return "No Atlas activity for this project yet.\n" +
			ui.Hint("Press g to view all projects · q to quit")
	}
	if global && hasHistory {
		return "No Atlas activity in the last 7 days.\n" +
			ui.Hint("? help · q quit")
	}
	return "Nothing captured yet — start a Claude Code session in a wired project.\n" +
		ui.Hint("Check `gated status` if you expected data.")
}

func renderEmptyList(coach analytics.PlanAssociation, empty string) string {
	return renderCoach(coach) + "\n\n" + empty
}

func renderCoach(a analytics.PlanAssociation) string {
	title := "What's moving your needle · across your projects"
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
			return title + "\n  Atlas is learning your workflow · no comparable delivery sessions yet."
		}
		return strings.Join([]string{
			title,
			fmt.Sprintf("  Atlas is learning your workflow · %d comparable sessions", a.Total),
			fmt.Sprintf("  plan-first %d · direct-to-edit %d · need 20 total and 5 each way", a.Planned, a.Direct),
		}, "\n")
	}
}

func renderList(agg aggregates, coach analytics.PlanAssociation, tableView string) string {
	return strings.Join([]string{
		renderHeader(agg),
		"",
		"What the AI worked on",
		renderTaskMix(agg.taskMix),
		"",
		"Needs attention",
		renderAttentionBlock(agg.needsAttention),
		"",
		renderCoach(coach),
		"",
		tableView,
		ui.Hint("↑↓ (k/j) move · ↵ open · s sort · h show/hide empty · g/p scope · ? help · q quit"),
	}, "\n")
}

// renderHelp is the full keybinding overlay, opened with ? from either the
// list or detail screen. It only lists keys the model actually handles —
// no aspirational bindings (e.g. no "/" filter, which isn't implemented yet).
func renderHelp() string {
	return strings.Join([]string{
		ui.Bold("Keybindings — help"),
		"",
		"↑↓ (k/j) move · ↵ open · esc back · s sort · h show/hide empty · g/p scope · r raw (detail) · ? help · q quit",
		"",
		"⚠ untrusted input · ⚑ flagged actions · ★ skills",
		"",
		"Shipped = a successful git push in a delivery session",
		"Delivery session = changed files or successfully pushed, captured with v2 telemetry",
		"Observed plan-first = explicit planning, or a completed preparatory prompt before editing",
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
	case class == analytics.Success && sess.CorrectionTurns > 0:
		return ui.Warn(fmt.Sprintf("✓ succeeded (%d fix)", sess.CorrectionTurns))
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
