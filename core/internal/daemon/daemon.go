package daemon

import (
	"context"
	"encoding/json"
	"net"

	"github.com/Hypership-Software/atlas/internal/ipc"
)

// Serve accepts control-plane connections and handles one framed Request /
// Response exchange per connection (the shim dials, sends one event, reads the
// verdict, and closes). It returns when ctx is cancelled.
//
// A connection dropped without a response is the shim's fail-closed signal
// (Task 9: no verdict => exit 2 for pre_tool), so on any protocol error we close
// the connection rather than inventing a verdict.
func Serve(ctx context.Context, ln net.Listener, h *Handler) error {
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				return err
			}
		}
		go serveConn(conn, h)
	}
}

func serveConn(conn net.Conn, h *Handler) {
	defer conn.Close()
	raw, err := ipc.ReadFrame(conn)
	if err != nil {
		return
	}
	var req Request
	if err := json.Unmarshal(raw, &req); err != nil {
		return
	}
	resp, err := h.Handle(req)
	if err != nil {
		return
	}
	out, err := json.Marshal(resp)
	if err != nil {
		return
	}
	_ = ipc.WriteFrame(conn, out)
}
