package taint

import (
	"testing"

	"github.com/Hypership-Software/atlas/internal/schema"
)

func fetch(session, domain string) schema.Descriptor {
	return schema.Descriptor{SessionID: session, ToolClass: schema.ClassNetFetch, Domain: domain}
}

func TestLocalReadDoesNotTaint(t *testing.T) {
	l := NewLedger(nil)
	l.MarkFromResult("s1", schema.Descriptor{SessionID: "s1", ToolClass: schema.ClassFileRead, Files: []string{"/p/main.go"}})
	if l.IsTainted("s1") {
		t.Fatal("a local file read must not taint the session")
	}
}

func TestUntrustedFetchTaints(t *testing.T) {
	l := NewLedger([]string{"internal.example.com"})
	l.MarkFromResult("s1", fetch("s1", "evil.example.com"))
	if !l.IsTainted("s1") {
		t.Fatal("a fetch to an untrusted domain must taint the session")
	}
}

func TestTrustedFetchDoesNotTaint(t *testing.T) {
	l := NewLedger([]string{"internal.example.com"})
	l.MarkFromResult("s1", fetch("s1", "internal.example.com"))
	if l.IsTainted("s1") {
		t.Fatal("a fetch to a trusted domain must not taint the session")
	}
}

func TestMCPTaints(t *testing.T) {
	l := NewLedger(nil)
	l.MarkFromResult("s1", schema.Descriptor{SessionID: "s1", ToolClass: schema.ClassMCP, MCPServer: "github", MCPTool: "read_issue"})
	if !l.IsTainted("s1") {
		t.Fatal("an MCP call must taint (content provenance unverifiable)")
	}
}

func TestApplyReflectsStoredTaint(t *testing.T) {
	l := NewLedger(nil)
	l.MarkFromResult("s1", fetch("s1", "evil.example.com"))

	d := schema.Descriptor{SessionID: "s1", ToolClass: schema.ClassExec}
	l.Apply(&d)
	if !d.Tainted {
		t.Fatal("Apply must inject the session's stored taint into the descriptor")
	}

	clean := schema.Descriptor{SessionID: "other", ToolClass: schema.ClassExec}
	l.Apply(&clean)
	if clean.Tainted {
		t.Fatal("Apply must not taint a different, clean session")
	}
}

func TestRebuildFromLogReconstructsTaint(t *testing.T) {
	l := NewLedger([]string{"internal.example.com"})
	events := []schema.TelemetryEvent{
		{SessionID: "s1", ToolClass: schema.ClassNetFetch, Domain: "evil.example.com", Verdict: schema.VerdictAllow},
		{SessionID: "s2", ToolClass: schema.ClassNetFetch, Domain: "internal.example.com", Verdict: schema.VerdictAllow},
		// a denied fetch never ran, so it must not taint
		{SessionID: "s3", ToolClass: schema.ClassNetFetch, Domain: "evil.example.com", Verdict: schema.VerdictDeny},
	}
	l.Rebuild(events)
	if !l.IsTainted("s1") {
		t.Error("s1 (allowed untrusted fetch) should be tainted after rebuild")
	}
	if l.IsTainted("s2") {
		t.Error("s2 (trusted fetch) should not be tainted")
	}
	if l.IsTainted("s3") {
		t.Error("s3 (denied fetch) should not be tainted — it never ran")
	}
}
