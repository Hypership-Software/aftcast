// Package daemon defines the request/response protocol between the hook shim and
// the resident gate, and the Handler that orchestrates a single gating decision:
// taint -> evaluate -> (approve) -> record.
package daemon

import "github.com/Hypership-Software/atlas/internal/schema"

// Request is one gating/telemetry message from a harness (via the shim or the
// HTTP hook path). Descriptor is populated for gating events (pre_tool); Event
// carries the telemetry record for every event type.
type Request struct {
	Event      schema.TelemetryEvent `json:"event"`
	Descriptor schema.Descriptor     `json:"descriptor"`
}

// Response is the gate's verdict for a Request.
type Response struct {
	Verdict schema.Verdict `json:"verdict"`
	RuleID  string         `json:"rule_id"`
	Reason  string         `json:"reason"`
}

// The Handler depends on these behaviours as interfaces (defined by the consumer,
// Go-style) so the real policy engine, taint ledger, approval queue, and audit
// log slot in without an import cycle. (Integrity is wired later, with Task 15's
// drift model and Task 23's daemon lifecycle — it is not part of the per-request
// path, so adding it here now would be speculative.)
type (
	Evaluator interface {
		Eval(d schema.Descriptor) (schema.Verdict, string)
	}
	Tainter interface {
		Apply(d *schema.Descriptor)
		MarkFromResult(sessionID string, d schema.Descriptor)
	}
	Approver interface {
		Request(d schema.Descriptor) (verdict schema.Verdict, reason string)
	}
	Recorder interface {
		Record(e schema.TelemetryEvent) error
	}
)

// Deps are the Handler's collaborators.
type Deps struct {
	Eval    Evaluator
	Taint   Tainter
	Approve Approver
	Record  Recorder
}

// Handler resolves one Request into a Response and records telemetry.
type Handler struct{ deps Deps }

func NewHandler(d Deps) *Handler { return &Handler{deps: d} }

// Handle gates pre_tool events and records telemetry for every event. Only
// pre_tool consults the evaluator; all other event types are recorded and
// allowed (they are observations, not gated actions).
func (h *Handler) Handle(req Request) (Response, error) {
	if req.Event.EventType != schema.EventPreTool {
		if err := h.deps.Record.Record(req.Event); err != nil {
			return Response{}, err
		}
		return Response{Verdict: schema.VerdictAllow}, nil
	}

	// pre_tool: inject stored session taint before eval so taint-effector rules
	// see it, then evaluate.
	d := req.Descriptor
	h.deps.Taint.Apply(&d)
	verdict, ruleID := h.deps.Eval.Eval(d)

	reason := ""
	if verdict == schema.VerdictAsk {
		// No policy matched; delegate to the approver, which blocks and resolves
		// to a concrete allow/deny (never ask).
		verdict, reason = h.deps.Approve.Request(d)
	}

	// An allowed taint-source action (e.g. a permitted WebFetch) taints the
	// session for subsequent actions. A denied action never ran, so it can't
	// taint. Marking at pre_tool is conservative (taints even if the call later
	// errors) — safe by design; precise ingestion-time semantics land with Task 12.
	if verdict == schema.VerdictAllow {
		h.deps.Taint.MarkFromResult(d.SessionID, d)
	}

	ev := req.Event
	ev.Verdict = verdict
	ev.RuleID = ruleID
	if verdict == schema.VerdictDeny {
		ev.EventType = schema.EventBlock // a blocked action, for danger-prevented analytics
	}
	if err := h.deps.Record.Record(ev); err != nil {
		return Response{}, err
	}
	return Response{Verdict: verdict, RuleID: ruleID, Reason: reason}, nil
}
