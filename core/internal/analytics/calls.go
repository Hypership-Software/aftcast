package analytics

import "github.com/Hypership-Software/aftcast/internal/schema"

type observedCall struct {
	Pre  schema.TelemetryEvent
	Post schema.TelemetryEvent
}

func pairCalls(events []schema.TelemetryEvent) []observedCall {
	posts := make(map[string]schema.TelemetryEvent)
	for _, event := range events {
		if event.EventType == schema.EventPostTool && event.ToolUseID != "" {
			posts[event.ToolUseID] = event
		}
	}

	var calls []observedCall
	for _, event := range events {
		if event.EventType != schema.EventPreTool || event.ToolUseID == "" {
			continue
		}
		post, ok := posts[event.ToolUseID]
		if !ok {
			continue
		}
		calls = append(calls, observedCall{Pre: event, Post: post})
	}
	return calls
}
