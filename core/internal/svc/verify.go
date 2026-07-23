package svc

import (
	"fmt"
	"path/filepath"

	"github.com/Hypership-Software/aftcast/internal/audit"
)

// VerifyLog replays the home's event log against its HMAC chain. The digest's
// attestation must come from this check, never from trusting the file.
func VerifyLog(home string) (audit.Report, error) {
	home = resolveHome(home)
	key, err := loadOrCreateKey(filepath.Join(home, "audit.key"))
	if err != nil {
		return audit.Report{}, fmt.Errorf("audit key: %w", err)
	}
	alog, err := audit.NewLog(filepath.Join(home, "log"), key)
	if err != nil {
		return audit.Report{}, fmt.Errorf("open audit log: %w", err)
	}
	defer alog.Close()
	return alog.Verify()
}
