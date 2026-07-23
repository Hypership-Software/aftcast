package handoff

import (
	"github.com/Hypership-Software/aftcast/internal/schema"
	"github.com/Hypership-Software/aftcast/internal/telemetry"
)

type Source interface {
	Sessions() ([]telemetry.Session, error)
	EventsForSession(id string) ([]schema.TelemetryEvent, error)
}

type Selected struct {
	Session telemetry.Session
	Events  []schema.TelemetryEvent
	SHAs    []string
}

// SelectSessions returns the sessions whose captured commit SHAs prefix-match
// the ref's history — the deterministic artifact→conduct join. Sessions
// predating commit_sha capture never match; the renderer states that boundary
// rather than guessing.
func SelectSessions(src Source, fullSHAs []string) ([]Selected, error) {
	sessions, err := src.Sessions()
	if err != nil {
		return nil, err
	}
	var out []Selected
	for _, s := range sessions {
		evs, err := src.EventsForSession(s.SessionID)
		if err != nil {
			return nil, err
		}
		seen := map[string]bool{}
		var shas []string
		for _, e := range evs {
			if e.CommitSHA != "" && !seen[e.CommitSHA] && MatchesAny(e.CommitSHA, fullSHAs) {
				seen[e.CommitSHA] = true
				shas = append(shas, e.CommitSHA)
			}
		}
		if len(shas) > 0 {
			out = append(out, Selected{Session: s, Events: evs, SHAs: shas})
		}
	}
	return out, nil
}
