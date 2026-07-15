package analytics

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Hypership-Software/atlas/internal/schema"
)

const (
	frictionMinSessions = 3
	frictionMinDays     = 2
)

type SessionFailures struct {
	SessionID string
	Failures  int
	First     time.Time
}

// FrictionCluster is one recurring failure signature: the same kind of tool
// call failing the same way, counted across sessions. It carries only receipts
// (counts, sessions, dates) — never command content.
type FrictionCluster struct {
	ToolClass schema.ToolClass
	ToolName  string
	Verbs     []string
	ExitCode  int
	Failures  int
	Sessions  []SessionFailures
	Days      int
	Projects  int
	First     time.Time
	Last      time.Time
}

func (c FrictionCluster) Slug() string {
	base := c.ToolName
	if c.ToolClass == schema.ClassExec && len(c.Verbs) > 0 {
		base = strings.Join(c.Verbs, "-")
	}
	slug := sanitizeSlug(base)
	if c.ExitCode != 0 {
		slug += fmt.Sprintf("-exit-%d", c.ExitCode)
	}
	return slug
}

func sanitizeSlug(s string) string {
	var b strings.Builder
	lastDash := true
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}

func FrictionClusters(events []schema.TelemetryEvent) []FrictionCluster {
	type accumulator struct {
		cluster  FrictionCluster
		sessions map[string]*SessionFailures
		days     map[string]bool
		projects map[string]bool
	}
	byKey := map[string]*accumulator{}
	var order []string

	for _, e := range events {
		if e.EventType != schema.EventPostTool || e.ToolOK != schema.OutcomeFailed {
			continue
		}
		key := fmt.Sprintf("%s|%s|%s|%d", e.ToolClass, e.ToolRaw, strings.Join(e.Verbs, " "), e.BashExitCode)
		acc := byKey[key]
		if acc == nil {
			acc = &accumulator{
				cluster: FrictionCluster{
					ToolClass: e.ToolClass,
					ToolName:  e.ToolRaw,
					Verbs:     e.Verbs,
					ExitCode:  e.BashExitCode,
				},
				sessions: map[string]*SessionFailures{},
				days:     map[string]bool{},
				projects: map[string]bool{},
			}
			byKey[key] = acc
			order = append(order, key)
		}
		acc.cluster.Failures++
		if e.Project != "" {
			acc.projects[e.Project] = true
		}
		sess := acc.sessions[e.SessionID]
		if sess == nil {
			sess = &SessionFailures{SessionID: e.SessionID}
			acc.sessions[e.SessionID] = sess
		}
		sess.Failures++
		ts, err := time.Parse(time.RFC3339Nano, e.TS)
		if err != nil {
			continue
		}
		acc.days[ts.UTC().Format("2006-01-02")] = true
		if sess.First.IsZero() || ts.Before(sess.First) {
			sess.First = ts
		}
		if acc.cluster.First.IsZero() || ts.Before(acc.cluster.First) {
			acc.cluster.First = ts
		}
		if ts.After(acc.cluster.Last) {
			acc.cluster.Last = ts
		}
	}

	out := make([]FrictionCluster, 0, len(order))
	for _, key := range order {
		acc := byKey[key]
		acc.cluster.Days = len(acc.days)
		acc.cluster.Projects = len(acc.projects)
		acc.cluster.Sessions = sortedSessionFailures(acc.sessions)
		out = append(out, acc.cluster)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if len(out[i].Sessions) != len(out[j].Sessions) {
			return len(out[i].Sessions) > len(out[j].Sessions)
		}
		if out[i].Failures != out[j].Failures {
			return out[i].Failures > out[j].Failures
		}
		return out[i].Slug() < out[j].Slug()
	})
	return out
}

func sortedSessionFailures(sessions map[string]*SessionFailures) []SessionFailures {
	out := make([]SessionFailures, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, *s)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].First.Equal(out[j].First) {
			return out[i].First.Before(out[j].First)
		}
		return out[i].SessionID < out[j].SessionID
	})
	return out
}

func WorthFixing(clusters []FrictionCluster) []FrictionCluster {
	out := make([]FrictionCluster, 0, len(clusters))
	for _, c := range clusters {
		if len(c.Sessions) >= frictionMinSessions && c.Days >= frictionMinDays {
			out = append(out, c)
		}
	}
	return out
}
