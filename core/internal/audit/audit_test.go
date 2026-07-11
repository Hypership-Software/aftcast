package audit

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Hypership-Software/atlas/internal/schema"
)

var testKey = []byte("test-hmac-key-0123456789")

func ev(session string, ts string) schema.TelemetryEvent {
	return schema.TelemetryEvent{
		SessionID: session,
		Harness:   "claudecode",
		EventType: schema.EventPreTool,
		ToolClass: schema.ClassExec,
		ToolRaw:   "Bash",
		Risk:      schema.RiskSafe,
		TS:        ts,
	}
}

func TestRecordThenVerifyOK(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLog(dir, testKey)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	for i := 0; i < 3; i++ {
		if err := l.Record(ev("s1", "2026-07-10T12:00:0"+string(rune('0'+i))+"Z")); err != nil {
			t.Fatal(err)
		}
	}
	rep, err := l.Verify()
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK || rep.Count != 3 {
		t.Fatalf("verify = %+v, want OK with count 3", rep)
	}
}

func TestVerifyDetectsTamper(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLog(dir, testKey)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	for i := 0; i < 3; i++ {
		if err := l.Record(ev("s1", "2026-07-10T12:00:0"+string(rune('0'+i))+"Z")); err != nil {
			t.Fatal(err)
		}
	}

	// Tamper the middle record's content on disk, keeping it valid JSON.
	path := filepath.Join(dir, eventsFile)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	var mid schema.TelemetryEvent
	if err := json.Unmarshal([]byte(lines[1]), &mid); err != nil {
		t.Fatal(err)
	}
	mid.ToolRaw = "TAMPERED"
	tampered, _ := json.Marshal(mid)
	lines[1] = string(tampered)
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	rep, err := l.Verify()
	if err != nil {
		t.Fatal(err)
	}
	if rep.OK {
		t.Fatal("verify passed on a tampered log")
	}
	if rep.BadSeq != 2 {
		t.Errorf("bad seq = %d, want 2 (the tampered record)", rep.BadSeq)
	}
}

func TestExportFiltersBySince(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLog(dir, testKey)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	_ = l.Record(ev("old", "2026-07-01T00:00:00Z"))
	_ = l.Record(ev("new", "2026-07-10T00:00:00Z"))

	var buf bytes.Buffer
	since, _ := time.Parse(time.RFC3339, "2026-07-05T00:00:00Z")
	if err := l.Export(&buf, since); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, `"old"`) {
		t.Error("export included a record before `since`")
	}
	if !strings.Contains(out, `"new"`) {
		t.Error("export dropped a record at/after `since`")
	}
}

func TestChainContinuesAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	l1, err := NewLog(dir, testKey)
	if err != nil {
		t.Fatal(err)
	}
	_ = l1.Record(ev("s1", "2026-07-10T12:00:00Z"))
	_ = l1.Record(ev("s1", "2026-07-10T12:00:01Z"))
	if err := l1.Close(); err != nil {
		t.Fatal(err)
	}

	l2, err := NewLog(dir, testKey)
	if err != nil {
		t.Fatal(err)
	}
	defer l2.Close()
	if err := l2.Record(ev("s1", "2026-07-10T12:00:02Z")); err != nil {
		t.Fatal(err)
	}
	rep, err := l2.Verify()
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK || rep.Count != 3 {
		t.Fatalf("verify after reopen = %+v, want OK count 3 (chain continued)", rep)
	}
}

func TestEmptyKeyRejected(t *testing.T) {
	if _, err := NewLog(t.TempDir(), nil); err == nil {
		t.Fatal("expected NewLog to reject an empty HMAC key")
	}
}

func TestRecordStampsTimestampWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLog(dir, testKey)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	// Record an event with no timestamp (as the adapter currently produces).
	if err := l.Record(schema.TelemetryEvent{SessionID: "s1", EventType: schema.EventPreTool}); err != nil {
		t.Fatal(err)
	}
	evs, err := l.Events()
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 {
		t.Fatalf("got %d events, want 1", len(evs))
	}
	if evs[0].TS == "" {
		t.Fatal("Record left ts empty; it must stamp a capture time")
	}
	if _, err := time.Parse(time.RFC3339Nano, evs[0].TS); err != nil {
		t.Errorf("stamped ts %q is not RFC3339Nano: %v", evs[0].TS, err)
	}
}

func TestRecordPreservesProvidedTimestamp(t *testing.T) {
	dir := t.TempDir()
	l, _ := NewLog(dir, testKey)
	defer l.Close()
	want := "2026-07-01T00:00:00Z"
	if err := l.Record(schema.TelemetryEvent{SessionID: "s1", EventType: schema.EventPreTool, TS: want}); err != nil {
		t.Fatal(err)
	}
	evs, _ := l.Events()
	if evs[0].TS != want {
		t.Errorf("ts = %q, want %q (provided ts must not be overwritten)", evs[0].TS, want)
	}
}

func TestRecordStampsIdentityWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	l, _ := NewLog(dir, testKey)
	defer l.Close()
	l.SetIdentity("kyle", "devbox")
	// event with empty user/host gets the daemon identity...
	_ = l.Record(schema.TelemetryEvent{SessionID: "s1", EventType: schema.EventPreTool})
	// ...but an event that already carries them is left alone.
	_ = l.Record(schema.TelemetryEvent{SessionID: "s2", EventType: schema.EventPreTool, User: "other", Host: "elsewhere"})
	evs, _ := l.Events()
	if evs[0].User != "kyle" || evs[0].Host != "devbox" {
		t.Errorf("stamped identity = {%q,%q}, want {kyle,devbox}", evs[0].User, evs[0].Host)
	}
	if evs[1].User != "other" || evs[1].Host != "elsewhere" {
		t.Errorf("overwrote provided identity = {%q,%q}, want {other,elsewhere}", evs[1].User, evs[1].Host)
	}
}

func TestEventsReturnsAllInOrder(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLog(dir, testKey)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	_ = l.Record(ev("s1", "2026-07-10T12:00:00Z"))
	_ = l.Record(ev("s2", "2026-07-10T12:00:01Z"))
	_ = l.Record(ev("s1", "2026-07-10T12:00:02Z"))

	evs, err := l.Events()
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 3 {
		t.Fatalf("Events() returned %d, want 3", len(evs))
	}
	for i, e := range evs {
		if e.Seq != uint64(i+1) {
			t.Errorf("event %d has seq %d, want %d", i, e.Seq, i+1)
		}
	}
	if evs[0].SessionID != "s1" || evs[1].SessionID != "s2" || evs[2].SessionID != "s1" {
		t.Errorf("session order = %q,%q,%q", evs[0].SessionID, evs[1].SessionID, evs[2].SessionID)
	}
}

func TestEventsEmptyLog(t *testing.T) {
	l, err := NewLog(t.TempDir(), testKey)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	evs, err := l.Events()
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 0 {
		t.Fatalf("empty log Events() = %d, want 0", len(evs))
	}
}
