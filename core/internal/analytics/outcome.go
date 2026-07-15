package analytics

import "github.com/Hypership-Software/aftcast/internal/schema"

// completionVerbs are exec first-tokens that stand in for verified progress. The
// persisted event carries only the verb (no argv — ADR keeps argv out of the log),
// so this is a deliberately coarse signal: a successful build/test/VCS command is
// treated as evidence the session did real, checkable work.
var completionVerbs = map[string]bool{
	"git": true, "go": true, "pytest": true, "npm": true, "pnpm": true,
	"yarn": true, "cargo": true, "make": true, "gradle": true, "mvn": true,
	"vitest": true, "jest": true, "tsc": true, "rspec": true, "gotestsum": true,
}

// testVerbs are a subset used by Taxonomy to recognize a testing session.
var testVerbs = map[string]bool{
	"pytest": true, "vitest": true, "jest": true, "rspec": true, "gotestsum": true,
}

// Outcome derives a session's result: Failure if it ends on a failed tool call,
// Success if it shows a completion signal and does not end failed, else Unknown.
func Outcome(evts []schema.TelemetryEvent) OutcomeClass {
	var lastPost schema.ToolOutcome
	completion := false
	for _, e := range evts {
		if e.EventType != schema.EventPostTool {
			continue
		}
		if e.ToolOK != "" {
			lastPost = e.ToolOK
		}
		if e.ToolClass == schema.ClassExec && e.ToolOK == schema.OutcomeOK && hasVerb(e.Verbs, completionVerbs) {
			completion = true
		}
	}
	switch {
	case lastPost == schema.OutcomeFailed:
		return Failure
	case completion:
		return Success
	default:
		return Unknown
	}
}

func hasVerb(verbs []string, set map[string]bool) bool {
	for _, v := range verbs {
		if set[v] {
			return true
		}
	}
	return false
}
