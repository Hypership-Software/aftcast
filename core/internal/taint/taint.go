// Package taint tracks session-level taint: once a session ingests untrusted
// content (an external fetch/search or an MCP call), it is marked tainted and
// the taint-effector policies cut its sensitive-effector rights (ADR-005, the
// lethal-trifecta break).
//
// Taint is held in-memory and rebuilt from the event log on daemon restart
// (Kyle 2026-07-10 — bbolt dropped). The HMAC event log is the single source of
// truth; taint, like the SQLite read-model, is a rebuildable projection of it.
package taint

import (
	"strings"
	"sync"

	"github.com/Hypership-Software/atlas/internal/schema"
)

// Ledger records which sessions are tainted.
type Ledger struct {
	mu      sync.Mutex
	tainted map[string]bool
	trusted map[string]bool // immutable after construction
}

// NewLedger creates a ledger with a set of trusted domains — fetches/searches to
// these do not taint.
func NewLedger(trustedDomains []string) *Ledger {
	trusted := make(map[string]bool, len(trustedDomains))
	for _, d := range trustedDomains {
		trusted[strings.ToLower(d)] = true
	}
	return &Ledger{tainted: map[string]bool{}, trusted: trusted}
}

// Apply injects the session's stored taint into the descriptor before evaluation
// so taint-effector policies can gate on it.
func (l *Ledger) Apply(d *schema.Descriptor) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.tainted[d.SessionID] {
		d.Tainted = true
	}
}

// MarkFromResult taints the session if this action ingests untrusted content.
func (l *Ledger) MarkFromResult(sessionID string, d schema.Descriptor) {
	if !l.isTaintSource(d) {
		return
	}
	l.mu.Lock()
	l.tainted[sessionID] = true
	l.mu.Unlock()
}

// IsTainted reports whether a session is tainted.
func (l *Ledger) IsTainted(sessionID string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.tainted[sessionID]
}

// Rebuild reconstructs taint state by replaying the event log (called on daemon
// startup). Denied actions never ran, so they don't taint.
func (l *Ledger) Rebuild(events []schema.TelemetryEvent) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.tainted = map[string]bool{}
	for _, e := range events {
		if e.Verdict == schema.VerdictDeny {
			continue
		}
		if l.isTaintSource(schema.Descriptor{ToolClass: e.ToolClass, Domain: e.Domain}) {
			l.tainted[e.SessionID] = true
		}
	}
}

// isTaintSource reads only immutable state (l.trusted), so it needs no lock.
func (l *Ledger) isTaintSource(d schema.Descriptor) bool {
	switch d.ToolClass {
	case schema.ClassNetFetch, schema.ClassNetSearch:
		// An unparseable/empty domain is treated as untrusted — fail safe.
		return !l.trusted[strings.ToLower(d.Domain)]
	case schema.ClassMCP:
		// MCP responses can carry arbitrary external content we can't attribute
		// to a domain, so any MCP call taints.
		return true
	default:
		return false
	}
}
