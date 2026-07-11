package schema

import (
	"bytes"
	"encoding/json"
	"reflect"
	"sort"
	"testing"
)

func sampleDescriptor() Descriptor {
	return Descriptor{
		Version:     SchemaVersion,
		SessionID:   "sess-123",
		Org:         "acme",
		ToolClass:   ClassExec,
		ToolRaw:     "Bash",
		Argv:        []string{"rm", "-rf", "/tmp/x"},
		Verbs:       []string{"rm"},
		Files:       []string{"/tmp/x"},
		Cwd:         "/home/dev/proj",
		ProjectRoot: "/home/dev/proj",
		Tainted:     true,
	}
}

func sampleEvent() TelemetryEvent {
	return TelemetryEvent{
		V:          SchemaVersion,
		TS:         "2026-07-10T12:00:00Z",
		Seq:        42,
		SessionID:  "sess-123",
		OrgID:      "acme",
		User:       "dev",
		Host:       "box",
		Harness:    "claudecode",
		EventType:  EventPreTool,
		TurnIndex:  3,
		ToolClass:  ClassExec,
		ToolRaw:    "Bash",
		Risk:       RiskDanger,
		RuleID:     "danger-rm-rf",
		ToolOK:     OutcomeNotRun,
		LatencyMS:  12,
		Files:      []string{"/tmp/x"},
		Verbs:      []string{"rm"},
		Taint:      true,
		PolicyHash: "policyabc",
		PrevHash:   "prevdef",
		Hash:       "selfghi",
	}
}

// assertSortedKeys checks that the top-level JSON object keys appear in
// ascending order (the canonical-form guarantee the hash chain relies on).
// Both schema structs are flat (values are scalars or string arrays), so a
// `"key":` substring search is unambiguous.
func assertSortedKeys(t *testing.T, data []byte) {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("canonical output is not valid JSON: %v", err)
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	last := -1
	for _, k := range keys {
		idx := bytes.Index(data, []byte(`"`+k+`":`))
		if idx <= last {
			t.Fatalf("key %q is out of sorted order in %s", k, data)
		}
		last = idx
	}
}

func TestDescriptorCanonicalIsStableAndSorted(t *testing.T) {
	d := sampleDescriptor()
	a, err := d.Canonical()
	if err != nil {
		t.Fatal(err)
	}
	b, err := d.Canonical()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Fatalf("Descriptor.Canonical not byte-stable:\n%s\n%s", a, b)
	}
	assertSortedKeys(t, a)
}

func TestEventCanonicalIsStableSortedAndExcludesHash(t *testing.T) {
	e := sampleEvent()
	a, err := e.Canonical()
	if err != nil {
		t.Fatal(err)
	}
	b, err := e.Canonical()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Fatalf("TelemetryEvent.Canonical not byte-stable")
	}
	assertSortedKeys(t, a)

	// The hash field is self-referential — it cannot be part of the bytes the
	// hash is computed over (Task 13). Canonical must exclude it, so changing
	// only Hash must not change the canonical bytes.
	var m map[string]any
	if err := json.Unmarshal(a, &m); err != nil {
		t.Fatal(err)
	}
	if _, present := m["hash"]; present {
		t.Fatalf("canonical form must exclude the 'hash' field, got: %s", a)
	}
	e2 := sampleEvent()
	e2.Hash = "TOTALLY-DIFFERENT"
	c, err := e2.Canonical()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, c) {
		t.Fatalf("canonical changed when only the hash field changed")
	}
}

func TestEventJSONRoundTripPreservesFields(t *testing.T) {
	e := sampleEvent()
	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	var back TelemetryEvent
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(e, back) {
		t.Fatalf("round trip lost fields:\n want %+v\n got  %+v", e, back)
	}
}

func TestEnumWireValuesAreStable(t *testing.T) {
	// These strings are part of the append-only SIEM/rollup contract — pin them.
	cases := map[string]string{
		string(RiskSafe):       "safe",
		string(RiskDanger):     "danger",
		string(RiskUnknown):    "unknown",
		string(OutcomeOK):      "ok",
		string(OutcomeFailed):  "failed",
		string(OutcomeNotRun):  "not_run",
		string(EventPreTool):   "pre_tool",
		string(EventPostTool):  "post_tool",
		string(ClassExec):      "exec",
		string(ClassFileRead):  "file_read",
		string(ClassFileWrite): "file_write",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("wire value drift: got %q want %q", got, want)
		}
	}
}
