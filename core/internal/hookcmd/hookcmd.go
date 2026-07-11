// Package hookcmd is the `gated hook` shim. HTTP hooks are the per-call hot path
// (ADR-001); this shim survives only for the SessionStart command-hook fallback,
// since SessionStart does not fire over HTTP in 2.1.205. Pure observation — it
// records via the daemon (spooling if it is down) and never blocks or emits a
// hook decision.
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

// Run always exits 0: a telemetry shim must never disrupt the session. The event
// is spooled if the daemon is unreachable.
func Run(harness string, stdin io.Reader, stdout, stderr io.Writer) int {
	a, ok := adapter.Get(harness)
	if !ok {
		fmt.Fprintf(stderr, "gated: unknown harness %q\n", harness)
		return 0
	}
	raw, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintf(stderr, "gated: read hook payload: %v\n", err)
		return 0
	}
	desc, ev, err := a.Normalize("", raw)
	if err != nil {
		fmt.Fprintf(stderr, "gated: normalize hook payload: %v\n", err)
		return 0
	}

	if derr := deliver(daemon.Request{Event: ev, Descriptor: desc}); derr != nil {
		if serr := spool(ev); serr != nil {
			fmt.Fprintf(stderr, "gated: spool telemetry: %v\n", serr)
		}
	}
	return 0
}

// deliver sends the event to the daemon. The reply is a classification we don't
// act on (Atlas does not gate), so the ack is read and discarded.
func deliver(req daemon.Request) error {
	conn, err := ipc.Dial(dialTimeout)
	if err != nil {
		return err
	}
	defer conn.Close()

	reqRaw, err := json.Marshal(req)
	if err != nil {
		return err
	}
	if err := ipc.WriteFrame(conn, reqRaw); err != nil {
		return err
	}
	_, _ = ipc.ReadFrame(conn)
	return nil
}

// Live reports whether the daemon is reachable. HTTP hooks fail open, so a down
// daemon means sessions run UNOBSERVED — that gap must be detectable, not silent.
func Live(timeout time.Duration) bool {
	conn, err := ipc.Dial(timeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

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
