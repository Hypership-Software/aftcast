package analytics

import (
	"testing"
	"time"

	"github.com/Hypership-Software/atlas/internal/schema"
)

func failedCall(session, project, ts string, class schema.ToolClass, tool string, verbs []string, exit int) schema.TelemetryEvent {
	return schema.TelemetryEvent{
		TS:           ts,
		SessionID:    session,
		Project:      project,
		EventType:    schema.EventPostTool,
		ToolClass:    class,
		ToolRaw:      tool,
		Verbs:        verbs,
		ToolOK:       schema.OutcomeFailed,
		BashExitCode: exit,
	}
}

func okCall(session, ts string) schema.TelemetryEvent {
	return schema.TelemetryEvent{
		TS:        ts,
		SessionID: session,
		EventType: schema.EventPostTool,
		ToolClass: schema.ClassExec,
		ToolRaw:   "Bash",
		Verbs:     []string{"cd"},
		ToolOK:    schema.OutcomeOK,
	}
}

func TestFrictionClustersGroupsFailedExecByVerbsAndExit(t *testing.T) {
	events := []schema.TelemetryEvent{
		failedCall("s1", "p1", "2026-07-13T09:00:00Z", schema.ClassExec, "Bash", []string{"cd"}, 1),
		failedCall("s1", "p1", "2026-07-13T09:05:00Z", schema.ClassExec, "Bash", []string{"cd"}, 1),
		failedCall("s2", "p1", "2026-07-14T10:00:00Z", schema.ClassExec, "Bash", []string{"cd"}, 1),
		failedCall("s3", "p2", "2026-07-15T11:00:00Z", schema.ClassExec, "Bash", []string{"cd"}, 1),
		failedCall("s3", "p2", "2026-07-15T11:30:00Z", schema.ClassExec, "Bash", []string{"cd"}, 2),
		okCall("s1", "2026-07-13T09:06:00Z"),
		{TS: "2026-07-13T09:07:00Z", SessionID: "s1", EventType: schema.EventPreTool,
			ToolClass: schema.ClassExec, ToolRaw: "Bash", Verbs: []string{"cd"}, ToolOK: schema.OutcomeFailed},
	}

	clusters := FrictionClusters(events)
	if len(clusters) != 2 {
		t.Fatalf("clusters = %d, want 2 (cd exit 1 and cd exit 2)", len(clusters))
	}

	top := clusters[0]
	if top.Failures != 4 || len(top.Sessions) != 3 || top.ExitCode != 1 {
		t.Fatalf("top cluster = %+v, want 4 failures over 3 sessions with exit 1", top)
	}
	if top.Days != 3 {
		t.Fatalf("top.Days = %d, want 3", top.Days)
	}
	if top.Projects != 2 {
		t.Fatalf("top.Projects = %d, want 2", top.Projects)
	}
	if top.Sessions[0].SessionID != "s1" || top.Sessions[0].Failures != 2 {
		t.Fatalf("top.Sessions[0] = %+v, want s1 with 2 failures", top.Sessions[0])
	}
	wantFirst := time.Date(2026, 7, 13, 9, 0, 0, 0, time.UTC)
	wantLast := time.Date(2026, 7, 15, 11, 0, 0, 0, time.UTC)
	if !top.First.Equal(wantFirst) || !top.Last.Equal(wantLast) {
		t.Fatalf("top window = %v → %v, want %v → %v", top.First, top.Last, wantFirst, wantLast)
	}
}

func TestFrictionClustersGroupsNonExecByToolName(t *testing.T) {
	events := []schema.TelemetryEvent{
		failedCall("s1", "", "2026-07-13T09:00:00Z", schema.ClassFileRead, "Read", nil, 0),
		failedCall("s2", "", "2026-07-14T09:00:00Z", schema.ClassFileRead, "Read", nil, 0),
		failedCall("s2", "", "2026-07-14T09:01:00Z", schema.ClassMCP, "mcp__context7__resolve-library-id", nil, 0),
	}

	clusters := FrictionClusters(events)
	if len(clusters) != 2 {
		t.Fatalf("clusters = %d, want 2", len(clusters))
	}
	if clusters[0].ToolName != "Read" || clusters[0].Failures != 2 {
		t.Fatalf("clusters[0] = %+v, want Read with 2 failures", clusters[0])
	}
}

