//go:build !windows

package svc

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// acquireInstanceLock takes an exclusive flock on <dir>/daemon.lock, held for the
// daemon's lifetime and released by the OS on exit (so a crash leaves no stale
// lock). ok=false with a nil error means another daemon holds it. A non-nil error
// is a real filesystem failure.
func acquireInstanceLock(dir string) (release func(), ok bool, err error) {
	f, err := os.OpenFile(filepath.Join(dir, "daemon.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, false, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, false, nil
	}
	return func() { f.Close() }, true, nil
}

// spawnDetached launches the daemon in its own session (Setsid) with stdio to
// /dev/null, so it outlives the launching process and holds no terminal.
func spawnDetached(bin, home string) (int, error) {
	cmd := exec.Command(bin, "daemon", "run")
	cmd.Env = append(os.Environ(), "GATED_HOME="+home)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	devNull, err := os.OpenFile(os.DevNull, os.O_RDONLY, 0)
	if err != nil {
		return 0, err
	}
	logf, err := openDaemonLog(home)
	if err != nil {
		devNull.Close()
		return 0, err
	}
	cmd.Stdin = devNull
	cmd.Stdout, cmd.Stderr = logf, logf
	if err := cmd.Start(); err != nil {
		devNull.Close()
		logf.Close()
		return 0, err
	}
	pid := cmd.Process.Pid
	_ = cmd.Process.Release()
	devNull.Close()
	logf.Close()
	return pid, nil
}

// terminate asks the daemon to shut down cleanly, falling back to a hard kill.
func terminate(p *os.Process) error {
	if err := p.Signal(syscall.SIGTERM); err != nil {
		return p.Kill()
	}
	return nil
}
