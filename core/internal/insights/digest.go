package insights

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

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
	b.WriteString(verdictHeader(sess))
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
func verdictHeader(sess telemetry.Session) string {
	top := []string{taskCell(sess.TaskType), verdictOutcome(sess)}
	if d := humanizeDuration(sess.DurationMS); d != "" {
		top = append(top, d)
	}
	line1 := "Session " + ui.Hint(shortID(sess.SessionID)) + " · " + strings.Join(top, " · ")

	line2 := fmt.Sprintf("%d calls · %d files", sess.ToolCalls, sess.FilesTouched)
	if f := flagsCell(sess); f != "" {
		line2 += " · " + f
	}
	return line1 + "\n" + line2
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
		lines = append(lines, highlightLine(ui.Bad("✗"), "failures", failureText(fails, sess.ToolCalls)))
	}
	if slowest.row.DurMS >= slowThresholdMS {
		lines = append(lines, highlightLine(ui.Hint("⏱"), "slowest",
			fmt.Sprintf("%s · %s · turn %d", describeRow(slowest.row), humanizeDuration(slowest.row.DurMS), slowest.turn)))
	}
	return lines
}

const slowThresholdMS = 5000

func failureText(fails []rowRef, total int) string {
	text := fmt.Sprintf("%d of %d calls failed", len(fails), total)
	verbs := map[string]struct{}{}
	for _, f := range fails {
		if f.row.Verb != "" {
			verbs[strings.TrimSpace(f.row.Verb+" "+f.row.Detail)] = struct{}{}
		}
	}
	if len(verbs) == 1 {
		for v := range verbs {
			text += " — " + v
		}
	}
	return text
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

// timelineLines renders one aligned line per turn: its index, phase, call count
// and duration, then a dimmed activity breakdown. Call counts are right-aligned
// and the stats column padded to a common width so the breakdowns line up — this
// is the backbone the Highlights hang off, each naming the turn a reader jumps to.
func timelineLines(turns []traceTurn) []string {
	phaseW, callW, statW := 0, 1, 0
	stats := make([]string, len(turns))
	for _, t := range turns {
		if w := len(t.Phase); w > phaseW {
			phaseW = w
		}
		if w := len(fmt.Sprintf("%d", t.Calls)); w > callW {
			callW = w
		}
	}
	for i, t := range turns {
		s := fmt.Sprintf("%*d calls", callW, t.Calls)
		if d := humanizeDuration(t.DurMS); d != "" {
			s += " · " + d
		}
		stats[i] = s
		if w := utf8.RuneCountInString(s); w > statW {
			statW = w
		}
	}
	lines := make([]string, 0, len(turns))
	for i, t := range turns {
		line := fmt.Sprintf("  %-7s  %-*s  ", fmt.Sprintf("turn %d", t.Index), phaseW, t.Phase)
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
// 34 read · 12 subagents", keeping the busiest few categories. Skill rows are
// omitted (they live in Highlights) and collapsed runs count their folded rows.
func turnBreakdown(t traceTurn) string {
	counts := map[string]int{}
	var order []string
	for _, r := range t.Rows {
		if r.Verb == "" || r.Skill {
			continue
		}
		n := 1
		if r.CollapsedN > 0 {
			n = r.CollapsedN
		}
		label := bucketLabel(r.Verb)
		if _, ok := counts[label]; !ok {
			order = append(order, label)
		}
		counts[label] += n
	}
	sort.SliceStable(order, func(i, j int) bool { return counts[order[i]] > counts[order[j]] })
	if len(order) > maxBreakdownParts {
		order = order[:maxBreakdownParts]
	}
	parts := make([]string, len(order))
	for i, l := range order {
		parts[i] = fmt.Sprintf("%d %s", counts[l], l)
	}
	return strings.Join(parts, " · ")
}

func bucketLabel(verb string) string {
	switch verb {
	case "subagent":
		return "subagents"
	default:
		return strings.ToLower(verb)
	}
}
