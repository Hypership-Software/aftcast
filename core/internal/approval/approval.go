// Package approval is the auto-mode-safe approval queue: when a policy resolves
// to `ask`, the daemon blocks on Queue.Request until a human resolves it via the
// `gated approvals` TUI or the timeout elapses. In auto/bypass mode there is no
// native prompt, so the bounded timeout (default 100s) guarantees the agent
// never hangs and the default outcome is DENY. Repeated approvals of the same
// shape can draft a standing permit rule for review.
package approval

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Hypership-Software/atlas/internal/schema"
)

// DefaultTimeout bounds how long a gating action waits for a human before the
// queue denies it. It matches the installed hook timeout so the harness doesn't
// give up first.
const DefaultTimeout = 100 * time.Second

type item struct {
	id      string
	desc    schema.Descriptor
	at      time.Time
	resolve chan resolution
}

type resolution struct {
	verdict schema.Verdict
	reason  string
}

// Queue holds pending approvals. It implements daemon.Approver.
type Queue struct {
	mu        sync.Mutex
	items     map[string]*item
	order     []string
	seq       int
	timeout   time.Duration
	policyDir string
	notify    func(schema.Descriptor)
	now       func() time.Time
}

// NewQueue creates a queue. policyDir is where "always allow" drafts a permit
// (under policyDir/pending); pass "" to disable drafting.
func NewQueue(timeout time.Duration, policyDir string) *Queue {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return &Queue{
		items:     map[string]*item{},
		timeout:   timeout,
		policyDir: policyDir,
		notify:    notifyDesktop,
		now:       time.Now,
	}
}

// Request enqueues an approval, fires a best-effort desktop notification, and
// blocks until the item is resolved or the timeout elapses (then denies).
func (q *Queue) Request(d schema.Descriptor) (schema.Verdict, string) {
	it := q.enqueue(d)
	if q.notify != nil {
		go q.notify(d)
	}
	select {
	case r := <-it.resolve:
		q.dequeue(it.id)
		return r.verdict, r.reason
	case <-time.After(q.timeout):
		q.dequeue(it.id)
		return schema.VerdictDeny, "approval timed out"
	}
}

// Pending is a snapshot of the waiting approvals, oldest first.
type Pending struct {
	ID   string
	Desc schema.Descriptor
	At   time.Time
}

// Pending returns the current queue in stable (enqueue) order.
func (q *Queue) Pending() []Pending {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]Pending, 0, len(q.order))
	for _, id := range q.order {
		if it := q.items[id]; it != nil {
			out = append(out, Pending{ID: it.id, Desc: it.desc, At: it.at})
		}
	}
	return out
}

// Resolve settles a pending approval. If makeRule is set on an allow, a draft
// permit for this action's shape is written to policyDir/pending for review
// (nothing is enforced until reviewed).
func (q *Queue) Resolve(id string, v schema.Verdict, makeRule bool) error {
	q.mu.Lock()
	it, ok := q.items[id]
	q.mu.Unlock()
	if !ok {
		return fmt.Errorf("approval %q not found (already resolved or timed out)", id)
	}
	if makeRule && v == schema.VerdictAllow {
		if err := q.writeDraftRule(it.desc); err != nil {
			return err
		}
	}
	reason := "approved by user"
	if v == schema.VerdictDeny {
		reason = "denied by user"
	}
	// resolve is buffered (cap 1) so this never blocks even if Request already
	// timed out; the default guards a second Resolve.
	select {
	case it.resolve <- resolution{verdict: v, reason: reason}:
	default:
	}
	return nil
}

// sanitizeMsg strips anything but a safe alphanumeric set before a tool name is
// interpolated into a shell/AppleScript notification command — defense against
// command injection via a crafted tool name.
func sanitizeMsg(s string) string {
	b := make([]rune, 0, len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == ' ', r == '_', r == '-', r == '.', r == ':':
			b = append(b, r)
		}
	}
	if len(b) > 120 {
		b = b[:120]
	}
	return string(b)
}

func (q *Queue) enqueue(d schema.Descriptor) *item {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.seq++
	it := &item{
		id:      fmt.Sprintf("a%d", q.seq),
		desc:    d,
		at:      q.now(),
		resolve: make(chan resolution, 1),
	}
	q.items[it.id] = it
	q.order = append(q.order, it.id)
	return it
}

func (q *Queue) dequeue(id string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.items, id)
	for i, o := range q.order {
		if o == id {
			q.order = append(q.order[:i], q.order[i+1:]...)
			break
		}
	}
}

func (q *Queue) writeDraftRule(d schema.Descriptor) error {
	if q.policyDir == "" {
		return nil
	}
	dir := filepath.Join(q.policyDir, "pending")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	name, rule := draftRule(d)
	return os.WriteFile(filepath.Join(dir, name+".cedar"), []byte(rule), 0o600)
}

// draftRule builds a permit scoped to the action's shape. The filename encodes
// the shape so re-approving the same shape overwrites rather than piling up.
func draftRule(d schema.Descriptor) (name, cedar string) {
	header := "// DRAFT — written by `always allow`. Review with `gated policy review`;\n// nothing here is enforced until it is accepted into an active policy dir.\n"
	switch d.ToolClass {
	case schema.ClassExec:
		verb := "cmd"
		if len(d.Verbs) > 0 {
			verb = d.Verbs[0]
		}
		name = "draft-permit-exec-" + verb
		return name, fmt.Sprintf("%s@id(%q)\npermit(principal, action == Action::\"exec\", resource)\nwhen { context.verbs.contains(%q) };\n", header, name, verb)
	case schema.ClassNetFetch:
		name = "draft-permit-fetch-" + d.Domain
		return name, fmt.Sprintf("%s@id(%q)\npermit(principal, action == Action::\"net_fetch\", resource)\nwhen { context.domain == %q };\n", header, name, d.Domain)
	default:
		name = "draft-permit-" + string(d.ToolClass)
		return name, fmt.Sprintf("%s@id(%q)\npermit(principal, action == Action::%q, resource);\n", header, name, string(d.ToolClass))
	}
}
