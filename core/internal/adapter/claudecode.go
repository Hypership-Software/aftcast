package adapter

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/Hypership-Software/atlas/internal/schema"
	"github.com/google/shlex"
)

func init() { register("claudecode", claudeCode{}) }

type claudeCode struct{}

// ccHook is the subset of the Claude Code hook JSON we consume, verified against
// captured 2.1.205 payloads (spike/samples). tool_response is present on a
// successful PostToolUse; error ("Exit code N\n...") on PostToolUseFailure.
type ccHook struct {
	SessionID     string          `json:"session_id"`
	Cwd           string          `json:"cwd"`
	HookEventName string          `json:"hook_event_name"`
	ToolName      string          `json:"tool_name"`
	ToolInput     json.RawMessage `json:"tool_input"`
	ToolUseID     string          `json:"tool_use_id"`
	Error         string          `json:"error"`
}

var exitCodeRe = regexp.MustCompile(`^Exit code (\d+)`)

func (claudeCode) Normalize(event string, raw []byte) (schema.Descriptor, schema.TelemetryEvent, error) {
	var h ccHook
	if err := json.Unmarshal(raw, &h); err != nil {
		return schema.Descriptor{}, schema.TelemetryEvent{}, fmt.Errorf("claudecode: parse hook payload: %w", err)
	}
	if event == "" {
		event = h.HookEventName
	}

	desc := schema.Descriptor{
		Version:   schema.SchemaVersion,
		SessionID: h.SessionID,
		ToolRaw:   h.ToolName,
		Cwd:       h.Cwd,
	}
	ev := schema.TelemetryEvent{
		V:         schema.SchemaVersion,
		SessionID: h.SessionID,
		Harness:   "claudecode",
		EventType: eventType(event),
		ToolRaw:   h.ToolName,
	}

	if h.ToolName != "" {
		class := classify(h.ToolName)
		desc.ToolClass = class
		ev.ToolClass = class
		extract(&desc, class, h)
		ev.Files, ev.Verbs, ev.Domain = desc.Files, desc.Verbs, desc.Domain
	}

	// tool_ok is tri-state and only meaningful for post-execution events. A
	// PostToolUseFailure carries the numeric exit code in its error field.
	if ev.EventType == schema.EventPostTool {
		if event == "PostToolUseFailure" || h.Error != "" {
			ev.ToolOK = schema.OutcomeFailed
			ev.BashExitCode = parseExitCode(h.Error)
		} else {
			ev.ToolOK = schema.OutcomeOK
		}
	}

	return desc, ev, nil
}

func eventType(event string) schema.EventType {
	switch event {
	case "PreToolUse":
		return schema.EventPreTool
	case "PostToolUse", "PostToolUseFailure":
		return schema.EventPostTool
	case "UserPromptSubmit":
		return schema.EventUserPrompt
	case "Stop", "SubagentStop", "SessionEnd":
		return schema.EventStop
	case "SessionStart":
		return schema.EventSessionStart
	default:
		return schema.EventType(strings.ToLower(event))
	}
}

// classify maps a Claude Code tool name onto a harness-independent class. On
// Windows the shell tool may be Bash (Git Bash) or PowerShell — both are exec.
func classify(tool string) schema.ToolClass {
	switch {
	case tool == "Bash" || tool == "PowerShell":
		return schema.ClassExec
	case tool == "Read":
		return schema.ClassFileRead
	case tool == "Write" || tool == "Edit" || tool == "MultiEdit" || tool == "NotebookEdit":
		return schema.ClassFileWrite
	case tool == "WebFetch":
		return schema.ClassNetFetch
	case tool == "WebSearch":
		return schema.ClassNetSearch
	case tool == "Task":
		return schema.ClassAgentSpawn
	case tool == "Skill":
		return schema.ClassSkill
	case strings.HasPrefix(tool, "mcp__"):
		return schema.ClassMCP
	default:
		return schema.ClassOther
	}
}

func extract(d *schema.Descriptor, class schema.ToolClass, h ccHook) {
	switch class {
	case schema.ClassExec:
		var in struct {
			Command string `json:"command"`
		}
		_ = json.Unmarshal(h.ToolInput, &in)
		// shlex gives POSIX-shell tokenization (correct for Git Bash; a close
		// approximation for PowerShell). Best-effort per ADR-006 — fall back to
		// whitespace splitting if the command doesn't tokenize.
		toks, err := shlex.Split(in.Command)
		if err != nil || len(toks) == 0 {
			toks = strings.Fields(in.Command)
		}
		d.Argv = toks
		if len(toks) > 0 {
			d.Verbs = []string{commandVerb(toks[0])}
		}
	case schema.ClassFileRead, schema.ClassFileWrite:
		var in struct {
			FilePath string `json:"file_path"`
		}
		_ = json.Unmarshal(h.ToolInput, &in)
		if in.FilePath != "" {
			d.Files = []string{in.FilePath}
		}
	case schema.ClassNetFetch:
		var in struct {
			URL string `json:"url"`
		}
		_ = json.Unmarshal(h.ToolInput, &in)
		if u, err := url.Parse(in.URL); err == nil {
			d.Domain = u.Hostname()
		}
	case schema.ClassMCP:
		d.MCPServer, d.MCPTool = splitMCP(h.ToolName)
	}
}

// commandVerb reduces a command token to its bare verb: strips any directory and
// a trailing .exe, so "/usr/bin/git" and "git.exe" both become "git".
func commandVerb(tok string) string {
	base := path.Base(filepath.ToSlash(tok))
	return strings.TrimSuffix(base, ".exe")
}

func splitMCP(tool string) (server, name string) {
	parts := strings.SplitN(strings.TrimPrefix(tool, "mcp__"), "__", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return parts[0], ""
}

func parseExitCode(errText string) int {
	m := exitCodeRe.FindStringSubmatch(errText)
	if len(m) < 2 {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}
