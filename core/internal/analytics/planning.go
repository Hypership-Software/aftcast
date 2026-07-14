package analytics

import (
	"strings"
	"unicode"

	"github.com/Hypership-Software/atlas/internal/schema"
)

type PlanStyle string

const (
	PlanUnknown PlanStyle = ""
	PlanFirst   PlanStyle = "plan_first"
	PlanDirect  PlanStyle = "direct_to_edit"
)

func ObservedPlanStyle(events []schema.TelemetryEvent) PlanStyle {
	firstWrite := -1
	for i, e := range events {
		if e.EventType == schema.EventPreTool && e.ToolClass == schema.ClassFileWrite {
			firstWrite = i
			break
		}
	}
	if firstWrite < 0 {
		return PlanUnknown
	}

	for _, e := range events[:firstWrite] {
		if explicitPlanningMarker(e) {
			return PlanFirst
		}
	}

	writePrompt := events[firstWrite].PromptID
	if writePrompt == "" {
		return PlanUnknown
	}

	var order []string
	seen := map[string]bool{}
	prep := map[string]int{}
	for i, e := range events[:firstWrite+1] {
		if e.PromptID != "" && !seen[e.PromptID] {
			seen[e.PromptID] = true
			order = append(order, e.PromptID)
		}
		if i < firstWrite && e.PromptID != "" && e.EventType == schema.EventPreTool && preparatoryClass(e.ToolClass) {
			prep[e.PromptID]++
		}
	}
	if len(order) == 0 {
		return PlanUnknown
	}
	if order[0] == writePrompt {
		return PlanDirect
	}
	for _, promptID := range order {
		if promptID == writePrompt {
			break
		}
		if prep[promptID] >= 2 {
			return PlanFirst
		}
	}
	return PlanUnknown
}

func preparatoryClass(class schema.ToolClass) bool {
	return class == schema.ClassFileRead || class == schema.ClassNetSearch || class == schema.ClassNetFetch || class == schema.ClassAgentSpawn
}

func explicitPlanningMarker(e schema.TelemetryEvent) bool {
	return strings.EqualFold(e.ToolRaw, "EnterPlanMode") || planningName(e.Skill) || planningName(e.Command)
}

func planningName(name string) bool {
	lower := strings.ToLower(name)
	if strings.Contains(lower, "brainstorm") || strings.Contains(lower, "writing-plans") {
		return true
	}
	parts := strings.FieldsFunc(lower, func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune(":-_/.", r)
	})
	for _, part := range parts {
		if part == "plan" {
			return true
		}
	}
	return false
}
