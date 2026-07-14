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
		ui.Hint("↑↓ (k/j) move · ↵ open · s sort · h show/hide empty · ? help · q quit"),
	}, "\n")
}

// renderHelp is the full keybinding overlay, opened with ? from either the
// list or detail screen. It only lists keys the model actually handles —
// no aspirational bindings (e.g. no "/" filter, which isn't implemented yet).
func renderHelp() string {
	return strings.Join([]string{
		ui.Bold("Keybindings — help"),
		"",
		"↑↓ (k/j) move · ↵ open · esc back · s sort · h show/hide empty · r raw (detail) · ? help · q quit",
		"",
		"⚠ untrusted input · ⚑ flagged actions · ★ skills",
		"",
		ui.Hint("esc or ? to close"),
	}, "\n")
}

func hiddenNote(n int) string {
	if n <= 0 {
		return ""
	}
	word := "sessions"
	if n == 1 {
		word = "session"
	}
	return ui.Hint(fmt.Sprintf("… %d empty %s hidden (press h to show)", n, word))
}

const traceBarWidth = 10

// renderTrace is the session-detail centerpiece: a verdict header (Sentry-style
// one-liner) followed by a turn-grouped, run-collapsed call timeline. It mostly
// formats buildTrace's model — pairing, grouping, collapsing, and annotation are
// already decided there.
func renderTrace(sess telemetry.Session, evs []schema.TelemetryEvent) string {
	var b strings.Builder
	b.WriteString(renderVerdictHeader(sess))
	b.WriteString("\n\n")
	turns := buildTrace(evs)
	for i, t := range turns {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "  %s\n", turnHeader(t))
		maxDur := maxRowDur(t.Rows)
		for _, r := range t.Rows {
			if row := renderTraceRow(r, maxDur); row != "" {
				b.WriteString(row + "\n")
			}
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func renderVerdictHeader(sess telemetry.Session) string {
	rest := []string{
		taskCell(sess.TaskType),
		verdictOutcome(sess),
	}
	if dur := humanizeDuration(sess.DurationMS); dur != "" {
		rest = append(rest, dur)
	}
	rest = append(rest,
		fmt.Sprintf("%d calls", sess.ToolCalls),
		fmt.Sprintf("%d files", sess.FilesTouched),
	)
	if flags := flagsCell(sess); flags != "" {
		rest = append(rest, flags)
	}
	return "Session " + ui.Hint(shortID(sess.SessionID)) + " · " + strings.Join(rest, " · ")
}

func verdictOutcome(sess telemetry.Session) string {
	class := analytics.OutcomeClass(sess.Outcome)
	switch {
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

func turnHeader(t traceTurn) string {
	phase := t.Phase
	if phase != "" {
		phase += " "
	}
	parts := []string{fmt.Sprintf("turn %d", t.Index), fmt.Sprintf("%s%d calls", phase, t.Calls)}
	if dur := humanizeDuration(t.DurMS); dur != "" {
		parts = append(parts, dur)
	}
	return strings.Join(parts, " · ")
}

func maxRowDur(rows []traceRow) int64 {
	var max int64
	for _, r := range rows {
		if r.DurMS > max {
			max = r.DurMS
		}
	}
	return max
}

func renderTraceRow(r traceRow, maxDur int64) string {
	head := strings.TrimRight(rowGlyph(r)+" "+r.Verb, " ")
	if head == "" && r.Detail == "" {
		return ""
	}
	var parts []string
	if head != "" {
		parts = append(parts, head)
	}
	if r.Detail != "" {
		parts = append(parts, r.Detail)
	}
	if n := scaleBar(r.DurMS, maxDur, traceBarWidth); n > 0 {
		parts = append(parts, strings.Repeat("▇", n))
	}
	if dur := humanizeDuration(r.DurMS); dur != "" {
		parts = append(parts, dur)
	}
	if og := outcomeGlyph(r.Outcome); og != "" {
		parts = append(parts, og)
	}
	if r.Subagent != "" {
		parts = append(parts, fmt.Sprintf("(subagent %s)", r.Subagent))
	}
	line := "      " + strings.Join(parts, "  ")
	if r.Untrusted {
		line += "  ← untrusted content enters here"
	}
	return line
}

func rowGlyph(r traceRow) string {
	switch {
	case r.Skill:
		return "★"
	case r.Untrusted:
		return ui.Warn("⚠")
	case r.Failed:
		return ui.Bad("✗")
	case r.Danger:
		return ui.Bad("⚑")
	default:
		return " "
	}
}

func outcomeGlyph(o schema.ToolOutcome) string {
	switch o {
	case schema.OutcomeOK:
		return ui.OK("✓")
	case schema.OutcomeFailed:
		return ui.Bad("✗")
	default:
		return ""
	}
}

// scaleBar scales dur against the turn's max row duration into a 1..width block
// count so the slowest calls in a turn pop visually; sub-100ms calls get none.
func scaleBar(dur, maxDur int64, width int) int {
	if dur < 100 || maxDur <= 0 {
		return 0
	}
	n := int(math.Round(float64(dur) / float64(maxDur) * float64(width)))
	if n < 1 {
		n = 1
	}
	if n > width {
		n = width
	}
	return n
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
