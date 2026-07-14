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

	"github.com/Hypership-Software/atlas/internal/project"
	"github.com/Hypership-Software/atlas/internal/schema"
	"github.com/google/shlex"
)

func init() { register("claudecode", claudeCode{}) }

type claudeCode struct{}

// ccHook is the subset of the Claude Code hook JSON we consume. The error field
// carries "Exit code N\n..." on PostToolUseFailure.
type ccHook struct {
	SessionID     string          `json:"session_id"`
	Cwd           string          `json:"cwd"`
	HookEventName string          `json:"hook_event_name"`
	ToolName      string          `json:"tool_name"`
	ToolInput     json.RawMessage `json:"tool_input"`
	ToolUseID     string          `json:"tool_use_id"`
	DurationMS    int64           `json:"duration_ms"`
	Error         string          `json:"error"`
	// PromptID is present on every hook (parent and subagent); AgentID/AgentType
	// are present only when a subagent made the call, so AgentType distinguishes
	// subagent work from the main agent's.
	PromptID  string `json:"prompt_id"`
	AgentID   string `json:"agent_id"`
	AgentType string `json:"agent_type"`
	// ExpansionType/CommandName come from a UserPromptExpansion hook. The sibling
	// command_args and prompt fields are content and are deliberately not decoded,
	// so they cannot leak into the log (ADR-011 metadata-only).
	ExpansionType string `json:"expansion_type"`
	CommandName   string `json:"command_name"`
}

var exitCodeRe = regexp.MustCompile(`^Exit code (\d+)`)

// envAssignRe matches a leading shell environment assignment (NAME=value) so its
// value is never mistaken for the command verb.
var envAssignRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

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
		ToolUseID: h.ToolUseID,
		LatencyMS: h.DurationMS,
		PromptID:  h.PromptID,
		AgentID:   h.AgentID,
		Subagent:  h.AgentType,
	}

	root, id := project.Identify(h.Cwd)
	desc.ProjectRoot = root
	ev.Project = id

	command := ""
	if h.ToolName != "" {
		class := classify(h.ToolName)
		desc.ToolClass = class
		ev.ToolClass = class
		command = extract(&desc, class, h)
		ev.Files, ev.Verbs, ev.Domain, ev.Skill = desc.Files, desc.Verbs, desc.Domain, desc.Skill
	}

	if ev.EventType == schema.EventPromptExpansion {
		ev.Command = h.CommandName
		ev.ExpansionType = h.ExpansionType
	}

	// tool_ok is only meaningful for post-execution events; a PostToolUseFailure
	// carries the exit code in its error field.
	if ev.EventType == schema.EventPostTool {
		if event == "PostToolUseFailure" || h.Error != "" {
			ev.ToolOK = schema.OutcomeFailed
			ev.BashExitCode = parseExitCode(h.Error)
		} else {
			ev.ToolOK = schema.OutcomeOK
		}
		if ev.ToolOK == schema.OutcomeOK && ev.ToolClass == schema.ClassExec {
			ev.DeliverySignal = deliverySignal(command)
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
	case "UserPromptExpansion":
		return schema.EventPromptExpansion
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

func extract(d *schema.Descriptor, class schema.ToolClass, h ccHook) string {
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
		if v := commandVerb(toks); v != "" {
			d.Verbs = []string{v}
		}
		return in.Command
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
	case schema.ClassSkill:
		var in struct {
			Skill string `json:"skill"`
		}
		_ = json.Unmarshal(h.ToolInput, &in)
		d.Skill = in.Skill
	}
	return ""
}

// commandVerb returns the invoked program's name. It skips leading NAME=value
// environment assignments (a `API_KEY=… cmd` prefix — the assignment value is
// untrusted content that must never reach the logged verb) and strips any
// directory and a trailing .exe, so "/usr/bin/git" and "git.exe" both become
// "git". Returns "" when the command is only assignments.
func commandVerb(toks []string) string {
	for _, tok := range toks {
		if envAssignRe.MatchString(tok) {
			continue
		}
		base := path.Base(filepath.ToSlash(tok))
		return strings.TrimSuffix(base, ".exe")
	}
	return ""
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
