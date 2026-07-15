package insights

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/Hypership-Software/aftcast/internal/analytics"
	"github.com/Hypership-Software/aftcast/internal/telemetry"
	"github.com/Hypership-Software/aftcast/internal/ui"
)

func buildProjectColumns(projects []projectSummary, now time.Time) []tableColumn {
	titles := []string{"Project", "Active", "Sessions", "Shipped", "Duration", "Changes"}
	floors := []int{10, 0, 0, 0, 0, 12}
	columns := make([]tableColumn, len(titles))
	for i := range titles {
		columns[i] = tableColumn{title: titles[i], floor: floors[i], cells: make([]string, len(projects))}
	}
	for row, project := range projects {
		cells := []string{
			project.Name,
			humanize(project.LastStarted, now),
			strconv.Itoa(len(project.Sessions)),
			projectShippedCell(project),
			humanizeDuration(project.DurationMS),
			projectChangesCell(project),
		}
		for column := range columns {
			columns[column].cells[row] = cells[column]
		}
	}
	return columns
}

func buildProjectSessionColumns(sessions []telemetry.Session, now time.Time) []tableColumn {
	titles := []string{"When", "Task", "Result", "Duration", "Changes", "Delivery"}
	floors := []int{0, 0, 0, 0, 12, 0}
	columns := make([]tableColumn, len(titles))
	for i := range titles {
		columns[i] = tableColumn{title: titles[i], floor: floors[i], cells: make([]string, len(sessions))}
	}
	for row, session := range sessions {
		cells := []string{
			humanize(session.Started, now),
			taskCell(session.TaskType),
			resultCell(session),
			humanizeDuration(session.DurationMS),
			sessionChangesCell(session),
			deliveryCell(session),
		}
		for column := range columns {
			columns[column].cells[row] = cells[column]
		}
	}
	return columns
}

func renderProjects(agg aggregates, coach analytics.PlanAssociation, friction []analytics.FrictionCluster, tableView string) string {
	sections := []string{
		renderHeader(agg),
		"",
		renderCoach(coach),
		"",
	}
	if card := renderFriction(friction); card != "" {
		sections = append(sections, card, "")
	}
	sections = append(sections,
		ui.Bold("Projects"),
		tableView,
		ui.Hint("↑↓ move · ↵ open · tab security · g/p scope · ? help · q quit"),
	)
	return strings.Join(sections, "\n")
}

func renderProjectWorkspace(project projectSummary, tableView string) string {
	return strings.Join([]string{
		ui.Bold(project.Name + " · last 7 days"),
		"",
		renderProjectShipped(project),
		renderProjectDuration(project),
		renderProjectChanges(project),
		renderProjectWorkMix(project),
		renderCorrections(project.Aggregate.profile),
		renderProjectSecurity(project),
		"",
		ui.Bold("Recent work"),
		tableView,
		ui.Hint("↑↓ move · ↵ open · esc projects · g all projects · ? help · q quit"),
	}, "\n")
}

func renderProjectShipped(project projectSummary) string {
	if project.Shipping.Eligible > 0 {
		return fmt.Sprintf("%s %d of %d sessions (%.0f%%)", metricLabel("Shipped"),
			project.Shipping.Shipped, project.Shipping.Eligible, project.Shipping.Rate*100)
	}
	if !project.Shipping.TrackingSince.IsZero() {
		return fmt.Sprintf("%s Tracking since %s · waiting for first delivery session", metricLabel("Shipped"), project.Shipping.TrackingSince.Format("2 Jan"))
	}
	return fmt.Sprintf("%s Starts with your next captured session", metricLabel("Shipped"))
}

