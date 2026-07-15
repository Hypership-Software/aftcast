package policy

import (
	"math/rand"
	"testing"

	"github.com/Hypership-Software/aftcast/internal/schema"
	"github.com/cedar-policy/cedar-go"
)

var allClasses = []schema.ToolClass{
	schema.ClassExec, schema.ClassFileRead, schema.ClassFileWrite,
	schema.ClassNetFetch, schema.ClassNetSearch, schema.ClassMCP,
	schema.ClassAgentSpawn, schema.ClassSkill, schema.ClassOther,
}

func randString(r *rand.Rand) string {
	const alpha = "abcdefghijklmnopqrstuvwxyz"
	b := make([]byte, 3+r.Intn(6))
	for i := range b {
		b[i] = alpha[r.Intn(len(alpha))]
	}
	return string(b)
}

func randDesc(r *rand.Rand) schema.Descriptor {
	d := schema.Descriptor{
		Version:   schema.SchemaVersion,
		SessionID: randString(r),
		ToolClass: allClasses[r.Intn(len(allClasses))],
		ToolRaw:   randString(r),
		Tainted:   r.Intn(2) == 0,
	}
	switch d.ToolClass {
	case schema.ClassExec:
		d.Verbs = []string{randString(r)}
		d.Argv = []string{randString(r), randString(r)}
	case schema.ClassFileRead, schema.ClassFileWrite:
		d.Files = []string{"/" + randString(r)}
	case schema.ClassNetFetch, schema.ClassNetSearch:
		d.Domain = randString(r) + ".com"
	}
	return d
}

func TestEvalIsDeterministic(t *testing.T) {
	ps := cedar.NewPolicySet()
	ps.Add("permit-all", mustPolicy(t, `permit(principal, action, resource);`))
	ps.Add("no-exec", mustPolicy(t, `forbid(principal, action == Action::"exec", resource);`))
	eng := NewEngine(ps)
	r := rand.New(rand.NewSource(1))
	for i := 0; i < 10000; i++ {
		d := randDesc(r)
		v1, id1 := eng.Eval(d)
		v2, id2 := eng.Eval(d)
		if v1 != v2 || id1 != id2 {
			t.Fatalf("non-deterministic for %+v: (%v,%s) vs (%v,%s)", d, v1, id1, v2, id2)
		}
	}
}

func TestEmptyPolicySetAlwaysAsks(t *testing.T) {
	eng := NewEngine(cedar.NewPolicySet())
	r := rand.New(rand.NewSource(2))
	for i := 0; i < 2000; i++ {
		d := randDesc(r)
		if v, _ := eng.Eval(d); v != schema.RiskUnknown {
			t.Fatalf("empty policy set gave %v for %+v, want ask", v, d)
		}
	}
}

func TestForbidDominatesPermitAll(t *testing.T) {
	ps := cedar.NewPolicySet()
	ps.Add("permit-all", mustPolicy(t, `permit(principal, action, resource);`))
	ps.Add("no-exec", mustPolicy(t, `forbid(principal, action == Action::"exec", resource);`))
	eng := NewEngine(ps)
	r := rand.New(rand.NewSource(3))
	for i := 0; i < 2000; i++ {
		d := randDesc(r)
		v, _ := eng.Eval(d)
		if d.ToolClass == schema.ClassExec {
			if v != schema.RiskDanger {
				t.Fatalf("exec must be denied (forbid dominates), got %v", v)
			}
		} else if v != schema.RiskSafe {
			t.Fatalf("non-exec must be allowed by permit-all, got %v for %+v", v, d)
		}
	}
}
