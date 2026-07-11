package svc

import (
	"os"
	"os/exec"
	"syscall"
)

// Win32 process-creation flags: DETACHED_PROCESS gives the child no console (so
// it never inherits or holds the launching terminal), CREATE_NEW_PROCESS_GROUP
// isolates it from the parent's Ctrl-C. Values are the stable Win32 constants.
const (
	detachedProcess       = 0x00000008
	createNewProcessGroup = 0x00000200
)

func spawnDetached(bin, home string) (int, error) {
	cmd := exec.Command(bin, "daemon", "run")
	cmd.Env = append(os.Environ(), "GATED_HOME="+home)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: detachedProcess | createNewProcessGroup,
		HideWindow:    true,
	}
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	pid := cmd.Process.Pid
	_ = cmd.Process.Release()
	return pid, nil
}

// terminate hard-kills the daemon. Windows has no SIGTERM; the audit log fsyncs
// each record, so a hard kill loses no recorded events.
func terminate(p *os.Process) error { return p.Kill() }
