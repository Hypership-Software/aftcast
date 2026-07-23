// Package handoff assembles the digest skeleton for a git ref: deterministic
// facts and attestation computed from the record; narrative left as
// instructions + coordinates for the user's own agent (ADR-011 — content
// never enters Aftcast).
package handoff

import (
	"fmt"
	"os/exec"
	"strings"
)

// ResolveSHAs lists the ref's history as full SHAs, newest first. Captured
// commit_sha values are abbreviated, so callers match by prefix (MatchesAny).
func ResolveSHAs(repoDir, ref string, limit int) ([]string, error) {
	if ref == "" {
		ref = "HEAD"
	}
	cmd := exec.Command("git", "rev-list", fmt.Sprintf("--max-count=%d", limit), "--end-of-options", ref)
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("handoff: resolve %q: %w", ref, err)
	}
	var shas []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			shas = append(shas, line)
		}
	}
	return shas, nil
}

func MatchesAny(captured string, full []string) bool {
	if captured == "" {
		return false
	}
	for _, f := range full {
		if strings.HasPrefix(f, captured) {
			return true
		}
	}
	return false
}
