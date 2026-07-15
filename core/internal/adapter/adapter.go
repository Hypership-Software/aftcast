// Package adapter normalizes each harness's hook payloads into the shared schema
// (Descriptor + TelemetryEvent). The Adapter interface + registry keep additional
// harnesses a drop-in with no daemon changes.
package adapter

import "github.com/Hypership-Software/aftcast/internal/schema"

type Adapter interface {
	// Normalize turns a raw hook payload into a Descriptor (for tool events) and a
	// TelemetryEvent (every event). A non-empty event name is authoritative; if
	// empty, the payload's own event field is used.
	Normalize(event string, raw []byte) (schema.Descriptor, schema.TelemetryEvent, error)
}

var registry = map[string]Adapter{}

func register(name string, a Adapter) { registry[name] = a }

func Get(name string) (Adapter, bool) {
	a, ok := registry[name]
	return a, ok
}
