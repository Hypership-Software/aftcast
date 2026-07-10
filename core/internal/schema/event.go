package schema

import "encoding/json"

// TelemetryEvent is the single record type written to the hash-chained log for
// every hook, carrying BOTH enforcement records and full telemetry (one
// stream). Its field set is an append-only contract shared with the SIEM export
// and the org rollup: never remove or repurpose a field, only add.
//
// It intentionally does NOT embed Descriptor. The two share enum types
// (ToolClass, Verdict, ToolOutcome) so those can't drift, but the persisted
// contract stays an explicit, frozen list — eval-only context on the Descriptor
// (argv, cwd, project_root, mcp split) must never leak into the log.
type TelemetryEvent struct {
	V         int         `json:"v"`
	TS        string      `json:"ts"`
	Seq       uint64      `json:"seq"`
	SessionID string      `json:"session_id"`
	OrgID     string      `json:"org_id,omitempty"`
	User      string      `json:"user"`
	Host      string      `json:"host"`
	Harness   string      `json:"harness"`
	EventType EventType   `json:"event_type"`
	TurnIndex int         `json:"turn_index"`
	ToolClass ToolClass   `json:"tool_class,omitempty"`
	ToolRaw   string      `json:"tool_raw,omitempty"`
	Verdict   Verdict     `json:"verdict,omitempty"`
	RuleID    string      `json:"rule_id,omitempty"`
	ToolOK    ToolOutcome `json:"tool_ok,omitempty"`
	// BashExitCode is the numeric exit code parsed from a PostToolUseFailure
	// (findings §E rev 2). Added post-Task-2 per the append-only rule; omitempty
	// so it's absent on non-failing / non-exec events.
	BashExitCode int      `json:"bash_exit_code,omitempty"`
	LatencyMS    int64    `json:"latency_ms,omitempty"`
	Files        []string `json:"files,omitempty"`
	Verbs        []string `json:"verbs,omitempty"`
	Domain       string   `json:"domain,omitempty"`
	Taint        bool     `json:"taint"`
	Skill        string   `json:"skill,omitempty"`
	Subagent     string   `json:"subagent,omitempty"`
	PolicyHash   string   `json:"policy_hash,omitempty"`
	PrevHash     string   `json:"prev_hash,omitempty"`
	Hash         string   `json:"hash,omitempty"`
}

// Canonical returns deterministic, sorted-key JSON for the event with the Hash
// field excluded. Hash is derived from these bytes (Task 13), so it cannot be
// part of the bytes it signs. PrevHash IS included — it is an input to this
// record's identity, not a derived value.
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
