// Package ipc provides the gate's two local transports: a control-plane stream
// (Unix socket / Windows named pipe) for CLI<->daemon commands and the shim, and
// a localhost HTTP listener for harness hooks. Both are local-only.
package ipc

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"time"
)

// instanceID isolates the control-plane endpoint via AFTCAST_IPC_ID. Empty in
// production; set by tests so parallel test binaries don't collide on the one
// fixed socket/pipe path.
func instanceID() string { return os.Getenv("AFTCAST_IPC_ID") }

// MaxFrame bounds a single control-plane message — a guard against a hostile or
// buggy peer. 1 MiB is far above any real descriptor or telemetry event.
const MaxFrame = 1 << 20

// WriteFrame writes b as a length-prefixed frame (4-byte big-endian length, then
// payload).
func WriteFrame(w io.Writer, b []byte) error {
	if len(b) > MaxFrame {
		return fmt.Errorf("ipc: frame too large: %d bytes (max %d)", len(b), MaxFrame)
	}
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(b)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err := w.Write(b)
	return err
}

func ReadFrame(r io.Reader) ([]byte, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n > MaxFrame {
		return nil, fmt.Errorf("ipc: frame too large: %d bytes (max %d)", n, MaxFrame)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// Listen creates the control-plane listener, restricted to the current user.
func Listen() (net.Listener, error) { return platformListen() }

func Dial(timeout time.Duration) (net.Conn, error) { return platformDial(timeout) }

// DefaultHTTPPort is the fixed localhost port baked into the harness hook
// settings; it must stay stable across installs. HTTPListen falls back to a
// nearby port only if it is taken, and `aftcast doctor` reconciles settings with
// the bound port.
const DefaultHTTPPort = 47100

// httpPortScan is how many ports past the preferred one HTTPListen will try.
const httpPortScan = 16

// HTTPListen binds a localhost-only TCP listener for hooks. It prefers the given
// port (0 => DefaultHTTPPort), falling back to the next few ports if it is in
// use, and returns the port actually bound.
func HTTPListen(preferred int) (net.Listener, int, error) {
	if preferred == 0 {
		preferred = DefaultHTTPPort
	}
	var lastErr error
	for port := preferred; port < preferred+httpPortScan; port++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			return ln, port, nil
		}
		lastErr = err
	}
	return nil, 0, fmt.Errorf("ipc: no free localhost port in [%d,%d): %w", preferred, preferred+httpPortScan, lastErr)
}
