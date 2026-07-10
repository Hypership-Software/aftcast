// Package hookcmd implements the `gated hook` shim. In production the per-call
// hot path is HTTP hooks straight to the daemon (Rev 4); this shim survives for
// the once-per-session SessionStart command-hook fallback (SessionStart does not
// fire over HTTP in 2.1.205) and an optional per-call hybrid where an org
// contractually requires fail-closed. It also exposes the daemon-liveness check
// the watchdog relies on.
package hookcmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/Hypership-Software/atlas/internal/adapter"
	"github.com/Hypership-Software/atlas/internal/daemon"
	"github.com/Hypership-Software/atlas/internal/ipc"
	"github.com/Hypership-Software/atlas/internal/schema"
)

const dialTimeout = 2 * time.Second

// Run reads a hook payload from stdin, asks the daemon for a verdict over the
// control-plane transport, and writes the harness hook response to stdout.
//
// Fail mode is PER EVENT (Rev 2.1): a PreToolUse with an unreachable daemon
// fails CLOSED (exit 2, no verdict). Every other event fails QUIET (exit 0)
// after spooling its telemetry for later ingestion — exit 2 on
// UserPromptSubmit/Stop would erase the prompt or force the agent to keep
// running, both safety inversions. A deny verdict is exit 0 plus deny JSON
// (Claude Code honors a deny only via exit-0 + JSON; exit 2 is the no-verdict
// fail-closed path).
func Run(harness string, stdin io.Reader, stdout, stderr io.Writer) int {
	a, ok := adapter.Get(harness)
	if !ok {
		fmt.Fprintf(stderr, "gated: unknown harness %q\n", harness)
		return 2
	}
	raw, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintf(stderr, "gated: read hook payload: %v\n", err)
		return 2
	}
	desc, ev, err := a.Normalize("", raw)
	if err != nil {
		fmt.Fprintf(stderr, "gated: normalize hook payload: %v\n", err)
		return 2
	}

	resp, err := ask(daemon.Request{Event: ev, Descriptor: desc})
	if err != nil {
		return daemonDown(ev, stderr)
	}

	// Only gating events need a hook-response body; observations return exit 0.
	if ev.EventType == schema.EventPreTool {
		out, rerr := a.Respond(resp.Verdict, resp.Reason)
		if rerr != nil {
			fmt.Fprintf(stderr, "gated: render response: %v\n", rerr)
			return 2
		}
		_, _ = stdout.Write(out)
	}
	return 0
}

func ask(req daemon.Request) (daemon.Response, error) {
	conn, err := ipc.Dial(dialTimeout)
	if err != nil {
		return daemon.Response{}, err
	}
	defer conn.Close()

	reqRaw, err := json.Marshal(req)
	if err != nil {
		return daemon.Response{}, err
	}
	if err := ipc.WriteFrame(conn, reqRaw); err != nil {
		return daemon.Response{}, err
	}
	respRaw, err := ipc.ReadFrame(conn)
	if err != nil {
		return daemon.Response{}, err
	}
	var resp daemon.Response
	if err := json.Unmarshal(respRaw, &resp); err != nil {
		return daemon.Response{}, err
	}
	return resp, nil
}

func daemonDown(ev schema.TelemetryEvent, stderr io.Writer) int {
	if ev.EventType == schema.EventPreTool {
		fmt.Fprintln(stderr, "gated: gate daemon unreachable — blocking this action (fail-closed). Run `gated doctor`.")
		return 2
	}
	if err := spool(ev); err != nil {
		fmt.Fprintf(stderr, "gated: spool telemetry: %v\n", err)
	}
	return 0
}

// Live reports whether the daemon is reachable on the control-plane transport.
// The daemon-liveness watchdog (surfaced by `gated status`/insights) uses this:
// HTTP hooks fail OPEN, so a down daemon means sessions run UNGATED, and that
// gap must be detectable rather than silent.
func Live(timeout time.Duration) bool {
	conn, err := ipc.Dial(timeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// spool appends a telemetry event to the on-disk buffer the daemon drains on
// startup, so an event isn't lost while the daemon is down.
func spool(ev schema.TelemetryEvent) error {
	dir := spoolDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	line, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, "spool.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(line, '\n'))
	return err
}

func spoolDir() string {
	if home := os.Getenv("GATED_HOME"); home != "" {
		return filepath.Join(home, "spool")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".gated", "spool")
}
