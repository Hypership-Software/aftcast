// Package daemon defines the shim<->daemon protocol and the Handler that observes
// one tool call: classify risk, record. Atlas never blocks; the classification is
// telemetry.
package daemon

import "github.com/Hypership-Software/atlas/internal/schema"

// Request is one telemetry message from a harness. Descriptor is populated for
// tool events (pre_tool); Event carries the record for every event type.
type Request struct {
	Event      schema.TelemetryEvent `json:"event"`
	Descriptor schema.Descriptor     `json:"descriptor"`
}

// Response reports the risk classification. It is informational — a caller may
// surface it but is never expected to act on it (Atlas observes, does not block).
type Response struct {
	Risk   schema.Risk `json:"risk"`
	RuleID string      `json:"rule_id"`
}

// Handler depends on these as consumer-defined interfaces so the classifier,
// taint ledger, and audit log slot in without an import cycle.
type (
	Evaluator interface {
		Eval(d schema.Descriptor) (schema.Risk, string)
	}
	Tainter interface {
		Apply(d *schema.Descriptor)
		MarkFromResult(sessionID string, d schema.Descriptor)
	}
	Recorder interface {
		Record(e schema.TelemetryEvent) error
	}
)

type Deps struct {
	Eval   Evaluator
	Taint  Tainter
	Record Recorder
}

// Handler classifies and records one Request. The action always proceeds — Atlas
// observes, it does not gate.
type Handler struct{ deps Deps }

func NewHandler(d Deps) *Handler { return &Handler{deps: d} }

// Handle records every event; only pre_tool consults the classifier and taint
// ledger.
func (h *Handler) Handle(req Request) (Response, error) {
	if req.Event.EventType != schema.EventPreTool {
		if err := h.deps.Record.Record(req.Event); err != nil {
			return Response{}, err
		}
		return Response{}, nil
	}

	// Inject stored session taint so taint-effector rules see it, then classify.
	d := req.Descriptor
	h.deps.Taint.Apply(&d)
	risk, ruleID := h.deps.Eval.Eval(d)

	// The action runs regardless (Atlas does not block). A taint-source action
	// taints the session as a risk signal for the actions that follow.
	h.deps.Taint.MarkFromResult(d.SessionID, d)

	ev := req.Event
	ev.Risk = risk
	ev.RuleID = ruleID
	if err := h.deps.Record.Record(ev); err != nil {
		return Response{}, err
	}
	return Response{Risk: risk, RuleID: ruleID}, nil
}
