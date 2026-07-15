package insights

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/Hypership-Software/atlas/internal/analytics"
	"github.com/Hypership-Software/atlas/internal/schema"
	"github.com/Hypership-Software/atlas/internal/telemetry"
	"github.com/Hypership-Software/atlas/internal/ui"
)

// renderTrace is the session-detail centerpiece: a story, not a call log. A
// verdict header, a "Highlights" block that surfaces the signal an observer
// cares about (skills, untrusted-input entry, flagged/failed calls, the slowest
// operation), and a one-line-per-turn timeline with an activity breakdown. The
// full call-by-call sequence lives behind `r` (raw JSON) — folding a 600-call
// session into ~20 readable lines is the whole point.
func renderTrace(sess telemetry.Session, evs []schema.TelemetryEvent) string {
	turns := buildTrace(evs)
	var b strings.Builder
	b.WriteString(verdictHeader(sess, turns))
	if hs := highlightLines(sess, turns); len(hs) > 0 {
		b.WriteString("\n\n" + ui.Bold("Highlights") + "\n" + strings.Join(hs, "\n"))
	}
	if tl := timelineLines(turns); len(tl) > 0 {
		b.WriteString("\n\n" + ui.Bold("Timeline") + "\n" + strings.Join(tl, "\n"))
	}
	return b.String()
}

// verdictHeader is the Sentry-style two-line summary: identity + outcome + time
// on top, the counts and flag glyphs beneath. Fields that are empty (no
// duration, no flags) are dropped rather than left as dangling separators.
func verdictHeader(sess telemetry.Session, turns []traceTurn) string {
	projectName := sess.ProjectName
	if projectName == "" {
		projectName = "other project"
	}
	line1 := strings.Join([]string{projectName, taskCell(sess.TaskType), verdictOutcome(sess)}, " · ")

	timing := []string{"Session " + ui.Hint(shortID(sess.SessionID))}
	if d := humanizeDuration(sess.DurationMS); d != "" {
		timing = append(timing, "wall span "+d)
	}
	if d := humanizeDuration(observedToolTime(turns)); d != "" {
		timing = append(timing, "observed tool time "+d)
	}

	counts := []string{
		countNoun(sess.ToolCalls, "call", "calls"),
		fmt.Sprintf("%d changed", sess.FilesChanged),
		fmt.Sprintf("%d touched", sess.FilesTouched),
	}
	if n := len(splitSkills(sess.SkillsUsed)); n > 0 {
		counts = append(counts, "★ "+countNoun(n, "skill", "skills"))
	}
	return line1 + "\n" + strings.Join(timing, " · ") + "\n" + strings.Join(counts, " · ")
}

func observedToolTime(turns []traceTurn) int64 {
	var total int64
	for _, turn := range turns {
		total += turn.DurMS
	}
	return total
}

func countNoun(n int, singular, plural string) string {
	word := plural
	if n == 1 {
		word = singular
	}
	return fmt.Sprintf("%d %s", n, word)
}

const highlightLabelWidth = 10

func highlightLine(glyph, label, text string) string {
	return fmt.Sprintf("  %s  %-*s %s", glyph, highlightLabelWidth, label, text)
}

// rowRef remembers where a notable row happened so a highlight can name its turn.
type rowRef struct {
	row  traceRow
	turn int
}

func highlightLines(sess telemetry.Session, turns []traceTurn) []string {
	var lines []string
	if sess.Shipped {
		lines = append(lines, highlightLine(ui.OK("↑"), "shipped", "successful git push"))
	}
	if skills := splitSkills(sess.SkillsUsed); len(skills) > 0 {
		lines = append(lines, highlightLine("★", "skills", strings.Join(prettySkills(skills), ", ")))
	}

	var untrusted *rowRef
	var fails []rowRef
	var flagged []rowRef
	var slowest rowRef
	for _, t := range turns {
		for _, r := range t.Rows {
			if r.Untrusted && untrusted == nil {
				untrusted = &rowRef{r, t.Index}
			}
			if r.Failed {
				fails = append(fails, rowRef{r, t.Index})
			}
			if r.Danger {
				flagged = append(flagged, rowRef{r, t.Index})
			}
			if r.DurMS > slowest.row.DurMS {
				slowest = rowRef{r, t.Index}
			}
		}
	}

	if untrusted != nil {
		via := describeSource(untrusted.row)
		lines = append(lines, highlightLine(ui.Warn("⚠"), "untrusted",
			strings.TrimSpace(fmt.Sprintf("entered turn %d %s", untrusted.turn, via))))
	}
	if len(flagged) > 0 {
		lines = append(lines, highlightLine(ui.Bad("⚑"), "flagged",
			fmt.Sprintf("%d — %s in turn %d", len(flagged), describeRow(flagged[0].row), flagged[0].turn)))
	}
	if len(fails) > 0 {
		glyph, label := ui.Bad("✗"), "failures"
		if analytics.OutcomeClass(sess.Outcome) == analytics.Success {
			glyph, label = ui.OK("↻"), "recovery"
		}
		lines = append(lines, highlightLine(glyph, label, failureText(fails, sess)))
	}
	if slowest.row.DurMS >= slowThresholdMS {
		lines = append(lines, highlightLine(ui.Hint("⏱"), "slowest",
			fmt.Sprintf("%s · %s · turn %d", describeRow(slowest.row), humanizeDuration(slowest.row.DurMS), slowest.turn)))
	}
	return lines
}

