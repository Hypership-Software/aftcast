// Package schema defines the canonical data contracts shared across Atlas: the
// Descriptor a tool call is classified from, the three-valued Risk level, and
// the append-only TelemetryEvent written to the hash-chained log. The enum wire
// values here are part of the SIEM + org-rollup contract and must never change.
package schema

// SchemaVersion is stamped into every Descriptor and TelemetryEvent so the
// append-only log can be read forward across format additions.
const SchemaVersion = 1

// Risk is the three-valued classification Atlas assigns each tool call: a
// danger rule matched (danger), a known-safe rule matched (safe), or nothing
// matched (unknown). It is a label, not a decision — Atlas observes and records,
// it never blocks.
type Risk string

const (
	RiskSafe    Risk = "safe"
	RiskDanger  Risk = "danger"
	RiskUnknown Risk = "unknown"
)

// ToolClass is the harness-independent classification of a tool call. Adapters
// map each harness's raw tool name onto one of these.
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

// ToolOutcome is the tri-state result of a tool call, serialized as tool_ok.
// It is deliberately NOT a bool: a bool cannot distinguish "ran and passed"
// from "never ran" (interrupted, or no PostToolUse signal). Frozen at the schema
// layer because the log is append-only.
type ToolOutcome string

const (
	OutcomeOK     ToolOutcome = "ok"
	OutcomeFailed ToolOutcome = "failed"
	OutcomeNotRun ToolOutcome = "not_run"
)

// EventType tags each record in the telemetry stream.
type EventType string

const (
	EventSessionStart EventType = "session_start"
	EventUserPrompt   EventType = "user_prompt"
	EventPreTool      EventType = "pre_tool"
	EventPostTool     EventType = "post_tool"
	EventStop         EventType = "stop"
	EventIntegrity    EventType = "integrity"
)
