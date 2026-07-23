package handoff

import (
	"reflect"
	"testing"

	"github.com/Hypership-Software/aftcast/internal/schema"
	"github.com/Hypership-Software/aftcast/internal/telemetry"
)

func TestGatherFacts(t *testing.T) {
	sel := []Selected{{
		Session: telemetry.Session{SessionID: "s1", Started: "2026-07-23T10:00:00Z", Ended: "2026-07-23T11:00:00Z"},
		SHAs:    []string{"bb16536"},
		Events: []schema.TelemetryEvent{
			{EventType: schema.EventUserPrompt, PermissionMode: "default"},
			{EventType: schema.EventPreTool, Risk: schema.RiskDanger, RuleID: "danger-rm-rf", PermissionMode: "default"},
			{EventType: schema.EventPostTool, ToolOK: schema.OutcomeFailed, PermissionMode: "default"},
			{EventType: schema.EventPostTool, ToolOK: schema.OutcomeOK, DeliverySignal: schema.DeliveryGitPush, CommitSHA: "bb16536", PermissionMode: "default"},
			{EventType: schema.EventPreTool, Skill: "superpowers:test-driven-development", PermissionMode: "default"},
			{EventType: schema.EventStop, ContextTokens: 91000, PermissionMode: "default"},
			{EventType: schema.EventPreTool, Subagent: "Explore", Taint: true, PermissionMode: "bypassPermissions"},
		},
	}}
	got := GatherFacts(sel)
	if len(got) != 1 {
		t.Fatalf("got %d fact sets, want 1", len(got))
	}
	f := got[0]
	want := SessionFacts{
		ID: "s1", Started: "2026-07-23T10:00:00Z", Ended: "2026-07-23T11:00:00Z",
		Events: 7, Prompts: 1, Failures: 1, Deliveries: 1,
		CommitSHAs:      []string{"bb16536"},
		DangerRules:     []string{"danger-rm-rf"},
		Skills:          []string{"superpowers:test-driven-development"},
		Subagents:       []string{"Explore"},
		PermissionModes: []string{"default", "bypassPermissions"},
		MaxContext:      91000,
		Tainted:         true,
	}
	if !reflect.DeepEqual(f, want) {
		t.Errorf("facts:\n got %+v\nwant %+v", f, want)
	}
}
