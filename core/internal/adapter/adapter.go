// Package adapter normalizes each harness's hook payloads into the shared schema
// (Descriptor + TelemetryEvent) and renders verdicts back into the harness's
// hook-response format. Claude Code is the MVP harness; the Adapter interface +
// registry keep additional harnesses (Codex — deferred post-MVP) a drop-in with
// no changes to the daemon.
package adapter

import "github.com/Hypership-Software/atlas/internal/schema"

// Adapter translates between a harness's hook wire format and the gate's schema.
type Adapter interface {
	// Normalize turns a raw hook payload for the named event into a Descriptor
	// (populated for gating events) and a TelemetryEvent (for every event). The
	// event name is authoritative from the shim/HTTP path; if empty, the payload's
	// own event field is used.
	Normalize(event string, raw []byte) (schema.Descriptor, schema.TelemetryEvent, error)
	// Respond renders a verdict into the harness's hook-response JSON.
	Respond(v schema.Verdict, reason string) ([]byte, error)
}

var registry = map[string]Adapter{}

func register(name string, a Adapter) { registry[name] = a }

// Get returns the adapter registered for a harness name (e.g. "claudecode").
func Get(name string) (Adapter, bool) {
	a, ok := registry[name]
	return a, ok
}
