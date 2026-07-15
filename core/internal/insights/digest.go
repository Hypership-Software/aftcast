package insights

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/Hypership-Software/aftcast/internal/analytics"
	"github.com/Hypership-Software/aftcast/internal/schema"
	"github.com/Hypership-Software/aftcast/internal/telemetry"
	"github.com/Hypership-Software/aftcast/internal/ui"
)

// renderTrace is the developer-facing session summary. It leads with the work
// that happened — time, changes, work mix, and files — while keeping the full
// event sequence behind `r` as raw JSON.
func renderTrace(sess telemetry.Session, evs []schema.TelemetryEvent) string {
	turns := buildTrace(evs)
	sections := []string{verdictHeader(sess, turns)}
	sections = append(sections, ui.Bold("Work mix")+"\n"+renderSessionWorkMix(evs))
	if hs := sessionHighlightLines(sess, turns); len(hs) > 0 {
		sections = append(sections, ui.Bold("Highlights")+"\n"+strings.Join(hs, "\n"))
	}
	if files := renderChangedFiles(sess, evs); files != "" {
		sections = append(sections, ui.Bold("Files changed")+"\n"+files)
	}
	return strings.Join(sections, "\n\n")
}

// verdictHeader deliberately omits tool-call and turn counts. Those are capture
// implementation details; developers need elapsed time and the code changed.
func verdictHeader(sess telemetry.Session, turns []traceTurn) string {
	projectName := sess.ProjectName
	if projectName == "" {
		projectName = "other project"
	}
	identity := []string{projectName, taskCell(sess.TaskType), detailOutcome(sess)}
	if sess.Shipped {
		identity = append(identity, "pushed")
	}
	line1 := strings.Join(identity, " · ")

	timing := []string{"Session " + ui.Hint(shortID(sess.SessionID))}
	if d := humanizeDuration(sess.DurationMS); d != "" {
		timing = append(timing, "wall span "+d)
	}
	toolMS := observedToolTime(turns)
	if toolMS == 0 {
		toolMS = sess.ObservedToolMS
	}
	if d := humanizeDuration(toolMS); d != "" {
		timing = append(timing, "observed tool time "+d)
	}

	counts := []string{countNoun(sess.FilesChanged, "file changed", "files changed")}
	if sess.ChangeStatsCovered {
		counts = append(counts, fmt.Sprintf("+%s / −%s observed", formatNumber(sess.LinesAdded), formatNumber(sess.LinesRemoved)))
	}
	if n := len(splitSkills(sess.SkillsUsed)); n > 0 {
		counts = append(counts, countNoun(n, "invoked skill", "invoked skills"))
	}
	return line1 + "\n" + strings.Join(timing, " · ") + "\n" + strings.Join(counts, " · ")
}

func detailOutcome(sess telemetry.Session) string {
	switch analytics.OutcomeClass(sess.Outcome) {
	case analytics.Success:
		return "succeeded"
	case analytics.Failure:
		return "failed"
	default:
		return "outcome not captured"
	}
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

func sessionHighlightLines(sess telemetry.Session, turns []traceTurn) []string {
	var lines []string
	if sess.Shipped {
		lines = append(lines, highlightLine(ui.OK("↑"), "pushed", "successful git push"))
	}
	if skills := splitSkills(sess.SkillsUsed); len(skills) > 0 {
		label := "invoked skill"
		if len(skills) > 1 {
			label = "invoked skills"
		}
		lines = append(lines, "  ★  "+label+" "+strings.Join(prettySkills(skills), ", "))
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
			strings.TrimSpace("entered "+via)))
	}
	if len(flagged) > 0 {
		lines = append(lines, highlightLine(ui.Bad("⚑"), "flagged",
			fmt.Sprintf("%d — %s", len(flagged), describeRow(flagged[0].row))))
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
			fmt.Sprintf("%s · %s", describeRow(slowest.row), humanizeDuration(slowest.row.DurMS))))
	}
	return lines
}

