package daemon

import (
	"context"
	"encoding/json"
	"net"

	"github.com/Hypership-Software/atlas/internal/ipc"
)

// Serve accepts control-plane connections, one framed Request/Response exchange
// each, until ctx is cancelled. Atlas observes rather than gates, so a dropped
// connection is not a failure to guard against — on any protocol error we just
// close the connection.
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
