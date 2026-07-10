package hookcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Hypership-Software/atlas/internal/daemon"
	"github.com/Hypership-Software/atlas/internal/ipc"
	"github.com/Hypership-Software/atlas/internal/schema"
)

type denyEval struct{}

func (denyEval) Eval(schema.Descriptor) (schema.Verdict, string) {
	return schema.VerdictDeny, "no-exec"
}

type noTaint struct{}

func (noTaint) Apply(*schema.Descriptor)                 {}
func (noTaint) MarkFromResult(string, schema.Descriptor) {}

type denyApprover struct{}

func (denyApprover) Request(schema.Descriptor) (schema.Verdict, string) {
	return schema.VerdictDeny, ""
}

type nopRecorder struct{}

func (nopRecorder) Record(schema.TelemetryEvent) error { return nil }

const preToolBash = `{"session_id":"t","cwd":"/p","hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"rm -rf /"},"tool_use_id":"x"}`
const postToolBash = `{"session_id":"t","hook_event_name":"PostToolUse","tool_name":"Bash","tool_input":{"command":"ls"},"tool_response":{"stdout":"x"},"tool_use_id":"x"}`

func TestRunDenyIsExitZeroWithDenyJSON(t *testing.T) {
	t.Setenv("GATED_IPC_ID", "hookcmd-deny")
	ln, err := ipc.Listen()
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	h := daemon.NewHandler(daemon.Deps{Eval: denyEval{}, Taint: noTaint{}, Approve: denyApprover{}, Record: nopRecorder{}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = daemon.Serve(ctx, ln, h) }()

	var out, errb bytes.Buffer
	code := Run("claudecode", strings.NewReader(preToolBash), &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d, want 0 (a deny is exit-0 + JSON), stderr=%s", code, errb.String())
	}
	if !strings.Contains(out.String(), `"permissionDecision":"deny"`) {
		t.Fatalf("stdout missing deny decision: %s", out.String())
	}
}

func TestRunPreToolFailsClosedWhenDaemonDown(t *testing.T) {
	t.Setenv("GATED_IPC_ID", "hookcmd-down-pre")
	var out, errb bytes.Buffer
	code := Run("claudecode", strings.NewReader(preToolBash), &out, &errb)
	if code != 2 {
		t.Fatalf("exit %d, want 2 (fail-closed on unreachable daemon)", code)
	}
	if !strings.Contains(errb.String(), "doctor") {
		t.Errorf("stderr missing actionable hint: %q", errb.String())
	}
}

func TestRunPostToolFailsQuietAndSpools(t *testing.T) {
	t.Setenv("GATED_IPC_ID", "hookcmd-down-post")
	home := t.TempDir()
	t.Setenv("GATED_HOME", home)

	var out, errb bytes.Buffer
	code := Run("claudecode", strings.NewReader(postToolBash), &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d, want 0 (fail-quiet on non-gating event)", code)
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
	if code := Run("nope", strings.NewReader("{}"), &out, &errb); code != 2 {
		t.Fatalf("exit %d, want 2 for unknown harness", code)
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
