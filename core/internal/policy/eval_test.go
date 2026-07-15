package policy

import (
	"testing"

	"github.com/Hypership-Software/aftcast/internal/schema"
	"github.com/cedar-policy/cedar-go"
)

func mustPolicy(t *testing.T, src string) *cedar.Policy {
	t.Helper()
	var p cedar.Policy
	if err := p.UnmarshalCedar([]byte(src)); err != nil {
		t.Fatalf("parse policy %q: %v", src, err)
	}
	return &p
}

func execDesc() schema.Descriptor {
	return schema.Descriptor{
		SessionID: "s1",
		ToolClass: schema.ClassExec,
		ToolRaw:   "Bash",
		Argv:      []string{"rm", "-rf", "/"},
		Verbs:     []string{"rm"},
	}
}

func TestEvalPermitAllows(t *testing.T) {
	ps := cedar.NewPolicySet()
	ps.Add("permit-all", mustPolicy(t, `permit(principal, action, resource);`))
	v, _ := NewEngine(ps).Eval(execDesc())
	if v != schema.RiskSafe {
		t.Fatalf("risk = %v, want safe", v)
	}
}

func TestEvalForbidDenies(t *testing.T) {
	ps := cedar.NewPolicySet()
	ps.Add("no-exec", mustPolicy(t, `forbid(principal, action == Action::"exec", resource);`))
	v, id := NewEngine(ps).Eval(execDesc())
	if v != schema.RiskDanger {
		t.Fatalf("risk = %v, want danger", v)
	}
	if id != "no-exec" {
		t.Errorf("ruleID = %q, want no-exec", id)
	}
}

func TestEvalNoMatchAsks(t *testing.T) {
	ps := cedar.NewPolicySet()
	v, id := NewEngine(ps).Eval(execDesc())
	if v != schema.RiskUnknown {
		t.Fatalf("risk = %v, want unknown", v)
	}
	if id != "unclassified" {
		t.Errorf("ruleID = %q, want unclassified", id)
	}
}

func TestEvalForbidDominatesPermit(t *testing.T) {
	ps := cedar.NewPolicySet()
	ps.Add("permit-all", mustPolicy(t, `permit(principal, action, resource);`))
	ps.Add("no-exec", mustPolicy(t, `forbid(principal, action == Action::"exec", resource);`))
	v, _ := NewEngine(ps).Eval(execDesc())
	if v != schema.RiskDanger {
		t.Fatalf("risk = %v, want danger (forbid dominates)", v)
	}
}

// The critical safety property (Rev 2.1): Cedar silently SKIPS a policy that
// errors during evaluation. A forbid that references a missing attribute would
// therefore not fire — flipping a would-be deny to allow/ask. The engine must
// treat any forbid eval-error as a deny.
func TestEvalForbidWithEvalErrorFailsClosed(t *testing.T) {
	ps := cedar.NewPolicySet()
	ps.Add("bad-forbid", mustPolicy(t, `forbid(principal, action, resource) when { context.nope == "x" };`))
	v, id := NewEngine(ps).Eval(execDesc())
	if v != schema.RiskDanger {
		t.Fatalf("forbid eval-error must fail closed to danger, got %v", v)
	}
	if id != "bad-forbid" {
		t.Errorf("ruleID = %q, want bad-forbid", id)
	}
}
