package integrity

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Hypership-Software/aftcast/internal/schema"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestNoDriftWhenIntact(t *testing.T) {
	dir := t.TempDir()
	settings := filepath.Join(dir, "settings.json")
	writeFile(t, settings, `{"hooks":{"PreToolUse":[{"hooks":[{"type":"http","url":"http://127.0.0.1:47100/hook"}]}]}}`)
	bin := filepath.Join(dir, "aftcast")
	writeFile(t, bin, "binary-v1")
	hash, err := hashFile(bin)
	if err != nil {
		t.Fatal(err)
	}

	c := NewChecker(Config{SettingsPath: settings, HookMarker: "127.0.0.1:47100", BinaryPath: bin, BinaryHash: hash})
	if d := c.Check(); len(d) != 0 {
		t.Fatalf("expected no drift, got %+v", d)
	}
}

func TestDriftWhenHookEntryMissing(t *testing.T) {
	dir := t.TempDir()
	settings := filepath.Join(dir, "settings.json")
	writeFile(t, settings, `{"hooks":{}}`)

	d := NewChecker(Config{SettingsPath: settings, HookMarker: "127.0.0.1:47100"}).Check()
	if len(d) != 1 || d[0].Kind != DriftHookMissing {
		t.Fatalf("expected hook_missing drift, got %+v", d)
	}
}

func TestDriftWhenSettingsUnreadable(t *testing.T) {
	d := NewChecker(Config{SettingsPath: filepath.Join(t.TempDir(), "does-not-exist.json"), HookMarker: "x"}).Check()
	if len(d) != 1 || d[0].Kind != DriftHookMissing {
		t.Fatalf("expected hook_missing drift for a missing settings file, got %+v", d)
	}
}

func TestDriftWhenBinaryAltered(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "aftcast")
	writeFile(t, bin, "binary-v1")
	original, err := hashFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	// swap the binary out from under the recorded manifest hash
	writeFile(t, bin, "binary-v2-tampered")

	d := NewChecker(Config{BinaryPath: bin, BinaryHash: original}).Check()
	if len(d) != 1 || d[0].Kind != DriftBinaryChanged {
		t.Fatalf("expected binary_changed drift, got %+v", d)
	}
}

func TestDriftEventShape(t *testing.T) {
	ev := DriftEvent(Drift{Kind: DriftBinaryChanged, Detail: "..."})
	if ev.EventType != schema.EventIntegrity {
		t.Errorf("event type = %v, want integrity", ev.EventType)
	}
	if ev.RuleID != string(DriftBinaryChanged) {
		t.Errorf("rule_id = %q, want %q", ev.RuleID, DriftBinaryChanged)
	}
}
