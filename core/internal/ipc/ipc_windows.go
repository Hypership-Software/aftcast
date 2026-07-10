//go:build windows

package ipc

import (
	"fmt"
	"net"
	"os/user"
	"time"

	winio "github.com/Microsoft/go-winio"
)

const pipePath = `\\.\pipe\gated`

func platformListen() (net.Listener, error) {
	sddl, err := ownerOnlySDDL()
	if err != nil {
		return nil, err
	}
	return winio.ListenPipe(pipePath, &winio.PipeConfig{SecurityDescriptor: sddl})
}

func platformDial(timeout time.Duration) (net.Conn, error) {
	return winio.DialPipe(pipePath, &timeout)
}

// ownerOnlySDDL grants full access to only the current user's SID, protected
// from inheritance, so no other account can open the control pipe. On Windows
// os/user reports the account SID as Uid.
func ownerOnlySDDL() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("ipc: resolve current user SID: %w", err)
	}
	return fmt.Sprintf("D:P(A;;GA;;;%s)", u.Uid), nil
}
