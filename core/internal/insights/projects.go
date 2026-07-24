package insights

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Hypership-Software/aftcast/internal/analytics"
	"github.com/Hypership-Software/aftcast/internal/telemetry"
)

type projectSummary struct {
	Key                string
	ID                 string
	Name               string
	Sessions           []telemetry.Session
	LastStarted        string
	DurationMS         int64
	ObservedToolMS     int64
	ChangedFiles       []string
	FilesChanged       int
	LinesAdded         int
	LinesRemoved       int
	ChangeStatsCovered bool
	PlanMS             int64
	BuildMS            int64
	ReviewMS           int64
	WorkMixCovered     bool
	WorkMixSince       time.Time
	Shipping           analytics.ShippedProfile
	Aggregate          aggregates
}

func groupProjects(sessions []telemetry.Session, scope Scope, now time.Time) []projectSummary {
	names := repositoryNamesByProjectID(sessions)
	groups := make(map[string][]telemetry.Session)
	for _, session := range sessions {
		key := projectGroupKey(session, names)
		groups[key] = append(groups[key], session)
	}

	projects := make([]projectSummary, 0, len(groups))
	for key, group := range groups {
		projects = append(projects, summarizeProject(key, group, scope, now))
	}
	sort.SliceStable(projects, func(i, j int) bool {
		left, right := timestampValue(projects[i].LastStarted), timestampValue(projects[j].LastStarted)
		if left != right {
			return left > right
		}
		if projects[i].Name != projects[j].Name {
			return projects[i].Name < projects[j].Name
		}
		return projects[i].Key < projects[j].Key
	})
	return projects
}

func summarizeProject(key string, sessions []telemetry.Session, scope Scope, now time.Time) projectSummary {
	ordered := append([]telemetry.Session(nil), sessions...)
	sortSessions(ordered, sortRecent)
	out := projectSummary{
		Key:         key,
		Sessions:    ordered,
		LastStarted: newestStarted(ordered),
		Aggregate:   aggregate(ordered, now),
	}
	if len(ordered) > 0 {
		out.ID = ordered[0].ProjectID
		out.Name = projectDisplayName(ordered, scope)
	}

	stats := make([]analytics.SessionStat, len(ordered))
	files := make(map[string]struct{})
	fileListsComplete := true
	changeCovered := true
	hasChanges := false
	mixCovered := true
	hasMix := false
	for i, session := range ordered {
		stats[i] = toStat(session)
		out.DurationMS += session.DurationMS
		out.ObservedToolMS += session.ObservedToolMS
		out.LinesAdded += session.LinesAdded
		out.LinesRemoved += session.LinesRemoved
		out.PlanMS += session.PlanMS
		out.BuildMS += session.BuildMS
		out.ReviewMS += session.ReviewMS

		if session.FilesChanged > 0 {
			hasChanges = true
			if !session.ChangeStatsCovered {
				changeCovered = false
			}
			if len(session.ChangedFiles) == 0 {
				fileListsComplete = false
			}
		}
		for _, file := range session.ChangedFiles {
			if file != "" {
				files[file] = struct{}{}
			}
		}
		if session.WorkMixCovered {
			hasMix = true
			started, err := time.Parse(time.RFC3339Nano, session.Started)
			if err == nil && (out.WorkMixSince.IsZero() || started.Before(out.WorkMixSince)) {
				out.WorkMixSince = started
			}
		} else {
			mixCovered = false
		}
	}
	out.Shipping = analytics.ShippingProfile(stats)
	out.ChangeStatsCovered = hasChanges && changeCovered
	out.WorkMixCovered = hasMix && mixCovered
	out.ChangedFiles = make([]string, 0, len(files))
	for file := range files {
		out.ChangedFiles = append(out.ChangedFiles, file)
	}
	sort.Strings(out.ChangedFiles)
	if fileListsComplete {
		out.FilesChanged = len(out.ChangedFiles)
	} else {
		for _, session := range ordered {
			out.FilesChanged += session.FilesChanged
		}
	}
	return out
}

func projectGroupKey(session telemetry.Session, names map[string]string) string {
	switch {
	case session.ProjectName != "":
		// Historical sessions can carry different project IDs for the same
		// repository as capture moved from path to remote-backed identity. The
		// local, proven repository name is the stable developer-facing join key.
		return "name:" + strings.ToLower(strings.TrimSpace(session.ProjectName))
	case names[session.ProjectID] != "":
		return "name:" + names[session.ProjectID]
	case session.ProjectID != "":
		return "id:" + session.ProjectID
	default:
		return "other"
	}
}

// A session that never touched a file has no path evidence to resolve a
// repository name from, so it reaches here carrying only its project id. Keying
// it on that id alone would split one repository into two cards; the named
// sessions sharing its id already prove the name. A session that edits files
// outside the repository it started in names that other repository instead, so
// the id can carry several names — the majority of its sessions wins.
func repositoryNamesByProjectID(sessions []telemetry.Session) map[string]string {
	counts := make(map[string]map[string]int)
	for _, session := range sessions {
		name := strings.ToLower(strings.TrimSpace(session.ProjectName))
		if session.ProjectID == "" || name == "" {
			continue
		}
		if counts[session.ProjectID] == nil {
			counts[session.ProjectID] = make(map[string]int)
		}
		counts[session.ProjectID][name]++
	}

	names := make(map[string]string, len(counts))
	for id, byName := range counts {
		best, bestCount := "", 0
		for name, count := range byName {
			if count > bestCount || (count == bestCount && (best == "" || name < best)) {
				best, bestCount = name, count
			}
		}
		names[id] = best
	}
	return names
}

func projectDisplayName(sessions []telemetry.Session, scope Scope) string {
	for _, session := range sessions {
		if session.ProjectName != "" {
			return session.ProjectName
		}
	}
	if len(sessions) > 0 && sessions[0].ProjectID == scope.ProjectID && scope.Name != "" {
		return scope.Name
	}
	return "other project"
}

func newestStarted(sessions []telemetry.Session) string {
	if len(sessions) == 0 {
		return ""
	}
	newest := sessions[0].Started
	for _, session := range sessions[1:] {
		if timestampValue(session.Started) > timestampValue(newest) {
			newest = session.Started
		}
	}
	return newest
}

func timestampValue(value string) int64 {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return 0
	}
	return parsed.UnixNano()
}

func projectShippedCell(project projectSummary) string {
	if project.Shipping.Eligible == 0 {
		return "—"
	}
	return fmt.Sprintf("%.0f%%", project.Shipping.Rate*100)
}
