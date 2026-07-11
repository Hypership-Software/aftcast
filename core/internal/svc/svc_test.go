package svc_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Hypership-Software/atlas/internal/audit"
	"github.com/Hypership-Software/atlas/internal/daemon"
	"github.com/Hypership-Software/atlas/internal/ipc"
	"github.com/Hypership-Software/atlas/internal/schema"
	"github.com/Hypership-Software/atlas/internal/svc"
)

// startDaemon runs svc.Run in the background with an isolated control-plane
// endpoint and the integrity ticker disabled, and returns once both listeners
// are bound. cancel + <-errc shut it down and surface Run's error.
func startDaemon(t *testing.T, ipcID string, opts svc.Options) (svc.Info, context.CancelFunc, <-chan error) {
	t.Helper()
	t.Setenv("GATED_IPC_ID", ipcID)
	ready := make(chan svc.Info, 1)
	opts.Ready = ready
	opts.IntegrityTick = -1 // no background ticker during tests
	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() { errc <- svc.Run(ctx, opts) }()
	select {
	case info := <-ready:
		return info, cancel, errc
	case err := <-errc:
		cancel()
		t.Fatalf("Run exited before ready: %v", err)
	case <-time.After(10 * time.Second):
		cancel()
		t.Fatal("daemon did not become ready within 10s")
	}
	return svc.Info{}, cancel, errc // unreachable
}

// TestRunClassifiesOverControlPlane is the acceptance test: svc.Run wires the
// real classifier + temp dirs and serves one loopback request end-to-end,
// returning the risk classification (not an enforcement decision), then shuts
// down cleanly on ctx cancel.
func TestRunClassifiesOverControlPlane(t *testing.T) {
	home := t.TempDir()
	_, cancel, errc := startDaemon(t, "svc-cp", svc.Options{Home: home})

	conn, err := ipc.Dial(2 * time.Second)
	if err != nil {
		t.Fatalf("dial control plane: %v", err)
	}
	req := daemon.Request{
		Event: schema.TelemetryEvent{
			V: schema.SchemaVersion, EventType: schema.EventPreTool,
			SessionID: "s1", ToolClass: schema.ClassExec, ToolRaw: "Bash",
		},
		Descriptor: schema.Descriptor{
			Version: schema.SchemaVersion, SessionID: "s1",
			ToolClass: schema.ClassExec, ToolRaw: "Bash",
			Verbs: []string{"rm"}, Argv: []string{"rm", "-rf", "/"},
		},
	}
	raw, _ := json.Marshal(req)
	if err := ipc.WriteFrame(conn, raw); err != nil {
		t.Fatal(err)
	}
	respRaw, err := ipc.ReadFrame(conn)
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()

	var resp daemon.Response
	if err := json.Unmarshal(respRaw, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Verdict != schema.VerdictDeny {
		t.Fatalf("classification = %q (rule %q), want deny (dangerous)", resp.Verdict, resp.RuleID)
	}
	if resp.RuleID != "deny-rm-rf" {
		t.Errorf("ruleID = %q, want deny-rm-rf", resp.RuleID)
	}

	cancel()
	if err := <-errc; err != nil {
		t.Fatalf("Run returned error on shutdown: %v", err)
	}
}

// TestRunObservesOverHTTP exercises the production transport: a raw Claude Code
// PreToolUse payload POSTed to the hook listener is normalized, classified, and
// recorded — and the response carries NO decision, because Atlas observes and
// never blocks. The classified event must be durable in the tamper-evident log.
func TestRunObservesOverHTTP(t *testing.T) {
	home := t.TempDir()
	info, cancel, errc := startDaemon(t, "svc-http", svc.Options{Home: home})

	payload := `{"hook_event_name":"PreToolUse","session_id":"s-http","tool_name":"Bash","tool_input":{"command":"rm -rf /"}}`
	resp, err := http.Post(info.HTTPURL, "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("POST hook: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	if strings.Contains(string(body), "permissionDecision") {
		t.Fatalf("hook returned a decision; Atlas observes only: %s", body)
	}

	cancel()
	if err := <-errc; err != nil {
		t.Fatalf("Run returned error on shutdown: %v", err)
	}

	key, err := os.ReadFile(filepath.Join(home, "audit.key"))
	if err != nil {
		t.Fatal(err)
	}
	l, err := audit.NewLog(filepath.Join(home, "log"), key)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	rep, err := l.Verify()
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK || rep.Count < 1 {
		t.Fatalf("audit log verify = %+v, want OK with >=1 record", rep)
	}
	// The dangerous action must be recorded AND classified as dangerous — the
	// whole point of observing without blocking.
	evs, err := l.Events()
	if err != nil {
		t.Fatal(err)
	}
	var got schema.TelemetryEvent
	for _, e := range evs {
		if e.SessionID == "s-http" && e.EventType == schema.EventPreTool {
			got = e
		}
	}
	if got.Verdict != schema.VerdictDeny {
		t.Errorf("rm -rf classified as %q, want deny (dangerous)", got.Verdict)
	}
}

// TestRunDrainsSpoolOnStartup: telemetry spooled while the daemon was down is
// folded into the log at startup and the spool file is cleared.
func TestRunDrainsSpoolOnStartup(t *testing.T) {
	home := t.TempDir()
	spoolDir := filepath.Join(home, "spool")
	if err := os.MkdirAll(spoolDir, 0o700); err != nil {
		t.Fatal(err)
	}
	ev := schema.TelemetryEvent{V: schema.SchemaVersion, EventType: schema.EventStop, SessionID: "spooled", Harness: "claudecode"}
	line, _ := json.Marshal(ev)
	spoolPath := filepath.Join(spoolDir, "spool.jsonl")
	if err := os.WriteFile(spoolPath, append(line, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}

	_, cancel, errc := startDaemon(t, "svc-spool", svc.Options{Home: home})
	cancel()
	if err := <-errc; err != nil {
		t.Fatalf("Run returned error on shutdown: %v", err)
	}

	if _, err := os.Stat(spoolPath); !os.IsNotExist(err) {
		t.Errorf("spool file not cleared after drain (stat err = %v)", err)
	}
	key, _ := os.ReadFile(filepath.Join(home, "audit.key"))
	l, err := audit.NewLog(filepath.Join(home, "log"), key)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	var buf bytes.Buffer
	if err := l.Export(&buf, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"session_id":"spooled"`) {
		t.Errorf("spooled event not folded into log; log = %s", buf.String())
	}
}
