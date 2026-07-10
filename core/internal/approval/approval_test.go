package approval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Hypership-Software/atlas/internal/schema"
)

// resolveWhenQueued resolves the single pending item once it appears.
func resolveWhenQueued(q *Queue, v schema.Verdict, makeRule bool) chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 400; i++ {
			if p := q.Pending(); len(p) == 1 {
				_ = q.Resolve(p[0].ID, v, makeRule)
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()
	return done
}

func TestRequestTimesOutToDeny(t *testing.T) {
	q := NewQueue(40*time.Millisecond, "")
	q.notify = nil
	v, reason := q.Request(schema.Descriptor{SessionID: "s", ToolClass: schema.ClassExec})
	if v != schema.VerdictDeny {
		t.Fatalf("verdict = %v, want deny on timeout", v)
	}
	if reason != "approval timed out" {
		t.Errorf("reason = %q, want %q", reason, "approval timed out")
	}
	if len(q.Pending()) != 0 {
		t.Errorf("queue not drained after timeout: %d", len(q.Pending()))
	}
}

func TestResolveAllowUnblocks(t *testing.T) {
	q := NewQueue(2*time.Second, "")
	q.notify = nil
	done := resolveWhenQueued(q, schema.VerdictAllow, false)
	v, _ := q.Request(schema.Descriptor{SessionID: "s", ToolClass: schema.ClassExec})
	<-done
	if v != schema.VerdictAllow {
		t.Fatalf("verdict = %v, want allow", v)
	}
	if len(q.Pending()) != 0 {
		t.Errorf("queue not drained: %d", len(q.Pending()))
	}
}

func TestAlwaysAllowWritesDraftRule(t *testing.T) {
	dir := t.TempDir()
	q := NewQueue(2*time.Second, dir)
	q.notify = nil
	done := resolveWhenQueued(q, schema.VerdictAllow, true)
	q.Request(schema.Descriptor{SessionID: "s", ToolClass: schema.ClassExec, Verbs: []string{"git"}, Argv: []string{"git", "push"}})
	<-done

	data, err := os.ReadFile(filepath.Join(dir, "pending", "draft-permit-exec-git.cedar"))
	if err != nil {
		t.Fatalf("draft rule not written: %v", err)
	}
	if !strings.Contains(string(data), `context.verbs.contains("git")`) {
		t.Errorf("draft rule does not match the action shape:\n%s", data)
	}
	if !strings.Contains(string(data), "DRAFT") {
		t.Errorf("draft rule missing the review notice:\n%s", data)
	}
}

func TestResolveUnknownIDErrors(t *testing.T) {
	q := NewQueue(time.Second, "")
	if err := q.Resolve("nope", schema.VerdictAllow, false); err == nil {
		t.Fatal("expected an error resolving an unknown approval id")
	}
}

func TestTUIViewShowsPendingAndHints(t *testing.T) {
	q := NewQueue(time.Minute, "")
	q.notify = nil
	q.enqueue(schema.Descriptor{ToolClass: schema.ClassExec, ToolRaw: "Bash", Verbs: []string{"rm"}, Argv: []string{"rm", "-rf", "/"}})
	view := newModel(q).View()
	if !strings.Contains(view, "rm -rf /") {
		t.Errorf("view missing the action detail:\n%s", view)
	}
	if !strings.Contains(view, "approve") || !strings.Contains(view, "deny") {
		t.Errorf("view missing key hints:\n%s", view)
	}
}

func TestTUIEmptyState(t *testing.T) {
	if v := newModel(NewQueue(time.Minute, "")).View(); !strings.Contains(v, "No approvals waiting") {
		t.Errorf("missing empty state:\n%s", v)
	}
}
