package schema

import "encoding/json"

// Descriptor is the classification-input view of a single tool call — everything
// the Cedar classifier needs and nothing it doesn't. It carries eval-only
// context (Argv, Cwd, ProjectRoot, MCP split) that is deliberately absent from
// the persisted TelemetryEvent contract, so growing the eval inputs never
// widens the append-only log.
//
// Descriptor.Canonical() is the eval cache key; it therefore excludes anything
// non-deterministic (timestamps, sequence numbers) by construction — a
// Descriptor has no such fields.
type Descriptor struct {
	Version     int       `json:"v"`
	SessionID   string    `json:"session_id"`
	Org         string    `json:"org,omitempty"`
	ToolClass   ToolClass `json:"tool_class"`
	ToolRaw     string    `json:"tool_raw"`
	Argv        []string  `json:"argv,omitempty"`
	Files       []string  `json:"files,omitempty"`
	Verbs       []string  `json:"verbs,omitempty"`
	Domain      string    `json:"domain,omitempty"`
	Cwd         string    `json:"cwd,omitempty"`
	ProjectRoot string    `json:"project_root,omitempty"`
	MCPServer   string    `json:"mcp_server,omitempty"`
	MCPTool     string    `json:"mcp_tool,omitempty"`
	Tainted     bool      `json:"taint"`
}

// Canonical returns deterministic, sorted-key JSON for the descriptor. It is
// used as the evaluation cache key, so byte-stability across calls is the
// contract the cache depends on.
func (d Descriptor) Canonical() ([]byte, error) {
	return canonicalJSON(d)
}

// canonicalJSON produces sorted-key JSON independent of struct field order by
// round-tripping through a generic value: encoding/json sorts object keys when
// marshaling a map, at every level. Array order is preserved (it is meaningful
// for argv/files/verbs).
func canonicalJSON(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return nil, err
	}
	return json.Marshal(generic)
}
