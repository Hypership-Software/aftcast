package analytics

import (
	"reflect"
	"testing"

	"github.com/Hypership-Software/atlas/internal/schema"
)

func TestObservedWorkMixAttributesPlanBuildAndReview(t *testing.T) {
	events := []schema.TelemetryEvent{}
	events = appendCall(events, "read", schema.ClassFileRead, schema.OperationRead, schema.OutcomeOK, 100)
	events = appendCall(events, "search", schema.ClassOther, schema.OperationSearch, schema.OutcomeOK, 200)
	events = appendCall(events, "exec", schema.ClassExec, schema.OperationExecute, schema.OutcomeOK, 300)
	events = appendCall(events, "edit", schema.ClassFileWrite, schema.OperationEdit, schema.OutcomeOK, 400)
	events = appendCall(events, "test", schema.ClassExec, schema.OperationTest, schema.OutcomeOK, 500)
	events = appendCall(events, "format", schema.ClassExec, schema.OperationFormat, schema.OutcomeOK, 0)
	events = appendCall(events, "lint", schema.ClassExec, schema.OperationLint, schema.OutcomeFailed, 600)

	got := ObservedWorkMix(events)
	if !got.Covered || got.Plan.Calls != 2 || got.Build.Calls != 2 || got.Review.Calls != 2 {
		t.Fatalf("mix = %+v", got)
	}
	if got.Plan.DurationMS != 300 || got.Build.DurationMS != 700 || got.Review.DurationMS != 500 {
		t.Fatalf("durations = %+v", got)
	}
	if !reflect.DeepEqual(got.Review.Operations, []schema.Operation{schema.OperationFormat, schema.OperationTest}) {
		t.Fatalf("review operations = %v", got.Review.Operations)
	}
}

func TestObservedWorkMixCoverageAndNoEditBehavior(t *testing.T) {
	legacy := appendCall(nil, "read", schema.ClassFileRead, schema.OperationRead, schema.OutcomeOK, 20)
	for i := range legacy {
		legacy[i].V = schema.DeliverySignalVersion
	}
	if got := ObservedWorkMix(legacy); got.Covered {
		t.Fatalf("legacy mix = %+v", got)
	}

	missingOperation := appendCall(nil, "missing", schema.ClassExec, "", schema.OutcomeOK, 20)
	if got := ObservedWorkMix(missingOperation); got.Covered {
		t.Fatalf("missing-operation mix = %+v", got)
	}

	var noEdit []schema.TelemetryEvent
	noEdit = appendCall(noEdit, "read", schema.ClassFileRead, schema.OperationRead, schema.OutcomeOK, 10)
	noEdit = appendCall(noEdit, "test", schema.ClassExec, schema.OperationTest, schema.OutcomeOK, 20)
	noEdit = appendCall(noEdit, "exec", schema.ClassExec, schema.OperationExecute, schema.OutcomeOK, 30)
	got := ObservedWorkMix(noEdit)
	if got.Plan.Calls != 1 || got.Review.Calls != 1 || got.Build.Calls != 1 {
		t.Fatalf("no-edit mix = %+v", got)
	}
}

func TestObservedWorkMixRecognizesExplicitPlanningSkill(t *testing.T) {
	pre := schema.TelemetryEvent{
		V: schema.ObservationVersion, EventType: schema.EventPreTool, ToolUseID: "skill",
		ToolClass: schema.ClassSkill, Operation: schema.OperationSkill, Skill: "superpowers:writing-plans",
	}
	post := schema.TelemetryEvent{V: schema.ObservationVersion, EventType: schema.EventPostTool, ToolUseID: "skill", ToolOK: schema.OutcomeOK, LatencyMS: 50}
	events := []schema.TelemetryEvent{pre, post}
	events = appendCall(events, "edit", schema.ClassFileWrite, schema.OperationEdit, schema.OutcomeOK, 100)
	got := ObservedWorkMix(events)
	if got.Plan.Calls != 1 || got.Build.Calls != 1 {
		t.Fatalf("planning skill mix = %+v", got)
	}
}

func appendCall(events []schema.TelemetryEvent, id string, class schema.ToolClass, operation schema.Operation, outcome schema.ToolOutcome, latency int64) []schema.TelemetryEvent {
	pre := schema.TelemetryEvent{
		V: schema.ObservationVersion, EventType: schema.EventPreTool, ToolUseID: id,
		ToolClass: class, Operation: operation,
	}
	post := schema.TelemetryEvent{
		V: schema.ObservationVersion, EventType: schema.EventPostTool, ToolUseID: id,
		ToolOK: outcome, LatencyMS: latency,
	}
	return append(events, pre, post)
}
