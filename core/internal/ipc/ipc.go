// Package ipc provides the gate's two local transports: a control-plane stream
// (Unix socket / Windows named pipe) for CLI<->daemon commands and the Task 9
// shim, and a localhost HTTP listener that receives harness hooks — the Rev-4
// transport the Sprint-0 spike measured and validated. Both are local-only.
package ipc

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"
)

// MaxFrame bounds a single control-plane message — a guard against a hostile or
// buggy peer. 1 MiB is far above any real descriptor or telemetry event.
const MaxFrame = 1 << 20

// WriteFrame writes b as a length-prefixed frame: a 4-byte big-endian length
// followed by the payload.
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

// ReadFrame reads one length-prefixed frame written by WriteFrame.
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

// Listen creates the control-plane listener (Unix socket / Windows named pipe),
// restricted to the current user.
func Listen() (net.Listener, error) { return platformListen() }

// Dial connects to the control-plane endpoint, timing out after timeout.
func Dial(timeout time.Duration) (net.Conn, error) { return platformDial(timeout) }

// DefaultHTTPPort is the fixed localhost port baked into the harness hook
// settings. It must stay stable across installs; HTTPListen falls back to a
// nearby port only if it is already taken, and `gated doctor` reconciles the
// written settings with the port actually bound.
const DefaultHTTPPort = 47100

// httpPortScan is how many ports past the preferred one HTTPListen will try.
const httpPortScan = 16

// HTTPListen binds a localhost-only TCP listener for the hook transport. It
// prefers the given port (0 means DefaultHTTPPort) and falls back to the next
// few ports if it is in use, returning the listener and the port actually bound.
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
