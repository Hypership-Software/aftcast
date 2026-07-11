package svc

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// errorSharingViolation is Win32 ERROR_SHARING_VIOLATION — CreateFile with no
// share mode failing because another handle already holds the file exclusively.
const errorSharingViolation = syscall.Errno(32)

// acquireInstanceLock opens <dir>/daemon.lock with an exclusive (no-share) handle,
// held for the daemon's lifetime and released by the OS on exit (so a crash leaves
// no stale lock). ok=false with a nil error means another daemon holds it.
func acquireInstanceLock(dir string) (release func(), ok bool, err error) {
	p, err := syscall.UTF16PtrFromString(filepath.Join(dir, "daemon.lock"))
	if err != nil {
		return nil, false, err
	}
	h, err := syscall.CreateFile(p, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0, nil,
		syscall.OPEN_ALWAYS, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		if err == errorSharingViolation {
			return nil, false, nil
		}
		return nil, false, err
	}
	return func() { syscall.CloseHandle(h) }, true, nil
}

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
	logf, err := openDaemonLog(home)
	if err != nil {
		return 0, err
	}
	cmd.Stdout, cmd.Stderr = logf, logf
	if err := cmd.Start(); err != nil {
		logf.Close()
		return 0, err
	}
	pid := cmd.Process.Pid
	_ = cmd.Process.Release()
	logf.Close()
	return pid, nil
}

// terminate hard-kills the daemon. Windows has no SIGTERM; the audit log fsyncs
// each record, so a hard kill loses no recorded events.
func terminate(p *os.Process) error { return p.Kill() }
