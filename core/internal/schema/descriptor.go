package schema

import "encoding/json"

// Descriptor is the input a tool call is classified from. Its eval-only fields
// (Argv, Cwd, ProjectRoot, MCP split) are deliberately kept out of the
// append-only TelemetryEvent, so widening classification inputs never widens the
// persisted log.
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
	Skill       string    `json:"skill,omitempty"`
	Tainted     bool      `json:"taint"`
}

func (d Descriptor) Canonical() ([]byte, error) {
	return canonicalJSON(d)
}

// canonicalJSON sorts object keys by round-tripping through a generic value
// (encoding/json sorts map keys, not struct fields). Array order is preserved —
// it is meaningful for argv/files/verbs.
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
