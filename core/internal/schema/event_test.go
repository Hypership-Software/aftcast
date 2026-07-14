package schema

import (
	"strings"
	"testing"
)

func TestTelemetryEvent_ProjectOmitEmpty(t *testing.T) {
	// An event with no project_id must canonicalize without the key, so historical
	// events keep their hash-chain byte-for-byte.
	e := TelemetryEvent{V: SchemaVersion, SessionID: "s1", User: "u", Host: "h",
		Harness: "claudecode", EventType: EventPreTool}
	got, err := e.Canonical()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "project_id") {
		t.Errorf("empty Project must be omitted:\n%s", got)
	}
	e.Project = "abc123def456"
	got2, err := e.Canonical()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got2), `"project_id":"abc123def456"`) {
		t.Errorf("set Project must appear:\n%s", got2)
	}
}

func TestTelemetryEventDeliverySignalWireContract(t *testing.T) {
	base := TelemetryEvent{V: 1, SessionID: "s1", User: "u", Host: "h",
		Harness: "claudecode", EventType: EventPostTool}

	old, err := base.Canonical()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(old), "delivery_signal") {
		t.Fatalf("v1 event gained delivery_signal bytes: %s", old)
	}

	base.V = SchemaVersion
	base.DeliverySignal = DeliveryGitPush
	got, err := base.Canonical()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `"delivery_signal":"git_push"`) {
		t.Fatalf("v2 event missing delivery signal: %s", got)
	}
}

func TestDeliverySignalVersionIsV2(t *testing.T) {
	if SchemaVersion != 2 || DeliverySignalVersion != 2 {
		t.Fatalf("schema=%d delivery=%d, want both 2", SchemaVersion, DeliverySignalVersion)
	}
}
