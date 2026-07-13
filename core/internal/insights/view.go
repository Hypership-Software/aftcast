package insights

import (
	"encoding/json"
	"fmt"
	"strings"

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

func renderHeader(agg aggregates) string {
	p := agg.profile
	danger := fmt.Sprintf("danger %d", agg.danger)
	if agg.danger > 0 {
		danger = ui.Bad(danger)
	}
	return fmt.Sprintf("%s   corr/deliv %.1f   sessions %d   %s   trend %+.0f%%",
		ui.Bold(fmt.Sprintf("clean %.0f%%", p.CleanDeliveryRate*100)),
		p.CorrectionLoad, p.Sessions, danger, p.Trend*100)
}

func renderTaskMix(mix []taskCount) string {
	var b strings.Builder
	for _, tc := range mix {
		n := tc.n
		if n > maxBar {
			n = maxBar
		}
		fmt.Fprintf(&b, "%-12s %s %d\n", tc.task, strings.Repeat("█", n), tc.n)
	}
	return strings.TrimRight(b.String(), "\n")
}

func renderOpportunities(sk analytics.SkillReport) string {
	if len(sk.Opportunities) == 0 {
		return ui.Hint("no skill opportunities")
	}
	return "skill opportunities: " + strings.Join(sk.Opportunities, ", ")
}

func renderEmpty() string {
	return "No sessions captured yet — run a Claude Code session with the gate active.\n" +
		ui.Hint("If you expected data, check `gated status` for a capture gap.")
}

func renderList(agg aggregates, tableView string) string {
	return strings.Join([]string{
		renderHeader(agg),
		"",
		renderTaskMix(agg.taskMix),
		"",
		renderOpportunities(agg.skills),
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
