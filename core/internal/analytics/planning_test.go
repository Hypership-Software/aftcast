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

func TestObservedPlanStyleBoundedExplicitMarkers(t *testing.T) {
	tests := map[string]schema.TelemetryEvent{
		"enter plan mode":         {EventType: schema.EventPreTool, ToolRaw: "eNtErPlAnMoDe"},
		"brainstorming skill":     {EventType: schema.EventPreTool, ToolClass: schema.ClassSkill, Skill: "superpowers:brainstorming"},
		"writing plans skill":     {EventType: schema.EventPreTool, ToolClass: schema.ClassSkill, Skill: "superpowers:writing-plans"},
		"plan skill":              {EventType: schema.EventPreTool, ToolClass: schema.ClassSkill, Skill: "superpowers:plan"},
		"plan slash command":      {EventType: schema.EventPromptExpansion, Command: "/PLAN"},
		"brainstorming command":   {EventType: schema.EventPromptExpansion, Command: "/brainstorming"},
		"writing plans command":   {EventType: schema.EventPromptExpansion, Command: "/writing-plans"},
		"namespaced plan command": {EventType: schema.EventPromptExpansion, Command: "superpowers:plan"},
	}
	for name, marker := range tests {
		t.Run(name, func(t *testing.T) {
			events := []schema.TelemetryEvent{
				marker,
				planEvent("p1", schema.EventPreTool, schema.ClassFileWrite),
			}
			if got := ObservedPlanStyle(events); got != PlanFirst {
				t.Fatalf("explicit marker = %q, want plan_first", got)
			}
		})
	}
}

func TestObservedPlanStyleIncidentalNamesAreNotPlanningMarkers(t *testing.T) {
	tests := map[string]schema.TelemetryEvent{
		"nonbrainstormer":          {EventType: schema.EventPreTool, ToolClass: schema.ClassSkill, Skill: "nonbrainstormer"},
		"rewriting plans":          {EventType: schema.EventPreTool, ToolClass: schema.ClassSkill, Skill: "rewriting-plans"},
		"subscription plan export": {EventType: schema.EventPromptExpansion, Command: "subscription_plan_export"},
		"planet":                   {EventType: schema.EventPromptExpansion, Command: "planet"},
		"brainstorm notes":         {EventType: schema.EventPreTool, ToolClass: schema.ClassSkill, Skill: "brainstorm-notes"},
		"planning report":          {EventType: schema.EventPromptExpansion, Command: "planning-report"},
	}
	for name, marker := range tests {
		t.Run(name, func(t *testing.T) {
			marker.PromptID = "p1"
			events := []schema.TelemetryEvent{
				planEvent("p1", schema.EventUserPrompt, ""),
				marker,
				planEvent("p1", schema.EventPreTool, schema.ClassFileWrite),
			}
			if got := ObservedPlanStyle(events); got != PlanDirect {
				t.Fatalf("incidental marker = %q, want direct_to_edit", got)
			}
		})
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

func TestObservedPlanStyleUnknownForUnidentifiedEarlierPrompt(t *testing.T) {
	events := []schema.TelemetryEvent{
		planEvent("", schema.EventUserPrompt, ""),
		planEvent("p2", schema.EventPreTool, schema.ClassFileWrite),
	}
	if got := ObservedPlanStyle(events); got != PlanUnknown {
		t.Fatalf("unidentified earlier prompt = %q, want unknown", got)
	}
}

func TestObservedPlanStyleUnknownForUnidentifiedPreWriteActivity(t *testing.T) {
	events := []schema.TelemetryEvent{
		planEvent("", schema.EventPreTool, schema.ClassFileRead),
		planEvent("p2", schema.EventUserPrompt, ""),
		planEvent("p2", schema.EventPreTool, schema.ClassFileWrite),
	}
	if got := ObservedPlanStyle(events); got != PlanUnknown {
		t.Fatalf("unidentified pre-write activity = %q, want unknown", got)
	}
}

func TestObservedPlanStyleUnknownForEarlierIdentifiedPromptGroup(t *testing.T) {
	events := []schema.TelemetryEvent{
		planEvent("p0", schema.EventPreTool, schema.ClassFileRead),
		planEvent("p1", schema.EventUserPrompt, ""),
		planEvent("p1", schema.EventPreTool, schema.ClassFileWrite),
	}
	if got := ObservedPlanStyle(events); got != PlanUnknown {
		t.Fatalf("earlier identified prompt group = %q, want unknown", got)
	}
}

func TestObservedPlanStyleExplicitMarkerWithIncompletePromptGrouping(t *testing.T) {
	events := []schema.TelemetryEvent{
		planEvent("", schema.EventUserPrompt, ""),
		{EventType: schema.EventPreTool, ToolRaw: "EnterPlanMode"},
		planEvent("p2", schema.EventPreTool, schema.ClassFileWrite),
	}
	if got := ObservedPlanStyle(events); got != PlanFirst {
		t.Fatalf("explicit marker with incomplete grouping = %q, want plan_first", got)
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
