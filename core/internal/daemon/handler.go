// Package daemon defines the request/response protocol between the hook shim and
// the resident gate, and the Handler that observes a single tool call:
// classify risk -> record. Atlas never blocks; the classification is telemetry.
package daemon

import "github.com/Hypership-Software/atlas/internal/schema"

// Request is one telemetry message from a harness (via the shim or the HTTP hook
// path). Descriptor is populated for tool events (pre_tool); Event carries the
// telemetry record for every event type.
type Request struct {
	Event      schema.TelemetryEvent `json:"event"`
	Descriptor schema.Descriptor     `json:"descriptor"`
}

// Response reports how the daemon classified the action's risk. It is
// informational — Atlas observes and records, it does not block — so a caller
// may surface the classification but is never expected to act on it.
type Response struct {
	Verdict schema.Verdict `json:"verdict"`
	RuleID  string         `json:"rule_id"`
}

// The Handler depends on these behaviours as interfaces (defined by the consumer,
// Go-style) so the real risk classifier, taint ledger, and audit log slot in
// without an import cycle.
type (
	Evaluator interface {
		Eval(d schema.Descriptor) (schema.Verdict, string)
	}
	Tainter interface {
		Apply(d *schema.Descriptor)
		MarkFromResult(sessionID string, d schema.Descriptor)
	}
	Recorder interface {
		Record(e schema.TelemetryEvent) error
	}
)

// Deps are the Handler's collaborators.
type Deps struct {
	Eval   Evaluator
	Taint  Tainter
	Record Recorder
}

// Handler classifies and records one Request. Every event is recorded; pre_tool
// events are additionally run through the risk classifier and taint ledger. The
// action always proceeds — Atlas observes, it does not gate.
type Handler struct{ deps Deps }

func NewHandler(d Deps) *Handler { return &Handler{deps: d} }

// Handle records telemetry for every event and classifies pre_tool events. Only
// pre_tool consults the classifier; all other event types are pure observations.
func (h *Handler) Handle(req Request) (Response, error) {
	if req.Event.EventType != schema.EventPreTool {
		if err := h.deps.Record.Record(req.Event); err != nil {
			return Response{}, err
		}
		return Response{}, nil
	}

	// pre_tool: inject stored session taint so taint-effector rules see it, then
	// classify the action's risk.
	d := req.Descriptor
	h.deps.Taint.Apply(&d)
	verdict, ruleID := h.deps.Eval.Eval(d)

	// The action runs regardless of classification (Atlas does not block). A
	// taint-source action (e.g. a WebFetch to an untrusted domain) taints the
	// session as a risk signal for the actions that follow it.
	h.deps.Taint.MarkFromResult(d.SessionID, d)

	ev := req.Event
	ev.Verdict = verdict
	ev.RuleID = ruleID
	if err := h.deps.Record.Record(ev); err != nil {
		return Response{}, err
	}
	return Response{Verdict: verdict, RuleID: ruleID}, nil
}