func renderProjectDuration(project projectSummary) string {
	parts := []string{}
	if duration := humanizeDuration(project.DurationMS); duration != "" {
		parts = append(parts, duration+" wall")
	}
	if duration := humanizeDuration(project.ObservedToolMS); duration != "" {
		parts = append(parts, duration+" observed tool time")
	}
	if len(parts) == 0 {
		return fmt.Sprintf("%s not captured", metricLabel("Duration"))
	}
	return fmt.Sprintf("%s %s", metricLabel("Duration"), strings.Join(parts, " · "))
}

func renderProjectChanges(project projectSummary) string {
	files := countNoun(project.FilesChanged, "file", "files")
	if !project.ChangeStatsCovered {
		return fmt.Sprintf("%s %s", metricLabel("Changes"), files)
	}
	return fmt.Sprintf("%s %s · +%s / −%s observed", metricLabel("Changes"), files,
		formatNumber(project.LinesAdded), formatNumber(project.LinesRemoved))
}

func renderProjectWorkMix(project projectSummary) string {
	if !project.WorkMixCovered {
		if project.PlanMS+project.BuildMS+project.ReviewMS == 0 || project.WorkMixSince.IsZero() {
			return fmt.Sprintf("%s Starts with your next captured session", metricLabel("Work mix"))
		}
	}
	plan, build, review := workPercentages(project.PlanMS, project.BuildMS, project.ReviewMS)
	value := fmt.Sprintf("Plan %d%% · Build %d%% · Review %d%%", plan, build, review)
	if !project.WorkMixCovered && !project.WorkMixSince.IsZero() {
		value += " · since " + project.WorkMixSince.Format("2 Jan")
	}
	return fmt.Sprintf("%s %s", metricLabel("Work mix"), value)
}

func renderProjectSecurity(project projectSummary) string {
	if project.Aggregate.securitySessions == 0 && project.Aggregate.danger == 0 {
		return fmt.Sprintf("%s nothing flagged", metricLabel("Security"))
	}
	return fmt.Sprintf("%s %s · %s", metricLabel("Security"),
		countNoun(project.Aggregate.securitySessions, "session", "sessions"),
		countNoun(project.Aggregate.danger, "flagged action", "flagged actions"))
}

func projectChangesCell(project projectSummary) string {
	if project.ChangeStatsCovered {
		return "+" + formatCompactNumber(project.LinesAdded) + "/−" + formatCompactNumber(project.LinesRemoved)
	}
	return countNoun(project.FilesChanged, "file", "files")
}

func sessionChangesCell(session telemetry.Session) string {
	if session.ChangeStatsCovered {
		return "+" + formatCompactNumber(session.LinesAdded) + "/−" + formatCompactNumber(session.LinesRemoved)
	}
	return countNoun(session.FilesChanged, "file", "files")
}

func resultCell(session telemetry.Session) string {
	switch analytics.OutcomeClass(session.Outcome) {
	case analytics.Success:
		return ui.OK("✓ succeeded")
	case analytics.Failure:
		return ui.Bad("✗ failed")
	default:
		return ui.Hint("—")
	}
}

func deliveryCell(session telemetry.Session) string {
	if session.Shipped {
		return ui.OK("pushed")
	}
	return ui.Hint("not pushed")
}

func workPercentages(plan, build, review int64) (int, int, int) {
	total := plan + build + review
	if total <= 0 {
		return 0, 0, 0
	}
	planPercent := int(math.Round(float64(plan) * 100 / float64(total)))
	buildPercent := int(math.Round(float64(build) * 100 / float64(total)))
	return planPercent, buildPercent, 100 - planPercent - buildPercent
}

func formatCompactNumber(value int) string {
	if value < 1000 {
		return strconv.Itoa(value)
	}
	if value%1000 == 0 {
		return fmt.Sprintf("%dk", value/1000)
	}
	return fmt.Sprintf("%.1fk", float64(value)/1000)
}

func formatNumber(value int) string {
	raw := strconv.Itoa(value)
	for i := len(raw) - 3; i > 0; i -= 3 {
		raw = raw[:i] + "," + raw[i:]
	}
	return raw
}
