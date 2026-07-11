package svc

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"
)

const (
	defaultEnsureWait = 5 * time.Second
	probeTimeout      = 500 * time.Millisecond
)

// spawnDaemon launches a detached `<bin> daemon run` bound to home and returns
// its PID. The default is the platform detached spawn; tests replace it.
var spawnDaemon = spawnDetached

// EnsureOptions configures Ensure.
type EnsureOptions struct {
	Home    string        // gate state dir; "" resolves like Options.Home
	Bin     string        // gated binary path; "" => os.Executable()
	WaitFor time.Duration // readiness deadline; 0 => defaultEnsureWait
}

// Running reports the recorded daemon's Info and whether it is answering. A stale
// daemon.json whose port no longer accepts connections reads as not-running:
// liveness is a probe of the bound port, not the file's mere presence.
func Running(home string) (Info, bool) {
	df, err := readDaemonFile(resolveHome(home))
	if err != nil || !portOpen(df.HTTPPort) {
		return Info{}, false
	}
	return infoOf(df), true
}

// Ensure makes sure a daemon is running for home, starting one detached if none
// answers, and returns its Info. started reports whether Ensure launched it.
func Ensure(opts EnsureOptions) (info Info, started bool, err error) {
	home := resolveHome(opts.Home)
	if info, ok := Running(home); ok {
		return info, false, nil
	}
	bin := opts.Bin
	if bin == "" {
		exe, err := os.Executable()
		if err != nil {
			return Info{}, false, fmt.Errorf("locate gated binary: %w", err)
		}
		bin = exe
	}
	pid, err := spawnDaemon(bin, home)
	if err != nil {
		return Info{}, false, fmt.Errorf("start daemon: %w", err)
	}
	wait := opts.WaitFor
	if wait <= 0 {
		wait = defaultEnsureWait
	}
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		// Any live daemon for this home counts as ready, not specifically the pid we
		// spawned: under a multi-tab race a sibling may win the single-instance lock
		// and be the one serving, while ours exited cleanly.
		if info, ok := Running(home); ok {
			return info, true, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return Info{}, true, fmt.Errorf("daemon started (pid %d) but no daemon was ready within %s", pid, wait)
}

// Stop terminates the daemon recorded for home and clears its record. It reports
// whether a daemon was stopped; a missing record is a no-op.
func Stop(home string) (bool, error) {
	dir := resolveHome(home)
	df, err := readDaemonFile(dir)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if df.PID <= 0 {
		return false, nil
	}
	proc, err := os.FindProcess(df.PID)
	if err != nil {
		return false, fmt.Errorf("find daemon process %d: %w", df.PID, err)
	}
	if err := terminate(proc); err != nil {
		return false, fmt.Errorf("terminate daemon %d: %w", df.PID, err)
	}
	_ = os.Remove(filepath.Join(dir, "daemon.json"))
	return true, nil
}

func infoOf(df daemonFile) Info {
	return Info{HTTPPort: df.HTTPPort, HTTPURL: df.HTTPURL, PolicyHash: df.PolicyHash}
}

func readDaemonFile(dir string) (daemonFile, error) {
	b, err := os.ReadFile(filepath.Join(dir, "daemon.json"))
	if err != nil {
		return daemonFile{}, err
	}
	var df daemonFile
	if err := json.Unmarshal(b, &df); err != nil {
		return daemonFile{}, err
	}
	return df, nil
}

// openDaemonLog opens <home>/daemon.log for the detached daemon's stdout/stderr,
// so its diagnostics survive (a background process with no visible log is
// undebuggable). Appends across restarts; volume is low (a line or two per start).
func openDaemonLog(home string) (*os.File, error) {
	if err := os.MkdirAll(home, 0o700); err != nil {
		return nil, err
	}
	return os.OpenFile(filepath.Join(home, "daemon.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
}

func portOpen(port int) bool {
	if port <= 0 {
		return false
	}
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), probeTimeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
