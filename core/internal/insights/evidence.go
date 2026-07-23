package insights

import (
	"sort"
	"time"

	"github.com/Hypership-Software/aftcast/internal/telemetry"
)

type RepoEvidence struct {
	Repo         string
	SessionIDs   []string
	Sessions     int
	Shipped      int
	Corrections  int
	Danger       int
	Tainted      int
	FilesChanged int
}

func EvidenceRows(sessions []telemetry.Session, since, now time.Time) []RepoEvidence {
	repoMap := make(map[string]*RepoEvidence)
	repoOrder := []string{}                        // track first appearance order
	sessionPairs := make(map[string][]sessionTime) // (Started, SessionID) pairs per repo

	for _, s := range sessions {
		started, err := time.Parse(time.RFC3339Nano, s.Started)
		if err != nil {
			continue // exclude unparseable
		}

		if started.Before(since) || started.After(now) {
			continue // exclude out-of-window
		}

		repo := s.ProjectName
		if repo == "" {
			repo = "other work"
		}

		if _, exists := repoMap[repo]; !exists {
			repoMap[repo] = &RepoEvidence{
				Repo:       repo,
				SessionIDs: []string{},
			}
			repoOrder = append(repoOrder, repo)
		}

		evidence := repoMap[repo]
		evidence.Sessions++
		if s.Shipped {
			evidence.Shipped++
		}
		evidence.Corrections += s.CorrectionTurns
		evidence.Danger += s.DangerDetected
		if s.Taint {
			evidence.Tainted++
		}
		evidence.FilesChanged += s.FilesChanged

		sessionPairs[repo] = append(sessionPairs[repo], sessionTime{
			started: started,
			id:      s.SessionID,
		})
	}

	// Sort SessionIDs per repo by Started time
	for repo := range sessionPairs {
		pairs := sessionPairs[repo]
		sort.Slice(pairs, func(i, j int) bool {
			return pairs[i].started.Before(pairs[j].started)
		})
		ids := make([]string, len(pairs))
		for i, p := range pairs {
			ids[i] = p.id
		}
		repoMap[repo].SessionIDs = ids
	}

	// Build result in first-appearance order
	result := make([]RepoEvidence, len(repoOrder))
	for i, repo := range repoOrder {
		result[i] = *repoMap[repo]
	}

	return result
}

type sessionTime struct {
	started time.Time
	id      string
}
