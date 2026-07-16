package svc

import (
	"context"
	"os"
	"testing"
	"time"
)

// fakeSpawn replaces spawnDaemon with an in-process daemon (no real detached
// child, which is not portably unit-testable). It returns the current PID — the
// PID the in-process Run records — so Ensure's readiness match succeeds.
func fakeSpawn(t *testing.T) *bool {
	t.Helper()
	called := false
	var stop func()
	prev := spawnDaemon
	spawnDaemon = func(bin, home string) (int, error) {
		called = true
		ctx, cancel := context.WithCancel(context.Background())
		ready := make(chan Info, 1)
		done := make(chan struct{})
		go func() { _ = Run(ctx, Options{Home: home, IntegrityTick: -1, Ready: ready}); close(done) }()
		<-ready
		stop = func() { cancel(); <-done }
		return os.Getpid(), nil
	}
	t.Cleanup(func() {
		spawnDaemon = prev
		if stop != nil {
			stop()
		}
	})
	return &called
}

// TestEnsureStartsDaemonWhenDown: with no daemon up, Ensure spawns one, waits
// until it answers, and reports started=true with the port it bound.
func TestEnsureStartsDaemonWhenDown(t *testing.T) {
	t.Setenv("AFTCAST_IPC_ID", "svc-ensure-down")
	home := t.TempDir()
	fakeSpawn(t)

	info, started, err := Ensure(EnsureOptions{Home: home, WaitFor: 5 * time.Second})
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if !started {
		t.Error("started = false, want true (daemon was down)")
	}
	if info.HTTPPort == 0 {
		t.Errorf("Ensure returned no port: %+v", info)
	}
}

// TestEnsureNoopWhenAlreadyRunning: a daemon already up is detected and returned
// without spawning a second one.
func TestEnsureNoopWhenAlreadyRunning(t *testing.T) {
	t.Setenv("AFTCAST_IPC_ID", "svc-ensure-up")
	home := t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan Info, 1)
	errc := make(chan error, 1)
	go func() { errc <- Run(ctx, Options{Home: home, IntegrityTick: -1, Ready: ready}) }()
	<-ready
	t.Cleanup(func() { cancel(); <-errc })

	called := fakeSpawn(t)
	_, started, err := Ensure(EnsureOptions{Home: home, WaitFor: 2 * time.Second})
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if started || *called {
		t.Errorf("Ensure spawned a second daemon (started=%v spawned=%v)", started, *called)
	}
}
