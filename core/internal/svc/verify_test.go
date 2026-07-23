package svc

import (
	"path/filepath"
	"testing"

	"github.com/Hypership-Software/aftcast/internal/audit"
	"github.com/Hypership-Software/aftcast/internal/schema"
)

func TestVerifyLogCountsRecords(t *testing.T) {
	home := t.TempDir()
	key, err := loadOrCreateKey(filepath.Join(home, "audit.key"))
	if err != nil {
		t.Fatal(err)
	}
	alog, err := audit.NewLog(filepath.Join(home, "log"), key)
	if err != nil {
		t.Fatal(err)
	}
	if err := alog.Record(schema.TelemetryEvent{V: 1, SessionID: "s", EventType: schema.EventStop}); err != nil {
		t.Fatal(err)
	}
	alog.Close()

	rep, err := VerifyLog(home)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK || rep.Count != 1 {
		t.Errorf("report = %+v, want OK with 1 record", rep)
	}
}
