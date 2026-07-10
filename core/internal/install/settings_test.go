package install

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

const testHTTPURL = "http://127.0.0.1:47100/hook"
const testCommand = "C:/Users/dev/.gated/bin/gated.exe hook claudecode"

func testConfig() HookConfig {
	return HookConfig{HTTPURL: testHTTPURL, Command: testCommand, Timeout: 30}
}

// parseHooks pulls the hooks subtree out of a settings blob for assertions.
func parseHooks(t *testing.T, b []byte) map[string][]group {
	t.Helper()
	var top map[string]json.RawMessage
	if err := json.Unmarshal(b, &top); err != nil {
		t.Fatalf("settings not valid JSON: %v\n%s", err, b)
	}
	raw, ok := top["hooks"]
	if !ok {
		return nil
	}
	var hooks map[string][]group
	if err := json.Unmarshal(raw, &hooks); err != nil {
		t.Fatalf("hooks subtree malformed: %v", err)
	}
	return hooks
}

func TestMergeHooksWiresHTTPAndSessionStartCommand(t *testing.T) {
	out, err := MergeHooks([]byte(`{}`), testConfig())
	if err != nil {
		t.Fatal(err)
	}
	hooks := parseHooks(t, out)

	// Every managed HTTP event is an http hook pointed at the daemon.
	for _, ev := range []string{"PreToolUse", "PostToolUse", "PostToolUseFailure", "UserPromptSubmit", "Stop"} {
		groups := hooks[ev]
		if len(groups) == 0 {
			t.Fatalf("%s: no hook group written", ev)
		}
		var found bool
		for _, g := range groups {
			for _, h := range g.Hooks {
				var probe hookProbe
				_ = json.Unmarshal(h, &probe)
				if probe.Type == "http" && probe.URL == testHTTPURL {
					found = true
				}
			}
		}
		if !found {
			t.Errorf("%s: expected an http hook at %s", ev, testHTTPURL)
		}
	}

	// Tool events must carry matcher "*"; non-tool events must omit it.
	pre := hooks["PreToolUse"][0]
	if pre.Matcher == nil || *pre.Matcher != "*" {
		t.Errorf("PreToolUse matcher = %v, want \"*\"", pre.Matcher)
	}
	ups := hooks["UserPromptSubmit"][0]
	if ups.Matcher != nil {
		t.Errorf("UserPromptSubmit matcher = %q, want omitted", *ups.Matcher)
	}

	// SessionStart is a COMMAND hook (HTTP doesn't fire it in 2.1.205).
	ss := hooks["SessionStart"]
	if len(ss) == 0 {
		t.Fatal("SessionStart: no hook group written")
	}
	var probe hookProbe
	_ = json.Unmarshal(ss[0].Hooks[0], &probe)
	if probe.Type != "command" || probe.Command != testCommand {
		t.Errorf("SessionStart hook = %+v, want command %q", probe, testCommand)
	}
}

func TestMergeHooksPreservesUserEntries(t *testing.T) {
	orig := []byte(`{
	  "permissions": {"allow": ["Bash(node:*)"]},
	  "hooks": {
	    "PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "my-linter"}]}]
	  }
	}`)
	out, err := MergeHooks(orig, testConfig())
	if err != nil {
		t.Fatal(err)
	}

	// Other top-level keys survive.
	var top map[string]json.RawMessage
	_ = json.Unmarshal(out, &top)
	if _, ok := top["permissions"]; !ok {
		t.Error("permissions key dropped")
	}

	// The user's own PreToolUse hook is still there alongside ours.
	if !strings.Contains(string(out), "my-linter") {
		t.Error("user's linter hook was dropped")
	}
	if !strings.Contains(string(out), testHTTPURL) {
		t.Error("gate http hook not added")
	}
}

func TestMergeHooksIdempotent(t *testing.T) {
	once, err := MergeHooks([]byte(`{}`), testConfig())
	if err != nil {
		t.Fatal(err)
	}
	twice, err := MergeHooks(once, testConfig())
	if err != nil {
		t.Fatal(err)
	}
	if !sameJSON(t, once, twice) {
		t.Errorf("merge not idempotent:\n once=%s\n twice=%s", once, twice)
	}
}

func TestRemoveHooksRestoresSemantically(t *testing.T) {
	cases := map[string][]byte{
		"empty":         []byte(`{}`),
		"only-perms":    []byte(`{"permissions":{"allow":["Bash(node:*)"]}}`),
		"user-has-hook": []byte(`{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"my-linter"}]}]}}`),
	}
	for name, orig := range cases {
		t.Run(name, func(t *testing.T) {
			merged, err := MergeHooks(orig, testConfig())
			if err != nil {
				t.Fatal(err)
			}
			restored, err := RemoveHooks(merged)
			if err != nil {
				t.Fatal(err)
			}
			if !sameJSON(t, orig, restored) {
				t.Errorf("remove did not restore original:\n orig=%s\n got =%s", orig, restored)
			}
		})
	}
}

// sameJSON compares two JSON blobs for semantic (not byte) equality. Byte
// identity is not guaranteed through a JSON round-trip (key order / whitespace
// normalize); semantic identity is the real contract — the user's config works
// the same after uninstall.
func sameJSON(t *testing.T, a, b []byte) bool {
	t.Helper()
	var av, bv any
	if err := json.Unmarshal(a, &av); err != nil {
		t.Fatalf("a not JSON: %v", err)
	}
	if err := json.Unmarshal(b, &bv); err != nil {
		t.Fatalf("b not JSON: %v", err)
	}
	return reflect.DeepEqual(av, bv)
}
