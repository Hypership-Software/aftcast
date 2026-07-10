package policy

import (
	"encoding/json"
	"testing"

	"github.com/Hypership-Software/atlas/internal/schema"
	"github.com/cedar-policy/cedar-go"
)

func contextMap(t *testing.T, req cedar.Request) map[string]any {
	t.Helper()
	raw, err := json.Marshal(req.Context)
	if err != nil {
		t.Fatalf("marshal context: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal context: %v", err)
	}
	return m
}

func TestToCedarExec(t *testing.T) {
	d := schema.Descriptor{
		Version:   schema.SchemaVersion,
		SessionID: "s1",
		ToolClass: schema.ClassExec,
		ToolRaw:   "Bash",
		Argv:      []string{"rm", "-rf", "/"},
		Verbs:     []string{"rm"},
	}
	req, entities := ToCedar(d)

	if want := cedar.NewEntityUID("Action", "exec"); req.Action != want {
		t.Errorf("action = %v, want %v", req.Action, want)
	}
	if want := cedar.NewEntityUID("Command", "rm"); req.Resource != want {
		t.Errorf("resource = %v, want %v", req.Resource, want)
	}
	if want := cedar.NewEntityUID("Session", "s1"); req.Principal != want {
		t.Errorf("principal = %v, want %v", req.Principal, want)
	}
	ctx := contextMap(t, req)
	argv, ok := ctx["argv"].([]any)
	if !ok || len(argv) != 3 {
		t.Errorf("context.argv = %v, want 3 tokens", ctx["argv"])
	}
	if _, hasPrincipal := entities[req.Principal]; !hasPrincipal {
		t.Errorf("entities missing the principal Session entity")
	}
}

func TestToCedarNetFetch(t *testing.T) {
	d := schema.Descriptor{
		Version:   schema.SchemaVersion,
		SessionID: "s1",
		ToolClass: schema.ClassNetFetch,
		ToolRaw:   "WebFetch",
		Domain:    "example.com",
	}
	req := mustReq(ToCedar(d))

	if want := cedar.NewEntityUID("Action", "net_fetch"); req.Action != want {
		t.Errorf("action = %v, want %v", req.Action, want)
	}
	if want := cedar.NewEntityUID("Domain", "example.com"); req.Resource != want {
		t.Errorf("resource = %v, want %v", req.Resource, want)
	}
	ctx := contextMap(t, req)
	if ctx["domain"] != "example.com" {
		t.Errorf("context.domain = %v, want example.com", ctx["domain"])
	}
}

func TestToCedarTaintOnPrincipal(t *testing.T) {
	d := schema.Descriptor{SessionID: "s1", ToolClass: schema.ClassFileRead, Files: []string{"/x"}, Tainted: true}
	_, entities := ToCedar(d)
	uid := cedar.NewEntityUID("Session", "s1")
	raw, err := json.Marshal(entities[uid].Attributes)
	if err != nil {
		t.Fatal(err)
	}
	var attrs map[string]any
	if err := json.Unmarshal(raw, &attrs); err != nil {
		t.Fatal(err)
	}
	if attrs["tainted"] != true {
		t.Errorf("principal attr tainted = %v, want true", attrs["tainted"])
	}
}

func mustReq(req cedar.Request, _ any) cedar.Request { return req }
