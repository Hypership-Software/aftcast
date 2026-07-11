package svc_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Hypership-Software/atlas/internal/svc"
)

// TestRunSecondInstanceForSameHomeExitsClean: two daemons for one home would both
// append to the same HMAC log and corrupt the chain — the risk when multiple
// terminal tabs each fire SessionStart with the daemon down. The second instance
// must refuse (single-instance lock) and exit cleanly without serving, leaving the
// first daemon healthy.
func TestRunSecondInstanceForSameHomeExitsClean(t *testing.T) {
	home := t.TempDir()
	_, cancelA, errcA := startDaemon(t, "svc-single-a", svc.Options{Home: home})
	t.Cleanup(func() { cancelA(); <-errcA })

	// A different control-plane id isolates the test to the home lock, not the pipe.
	t.Setenv("GATED_IPC_ID", "svc-single-b")
	ctxB, cancelB := context.WithCancel(context.Background())
	defer cancelB()
	readyB := make(chan svc.Info, 1)
	errcB := make(chan error, 1)
	go func() { errcB <- svc.Run(ctxB, svc.Options{Home: home, IntegrityTick: -1, Ready: readyB}) }()

	select {
	case err := <-errcB:
		if err != nil {
			t.Fatalf("second instance errored, want a clean nil exit: %v", err)
		}
	case <-readyB:
		t.Fatal("second instance became ready; expected it to refuse (single-instance)")
	case <-time.After(5 * time.Second):
		t.Fatal("second instance neither refused nor errored within 5s")
	}

	if _, ok := svc.Running(home); !ok {
		t.Error("first daemon no longer running after the second instance attempt")
	}
}

// writeTestDaemonFile writes a daemon.json recording pid, mirroring what a live
// daemon writes, so Stop has a record to act on.
func writeTestDaemonFile(t *testing.T, home string, pid int) {
	t.Helper()
	body := fmt.Sprintf(`{"pid":%d,"http_port":47100,"http_url":"http://127.0.0.1:47100/hook","policy_hash":"test"}`, pid)
	if err := os.WriteFile(filepath.Join(home, "daemon.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestMain(m *testing.M) {
	// Re-exec support for TestStop: a child launched with GATED_TEST_BLOCK blocks
	// forever so Stop has a real process to terminate.
	if os.Getenv("GATED_TEST_BLOCK") == "1" {
		select {}
	}
	os.Exit(m.Run())
}

// TestRunningReportsLiveDaemon: a daemon that is up is reported Running with the
// port it bound; once it shuts down, Running reports false even though the stale
// daemon.json still exists (liveness is a probe, not a file check).
func TestRunningReportsLiveDaemon(t *testing.T) {
	home := t.TempDir()
	info, cancel, errc := startDaemon(t, "svc-running", svc.Options{Home: home})

	got, ok := svc.Running(home)
	if !ok {
		t.Fatal("Running reported down while the daemon is up")
	}
	if got.HTTPPort != info.HTTPPort {
		t.Errorf("Running port = %d, want %d", got.HTTPPort, info.HTTPPort)
	}

	cancel()
	if err := <-errc; err != nil {
		t.Fatalf("Run returned error on shutdown: %v", err)
	}

	if _, ok := svc.Running(home); ok {
		t.Error("Running reported up after the daemon shut down")
	}
}

// TestStopTerminatesRecordedDaemon: Stop reads daemon.json, terminates that PID,
// and clears the record; with no record it is a no-op.
func TestStopTerminatesRecordedDaemon(t *testing.T) {
	home := t.TempDir()

	if stopped, err := svc.Stop(home); err != nil || stopped {
		t.Fatalf("Stop with no daemon.json = (%v, %v), want (false, nil)", stopped, err)
	}

	child := exec.Command(os.Args[0])
	child.Env = append(os.Environ(), "GATED_TEST_BLOCK=1")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	writeTestDaemonFile(t, home, child.Process.Pid)

	stopped, err := svc.Stop(home)
	if err != nil || !stopped {
		t.Fatalf("Stop = (%v, %v), want (true, nil)", stopped, err)
	}

	done := make(chan error, 1)
	go func() { done <- child.Wait() }()
	select {
	case <-done: // process exited — Stop worked
	case <-time.After(5 * time.Second):
		_ = child.Process.Kill()
		t.Fatal("Stop did not terminate the daemon within 5s")
	}

	if _, err := os.Stat(filepath.Join(home, "daemon.json")); !os.IsNotExist(err) {
		t.Errorf("daemon.json not cleared after Stop (stat err = %v)", err)
	}
}
