package adapter

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/Hypership-Software/aftcast/internal/schema"
)

func TestPowerShellCommandVerb(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    string
	}{
		{"plain program", `git push origin main`, "git"},
		{"cd chain keeps first program", `cd C:\Users\dev\agent-gate; git push`, "cd"},
		{"call operator resolves target", `& "C:\tools\playwright-cli.exe" open`, "playwright-cli"},
		{"preference assignment then program", `$ErrorActionPreference = 'Stop'; go build ./...`, "go"},
		{"inline env assignment", `$env:CGO_ENABLED='0'; go test ./...`, "go"},
		{"capture assignment consumes its statement", `$errs = go vet ./...; golangci-lint run`, "golangci-lint"},
		{"capture assignment alone yields nothing", `$errs = go vet ./...`, ""},
		{"variable in program position yields nothing", `& $tool arg`, ""},
		{"keyword digs into condition", `if (Test-Path dist) { Remove-Item dist }`, "Test-Path"},
		{"braces are not programs", `try { git push } catch {}`, "git"},
		{"multiline first line wins", "git status\n$x = 1", "git"},
		{"exe suffix stripped", `node.exe script.js`, "node"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := powerShellCommandVerb(tt.command); got != tt.want {
				t.Fatalf("powerShellCommandVerb(%q) = %q, want %q", tt.command, got, tt.want)
			}
		})
	}
}

func TestNormalizePowerShellVerbIsNotPOSIXGarbage(t *testing.T) {
	raw, err := json.Marshal(map[string]any{
		"session_id":      "ps-session",
		"hook_event_name": "PreToolUse",
		"tool_name":       "PowerShell",
		"tool_use_id":     "t1",
		"tool_input":      map[string]any{"command": `$errs = go vet ./...; golangci-lint run`},
	})
	if err != nil {
		t.Fatal(err)
	}
	d, e, err := cc(t).Normalize("", raw)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if !reflect.DeepEqual(d.Verbs, []string{"golangci-lint"}) {
		t.Fatalf("verbs = %v, want [golangci-lint]", d.Verbs)
	}
	if !reflect.DeepEqual(e.Verbs, d.Verbs) {
		t.Fatalf("event verbs = %v, want %v", e.Verbs, d.Verbs)
	}
	if e.ToolClass != schema.ClassExec {
		t.Fatalf("class = %v, want exec", e.ToolClass)
	}
}
