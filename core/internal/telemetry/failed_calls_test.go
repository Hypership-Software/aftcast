package telemetry

import (
	"path/filepath"
	"testing"

	"github.com/Hypership-Software/atlas/internal/audit"
	"github.com/Hypership-Software/atlas/internal/schema"
)

func TestFailedCalls(t *testing.T) {
	dir := t.TempDir()
	key := []byte("0123456789abcdef0123456789abcdef")
	log, err := audit.NewLog(filepath.Join(dir, "log"), key)
	if err != nil {
		t.Fatalf("NewLog: %v", err)
	}
	defer log.Close()

	for _, e := range []schema.TelemetryEvent{
		{SessionID: "s1", EventType: schema.EventPostTool, ToolClass: schema.ClassExec,
			ToolRaw: "Bash", Verbs: []string{"cd"}, ToolOK: schema.OutcomeFailed, BashExitCode: 1},
		{SessionID: "s1", EventType: schema.EventPostTool, ToolClass: schema.ClassExec,
			ToolRaw: "Bash", Verbs: []string{"cd"}, ToolOK: schema.OutcomeOK},
		{SessionID: "s1", EventType: schema.EventPreTool, ToolClass: schema.ClassExec,
			ToolRaw: "Bash", Verbs: []string{"cd"}, ToolOK: schema.OutcomeFailed},
		{SessionID: "s2", EventType: schema.EventPreTool, ToolClass: schema.ClassFileRead,
			ToolRaw: "Read"},
		{SessionID: "s2", EventType: schema.EventPostTool, ToolClass: schema.ClassFileRead,
			ToolRaw: "Read", ToolOK: schema.OutcomeFailed},
	} {
		if err := log.Record(e); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}

	store, err := OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()
	if err := store.Project(log); err != nil {
		t.Fatalf("Project: %v", err)
	}

	failed, err := store.FailedCalls()
	if err != nil {
		t.Fatalf("FailedCalls: %v", err)
	}
	if len(failed) != 2 {
		t.Fatalf("want 2 failed post-tool calls, got %d", len(failed))
	}
	if failed[0].SessionID != "s1" || failed[0].BashExitCode != 1 {
		t.Fatalf("failed[0] = %+v, want the s1 exec failure with exit 1", failed[0])
	}
	if failed[1].SessionID != "s2" || failed[1].ToolRaw != "Read" {
		t.Fatalf("failed[1] = %+v, want the s2 read failure", failed[1])
	}
}
