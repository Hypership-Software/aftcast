package schema

import "encoding/json"

// TelemetryEvent is the single record written to the hash-chained log for every
// hook. Its field set is an append-only contract shared with the SIEM export and
// org rollup: never remove or repurpose a field, only add.
//
// It intentionally does NOT embed Descriptor — the persisted contract stays an
// explicit frozen list, so eval-only Descriptor context (argv, cwd, mcp split)
// can never leak into the log.
type TelemetryEvent struct {
	V         int       `json:"v"`
	TS        string    `json:"ts"`
	Seq       uint64    `json:"seq"`
	SessionID string    `json:"session_id"`
	OrgID     string    `json:"org_id,omitempty"`
	User      string    `json:"user"`
	Host      string    `json:"host"`
	Harness   string    `json:"harness"`
	EventType EventType `json:"event_type"`
	TurnIndex int       `json:"turn_index"`
	ToolClass ToolClass `json:"tool_class,omitempty"`
	ToolRaw   string    `json:"tool_raw,omitempty"`
	// ToolUseID pairs a pre_tool with its post_tool (Claude Code sends the same id
	// on both), so latency and outcome attribute to the right call. Added later per
	// the append-only rule.
	ToolUseID      string         `json:"tool_use_id,omitempty"`
	Risk           Risk           `json:"risk,omitempty"`
	RuleID         string         `json:"rule_id,omitempty"`
	ToolOK         ToolOutcome    `json:"tool_ok,omitempty"`
	DeliverySignal DeliverySignal `json:"delivery_signal,omitempty"`
	// BashExitCode is parsed from a PostToolUseFailure. Added later per the
	// append-only rule; omitempty so it's absent on non-failing/non-exec events.
	BashExitCode int          `json:"bash_exit_code,omitempty"`
	LatencyMS    int64        `json:"latency_ms,omitempty"`
	Files        []string     `json:"files,omitempty"`
	Verbs        []string     `json:"verbs,omitempty"`
	Operation    Operation    `json:"operation,omitempty"`
	ChangeStats  *ChangeStats `json:"change_stats,omitempty"`
	Domain       string       `json:"domain,omitempty"`
	Taint        bool         `json:"taint"`
	Skill        string       `json:"skill,omitempty"`
	// Command is the slash-command or MCP-prompt name from a UserPromptExpansion
	// hook; ExpansionType is "slash_command" or "mcp_prompt". Metadata only — the
	// command's args and expanded prompt are content and are never captured
	// (ADR-011). Added later per the append-only rule.
	Command       string `json:"command,omitempty"`
	ExpansionType string `json:"expansion_type,omitempty"`
	// Subagent is the agent_type of the subagent that made this call (empty for
	// the main agent); AgentID is that subagent's instance id; PromptID ties the
	// event — parent or subagent — to the human prompt that initiated it. Added
	// later per the append-only rule (omitempty leaves old events' hashes intact).
	Subagent string `json:"subagent,omitempty"`
	AgentID  string `json:"agent_id,omitempty"`
	PromptID string `json:"prompt_id,omitempty"`
	// PermissionMode and Effort are the harness's session posture at the moment
	// of the call (e.g. "bypassPermissions", "xhigh"). CommitSHA is extracted
	// from a successful `git commit`'s output — the join key from this session's
	// conduct to the git/PR record of its outcome; only the SHA is captured,
	// never the surrounding output (ADR-011). Added later per the append-only
	// rule (omitempty leaves old events' hashes intact).
	PermissionMode string `json:"permission_mode,omitempty"`
	Effort         string `json:"effort,omitempty"`
	CommitSHA      string `json:"commit_sha,omitempty"`
	PolicyHash     string `json:"policy_hash,omitempty"`
	// Project is an opaque 12-hex hash of the project's identity (normalized git
	// remote or canonical path) — never the path or URL itself. Added later per the
	// append-only rule; omitempty keeps pre-field events' hashes intact.
	Project  string `json:"project_id,omitempty"`
	PrevHash string `json:"prev_hash,omitempty"`
	Hash     string `json:"hash,omitempty"`
}

// Canonical returns deterministic sorted-key JSON with the Hash field excluded —
// Hash is derived from these bytes, so it can't sign itself. PrevHash IS included;
// it is an input to this record's identity, not a derived value.
func (e TelemetryEvent) Canonical() ([]byte, error) {
	raw, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	delete(m, "hash")
	return json.Marshal(m)
}
