package svc

import (
	"fmt"
	"path/filepath"

	"github.com/Hypership-Software/aftcast/internal/audit"
	"github.com/Hypership-Software/aftcast/internal/telemetry"
)

// OpenReadModel resolves the gate home, opens the audit log, and folds it into a
// fresh in-memory read-model. The store is a throwaway projection — never the
// daemon's on-disk file — so it is always current and never contends with the
// daemon's writer. The caller closes the returned store.
func OpenReadModel(home string) (*telemetry.Store, error) {
	home = resolveHome(home)
	key, err := loadOrCreateKey(filepath.Join(home, "audit.key"))
	if err != nil {
		return nil, fmt.Errorf("audit key: %w", err)
	}
	alog, err := audit.NewLog(filepath.Join(home, "log"), key)
	if err != nil {
		return nil, fmt.Errorf("open audit log: %w", err)
	}
	defer alog.Close()

	store, err := telemetry.OpenStore(":memory:")
	if err != nil {
		return nil, fmt.Errorf("open read-model: %w", err)
	}
	if err := store.Project(alog); err != nil {
		store.Close()
		return nil, fmt.Errorf("project read-model: %w", err)
	}
	return store, nil
}
