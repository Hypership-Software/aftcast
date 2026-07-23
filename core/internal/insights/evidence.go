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
	repoMin := make(map[string]time.Time) // minimum Started per repo for ordering
	sessionPairs := make(map[string][]sessionTime)

	for _, s := range sessions {
		started, err := time.Parse(time.RFC3339Nano, s.Started)
		if err != nil {
			continue
		}

		if started.Before(since) || started.After(now) {
			continue
		}

		repo := s.ProjectName
		if repo == "" {
			repo = "other work"
		}

		if _, exists := repoMap[repo]; !exists {
			repoMap[repo] = &RepoEvidence{Repo: repo}
			repoMin[repo] = started
		} else if started.Before(repoMin[repo]) {
			repoMin[repo] = started
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

	repoNames := make([]string, 0, len(repoMap))
	for repo := range repoMap {
		repoNames = append(repoNames, repo)
	}
	sort.Slice(repoNames, func(i, j int) bool {
		if repoMin[repoNames[i]].Equal(repoMin[repoNames[j]]) {
			return repoNames[i] < repoNames[j]
		}
		return repoMin[repoNames[i]].Before(repoMin[repoNames[j]])
	})

	result := make([]RepoEvidence, len(repoNames))
	for i, repo := range repoNames {
		result[i] = *repoMap[repo]
	}

	return result
}

type sessionTime struct {
	started time.Time
	id      string
}
