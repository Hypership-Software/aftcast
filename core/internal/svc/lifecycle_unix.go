//go:build !windows

package svc

import (
	"os"
	"os/exec"
	"syscall"
)

// spawnDetached launches the daemon in its own session (Setsid) with stdio to
// /dev/null, so it outlives the launching process and holds no terminal.
func spawnDetached(bin, home string) (int, error) {
	cmd := exec.Command(bin, "daemon", "run")
	cmd.Env = append(os.Environ(), "GATED_HOME="+home)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return 0, err
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = devNull, devNull, devNull
	if err := cmd.Start(); err != nil {
		devNull.Close()
		return 0, err
	}
	pid := cmd.Process.Pid
	_ = cmd.Process.Release()
	devNull.Close()
	return pid, nil
}

// terminate asks the daemon to shut down cleanly, falling back to a hard kill.
func terminate(p *os.Process) error {
	if err := p.Signal(syscall.SIGTERM); err != nil {
		return p.Kill()
	}
	return nil
}
