package svc

import (
	"path/filepath"
	"testing"

	"github.com/Hypership-Software/aftcast/internal/audit"
	"github.com/Hypership-Software/aftcast/internal/schema"
)

func TestOpenReadModelFoldsLog(t *testing.T) {
	home := t.TempDir()
	key, err := loadOrCreateKey(filepath.Join(home, "audit.key"))
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	alog, err := audit.NewLog(filepath.Join(home, "log"), key)
	if err != nil {
		t.Fatalf("NewLog: %v", err)
	}
	if err := alog.Record(schema.TelemetryEvent{SessionID: "s1", EventType: schema.EventUserPrompt}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	alog.Close()

	store, err := OpenReadModel(home)
	if err != nil {
		t.Fatalf("OpenReadModel: %v", err)
	}
	defer store.Close()
	sessions, err := store.Sessions()
	if err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	if len(sessions) != 1 || sessions[0].SessionID != "s1" {
		t.Fatalf("want 1 session s1, got %+v", sessions)
	}
}
