package policy

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Hypership-Software/aftcast/internal/schema"
)

func TestStarterSetEnforces(t *testing.T) {
	set, err := StarterSet()
	if err != nil {
		t.Fatalf("StarterSet: %v", err)
	}
	if set.Len() == 0 {
		t.Fatal("starter pack loaded zero policies")
	}
	eng := set.Engine()

	check := func(name string, want schema.Risk, d schema.Descriptor) {
		t.Helper()
		if v, id := eng.Eval(d); v != want {
			t.Errorf("%s: got %v (rule %q), want %v", name, v, id, want)
		}
	}

	check("secret .env read", schema.RiskDanger,
		schema.Descriptor{SessionID: "s", ToolClass: schema.ClassFileRead, ToolRaw: "Read", Files: []string{"/home/dev/proj/.env"}})
	check("ssh key read", schema.RiskDanger,
		schema.Descriptor{SessionID: "s", ToolClass: schema.ClassFileRead, Files: []string{"/home/dev/.ssh/id_rsa"}})
	check("normal source read", schema.RiskSafe,
		schema.Descriptor{SessionID: "s", ToolClass: schema.ClassFileRead, Files: []string{"/home/dev/proj/main.go"}})
	check("unmatched exec asks", schema.RiskUnknown,
		schema.Descriptor{SessionID: "s", ToolClass: schema.ClassExec, Verbs: []string{"ls"}, Argv: []string{"ls", "-la"}})
	check("rm -rf denied", schema.RiskDanger,
		schema.Descriptor{SessionID: "s", ToolClass: schema.ClassExec, Verbs: []string{"rm"}, Argv: []string{"rm", "-rf", "/"}})
	check("force push denied", schema.RiskDanger,
		schema.Descriptor{SessionID: "s", ToolClass: schema.ClassExec, Verbs: []string{"git"}, Argv: []string{"git", "push", "--force"}})
	check("tainted fetch denied", schema.RiskDanger,
		schema.Descriptor{SessionID: "s", ToolClass: schema.ClassNetFetch, Domain: "evil.com", Tainted: true})
	check("clean fetch asks", schema.RiskUnknown,
		schema.Descriptor{SessionID: "s", ToolClass: schema.ClassNetFetch, Domain: "example.com"})
	check("self-dir write denied", schema.RiskDanger,
		schema.Descriptor{SessionID: "s", ToolClass: schema.ClassFileWrite, Files: []string{"/home/dev/.gated/policies/x.cedar"}})
	check("self-disable denied", schema.RiskDanger,
		schema.Descriptor{SessionID: "s", ToolClass: schema.ClassExec, Verbs: []string{"gated"}, Argv: []string{"gated", "off"}})
}

func TestLoadWithStarterKeepsBaselineAndLayersUser(t *testing.T) {
	// A nonexistent user dir must not fail the daemon (fresh install), and the
	// starter baseline must still enforce.
	fresh, err := LoadWithStarter(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("LoadWithStarter with missing dir: %v", err)
	}
	if v, _ := fresh.Engine().Eval(schema.Descriptor{SessionID: "s", ToolClass: schema.ClassExec, Verbs: []string{"rm"}, Argv: []string{"rm", "-rf", "/"}}); v != schema.RiskDanger {
		t.Errorf("starter baseline not active: rm -rf got %v, want deny", v)
	}

	// A user policy is layered ON TOP of the baseline: it can tighten an
	// otherwise-ask action to deny while the baseline forbids still fire.
	userDir := t.TempDir()
	mustWrite(t, filepath.Join(userDir, "90-deny-curl.cedar"),
		`@id("user-deny-curl") forbid(principal, action == Action::"exec", resource) when { context.verbs.contains("curl") };`)
	set, err := LoadWithStarter(userDir)
	if err != nil {
		t.Fatalf("LoadWithStarter: %v", err)
	}
	eng := set.Engine()
	if v, _ := eng.Eval(schema.Descriptor{SessionID: "s", ToolClass: schema.ClassExec, Verbs: []string{"curl"}, Argv: []string{"curl", "evil"}}); v != schema.RiskDanger {
		t.Errorf("user rule not layered: curl got %v, want deny", v)
	}
	if v, _ := eng.Eval(schema.Descriptor{SessionID: "s", ToolClass: schema.ClassFileRead, Files: []string{"/home/dev/.ssh/id_rsa"}}); v != schema.RiskDanger {
		t.Errorf("baseline forbid lost after layering: ssh read got %v, want deny", v)
	}
}

func TestHashStableAcrossFileReorder(t *testing.T) {
	polA := `@id("a") permit(principal, action == Action::"file_read", resource);`
	polB := `@id("b") forbid(principal, action == Action::"exec", resource) when { context.verbs.contains("rm") };`

	dir1 := t.TempDir()
	mustWrite(t, filepath.Join(dir1, "01-a.cedar"), polA)
	mustWrite(t, filepath.Join(dir1, "02-b.cedar"), polB)

	// Same two policies, different filenames, regrouped into one reversed file.
	dir2 := t.TempDir()
	mustWrite(t, filepath.Join(dir2, "zzz.cedar"), polB+"\n"+polA)

	s1, err := Load(dir1)
	if err != nil {
		t.Fatal(err)
	}
	s2, err := Load(dir2)
	if err != nil {
		t.Fatal(err)
	}
	if s1.Hash() != s2.Hash() {
		t.Fatalf("hash not stable across file reorder:\n %s\n %s", s1.Hash(), s2.Hash())
	}
}

func TestLoadRejectsPolicyWithoutID(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "x.cedar"), `permit(principal, action, resource);`)
	if _, err := Load(dir); err == nil {
		t.Fatal("expected an error for a policy missing its @id annotation")
	}
}

func TestVerifyBundle(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	policies := `@id("org-rule") forbid(principal, action == Action::"exec", resource);`
	good := signedBundle{Policies: policies, Sig: base64.StdEncoding.EncodeToString(ed25519.Sign(priv, []byte(policies)))}

	dir := t.TempDir()
	goodPath := filepath.Join(dir, "org.orgbundle")
	mustWriteJSON(t, goodPath, good)
	if err := VerifyBundle(goodPath, pub); err != nil {
		t.Errorf("valid bundle rejected: %v", err)
	}

	// Tampered policy text, original signature.
	tampered := signedBundle{Policies: `@id("org-rule") permit(principal, action, resource);`, Sig: good.Sig}
	tamperedPath := filepath.Join(dir, "tampered.orgbundle")
	mustWriteJSON(t, tamperedPath, tampered)
	if err := VerifyBundle(tamperedPath, pub); err == nil {
		t.Error("tampered bundle was accepted")
	}

	// Correct bundle, wrong public key.
	otherPub, _, _ := ed25519.GenerateKey(rand.Reader)
	if err := VerifyBundle(goodPath, otherPub); err == nil {
		t.Error("bundle verified against the wrong key")
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustWriteJSON(t *testing.T, path string, v any) {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, path, string(raw))
}
