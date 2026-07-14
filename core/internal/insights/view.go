package insights

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/Hypership-Software/atlas/internal/analytics"
	"github.com/Hypership-Software/atlas/internal/schema"
	"github.com/Hypership-Software/atlas/internal/telemetry"
	"github.com/Hypership-Software/atlas/internal/ui"
)

const (
	maxBar      = 40
	cleanBarLen = 20
	trendEps    = 0.0001
)

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func renderHeader(agg aggregates) string {
	return strings.Join([]string{
		headerContext(agg),
		"",
		renderLandedClean(agg.profile),
		renderRework(agg.profile),
		renderRisk(agg),
	}, "\n")
}

func headerContext(agg aggregates) string {
	line := fmt.Sprintf("Atlas — last 7 days · %d sessions", agg.profile.Sessions)
	if agg.user != "" {
		line += " · " + agg.user
	}
	return line
}

func renderLandedClean(p analytics.Profile) string {
	rate := p.CleanDeliveryRate
	filled := int(math.Round(rate * cleanBarLen))
	if filled > cleanBarLen {
		filled = cleanBarLen
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", cleanBarLen-filled)
	return fmt.Sprintf("%-16s %s  %.0f%%%s    %d of %d sessions, no rework needed",
		ui.Bold("Landed clean"), bar, rate*100, trendClause(p), p.CleanCount, p.Sessions)
}

func renderRework(p analytics.Profile) string {
	return fmt.Sprintf("%-16s %.1f fixes / session", ui.Bold("Rework"), p.CorrectionLoad)
}

// trendClause reads Profile.Trend, the clean-delivery-rate change (later half
// minus earlier half). analytics.trend returns 0 both for "flat" and for
// "too few sessions to compute"; we can't tell them apart, so a 0 trend omits
// the clause rather than fabricate a "→ flat" that might be a non-computation.
func trendClause(p analytics.Profile) string {
	prev := clampPercent(int(math.Round((p.CleanDeliveryRate - p.Trend) * 100)))
	switch {
	case p.Trend > trendEps:
		return fmt.Sprintf("  ↑ up from %d%%", prev)
	case p.Trend < -trendEps:
		return fmt.Sprintf("  ↓ down from %d%%", prev)
	default:
		return ""
	}
}

func clampPercent(n int) int {
	if n < 0 {
		return 0
	}
	if n > 100 {
		return 100
	}
	return n
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
	return fmt.Sprintf("%-16s %s   %s", ui.Bold("Risk"), base, suffix)
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

func renderEmpty() string {
	return "No sessions captured yet — run a Claude Code session with the gate active.\n" +
		ui.Hint("If you expected data, check `gated status` for a capture gap.")
}

func renderList(agg aggregates, tableView string) string {
	return strings.Join([]string{
		renderHeader(agg),
		"",
		"What the AI worked on",
		renderTaskMix(agg.taskMix),
		"",
		"Needs attention",
		renderAttentionBlock(agg.needsAttention),
		"",
		tableView,
		ui.Hint("↑↓ nav · ↵ inspect · q quit"),
	}, "\n")
}

func detailSummary(sess telemetry.Session, events []schema.TelemetryEvent) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s  %s  %s  outcome=%s  %dms  taint=%v\n\n",
		sess.SessionID, sess.Harness, sess.TaskType, sess.Outcome, sess.DurationMS, sess.Taint)
	for _, e := range events {
		tool := e.ToolRaw
		if tool == "" {
			tool = string(e.ToolClass)
		}
		fmt.Fprintf(&b, "[t%d] %-16v %-28s risk=%v ok=%v sub=%s %s\n",
			e.TurnIndex, e.EventType, tool, e.Risk, e.ToolOK, e.Subagent, e.TS)
	}
	return b.String()
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
	return detailSummary(sess, events)
}

func renderDetail(sessionID, body string) string {
	return strings.Join([]string{
		ui.Bold("session " + sessionID),
		body,
		ui.Hint("↑↓ scroll · r raw · esc back · q quit"),
	}, "\n")
}
