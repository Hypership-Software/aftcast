// Package install wires the gate into the harness: it merges the daemon's hook
// entries into Claude Code's settings.json (idempotently, without disturbing the
// user's own hooks) and provides init/uninstall/doctor. The hook schema and the
// SessionStart command-hook fallback were validated live against Claude Code
// 2.1.205 in the Sprint-0 spike (findings §A/§D).
package install

import (
	"bytes"
	"encoding/json"
	"net/url"
	"strings"
)

// HookConfig is what the settings writer needs to emit the gate's hook entries.
type HookConfig struct {
	// HTTPURL is the daemon's localhost hook endpoint, e.g.
	// http://127.0.0.1:47100/hook (baked into settings; the port must be stable).
	HTTPURL string
	// Command is the SessionStart command-hook invocation (forward slashes),
	// e.g. C:/Users/dev/.gated/bin/gated.exe hook claudecode. SessionStart does
	// not fire over HTTP in 2.1.205, so it uses the shim once per session.
	Command string
	// Timeout is the per-hook timeout in seconds (0 omits it).
	Timeout int
}

// managedHTTPEvents fire reliably over HTTP (Sprint-0). PostToolUseFailure
// carries the exit code that the outcome analytics depend on.
var managedHTTPEvents = []string{
	"PreToolUse", "PostToolUse", "PostToolUseFailure",
	"UserPromptSubmit", "Stop", "SessionEnd",
}

// managedCommandEvents must be command hooks (they do not fire over HTTP).
var managedCommandEvents = []string{"SessionStart"}

// isToolEvent reports whether an event matches on tool name (needs matcher "*").
func isToolEvent(e string) bool {
	switch e {
	case "PreToolUse", "PostToolUse", "PostToolUseFailure":
		return true
	}
	return false
}

// hookProbe is one hook entry — both the shape we emit and the shape we inspect
// to recognize our own entries.
type hookProbe struct {
	Type    string `json:"type"`
	URL     string `json:"url,omitempty"`
	Command string `json:"command,omitempty"`
	Timeout int    `json:"timeout,omitempty"`
}

// group is one {matcher?, hooks:[...]} entry under an event. User hooks are kept
// as raw bytes so a merge/unmerge round-trip preserves them.
type group struct {
	Matcher *string           `json:"matcher,omitempty"`
	Hooks   []json.RawMessage `json:"hooks"`
}

// MergeHooks returns settings.json with the gate's hook entries present for every
// managed event. It is idempotent (re-running replaces our entries rather than
// duplicating them) and never disturbs the user's own hooks or other keys.
func MergeHooks(orig []byte, cfg HookConfig) ([]byte, error) {
	top, hooks, err := parse(orig)
	if err != nil {
		return nil, err
	}
	stripGatedEverywhere(hooks)

	httpHook, err := json.Marshal(hookProbe{Type: "http", URL: cfg.HTTPURL, Timeout: cfg.Timeout})
	if err != nil {
		return nil, err
	}
	cmdHook, err := json.Marshal(hookProbe{Type: "command", Command: cfg.Command, Timeout: cfg.Timeout})
	if err != nil {
		return nil, err
	}
	for _, ev := range managedHTTPEvents {
		hooks[ev] = append(hooks[ev], newGroup(isToolEvent(ev), httpHook))
	}
	for _, ev := range managedCommandEvents {
		hooks[ev] = append(hooks[ev], newGroup(false, cmdHook))
	}
	pruneEmptyEvents(hooks)
	return assemble(top, hooks)
}

// RemoveHooks returns settings.json with only the gate's own hook entries
// removed, leaving the user's hooks and every other key intact.
func RemoveHooks(orig []byte) ([]byte, error) {
	top, hooks, err := parse(orig)
	if err != nil {
		return nil, err
	}
	stripGatedEverywhere(hooks)
	pruneEmptyEvents(hooks)
	return assemble(top, hooks)
}

func newGroup(tool bool, hook json.RawMessage) group {
	g := group{Hooks: []json.RawMessage{hook}}
	if tool {
		star := "*"
		g.Matcher = &star
	}
	return g
}

// parse splits settings into its top-level keys (preserved as raw bytes) and the
// decoded hooks subtree. Empty/whitespace input is treated as {}.
func parse(orig []byte) (map[string]json.RawMessage, map[string][]group, error) {
	top := map[string]json.RawMessage{}
	if trimmed := bytes.TrimSpace(orig); len(trimmed) > 0 {
		if err := json.Unmarshal(trimmed, &top); err != nil {
			return nil, nil, err
		}
	}
	hooks := map[string][]group{}
	if raw, ok := top["hooks"]; ok {
		if err := json.Unmarshal(raw, &hooks); err != nil {
			return nil, nil, err
		}
	}
	return top, hooks, nil
}

// stripGatedEverywhere drops the gate's own hooks from every event, keeping user
// hooks. Groups left with no hooks are dropped.
func stripGatedEverywhere(hooks map[string][]group) {
	for ev, groups := range hooks {
		var kept []group
		for _, g := range groups {
			var h []json.RawMessage
			for _, raw := range g.Hooks {
				if !isGatedHook(raw) {
					h = append(h, raw)
				}
			}
			if len(h) > 0 {
				g.Hooks = h
				kept = append(kept, g)
			}
		}
		hooks[ev] = kept
	}
}

func pruneEmptyEvents(hooks map[string][]group) {
	for ev, groups := range hooks {
		if len(groups) == 0 {
			delete(hooks, ev)
		}
	}
}

// isGatedHook recognizes an entry the gate wrote: an http hook to our loopback
// /hook endpoint, or the SessionStart command shim. Identifying by content
// convention (rather than an out-of-schema marker field, which Claude Code might
// reject) is deliberate.
func isGatedHook(raw json.RawMessage) bool {
	var p hookProbe
	if json.Unmarshal(raw, &p) != nil {
		return false
	}
	switch p.Type {
	case "http":
		return loopbackHookURL(p.URL)
	case "command":
		return strings.Contains(p.Command, "hook claudecode")
	}
	return false
}

func loopbackHookURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := u.Hostname()
	return (host == "127.0.0.1" || host == "localhost") && u.Path == "/hook"
}

// assemble re-serializes settings with uniform 2-space indentation. json.Indent
// pretties the whole document — including the hooks subtree and the user's
// preserved raw values — so the file reads as one consistently-formatted whole.
func assemble(top map[string]json.RawMessage, hooks map[string][]group) ([]byte, error) {
	if len(hooks) == 0 {
		delete(top, "hooks")
	} else {
		raw, err := json.Marshal(hooks)
		if err != nil {
			return nil, err
		}
		top["hooks"] = raw
	}
	compact, err := json.Marshal(top)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, compact, "", "  "); err != nil {
		return nil, err
	}
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}
