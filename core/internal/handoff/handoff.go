package handoff

import (
	"github.com/Hypership-Software/aftcast/internal/svc"
)

// Run assembles the digest skeleton for a ref: resolve, join, gather, verify,
// render. A ref with no joined sessions still renders — the skeleton's honesty
// lines are the answer, not an error.
func Run(home, repoDir, ref string) ([]byte, error) {
	shas, err := ResolveSHAs(repoDir, ref, 200)
	if err != nil {
		return nil, err
	}
	store, err := svc.OpenReadModel(home)
	if err != nil {
		return nil, err
	}
	defer store.Close()
	selected, err := SelectSessions(store, shas)
	if err != nil {
		return nil, err
	}
	rep, err := svc.VerifyLog(home)
	if err != nil {
		return nil, err
	}
	name := ref
	if name == "" {
		name = "HEAD"
	}
	return Render(name, GatherFacts(selected), rep), nil
}
