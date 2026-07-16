package adapter

import (
	"testing"

	"github.com/Hypership-Software/aftcast/internal/schema"
)

func TestObservedOperation(t *testing.T) {
	tests := []struct {
		name  string
		tool  string
		class schema.ToolClass
		argv  []string
		want  schema.Operation
	}{
		{"go test", "Bash", schema.ClassExec, []string{"go", "test", "./..."}, schema.OperationTest},
		{"go vet", "Bash", schema.ClassExec, []string{"go", "vet", "./..."}, schema.OperationLint},
		{"pytest", "Bash", schema.ClassExec, []string{"pytest", "-q"}, schema.OperationTest},
		{"pnpm test", "Bash", schema.ClassExec, []string{"pnpm", "test"}, schema.OperationTest},
		{"golangci lint", "Bash", schema.ClassExec, []string{"golangci-lint", "run"}, schema.OperationLint},
		{"eslint", "Bash", schema.ClassExec, []string{"eslint", "."}, schema.OperationLint},
		{"gofmt", "Bash", schema.ClassExec, []string{"gofmt", "-w", "x.go"}, schema.OperationFormat},
		{"prettier", "Bash", schema.ClassExec, []string{"prettier", "--write", "."}, schema.OperationFormat},
		{"git diff", "Bash", schema.ClassExec, []string{"git", "diff", "--check"}, schema.OperationInspect},
		{"git status", "Bash", schema.ClassExec, []string{"git", "status", "--short"}, schema.OperationInspect},
		{"unknown exec", "PowerShell", schema.ClassExec, []string{"make", "build"}, schema.OperationExecute},
		{"cd chained go test", "Bash", schema.ClassExec, []string{"cd", "core", "&&", "go", "test", "./..."}, schema.OperationTest},
		{"semicolon attached to token", "PowerShell", schema.ClassExec, []string{"cd", `C:\repo;`, "go", "vet", "./..."}, schema.OperationLint},
		{"test outranks inspect", "Bash", schema.ClassExec, []string{"git", "diff", "&&", "go", "test", "./..."}, schema.OperationTest},
		{"piped test still test", "Bash", schema.ClassExec, []string{"go", "test", "./...", "|", "tee", "out.log"}, schema.OperationTest},
		{"cd chained plain exec", "Bash", schema.ClassExec, []string{"cd", "docs", "&&", "ls"}, schema.OperationExecute},
		{"env prefix inside segment", "Bash", schema.ClassExec, []string{"cd", "x", "&&", "CGO_ENABLED=0", "go", "test", "./..."}, schema.OperationTest},
		{"read", "Read", schema.ClassFileRead, nil, schema.OperationRead},
		{"grep", "Grep", schema.ClassOther, nil, schema.OperationSearch},
		{"glob", "Glob", schema.ClassOther, nil, schema.OperationSearch},
		{"ask", "AskUserQuestion", schema.ClassOther, nil, schema.OperationAsk},
		{"plan", "EnterPlanMode", schema.ClassOther, nil, schema.OperationPlan},
		{"edit", "Edit", schema.ClassFileWrite, nil, schema.OperationEdit},
		{"skill", "Skill", schema.ClassSkill, nil, schema.OperationSkill},
		{"agent", "Task", schema.ClassAgentSpawn, nil, schema.OperationAgent},
		{"fetch", "WebFetch", schema.ClassNetFetch, nil, schema.OperationFetch},
		{"search", "WebSearch", schema.ClassNetSearch, nil, schema.OperationSearch},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := observedOperation(tt.tool, tt.class, tt.argv); got != tt.want {
				t.Fatalf("observedOperation(%q, %q, %v) = %q, want %q", tt.tool, tt.class, tt.argv, got, tt.want)
			}
		})
	}
}
