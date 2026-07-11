package policy

import (
	"github.com/Hypership-Software/atlas/internal/schema"
	"github.com/cedar-policy/cedar-go"
)

// cedar-go API notes (verified via `go doc`; context7 indexes only the Cedar
// language, not the Go binding):
//   - Cedar is DENY-BY-DEFAULT, so a bare Deny Decision cannot distinguish "a
//     forbid fired" from "nothing matched" — we resolve risk from the Diagnostic.
//   - Diagnostic.Errors lists policies that ERRORED during eval (and were
//     therefore silently skipped).
//   - Effect() is a bool whose zero value is forbid; cedar.Permit is the only
//     exported constant.
const unclassifiedRuleID = "unclassified"

// Engine classifies a Descriptor into a three-valued Risk over a compiled Cedar
// PolicySet. The result is a label for telemetry, never an enforcement action.
type Engine struct {
	ps *cedar.PolicySet
}

func NewEngine(ps *cedar.PolicySet) *Engine { return &Engine{ps: ps} }

func (e *Engine) Eval(d schema.Descriptor) (schema.Risk, string) {
	req, entities := ToCedar(d)
	decision, diag := e.ps.IsAuthorized(entities, req)

	// Fail safe first: a forbid that errored during eval was silently skipped by
	// Cedar. Treat it as danger regardless of the Decision — a danger rule
	// referencing a missing attribute must never be a path to "safe".
	for _, evalErr := range diag.Errors {
		if e.isForbid(evalErr.PolicyID) {
			return schema.RiskDanger, string(evalErr.PolicyID)
		}
	}

	if decision == cedar.Allow {
		return schema.RiskSafe, firstReason(diag)
	}

	// Decision is Deny: either a danger (forbid) rule was determining, or nothing
	// matched and this is Cedar's default deny — which for us means unknown.
	for _, r := range diag.Reasons {
		if e.isForbid(r.PolicyID) {
			return schema.RiskDanger, string(r.PolicyID)
		}
	}
	return schema.RiskUnknown, unclassifiedRuleID
}

func (e *Engine) isForbid(id cedar.PolicyID) bool {
	p := e.ps.Get(id)
	return p != nil && p.Effect() != cedar.Permit
}

func firstReason(diag cedar.Diagnostic) string {
	if len(diag.Reasons) > 0 {
		return string(diag.Reasons[0].PolicyID)
	}
	return ""
}
