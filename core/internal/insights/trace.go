package insights

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Hypership-Software/atlas/internal/schema"
)

type traceRow struct {
	Verb       string
	Detail     string
	DurMS      int64
	Outcome    schema.ToolOutcome
	Skill      bool
	Failed     bool
	Danger     bool
	Untrusted  bool
	Shipped    bool
	CollapsedN int
	Subagent   string
}

type traceTurn struct {
	Index int
	Calls int
	DurMS int64
	Rows  []traceRow
}

const (
	lowSignalGlob = "glob"
	lowSignalGrep = "grep"
)

// buildTrace mirrors analytics.splitTurns instead of importing it: that
// function is unexported, and duplicating ~10 lines here is cheaper than
// exporting a segmentation helper solely for this consumer.
func buildTrace(evs []schema.TelemetryEvent) []traceTurn {
	segs := splitTurns(evs)
	turns := make([]traceTurn, 0, len(segs))
	// untrustedSeen lives outside the turn loop: the marker is the session's
	// first untrusted-input entry point, not one per turn.
	untrustedSeen := false
	for i, seg := range segs {
		rows, calls := buildRows(seg, &untrustedSeen)
		rows = collapseRuns(rows)
		turns = append(turns, traceTurn{
			Index: i + 1,
			Calls: calls,
			DurMS: sumDur(rows),
			Rows:  rows,
		})
	}
	return turns
}

func splitTurns(evs []schema.TelemetryEvent) [][]schema.TelemetryEvent {
	var segs [][]schema.TelemetryEvent
	var cur []schema.TelemetryEvent
	seen := false
	for _, e := range evs {
		if e.EventType == schema.EventUserPrompt {
			if seen {
				segs = append(segs, cur)
				cur = nil
			}
			seen = true
		}
		cur = append(cur, e)
	}
	if len(cur) > 0 {
		segs = append(segs, cur)
	}
	return segs
}

func buildRows(seg []schema.TelemetryEvent, untrustedSeen *bool) ([]traceRow, int) {
	posts := make(map[string]schema.TelemetryEvent)
	for _, e := range seg {
		if e.EventType == schema.EventPostTool && e.ToolUseID != "" {
			posts[e.ToolUseID] = e
		}
	}
	consumed := make(map[string]bool)
	var rows []traceRow
	calls := 0
	for _, e := range seg {
		switch e.EventType {
		case schema.EventPreTool:
			calls++
			row := buildRow(e, untrustedSeen)
			if post, ok := posts[e.ToolUseID]; e.ToolUseID != "" && ok {
				row.DurMS = post.LatencyMS
				row.Outcome = post.ToolOK
				row.Failed = post.ToolOK == schema.OutcomeFailed
				row.Shipped = post.DeliverySignal == schema.DeliveryGitPush
				consumed[e.ToolUseID] = true
			}
			rows = append(rows, row)
		case schema.EventPostTool:
			if e.ToolUseID != "" && consumed[e.ToolUseID] {
				continue
			}
			rows = append(rows, traceRow{
				Outcome: e.ToolOK,
				Failed:  e.ToolOK == schema.OutcomeFailed,
				Danger:  e.Risk == schema.RiskDanger,
				Shipped: e.DeliverySignal == schema.DeliveryGitPush,
			})
		}
	}
	return rows, calls
}

func buildRow(e schema.TelemetryEvent, untrustedSeen *bool) traceRow {
	row := traceRow{
		Skill:    e.ToolClass == schema.ClassSkill,
		Danger:   e.Risk == schema.RiskDanger,
		Subagent: e.Subagent,
	}
	switch e.ToolClass {
	case schema.ClassExec:
		row.Verb = "ran"
		if len(e.Verbs) > 0 {
			row.Detail = e.Verbs[0]
		}
	case schema.ClassFileWrite:
		row.Verb = "edited"
		if len(e.Files) > 0 {
			row.Detail = filepath.Base(e.Files[0])
		}
	case schema.ClassFileRead:
		row.Verb = "read"
		if len(e.Files) > 0 {
			row.Detail = filepath.Base(e.Files[0])
		}
	case schema.ClassNetFetch:
		row.Verb = "fetched"
		row.Detail = e.Domain
	case schema.ClassNetSearch:
		row.Verb = "searched"
		row.Detail = e.Domain
	case schema.ClassSkill:
		row.Verb = "skill"
		row.Detail = e.Skill
	case schema.ClassAgentSpawn:
		row.Verb = "subagent"
		if e.Subagent != "" {
			row.Detail = e.Subagent
		} else {
			row.Detail = e.ToolRaw
		}
	case schema.ClassMCP:
		row.Verb = "mcp"
		if e.Command != "" {
			row.Detail = e.Command
		} else {
			row.Detail = e.ToolRaw
		}
	default:
		row.Verb = e.ToolRaw
	}

	if isUntrustedClass(e.ToolClass) && !*untrustedSeen {
		row.Untrusted = true
		*untrustedSeen = true
	}
	return row
}

func isUntrustedClass(c schema.ToolClass) bool {
	return c == schema.ClassNetFetch || c == schema.ClassNetSearch || c == schema.ClassMCP
}

// isLowSignal decides collapse eligibility. A danger/failed/untrusted row is
// never low-signal: collapsing it would erase the ⚑/✗/⚠ signal Atlas exists to
// surface, so it stays its own expanded row and breaks any surrounding run.
func isLowSignal(r traceRow) bool {
	if r.Danger || r.Failed || r.Untrusted {
		return false
	}
	if r.Verb == "read" {
		return true
	}
	return strings.EqualFold(r.Verb, lowSignalGlob) || strings.EqualFold(r.Verb, lowSignalGrep)
}

func collapseRuns(rows []traceRow) []traceRow {
	var out []traceRow
	i := 0
	for i < len(rows) {
		if !isLowSignal(rows[i]) {
			out = append(out, rows[i])
			i++
			continue
		}
		j := i + 1
		for j < len(rows) && rows[j].Verb == rows[i].Verb && isLowSignal(rows[j]) {
			j++
		}
		runLen := j - i
		if runLen >= 3 {
			out = append(out, traceRow{
				Verb:       rows[i].Verb,
				Detail:     fmt.Sprintf("×%d files", runLen),
				DurMS:      sumDur(rows[i:j]),
				CollapsedN: runLen,
			})
		} else {
			out = append(out, rows[i:j]...)
		}
		i = j
	}
	return out
}

func sumDur(rows []traceRow) int64 {
	var total int64
	for _, r := range rows {
		total += r.DurMS
	}
	return total
}