func TestFrictionClusterSlugs(t *testing.T) {
	cases := []struct {
		cluster FrictionCluster
		want    string
	}{
		{FrictionCluster{ToolClass: schema.ClassExec, Verbs: []string{"cd"}, ExitCode: 1}, "cd-exit-1"},
		{FrictionCluster{ToolClass: schema.ClassExec, Verbs: []string{"git", "go"}, ExitCode: 2}, "git-go-exit-2"},
		{FrictionCluster{ToolClass: schema.ClassFileRead, ToolName: "Read"}, "read"},
		{FrictionCluster{ToolClass: schema.ClassMCP, ToolName: "mcp__context7__resolve-library-id"}, "mcp-context7-resolve-library-id"},
	}
	for _, c := range cases {
		if got := c.cluster.Slug(); got != c.want {
			t.Errorf("Slug() = %q, want %q", got, c.want)
		}
	}
}

func TestFrictionClustersKeepsUnparseableTimestampsOutOfDays(t *testing.T) {
	events := []schema.TelemetryEvent{
		failedCall("s1", "", "", schema.ClassExec, "Bash", []string{"cd"}, 1),
		failedCall("s2", "", "2026-07-14T09:00:00Z", schema.ClassExec, "Bash", []string{"cd"}, 1),
	}
	clusters := FrictionClusters(events)
	if len(clusters) != 1 {
		t.Fatalf("clusters = %d, want 1", len(clusters))
	}
	if clusters[0].Failures != 2 || clusters[0].Days != 1 {
		t.Fatalf("cluster = %+v, want 2 failures on 1 datable day", clusters[0])
	}
}

func TestWorthFixingGatesOnSessionsAndDays(t *testing.T) {
	day := func(d, session string) schema.TelemetryEvent {
		return failedCall(session, "", d, schema.ClassExec, "Bash", []string{"cd"}, 1)
	}
	enoughSessionsOneDay := FrictionClusters([]schema.TelemetryEvent{
		day("2026-07-13T09:00:00Z", "s1"), day("2026-07-13T10:00:00Z", "s2"), day("2026-07-13T11:00:00Z", "s3"),
	})
	if got := WorthFixing(enoughSessionsOneDay); len(got) != 0 {
		t.Fatalf("one-day cluster passed the gate: %+v", got)
	}

	enoughDaysTwoSessions := FrictionClusters([]schema.TelemetryEvent{
		day("2026-07-13T09:00:00Z", "s1"), day("2026-07-14T10:00:00Z", "s2"),
	})
	if got := WorthFixing(enoughDaysTwoSessions); len(got) != 0 {
		t.Fatalf("two-session cluster passed the gate: %+v", got)
	}

	qualifying := FrictionClusters([]schema.TelemetryEvent{
		day("2026-07-13T09:00:00Z", "s1"), day("2026-07-14T10:00:00Z", "s2"), day("2026-07-15T11:00:00Z", "s3"),
	})
	if got := WorthFixing(qualifying); len(got) != 1 {
		t.Fatalf("qualifying cluster failed the gate")
	}
}

func TestFrictionClustersOrderIsDeterministic(t *testing.T) {
	events := []schema.TelemetryEvent{
		failedCall("s1", "", "2026-07-13T09:00:00Z", schema.ClassFileRead, "Read", nil, 0),
		failedCall("s2", "", "2026-07-14T09:00:00Z", schema.ClassFileRead, "Read", nil, 0),
		failedCall("s1", "", "2026-07-13T09:02:00Z", schema.ClassExec, "Bash", []string{"cd"}, 1),
		failedCall("s2", "", "2026-07-14T09:02:00Z", schema.ClassExec, "Bash", []string{"cd"}, 1),
		failedCall("s3", "", "2026-07-15T09:02:00Z", schema.ClassExec, "Bash", []string{"cd"}, 1),
	}
	clusters := FrictionClusters(events)
	if len(clusters) != 2 || clusters[0].Slug() != "cd-exit-1" || clusters[1].Slug() != "read" {
		t.Fatalf("order = %v", slugsOf(clusters))
	}
}

func slugsOf(clusters []FrictionCluster) []string {
	out := make([]string, len(clusters))
	for i, c := range clusters {
		out[i] = c.Slug()
	}
	return out
}
