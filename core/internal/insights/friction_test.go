package insights

import (
	"strings"
	"testing"
	"time"

	"github.com/Hypership-Software/atlas/internal/analytics"
	"github.com/Hypership-Software/atlas/internal/schema"
)

func frictionCluster(class schema.ToolClass, tool string, verbs []string, exit, failures, sessions, days int) analytics.FrictionCluster {
	sess := make([]analytics.SessionFailures, sessions)
	for i := range sess {
		sess[i] = analytics.SessionFailures{SessionID: strings.Repeat("a", 4) + string(rune('0'+i))}
	}
	return analytics.FrictionCluster{
		ToolClass: class, ToolName: tool, Verbs: verbs, ExitCode: exit,
		Failures: failures, Sessions: sess, Days: days,
	}
}

func TestDescribeFriction(t *testing.T) {
	cases := []struct {
		cluster analytics.FrictionCluster
		want    string
	}{
		{frictionCluster(schema.ClassExec, "Bash", []string{"cd"}, 1, 20, 4, 3), "failed to change directory"},
		{frictionCluster(schema.ClassExec, "Bash", []string{"go"}, 1, 3, 3, 2), "had go commands fail"},
		{frictionCluster(schema.ClassFileRead, "Read", nil, 0, 7, 3, 2), "had file reads fail"},
		{frictionCluster(schema.ClassFileWrite, "Edit", nil, 0, 3, 3, 2), "had file edits fail"},
		{frictionCluster(schema.ClassNetFetch, "WebFetch", nil, 0, 3, 3, 2), "had web fetches fail"},
		{frictionCluster(schema.ClassMCP, "mcp__context7__resolve-library-id", nil, 0, 4, 3, 2), "had context7 connector calls fail"},
		{frictionCluster(schema.ToolClass("other"), "Grep", nil, 0, 3, 3, 2), "had Grep calls fail"},
	}
	for _, c := range cases {
		if got := describeFriction(c.cluster); got != c.want {
			t.Errorf("describeFriction = %q, want %q", got, c.want)
		}
	}
}

func TestRenderFrictionEmpty(t *testing.T) {
	if got := renderFriction(nil); got != "" {
		t.Fatalf("renderFriction(nil) = %q, want empty", got)
	}
}

func TestRenderFrictionCard(t *testing.T) {
	out := renderFriction([]analytics.FrictionCluster{
		frictionCluster(schema.ClassExec, "Bash", []string{"cd"}, 1, 20, 4, 3),
	})
	for _, want := range []string{
		"Worth a permanent fix · across your projects",
		"Your agents failed to change directory 20 times across 4 sessions on 3 days this week.",
		"gated coach export cd-exit-1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("card missing %q in:\n%s", want, out)
		}
	}
	for _, banned := range []string{"exec", "tool_class", "post_tool"} {
		if strings.Contains(out, banned) {
			t.Errorf("card leaks internal register %q in:\n%s", banned, out)
		}
	}
}

func TestRenderFrictionCapsAtTwo(t *testing.T) {
	out := renderFriction([]analytics.FrictionCluster{
		frictionCluster(schema.ClassExec, "Bash", []string{"cd"}, 1, 20, 4, 3),
		frictionCluster(schema.ClassFileRead, "Read", nil, 0, 7, 3, 2),
		frictionCluster(schema.ClassExec, "Bash", []string{"go"}, 1, 5, 3, 2),
	})
	if !strings.Contains(out, "failed to change directory") || !strings.Contains(out, "had file reads fail") {
		t.Fatalf("top two clusters missing:\n%s", out)
	}
	if strings.Contains(out, "had go commands fail") {
		t.Fatalf("third cluster should be behind the more-hint:\n%s", out)
	}
	if !strings.Contains(out, "1 more worth a look · gated coach") {
		t.Fatalf("more-hint missing:\n%s", out)
	}
}

func TestFrictionWindowKeepsRecentParseableFailures(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	events := []schema.TelemetryEvent{
		{TS: "2026-07-14T09:00:00Z", SessionID: "s1"},
		{TS: "2026-07-01T09:00:00Z", SessionID: "s2"},
		{TS: "", SessionID: "s3"},
		{TS: "not-a-time", SessionID: "s4"},
	}
	got := frictionWindow(events, now)
	if len(got) != 1 || got[0].SessionID != "s1" {
		t.Fatalf("frictionWindow kept %d events, want only the recent parseable one", len(got))
	}
}

func TestBuildComputesWorthFixingFriction(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	fail := func(session, ts string) schema.TelemetryEvent {
		return schema.TelemetryEvent{
			TS: ts, SessionID: session, EventType: schema.EventPostTool,
			ToolClass: schema.ClassExec, ToolRaw: "Bash", Verbs: []string{"cd"},
			ToolOK: schema.OutcomeFailed, BashExitCode: 1,
		}
	}
	failures := []schema.TelemetryEvent{
		fail("s1", "2026-07-13T09:00:00Z"),
		fail("s2", "2026-07-14T09:00:00Z"),
		fail("s3", "2026-07-15T09:00:00Z"),
	}
	m := build(nil, Scope{StartGlobal: true}, nil, failures, now)
	if len(m.friction) != 1 {
		t.Fatalf("friction clusters = %d, want 1", len(m.friction))
	}
	if m.friction[0].Slug() != "cd-exit-1" {
		t.Fatalf("friction slug = %q", m.friction[0].Slug())
	}
}
