package adapter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Hypership-Software/atlas/internal/schema"
)

func cc(t *testing.T) Adapter {
	t.Helper()
	a, ok := Get("claudecode")
	if !ok {
		t.Fatal("claudecode adapter not registered")
	}
	return a
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "claudecode", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// normalize uses event="" so the payload's own hook_event_name drives the mapping.
func normalize(t *testing.T, name string) (schema.Descriptor, schema.TelemetryEvent) {
	t.Helper()
	d, e, err := cc(t).Normalize("", fixture(t, name))
	if err != nil {
		t.Fatalf("Normalize %s: %v", name, err)
	}
	return d, e
}

func TestNormalizeBashPreTool(t *testing.T) {
	d, e := normalize(t, "pretooluse-bash.json")
	if e.EventType != schema.EventPreTool {
		t.Errorf("event type = %v, want pre_tool", e.EventType)
	}
	if d.ToolClass != schema.ClassExec {
		t.Errorf("class = %v, want exec", d.ToolClass)
	}
	wantArgv := []string{"node", "-e", "console.log(42)"}
	if !reflect.DeepEqual(d.Argv, wantArgv) {
		t.Errorf("argv = %v, want %v", d.Argv, wantArgv)
	}
	if !reflect.DeepEqual(d.Verbs, []string{"node"}) {
		t.Errorf("verbs = %v, want [node]", d.Verbs)
	}
}

func TestNormalizePostToolUseFailureCarriesExitCode(t *testing.T) {
	_, e := normalize(t, "posttoolusefailure-bash.json")
	if e.EventType != schema.EventPostTool {
		t.Errorf("event type = %v, want post_tool", e.EventType)
	}
	if e.ToolOK != schema.OutcomeFailed {
		t.Errorf("tool_ok = %v, want failed", e.ToolOK)
	}
	if e.BashExitCode != 5 {
		t.Errorf("bash_exit_code = %d, want 5", e.BashExitCode)
	}
}

func TestNormalizePostToolUseSuccess(t *testing.T) {
	_, e := normalize(t, "posttooluse-bash-success.json")
	if e.ToolOK != schema.OutcomeOK {
		t.Errorf("tool_ok = %v, want ok", e.ToolOK)
	}
	if e.BashExitCode != 0 {
		t.Errorf("bash_exit_code = %d, want 0", e.BashExitCode)
	}
}

func TestNormalizeReadFileClass(t *testing.T) {
	d, _ := normalize(t, "pretooluse-read.json")
	if d.ToolClass != schema.ClassFileRead {
		t.Errorf("class = %v, want file_read", d.ToolClass)
	}
	if !reflect.DeepEqual(d.Files, []string{"/home/dev/project/.env"}) {
		t.Errorf("files = %v", d.Files)
	}
}

func TestNormalizeWebFetchDomain(t *testing.T) {
	d, _ := normalize(t, "pretooluse-webfetch.json")
	if d.ToolClass != schema.ClassNetFetch {
		t.Errorf("class = %v, want net_fetch", d.ToolClass)
	}
	if d.Domain != "evil.example.com" {
		t.Errorf("domain = %q, want evil.example.com", d.Domain)
	}
}

func TestNormalizeMCPSplit(t *testing.T) {
	d, _ := normalize(t, "pretooluse-mcp.json")
	if d.ToolClass != schema.ClassMCP {
		t.Errorf("class = %v, want mcp", d.ToolClass)
	}
	if d.MCPServer != "github" || d.MCPTool != "create_issue" {
		t.Errorf("mcp split = (%q,%q), want (github,create_issue)", d.MCPServer, d.MCPTool)
	}
}

func TestNormalizeSkillName(t *testing.T) {
	d, e := normalize(t, "pretooluse-skill.json")
	if d.ToolClass != schema.ClassSkill {
		t.Errorf("class = %v, want skill", d.ToolClass)
	}
	if e.Skill != "superpowers:brainstorming" {
		t.Errorf("skill = %q, want superpowers:brainstorming", e.Skill)
	}
}

// A PostToolUse Skill payload echoes tool_input (Claude Code's documented post
// schema carries tool_input + tool_response), so the skill name, latency, and
// outcome all attribute to the post event — no pre→post name-recovery needed.
func TestNormalizeSkillNameOnPostTool(t *testing.T) {
	_, e := normalize(t, "posttooluse-skill.json")
	if e.EventType != schema.EventPostTool {
		t.Errorf("event type = %v, want post_tool", e.EventType)
	}
	if e.Skill != "superpowers:brainstorming" {
		t.Errorf("skill = %q, want superpowers:brainstorming", e.Skill)
	}
	if e.ToolOK != schema.OutcomeOK {
		t.Errorf("tool_ok = %v, want ok", e.ToolOK)
	}
	if e.LatencyMS != 4 {
		t.Errorf("latency_ms = %d, want 4", e.LatencyMS)
	}
	// ADR-011: capture the skill name only, never the args content.
	if blob, _ := json.Marshal(e); strings.Contains(string(blob), "design the thing") {
		t.Errorf("event leaked skill args content: %s", blob)
	}
}

// A skill invocation's pre and post events share a tool_use_id and both carry the
// name, so an invocation is a single pairable unit for latency/outcome.
func TestNormalizeSkillPrePostPairByToolUseID(t *testing.T) {
	_, pre := normalize(t, "pretooluse-skill.json")
	_, post := normalize(t, "posttooluse-skill.json")
	if pre.ToolUseID == "" || pre.ToolUseID != post.ToolUseID {
		t.Fatalf("tool_use_id pre=%q post=%q, want equal and non-empty", pre.ToolUseID, post.ToolUseID)
	}
	if pre.Skill != post.Skill || pre.Skill == "" {
		t.Errorf("skill pre=%q post=%q, want equal and non-empty", pre.Skill, post.Skill)
	}
}

// Plugin skills are namespaced values in the same tool_input.skill key, so the
// value-agnostic extractor captures them unchanged.
func TestNormalizePluginSkillName(t *testing.T) {
	d, e := normalize(t, "pretooluse-skill-plugin.json")
	if d.ToolClass != schema.ClassSkill {
		t.Errorf("class = %v, want skill", d.ToolClass)
	}
	if e.Skill != "marketing-skills:cold-email" {
		t.Errorf("skill = %q, want marketing-skills:cold-email", e.Skill)
	}
}

func TestNormalizeUserPrompt(t *testing.T) {
	_, e := normalize(t, "userpromptsubmit.json")
	if e.EventType != schema.EventUserPrompt {
		t.Errorf("event type = %v, want user_prompt", e.EventType)
	}
}

func TestNormalizeSubagentIdentity(t *testing.T) {
	_, e := normalize(t, "pretooluse-subagent.json")
	if e.Subagent != "Explore" {
		t.Errorf("Subagent = %q, want Explore", e.Subagent)
	}
	if e.AgentID != "spike-agent-0001" {
		t.Errorf("AgentID = %q, want spike-agent-0001", e.AgentID)
	}
	if e.PromptID != "spike-prompt-aaaa-bbbb" {
		t.Errorf("PromptID = %q, want spike-prompt-aaaa-bbbb", e.PromptID)
	}
}

func TestNormalizeMainAgentHasPromptNoSubagent(t *testing.T) {
	_, e := normalize(t, "pretooluse-bash.json")
	if e.PromptID != "spike-prompt-aaaa-bbbb" {
		t.Errorf("PromptID = %q, want spike-prompt-aaaa-bbbb", e.PromptID)
	}
	if e.Subagent != "" {
		t.Errorf("main-agent Subagent = %q, want empty", e.Subagent)
	}
	if e.AgentID != "" {
		t.Errorf("main-agent AgentID = %q, want empty", e.AgentID)
	}
}

func TestNormalizeCapturesToolUseIDAndLatency(t *testing.T) {
	_, e := normalize(t, "posttooluse-bash-success.json")
	if e.ToolUseID != "toolu_SAMPLEsuccess0001" {
		t.Errorf("ToolUseID = %q, want toolu_SAMPLEsuccess0001", e.ToolUseID)
	}
	if e.LatencyMS != 9950 {
		t.Errorf("LatencyMS = %d, want 9950", e.LatencyMS)
	}
}

func TestGetUnknownHarness(t *testing.T) {
	if _, ok := Get("nope"); ok {
		t.Error("Get returned an adapter for an unknown harness")
	}
}
