package policy

import (
	"github.com/Hypership-Software/atlas/internal/schema"
	"github.com/cedar-policy/cedar-go"
)

// cedar-go v1.8.0 API notes (verified via `go doc` on the installed module,
// 2026-07-10 — context7 indexes only the Cedar language, not the Go binding):
//
//   - PolicySet.IsAuthorized(entities, req) (Decision, Diagnostic).
//   - Decision is a bool; cedar.Allow is the only exported constant (deny is the
//     zero value). Cedar is DENY-BY-DEFAULT: Allow iff some permit is satisfied
//     and no forbid is; otherwise Deny — so a bare Deny cannot, by itself,
//     distinguish "a forbid fired" from "nothing matched".
//   - Diagnostic.Reasons lists the determining policy IDs; Diagnostic.Errors
//     lists policies that ERRORED during eval (and were therefore skipped).
//   - PolicySet.Get(id).Effect() reports permit vs forbid; Effect is a bool and
//     cedar.Permit is the only exported constant (forbid is the zero value).
//
// This is why we resolve the three-valued verdict from the Diagnostic rather
// than trusting Cedar's binary Decision.
const defaultAskRuleID = "default-ask"

// Engine resolves a Descriptor to a three-valued Verdict over a compiled Cedar
// PolicySet: deny (a forbid fired), allow (a permit fired), or ask (no match).
type Engine struct {
	ps *cedar.PolicySet
}

func NewEngine(ps *cedar.PolicySet) *Engine { return &Engine{ps: ps} }

// Eval returns the verdict and the determining rule ID.
func (e *Engine) Eval(d schema.Descriptor) (schema.Verdict, string) {
	req, entities := ToCedar(d)
	decision, diag := e.ps.IsAuthorized(entities, req)

	// Fail safe first: a forbid that errored during eval was silently skipped by
	// Cedar and so did not deny. Treat it as a deny regardless of the Decision —
	// a forbid referencing a missing attribute must never be a path to allow.
	for _, evalErr := range diag.Errors {
		if e.isForbid(evalErr.PolicyID) {
			return schema.VerdictDeny, string(evalErr.PolicyID)
		}
	}

	if decision == cedar.Allow {
		return schema.VerdictAllow, firstReason(diag)
	}

	// Decision is Deny: either a forbid was determining, or nothing matched and
	// this is Cedar's default deny — which for us means Ask.
	for _, r := range diag.Reasons {
		if e.isForbid(r.PolicyID) {
			return schema.VerdictDeny, string(r.PolicyID)
		}
	}
	return schema.VerdictAsk, defaultAskRuleID
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
