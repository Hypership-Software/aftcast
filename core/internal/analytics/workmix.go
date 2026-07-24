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
	firstWrite := firstWritePerTurn(calls)

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
		case beforeTurnsFirstWrite(firstWrite, call.Pre.TurnIndex, i) && planningActivity(call.Pre):
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

// The planning window is per turn, not per session: reading and searching before
// you start writing is planning, and a prompt reopens that window. Scoped to the
// whole session, the first write would close planning permanently, so every long
// session decayed to Build-only however much later investigation it did.
func firstWritePerTurn(calls []observedCall) map[int]int {
	first := make(map[int]int)
	for i, call := range calls {
		if call.Post.ToolOK != schema.OutcomeOK || call.Pre.ToolClass != schema.ClassFileWrite {
			continue
		}
		if _, found := first[call.Pre.TurnIndex]; !found {
			first[call.Pre.TurnIndex] = i
		}
	}
	return first
}

func beforeTurnsFirstWrite(firstWrite map[int]int, turn, i int) bool {
	write, found := firstWrite[turn]
	return !found || i < write
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
