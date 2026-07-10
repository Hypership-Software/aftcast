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
	v     schema.Verdict
	id    string
	calls int
}

func (f *fakeEval) Eval(schema.Descriptor) (schema.Verdict, string) {
	f.calls++
	return f.v, f.id
}

type fakeTaint struct{ applyCalls, markCalls int }

func (f *fakeTaint) Apply(*schema.Descriptor)                 { f.applyCalls++ }
func (f *fakeTaint) MarkFromResult(string, schema.Descriptor) { f.markCalls++ }

type fakeApprover struct {
	v      schema.Verdict
	reason string
	calls  int
}

func (f *fakeApprover) Request(schema.Descriptor) (schema.Verdict, string) {
	f.calls++
	return f.v, f.reason
}

type fakeRecorder struct{ events []schema.TelemetryEvent }

func (f *fakeRecorder) Record(e schema.TelemetryEvent) error {
	f.events = append(f.events, e)
	return nil
}

func handlerWith(e Evaluator, tt Tainter, a Approver, r Recorder) *Handler {
	return NewHandler(Deps{Eval: e, Taint: tt, Approve: a, Record: r})
}

func preToolReq() Request {
	return Request{
		Event:      schema.TelemetryEvent{EventType: schema.EventPreTool, SessionID: "s", ToolClass: schema.ClassExec},
		Descriptor: schema.Descriptor{SessionID: "s", ToolClass: schema.ClassExec, Verbs: []string{"rm"}},
	}
}

func TestHandleDenyRecordsOnceNoApprover(t *testing.T) {
	ev := &fakeEval{v: schema.VerdictDeny, id: "no-exec"}
	ap := &fakeApprover{}
	rec := &fakeRecorder{}
	resp, err := handlerWith(ev, &fakeTaint{}, ap, rec).Handle(preToolReq())
	if err != nil {
		t.Fatal(err)
	}
	if resp.Verdict != schema.VerdictDeny {
		t.Fatalf("verdict = %v, want deny", resp.Verdict)
	}
	if ap.calls != 0 {
		t.Errorf("approver was called on a deny (%d times)", ap.calls)
	}
	if len(rec.events) != 1 {
		t.Fatalf("recorded %d events, want exactly 1", len(rec.events))
	}
	if rec.events[0].EventType != schema.EventBlock {
		t.Errorf("deny recorded as %v, want block", rec.events[0].EventType)
	}
	if rec.events[0].Verdict != schema.VerdictDeny {
		t.Errorf("recorded verdict = %v, want deny", rec.events[0].Verdict)
	}
}

func TestHandleAskConsultsApprover(t *testing.T) {
	ev := &fakeEval{v: schema.VerdictAsk, id: "default-ask"}
	ap := &fakeApprover{v: schema.VerdictAllow, reason: "user approved"}
	rec := &fakeRecorder{}
	resp, _ := handlerWith(ev, &fakeTaint{}, ap, rec).Handle(preToolReq())
	if ap.calls != 1 {
		t.Fatalf("approver calls = %d, want 1", ap.calls)
	}
	if resp.Verdict != schema.VerdictAllow {
		t.Fatalf("verdict = %v, want allow", resp.Verdict)
	}
	if resp.Reason != "user approved" {
		t.Errorf("reason = %q, want %q", resp.Reason, "user approved")
	}
	if len(rec.events) != 1 {
		t.Fatalf("recorded %d events, want 1", len(rec.events))
	}
}

func TestHandleAllowAppliesAndMarksTaint(t *testing.T) {
	ev := &fakeEval{v: schema.VerdictAllow, id: "permit"}
	tt := &fakeTaint{}
	resp, _ := handlerWith(ev, tt, &fakeApprover{}, &fakeRecorder{}).Handle(preToolReq())
	if resp.Verdict != schema.VerdictAllow {
		t.Fatalf("verdict = %v, want allow", resp.Verdict)
	}
	if tt.applyCalls != 1 {
		t.Errorf("Apply calls = %d, want 1", tt.applyCalls)
	}
	if tt.markCalls != 1 {
		t.Errorf("MarkFromResult calls = %d, want 1 (allowed action marks taint)", tt.markCalls)
	}
}

func TestHandlePostToolRecordsOnlyAndSkipsEval(t *testing.T) {
	ev := &fakeEval{v: schema.VerdictDeny, id: "should-not-be-used"}
	rec := &fakeRecorder{}
	req := Request{Event: schema.TelemetryEvent{EventType: schema.EventPostTool, SessionID: "s"}}
	resp, _ := handlerWith(ev, &fakeTaint{}, &fakeApprover{}, rec).Handle(req)
	if ev.calls != 0 {
		t.Errorf("evaluator was called on a post_tool event (%d times)", ev.calls)
	}
	if resp.Verdict != schema.VerdictAllow {
		t.Errorf("post_tool verdict = %v, want allow", resp.Verdict)
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

	h := handlerWith(&fakeEval{v: schema.VerdictDeny, id: "no-exec"}, &fakeTaint{}, &fakeApprover{}, &fakeRecorder{})
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
	if resp.Verdict != schema.VerdictDeny {
		t.Fatalf("verdict = %v, want deny", resp.Verdict)
	}
	if resp.RuleID != "no-exec" {
		t.Errorf("ruleID = %q, want no-exec", resp.RuleID)
	}
}
