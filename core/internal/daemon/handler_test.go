package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Hypership-Software/aftcast/internal/ipc"
	"github.com/Hypership-Software/aftcast/internal/schema"
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

type fakeTaint struct {
	applyCalls, markCalls int
	tainted               bool
}

func (f *fakeTaint) Apply(*schema.Descriptor)                 { f.applyCalls++ }
func (f *fakeTaint) MarkFromResult(string, schema.Descriptor) { f.markCalls++ }
func (f *fakeTaint) IsTainted(string) bool                    { return f.tainted }

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

// A tainted session stamps Taint onto the recorded event, so taint is durable in
// the log (not just in-memory). Both the pre_tool and non-pre paths must stamp it.
func TestHandleStampsSessionTaint(t *testing.T) {
	rec := &fakeRecorder{}
	h := handlerWith(&fakeEval{v: schema.RiskUnknown}, &fakeTaint{tainted: true}, rec)
	if _, err := h.Handle(preToolReq()); err != nil {
		t.Fatal(err)
	}
	stop := Request{Event: schema.TelemetryEvent{EventType: schema.EventStop, SessionID: "s"}}
	if _, err := h.Handle(stop); err != nil {
		t.Fatal(err)
	}
	if len(rec.events) != 2 {
		t.Fatalf("recorded %d events, want 2", len(rec.events))
	}
	for i, e := range rec.events {
		if !e.Taint {
			t.Errorf("event %d (%s) Taint = false, want true", i, e.EventType)
		}
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
	t.Setenv("AFTCAST_IPC_ID", "daemon-serve-test")
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

// A stop event samples context occupancy from the harness transcript; every
// other path leaves it absent. The sampler is optional — a handler built
// without one records stops unchanged.
func TestHandleSamplesContextOnStop(t *testing.T) {
	rec := &fakeRecorder{}
	h := NewHandler(Deps{
		Eval:   &fakeEval{v: schema.RiskSafe},
		Taint:  &fakeTaint{},
		Record: rec,
		Sample: func(path string) int64 {
			if path != "/tmp/session.jsonl" {
				t.Errorf("sample path = %q", path)
			}
			return 92500
		},
	})
	stop := Request{
		Event:      schema.TelemetryEvent{EventType: schema.EventStop, SessionID: "s"},
		Descriptor: schema.Descriptor{SessionID: "s", TranscriptPath: "/tmp/session.jsonl"},
	}
	if _, err := h.Handle(stop); err != nil {
		t.Fatal(err)
	}
	if got := rec.events[0].ContextTokens; got != 92500 {
		t.Errorf("context_tokens = %d, want 92500", got)
	}

	pre := preToolReq()
	pre.Descriptor.TranscriptPath = "/tmp/session.jsonl"
	if _, err := h.Handle(pre); err != nil {
		t.Fatal(err)
	}
	if got := rec.events[1].ContextTokens; got != 0 {
		t.Errorf("pre_tool context_tokens = %d, want 0", got)
	}
}

func TestHandleStopWithoutSamplerIsUnchanged(t *testing.T) {
	rec := &fakeRecorder{}
	h := handlerWith(&fakeEval{}, &fakeTaint{}, rec)
	stop := Request{
		Event:      schema.TelemetryEvent{EventType: schema.EventStop, SessionID: "s"},
		Descriptor: schema.Descriptor{SessionID: "s", TranscriptPath: "/tmp/session.jsonl"},
	}
	if _, err := h.Handle(stop); err != nil {
		t.Fatal(err)
	}
	if got := rec.events[0].ContextTokens; got != 0 {
		t.Errorf("context_tokens = %d, want 0", got)
	}
}

func TestHandleAssignsTurnIndexPerSession(t *testing.T) {
	rec := &fakeRecorder{}
	h := handlerWith(&fakeEval{v: schema.RiskUnknown}, &fakeTaint{}, rec)

	send := func(et schema.EventType, session string) {
		req := Request{Event: schema.TelemetryEvent{EventType: et, SessionID: session}}
		if et == schema.EventPreTool {
			req.Descriptor = schema.Descriptor{SessionID: session}
		}
		if _, err := h.Handle(req); err != nil {
			t.Fatal(err)
		}
	}

	send(schema.EventSessionStart, "a")
	send(schema.EventUserPrompt, "a")
	send(schema.EventPreTool, "a")
	send(schema.EventPostTool, "a")
	send(schema.EventUserPrompt, "a")
	send(schema.EventPreTool, "a")
	send(schema.EventSessionStart, "b")
	send(schema.EventUserPrompt, "b")

	want := []int{0, 1, 1, 1, 2, 2, 0, 1}
	if len(rec.events) != len(want) {
		t.Fatalf("recorded %d events, want %d", len(rec.events), len(want))
	}
	for i, w := range want {
		if got := rec.events[i].TurnIndex; got != w {
			t.Fatalf("event %d (%s/%s) turn_index = %d, want %d",
				i, rec.events[i].SessionID, rec.events[i].EventType, got, w)
		}
	}
}

// One Handler serves both the IPC listener and the HTTP hook endpoint from
// separate goroutines, so the turn counter is shared mutable state.
func TestHandleTurnIndexIsConcurrencySafe(t *testing.T) {
	h := handlerWith(&fakeEval{v: schema.RiskUnknown}, &fakeTaint{}, &syncRecorder{})
	var wg sync.WaitGroup
	for i := range 32 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			session := fmt.Sprintf("s%d", i%4)
			req := Request{Event: schema.TelemetryEvent{EventType: schema.EventUserPrompt, SessionID: session}}
			if _, err := h.Handle(req); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
}

type syncRecorder struct {
	mu     sync.Mutex
	events []schema.TelemetryEvent
}

func (s *syncRecorder) Record(e schema.TelemetryEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, e)
	return nil
}
