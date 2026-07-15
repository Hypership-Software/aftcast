// Package insights renders the telemetry read-model as an interactive terminal
// dashboard. It is a pure consumer of internal/telemetry and internal/analytics;
// nothing here observes or decides — it only shows what was captured.
package insights

import (
	"sort"
	"strings"
	"time"

	"github.com/Hypership-Software/atlas/internal/analytics"
	"github.com/Hypership-Software/atlas/internal/telemetry"
)

const (
	recentWindow    = 7 * 24 * time.Hour
	coachWindowSize = 60
)

type aggregates struct {
	profile          analytics.Profile
	shipping         analytics.ShippedProfile
	danger           int
	securitySessions int
	projects         int
	user             string
	scopeLabel       string
}

// recentSessions keeps sessions within the last 7 days of Started. A session
// whose Started is empty or fails to parse is kept rather than hidden: an
// observability tool must not silently drop data it can't time-place.
func recentSessions(sessions []telemetry.Session, now time.Time) []telemetry.Session {
	var out []telemetry.Session
	for _, s := range sessions {
		t, err := time.Parse(time.RFC3339Nano, s.Started)
		if err != nil {
			out = append(out, s)
			continue
		}
		if now.Sub(t) <= recentWindow {
			out = append(out, s)
		}
	}
	return out
}

func coachWindow(sessions []telemetry.Session) []analytics.SessionStat {
	ordered := append([]telemetry.Session(nil), sessions...)
	sort.SliceStable(ordered, func(i, j int) bool { return startedUnixNano(ordered[i]) > startedUnixNano(ordered[j]) })
	out := make([]analytics.SessionStat, 0, coachWindowSize)
	for _, session := range ordered {
		stat := toStat(session)
		if !analytics.DeliveryEligible(stat) || (stat.PlanStyle != analytics.PlanFirst && stat.PlanStyle != analytics.PlanDirect) {
			continue
		}
		out = append(out, stat)
		if len(out) == coachWindowSize {
			break
		}
	}
	return out
}

func aggregate(sessions []telemetry.Session, now time.Time) aggregates {
	stats := make([]analytics.SessionStat, len(sessions))
	projects := map[string]struct{}{}
	danger := 0
	securitySessions := 0
	user := ""
	for i, s := range sessions {
		stats[i] = toStat(s)
		danger += s.DangerDetected
		if s.Taint || s.DangerDetected > 0 {
			securitySessions++
		}
		if user == "" && s.User != "" {
			user = s.User
		}
		projectKey := "other"
		switch {
		case s.ProjectName != "":
			projectKey = "name:" + s.ProjectName
		case s.ProjectID != "":
			projectKey = "id:" + s.ProjectID
		}
		projects[projectKey] = struct{}{}
	}
	return aggregates{
		profile:          analytics.Productivity(stats),
		shipping:         analytics.ShippingProfile(stats),
		danger:           danger,
		securitySessions: securitySessions,
		projects:         len(projects),
		user:             user,
	}
}

func toStat(s telemetry.Session) analytics.SessionStat {
	return analytics.SessionStat{
		Started:         s.Started,
		Outcome:         analytics.OutcomeClass(s.Outcome),
		CleanDelivery:   s.CleanDelivery,
		CaptureVersion:  s.CaptureVersion,
		PlanStyle:       analytics.PlanStyle(s.PlanStyle),
		FilesChanged:    s.FilesChanged,
		Shipped:         s.Shipped,
		CorrectionTurns: s.CorrectionTurns,
		TurnCount:       s.TurnCount,
		ToolCalls:       s.ToolCalls,
		TaskType:        s.TaskType,
		Skills:          splitSkills(s.SkillsUsed),
		Tainted:         s.Taint,
	}
}

func splitSkills(csv string) []string {
	if csv == "" {
		return nil
	}
	return strings.Split(csv, ",")
}
