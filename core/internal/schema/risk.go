// Package schema defines the canonical data contracts shared across Atlas. The
// enum wire values here are part of the SIEM + org-rollup contract and must never
// change.
package schema

// SchemaVersion is stamped into every Descriptor and TelemetryEvent so the log
// can be read forward across format additions.
const SchemaVersion = 1

// Risk is the three-valued classification Atlas assigns each tool call: danger (a
// forbid matched), safe (a permit matched), or unknown (no match). It is a label,
// not a decision — Atlas observes, it never blocks.
type Risk string

const (
	RiskSafe    Risk = "safe"
	RiskDanger  Risk = "danger"
	RiskUnknown Risk = "unknown"
)

// ToolClass is the harness-independent class of a tool call; adapters map each
// harness's raw tool name onto one.
type ToolClass string

const (
	ClassExec       ToolClass = "exec"
	ClassFileRead   ToolClass = "file_read"
	ClassFileWrite  ToolClass = "file_write"
	ClassNetFetch   ToolClass = "net_fetch"
	ClassNetSearch  ToolClass = "net_search"
	ClassMCP        ToolClass = "mcp"
	ClassAgentSpawn ToolClass = "agent_spawn"
	ClassSkill      ToolClass = "skill"
	ClassOther      ToolClass = "other"
)

// ToolOutcome is the tri-state result of a tool call (tool_ok). Deliberately NOT
// a bool: a bool cannot distinguish "ran and passed" from "never ran". Frozen
// because the log is append-only.
type ToolOutcome string

const (
	OutcomeOK     ToolOutcome = "ok"
	OutcomeFailed ToolOutcome = "failed"
	OutcomeNotRun ToolOutcome = "not_run"
)

type EventType string

const (
	EventSessionStart EventType = "session_start"
	EventUserPrompt   EventType = "user_prompt"
	EventPreTool      EventType = "pre_tool"
	EventPostTool     EventType = "post_tool"
	EventStop         EventType = "stop"
	EventIntegrity    EventType = "integrity"
)
