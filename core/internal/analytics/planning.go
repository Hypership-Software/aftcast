package analytics

import (
	"strings"

	"github.com/Hypership-Software/aftcast/internal/schema"
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
	firstPrompt := ""
	sawPrompt := false
	incomplete := false
	for i, e := range events[:firstWrite+1] {
		if i < firstWrite && e.PromptID == "" && promptScoped(e.EventType) {
			incomplete = true
		}
		if e.EventType == schema.EventUserPrompt && !sawPrompt {
			sawPrompt = true
			firstPrompt = e.PromptID
		}
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
	if sawPrompt && !incomplete && firstPrompt == writePrompt && order[0] == writePrompt {
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

func promptScoped(event schema.EventType) bool {
	return event == schema.EventUserPrompt || event == schema.EventPromptExpansion || event == schema.EventPreTool || event == schema.EventPostTool
}

func preparatoryClass(class schema.ToolClass) bool {
	return class == schema.ClassFileRead || class == schema.ClassNetSearch || class == schema.ClassNetFetch || class == schema.ClassAgentSpawn
}

func explicitPlanningMarker(e schema.TelemetryEvent) bool {
	return strings.EqualFold(e.ToolRaw, "EnterPlanMode") || planningName(e.Skill) || planningName(e.Command)
}

func planningName(name string) bool {
	switch explicitName(name) {
	case "plan", "brainstorming", "writing-plans":
		return true
	default:
		return false
	}
}

func explicitName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if i := strings.LastIndexAny(name, ":/"); i >= 0 {
		name = name[i+1:]
	}
	return name
}
