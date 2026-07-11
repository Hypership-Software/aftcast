package daemon

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Hypership-Software/atlas/internal/ipc"
	"github.com/Hypership-Software/atlas/internal/schema"
)

type fakeEval struct {
	v     schema.Risk
	id    string
	calls int
}

func (f *fakeEval) Eval(schema.Descriptor) (schema.Risk, string) {
	f.calls++
	return f.v, f.id
}

type fakeTaint struct{ applyCalls, markCalls int }

func (f *fakeTaint) Apply(*schema.Descriptor)                 { f.applyCalls++ }
func (f *fakeTaint) MarkFromResult(string, schema.Descriptor) { f.markCalls++ }

type fakeRecorder struct{ events []schema.TelemetryEvent }

func (f *fakeRecorder) Record(e schema.TelemetryEvent) error {
	f.events = append(f.events, e)
	return nil
}

func handlerWith(e Evaluator, tt Tainter, r Recorder) *Handler {
	return NewHandler(Deps{Eval: e, Taint: tt, Record: r})
}

func preToolReq() Request {
	return Request{
		Event:      schema.TelemetryEvent{EventType: schema.EventPreTool, SessionID: "s", ToolClass: schema.ClassExec},
		Descriptor: schema.Descriptor{SessionID: "s", ToolClass: schema.ClassExec, Verbs: []string{"rm"}},
	}
}

// A dangerous action is classified and recorded, never blocked.
func TestHandleClassifiesDangerButDoesNotBlock(t *testing.T) {
	ev := &fakeEval{v: schema.RiskDanger, id: "no-exec"}
	rec := &fakeRecorder{}
	resp, err := handlerWith(ev, &fakeTaint{}, rec).Handle(preToolReq())
	if err != nil {
		t.Fatal(err)
	}
	if resp.Risk != schema.RiskDanger || resp.RuleID != "no-exec" {
		t.Fatalf("classification = %v/%q, want deny/no-exec", resp.Risk, resp.RuleID)
	}
	if len(rec.events) != 1 {
		t.Fatalf("recorded %d events, want exactly 1", len(rec.events))
	}
	if rec.events[0].EventType != schema.EventPreTool {
		t.Errorf("recorded as %v, want pre_tool (nothing is blocked)", rec.events[0].EventType)
	}
	if rec.events[0].Risk != schema.RiskDanger {
		t.Errorf("recorded classification = %v, want deny", rec.events[0].Risk)
	}
}

// An uncovered action is classified unknown and recorded; the action proceeds.
func TestHandleAskIsRecordedNotResolved(t *testing.T) {
	ev := &fakeEval{v: schema.RiskUnknown, id: "default-ask"}
	rec := &fakeRecorder{}
	resp, _ := handlerWith(ev, &fakeTaint{}, rec).Handle(preToolReq())
	if resp.Risk != schema.RiskUnknown {
		t.Fatalf("classification = %v, want ask", resp.Risk)
	}
	if len(rec.events) != 1 || rec.events[0].Risk != schema.RiskUnknown {
		t.Fatalf("recorded %d events (verdict %v), want 1 ask", len(rec.events), rec.events[0].Risk)
	}
}

// Every pre_tool applies stored taint and marks any new taint source.
func TestHandlePreToolAppliesAndMarksTaint(t *testing.T) {
	ev := &fakeEval{v: schema.RiskSafe, id: "permit"}
	tt := &fakeTaint{}
	handlerWith(ev, tt, &fakeRecorder{}).Handle(preToolReq())
	if tt.applyCalls != 1 {
		t.Errorf("Apply calls = %d, want 1", tt.applyCalls)
	}
	if tt.markCalls != 1 {
		t.Errorf("MarkFromResult calls = %d, want 1", tt.markCalls)
	}
}

func TestHandlePostToolRecordsOnlyAndSkipsEval(t *testing.T) {
	ev := &fakeEval{v: schema.RiskDanger, id: "should-not-be-used"}
	rec := &fakeRecorder{}
	req := Request{Event: schema.TelemetryEvent{EventType: schema.EventPostTool, SessionID: "s"}}
	handlerWith(ev, &fakeTaint{}, rec).Handle(req)
	if ev.calls != 0 {
		t.Errorf("classifier was called on a post_tool event (%d times)", ev.calls)
	}
	if len(rec.events) != 1 {
		t.Fatalf("recorded %d events, want 1", len(rec.events))
	}
}

func TestServeRoundTrip(t *testing.T) {
	t.Setenv("GATED_IPC_ID", "daemon-serve-test")
	ln, err := ipc.Listen()
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	h := handlerWith(&fakeEval{v: schema.RiskDanger, id: "no-exec"}, &fakeTaint{}, &fakeRecorder{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = Serve(ctx, ln, h) }()

	conn, err := ipc.Dial(2 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	raw, _ := json.Marshal(preToolReq())
	if err := ipc.WriteFrame(conn, raw); err != nil {
		t.Fatal(err)
	}
	respRaw, err := ipc.ReadFrame(conn)
	if err != nil {
		t.Fatal(err)
	}
	var resp Response
	if err := json.Unmarshal(respRaw, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Risk != schema.RiskDanger {
		t.Fatalf("classification = %v, want deny", resp.Risk)
	}
	if resp.RuleID != "no-exec" {
		t.Errorf("ruleID = %q, want no-exec", resp.RuleID)
	}
}