func renderSessionWorkMix(evs []schema.TelemetryEvent) string {
	mix := analytics.ObservedWorkMix(evs)
	if !mix.Covered {
		return "  Not captured for this session"
	}
	plan, build, review := workPercentages(mix.Plan.DurationMS, mix.Build.DurationMS, mix.Review.DurationMS)
	return strings.Join([]string{
		workMixLine("Plan", mix.Plan, plan),
		workMixLine("Build", mix.Build, build),
		workMixLine("Review", mix.Review, review),
	}, "\n")
}

func workMixLine(label string, bucket analytics.WorkBucket, percent int) string {
	duration := humanizeDuration(bucket.DurationMS)
	if duration == "" {
		duration = "0s"
	}
	parts := []string{duration, fmt.Sprintf("%d%%", percent)}
	if len(bucket.Operations) > 0 {
		operations := make([]string, len(bucket.Operations))
		for i, operation := range bucket.Operations {
			operations[i] = string(operation)
		}
		parts = append(parts, strings.Join(operations, ", "))
	}
	return fmt.Sprintf("  %-6s %s", label, strings.Join(parts, " · "))
}

type renderedFileChange struct {
	path         string
	linesAdded   int
	linesRemoved int
}

func renderChangedFiles(sess telemetry.Session, evs []schema.TelemetryEvent) string {
	observed := analytics.ObservedChanges(evs)
	files := make([]renderedFileChange, 0, len(observed.Files)+len(sess.ChangedFiles))
	if len(observed.Files) > 0 {
		for _, file := range observed.Files {
			files = append(files, renderedFileChange{
				path: displayFilePath(file.Path), linesAdded: file.LinesAdded, linesRemoved: file.LinesRemoved,
			})
		}
	} else {
		for _, path := range sess.ChangedFiles {
			files = append(files, renderedFileChange{path: displayFilePath(path)})
		}
	}
	if len(files) == 0 {
		return ""
	}
	sort.SliceStable(files, func(i, j int) bool {
		left := files[i].linesAdded + files[i].linesRemoved
		right := files[j].linesAdded + files[j].linesRemoved
		if left != right {
			return left > right
		}
		return files[i].path < files[j].path
	})
	lines := make([]string, len(files))
	for i, file := range files {
		lines[i] = "  " + file.path
		if observed.Covered {
			lines[i] += fmt.Sprintf("  +%s / −%s", formatNumber(file.linesAdded), formatNumber(file.linesRemoved))
		}
	}
	return strings.Join(lines, "\n")
}

func displayFilePath(path string) string {
	clean := filepath.Clean(path)
	if clean == "." || clean == "" {
		return "unknown file"
	}
	if filepath.IsAbs(clean) {
		if relative, ok := repositoryRelativePath(clean); ok {
			return filepath.ToSlash(relative)
		}
		return shortenedPath(clean)
	}
	if strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return shortenedPath(clean)
	}
	return filepath.ToSlash(clean)
}

func repositoryRelativePath(path string) (string, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return "", false
	}
	dir := path
	if !info.IsDir() {
		dir = filepath.Dir(path)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			relative, err := filepath.Rel(dir, path)
			return relative, err == nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func shortenedPath(path string) string {
	clean := filepath.ToSlash(filepath.Clean(path))
	parts := strings.FieldsFunc(clean, func(r rune) bool { return r == '/' || r == '\\' })
	if len(parts) == 0 {
		return "unknown file"
	}
	const keep = 3
	if len(parts) <= keep && !filepath.IsAbs(path) {
		return strings.Join(parts, "/")
	}
	if len(parts) > keep {
		parts = parts[len(parts)-keep:]
	}
	return "…/" + strings.Join(parts, "/")
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
	sort.SliceStable(order, func(i, j int) bool {
		if counts[order[i]] != counts[order[j]] {
			return counts[order[i]] > counts[order[j]]
		}
		left, right := bucketLabel(order[i]), bucketLabel(order[j])
		if left != right {
			return left < right
		}
		return order[i] < order[j]
	})
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
