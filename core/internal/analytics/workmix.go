package analytics

import (
	"sort"

	"github.com/Hypership-Software/aftcast/internal/schema"
)

type WorkBucket struct {
	Calls      int
	DurationMS int64
	Operations []schema.Operation
}

type WorkMix struct {
	Plan    WorkBucket
	Build   WorkBucket
	Review  WorkBucket
	Covered bool
}

func ObservedWorkMix(events []schema.TelemetryEvent) WorkMix {
	out := WorkMix{Covered: observationCaptured(events)}
	calls := pairCalls(events)
	firstWrite := -1
	for i, call := range calls {
		if call.Post.ToolOK == schema.OutcomeOK && call.Pre.ToolClass == schema.ClassFileWrite {
			firstWrite = i
			break
		}
	}

	seen := map[*WorkBucket]map[schema.Operation]struct{}{
		&out.Plan:   {},
		&out.Build:  {},
		&out.Review: {},
	}
	for i, call := range calls {
		if call.Post.ToolOK != schema.OutcomeOK {
			continue
		}
		if call.Pre.Operation == "" {
			out.Covered = false
		}
		bucket := &out.Build
		switch {
		case reviewOperation(call.Pre.Operation):
			bucket = &out.Review
		case (firstWrite < 0 || i < firstWrite) && planningActivity(call.Pre):
			bucket = &out.Plan
		}
		bucket.Calls++
		if call.Post.LatencyMS > 0 {
			bucket.DurationMS += call.Post.LatencyMS
		}
		if call.Pre.Operation != "" {
			seen[bucket][call.Pre.Operation] = struct{}{}
		}
	}

	for bucket, operations := range seen {
		for operation := range operations {
			bucket.Operations = append(bucket.Operations, operation)
		}
		sort.Slice(bucket.Operations, func(i, j int) bool { return bucket.Operations[i] < bucket.Operations[j] })
	}
	return out
}

func planningActivity(event schema.TelemetryEvent) bool {
	switch event.Operation {
	case schema.OperationRead, schema.OperationSearch, schema.OperationAsk, schema.OperationPlan:
		return true
	default:
		return explicitPlanningMarker(event)
	}
}

func reviewOperation(operation schema.Operation) bool {
	switch operation {
	case schema.OperationTest, schema.OperationLint, schema.OperationFormat, schema.OperationInspect:
		return true
	default:
		return false
	}
}