const slowThresholdMS = 5000

func failureText(fails []rowRef, sess telemetry.Session) string {
	text := countNoun(len(fails), "failed attempt", "failed attempts")
	switch {
	case analytics.OutcomeClass(sess.Outcome) == analytics.Failure:
		return text + " · session failed"
	case sess.CorrectionTurns > 0:
		return text + " · " + countNoun(sess.CorrectionTurns, "human correction", "human corrections")
	default:
		return text + " · agent recovered"
	}
}

// describeSource names where untrusted content entered, e.g. "via context7 (mcp)"
// or "via evil.example.com". Empty detail yields "" so the caller trims cleanly.
func describeSource(r traceRow) string {
	if r.Detail == "" {
		return ""
	}
	if r.Verb == "mcp" {
		return "via " + r.Detail + " (mcp)"
	}
	return "via " + r.Detail
}

func describeRow(r traceRow) string {
	label := strings.TrimSpace(r.Verb + " " + r.Detail)
	if label == "" {
		label = "a call"
	}
	if r.Subagent != "" && r.Verb != "subagent" {
		label += " (subagent " + r.Subagent + ")"
	}
	return label
}

// prettySkills drops a skill's plugin/namespace prefix ("superpowers:brainstorming"
// → "brainstorming") so the Highlights line reads as prose, not tool ids.
func prettySkills(skills []string) []string {
	out := make([]string, len(skills))
	for i, s := range skills {
		if idx := strings.LastIndex(s, ":"); idx >= 0 && idx+1 < len(s) {
			out[i] = s[idx+1:]
		} else {
			out[i] = s
		}
	}
	return out
}

// timelineLines renders one aligned line per turn: its index, call count and
// duration, then a dimmed activity breakdown. Call counts are right-aligned
// and the stats column padded to a common width so the breakdowns line up — this
// is the backbone the Highlights hang off, each naming the turn a reader jumps to.
func timelineLines(turns []traceTurn) []string {
	callW, statW := 1, 0
	stats := make([]string, len(turns))
	for _, t := range turns {
		if w := len(fmt.Sprintf("%d", t.Calls)); w > callW {
			callW = w
		}
	}
	for i, t := range turns {
		s := "no tool calls"
		if t.Calls > 0 {
			s = fmt.Sprintf("%*d", callW, t.Calls) + " "
			if t.Calls == 1 {
				s += "call"
			} else {
				s += "calls"
			}
			if d := humanizeDuration(t.DurMS); d != "" {
				s += " · " + d
			}
		}
		stats[i] = s
		if w := utf8.RuneCountInString(s); w > statW {
			statW = w
		}
	}
	lines := make([]string, 0, len(turns))
	for i, t := range turns {
		line := fmt.Sprintf("  %-7s  ", fmt.Sprintf("turn %d", t.Index))
		if bd := turnBreakdown(t); bd != "" {
			line += fmt.Sprintf("%-*s   %s", statW, stats[i], ui.Hint(bd))
		} else {
			line += stats[i]
		}
		lines = append(lines, strings.TrimRight(line, " "))
	}
	return lines
}

const maxBreakdownParts = 4

// turnBreakdown summarizes a turn's calls by activity, e.g. "18 edited · 61 ran ·
// 34 read · 12 subagents", keeping the busiest few categories. Collapsed runs
// count their folded rows and omitted activity is explicit.
func turnBreakdown(t traceTurn) string {
	counts := map[string]int{}
	var order []string
	accounted := 0
	for _, r := range t.Rows {
		if r.Verb == "" {
			continue
		}
		n := 1
		if r.CollapsedN > 0 {
			n = r.CollapsedN
		}
		raw := strings.ToLower(strings.TrimSpace(r.Verb))
		if _, ok := counts[raw]; !ok {
			order = append(order, raw)
		}
		counts[raw] += n
		accounted += n
	}
	sort.SliceStable(order, func(i, j int) bool { return counts[order[i]] > counts[order[j]] })
	omitted := 0
	if len(order) > maxBreakdownParts {
		for _, raw := range order[maxBreakdownParts:] {
			omitted += counts[raw]
		}
		order = order[:maxBreakdownParts]
	}
	if t.Calls > accounted {
		omitted += t.Calls - accounted
	}
	parts := make([]string, len(order))
	for i, raw := range order {
		parts[i] = breakdownCount(counts[raw], bucketLabel(raw))
	}
	if omitted > 0 {
		parts = append(parts, fmt.Sprintf("+%d other", omitted))
	}
	return strings.Join(parts, " · ")
}

func bucketLabel(verb string) string {
	switch strings.ToLower(verb) {
	case "askuserquestion":
		return "asked"
	case "grep", "glob":
		return "searched"
	case "subagent":
		return "subagent"
	default:
		return strings.ToLower(verb)
	}
}

func breakdownCount(n int, label string) string {
	switch label {
	case "skill":
		return countNoun(n, "skill", "skills")
	case "subagent":
		return countNoun(n, "subagent", "subagents")
	default:
		return fmt.Sprintf("%d %s", n, label)
	}
}
