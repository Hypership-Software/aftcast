package handoff

import (
	"strings"
	"testing"

	"github.com/Hypership-Software/aftcast/internal/audit"
)

func render(t *testing.T, facts []SessionFacts, rep audit.Report) string {
	t.Helper()
	return string(Render("feature/x", facts, rep))
}

func baseFacts() []SessionFacts {
	return []SessionFacts{{
		ID: "5c71c909-aaaa", Started: "2026-07-23T10:00:00Z", Ended: "2026-07-23T11:00:00Z",
		Events: 42, Prompts: 3, Failures: 1, Deliveries: 1,
		CommitSHAs: []string{"bb16536"}, Skills: []string{"superpowers:test-driven-development"},
		PermissionModes: []string{"default"}, MaxContext: 91000,
	}}
}

func TestRenderAttestationScopedAndVerified(t *testing.T) {
	out := render(t, baseFacts(), audit.Report{OK: true, Count: 9145})
	for _, want := range []string{
		"# Handoff digest",
		"feature/x",
		"Within the observed sessions",
		"no review agent or review skill ran",
		"9,145",
		"chain verified",
		"bb16536",
		"Not captured",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("digest missing %q", want)
		}
	}
}

func TestRenderBrokenChainIsLoud(t *testing.T) {
	out := render(t, baseFacts(), audit.Report{OK: false, Count: 10, BadSeq: 7, Detail: "hash mismatch (record was altered)"})
	if !strings.Contains(out, "FAILED") || !strings.Contains(out, "record 7") {
		t.Errorf("broken chain must be loud, got:\n%s", out)
	}
	if strings.Contains(out, "chain verified") {
		t.Error("broken chain must not read as verified")
	}
}

func TestRenderNamesReviewShapedActors(t *testing.T) {
	f := baseFacts()
	f[0].Subagents = []string{"code-reviewer"}
	out := render(t, f, audit.Report{OK: true, Count: 42})
	if strings.Contains(out, "no review agent or review skill ran") {
		t.Error("review-shaped subagent must suppress the absence claim")
	}
	if !strings.Contains(out, "code-reviewer") {
		t.Error("review-shaped subagent must be named")
	}
}

func TestRenderNoSessionsIsHonest(t *testing.T) {
	out := render(t, nil, audit.Report{OK: true, Count: 42})
	if !strings.Contains(out, "No captured session") {
		t.Errorf("empty selection needs the honesty line, got:\n%s", out)
	}
}

func TestRenderNarrationBlockCarriesCoordinatesOnly(t *testing.T) {
	out := render(t, baseFacts(), audit.Report{OK: true, Count: 42})
	if !strings.Contains(out, "5c71c909-aaaa") {
		t.Error("narration block must list session ids")
	}
	if !strings.Contains(out, "## Intent") || !strings.Contains(out, "## Journey") {
		t.Error("narrative sections missing")
	}
}
