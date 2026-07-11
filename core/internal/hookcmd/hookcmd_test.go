package hookcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Hypership-Software/atlas/internal/daemon"
	"github.com/Hypership-Software/atlas/internal/ipc"
	"github.com/Hypership-Software/atlas/internal/schema"
)

type dangerEval struct{}

func (dangerEval) Eval(schema.Descriptor) (schema.Verdict, string) {
	return schema.VerdictDeny, "no-exec"
}

type noTaint struct{}

func (noTaint) Apply(*schema.Descriptor)                 {}
func (noTaint) MarkFromResult(string, schema.Descriptor) {}

type capRecorder struct {
	mu     sync.Mutex
	events []schema.TelemetryEvent
}

func (c *capRecorder) Record(e schema.TelemetryEvent) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, e)
	return nil
}

func (c *capRecorder) all() []schema.TelemetryEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]schema.TelemetryEvent(nil), c.events...)
}

const preToolBash = `{"session_id":"t","cwd":"/p","hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"rm -rf /"},"tool_use_id":"x"}`
const postToolBash = `{"session_id":"t","hook_event_name":"PostToolUse","tool_name":"Bash","tool_input":{"command":"ls"},"tool_response":{"stdout":"x"},"tool_use_id":"x"}`

// The shim observes: it delivers the event to the daemon for recording and emits
// NO hook decision, even for an action the classifier flags as dangerous.
func TestRunObservesAndEmitsNoDecision(t *testing.T) {
	t.Setenv("GATED_IPC_ID", "hookcmd-observe")
	ln, err := ipc.Listen()
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	rec := &capRecorder{}
	h := daemon.NewHandler(daemon.Deps{Eval: dangerEval{}, Taint: noTaint{}, Record: rec})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = daemon.Serve(ctx, ln, h) }()

	var out, errb bytes.Buffer
	code := Run("claudecode", strings.NewReader(preToolBash), &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d, want 0 (observation never disrupts), stderr=%s", code, errb.String())
	}
	if strings.Contains(out.String(), "permissionDecision") {
		t.Fatalf("shim emitted a hook decision; Atlas observes only: %s", out.String())
	}
	evs := rec.all()
	if len(evs) != 1 {
		t.Fatalf("daemon recorded %d events, want 1", len(evs))
	}
	if evs[0].Verdict != schema.VerdictDeny || evs[0].EventType != schema.EventPreTool {
		t.Errorf("recorded %v/%v, want pre_tool classified deny", evs[0].EventType, evs[0].Verdict)
	}
}

// A pre_tool event with the daemon down is spooled and exits 0 — there is no
// fail-closed block, because Atlas does not gate.
func TestRunPreToolSpoolsWhenDaemonDown(t *testing.T) {
	t.Setenv("GATED_IPC_ID", "hookcmd-down-pre")
	home := t.TempDir()
	t.Setenv("GATED_HOME", home)

	var out, errb bytes.Buffer
	code := Run("claudecode", strings.NewReader(preToolBash), &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d, want 0 (observe shim never blocks)", code)
	}
	if strings.Contains(out.String(), "permissionDecision") {
		t.Fatalf("shim emitted a decision with the daemon down: %s", out.String())
	}
	data, err := os.ReadFile(filepath.Join(home, "spool", "spool.jsonl"))
	if err != nil {
		t.Fatalf("pre_tool telemetry was not spooled: %v", err)
	}
	if !strings.Contains(string(data), `"event_type":"pre_tool"`) {
		t.Errorf("spooled event looks wrong: %s", data)
	}
}

func TestRunPostToolSpoolsWhenDaemonDown(t *testing.T) {
	t.Setenv("GATED_IPC_ID", "hookcmd-down-post")
	home := t.TempDir()
	t.Setenv("GATED_HOME", home)

	var out, errb bytes.Buffer
	code := Run("claudecode", strings.NewReader(postToolBash), &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d, want 0", code)
	}
	data, err := os.ReadFile(filepath.Join(home, "spool", "spool.jsonl"))
	if err != nil {
		t.Fatalf("telemetry was not spooled: %v", err)
	}
	if !strings.Contains(string(data), `"event_type":"post_tool"`) {
		t.Errorf("spooled event looks wrong: %s", data)
	}
}

func TestRunUnknownHarness(t *testing.T) {
	var out, errb bytes.Buffer
	if code := Run("nope", strings.NewReader("{}"), &out, &errb); code != 0 {
		t.Fatalf("exit %d, want 0 (observation never disrupts the session)", code)
	}
	if !strings.Contains(errb.String(), "unknown harness") {
		t.Errorf("stderr missing diagnostic: %q", errb.String())
	}
}

func TestLiveReflectsDaemonReachability(t *testing.T) {
	t.Setenv("GATED_IPC_ID", "hookcmd-live")
	if Live(500 * time.Millisecond) {
		t.Error("Live reported true with no daemon listening")
	}
	ln, err := ipc.Listen()
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, e := ln.Accept()
			if e != nil {
				return
			}
			_ = c.Close()
		}
	}()
	if !Live(2 * time.Second) {
		t.Error("Live reported false with a daemon listening")
	}
}
