//go:build !windows

package ipc

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"
)

// socketPath places the control socket under $XDG_RUNTIME_DIR (a per-user,
// tmpfs-backed runtime dir) when available, else a per-UID directory in the
// system temp dir.
func socketPath() string {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = filepath.Join(os.TempDir(), fmt.Sprintf("gated-%d", os.Getuid()))
	}
	return filepath.Join(dir, "gated"+instanceID()+".sock")
}

func platformListen() (net.Listener, error) {
	path := socketPath()
	// A 0700 directory owned by us is the operative access boundary: no other
	// non-root user can reach the socket. Same-user isolation is explicitly out
	// of the free-tier threat model (ADR-006), so a peer-cred UID check would add
	// little on top of this — left as a hardening follow-up.
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	// Clear a stale socket from a previous run so Listen doesn't fail with
	// "address already in use".
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		ln.Close()
		return nil, err
	}
	return ln, nil
}

func platformDial(timeout time.Duration) (net.Conn, error) {
	return net.DialTimeout("unix", socketPath(), timeout)
}
