package analytics

import (
	"testing"

	"github.com/Hypership-Software/atlas/internal/schema"
)

func planEvent(prompt string, event schema.EventType, class schema.ToolClass) schema.TelemetryEvent {
	return schema.TelemetryEvent{PromptID: prompt, EventType: event, ToolClass: class}
}

func TestObservedPlanStyleExplicitMarker(t *testing.T) {
	events := []schema.TelemetryEvent{
		{PromptID: "p1", EventType: schema.EventPreTool, ToolClass: schema.ClassSkill, Skill: "superpowers:writing-plans"},
		planEvent("p1", schema.EventPreTool, schema.ClassFileWrite),
	}
	if got := ObservedPlanStyle(events); got != PlanFirst {
		t.Fatalf("explicit marker = %q, want plan_first", got)
	}
}

func TestObservedPlanStylePreparatoryPrompt(t *testing.T) {
	events := []schema.TelemetryEvent{
		planEvent("p1", schema.EventPreTool, schema.ClassFileRead),
		planEvent("p1", schema.EventPreTool, schema.ClassAgentSpawn),
		planEvent("p2", schema.EventPreTool, schema.ClassFileWrite),
	}
	if got := ObservedPlanStyle(events); got != PlanFirst {
		t.Fatalf("preparatory prompt = %q, want plan_first", got)
	}
}

func TestObservedPlanStyleDirectToEdit(t *testing.T) {
	events := []schema.TelemetryEvent{
		planEvent("p1", schema.EventUserPrompt, ""),
		planEvent("p1", schema.EventPreTool, schema.ClassFileWrite),
	}
	if got := ObservedPlanStyle(events); got != PlanDirect {
		t.Fatalf("first-prompt edit = %q, want direct_to_edit", got)
	}
}

func TestObservedPlanStyleUnknownWithoutPromptEvidence(t *testing.T) {
	events := []schema.TelemetryEvent{{EventType: schema.EventPreTool, ToolClass: schema.ClassFileWrite}}
	if got := ObservedPlanStyle(events); got != PlanUnknown {
		t.Fatalf("missing prompt id = %q, want unknown", got)
	}
}

func TestObservedPlanStyleUnknownForWeakEarlierPrompt(t *testing.T) {
	events := []schema.TelemetryEvent{
		planEvent("p1", schema.EventPreTool, schema.ClassFileRead),
		planEvent("p2", schema.EventPreTool, schema.ClassFileWrite),
	}
	if got := ObservedPlanStyle(events); got != PlanUnknown {
		t.Fatalf("one preparatory action = %q, want unknown", got)
	}
}
