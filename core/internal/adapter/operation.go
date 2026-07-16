package adapter

import (
	"strings"

	"github.com/Hypership-Software/aftcast/internal/schema"
)

func observedOperation(tool string, class schema.ToolClass, argv []string) schema.Operation {
	switch class {
	case schema.ClassFileRead:
		return schema.OperationRead
	case schema.ClassFileWrite:
		return schema.OperationEdit
	case schema.ClassNetSearch:
		return schema.OperationSearch
	case schema.ClassNetFetch, schema.ClassMCP:
		return schema.OperationFetch
	case schema.ClassSkill:
		return schema.OperationSkill
	case schema.ClassAgentSpawn:
		return schema.OperationAgent
	case schema.ClassExec:
		return execOperation(argv)
	}

	switch strings.ToLower(tool) {
	case "grep", "glob":
		return schema.OperationSearch
	case "askuserquestion":
		return schema.OperationAsk
	case "enterplanmode":
		return schema.OperationPlan
	default:
		return schema.OperationOther
	}
}

func execOperation(argv []string) schema.Operation {
	best := schema.OperationExecute
	for _, segment := range splitArgvSegments(argv) {
		if op := segmentOperation(segment); operationRank(op) > operationRank(best) {
			best = op
		}
	}
	return best
}

func splitArgvSegments(argv []string) [][]string {
	var segments [][]string
	var current []string
	flush := func() {
		if len(current) > 0 {
			segments = append(segments, current)
			current = nil
		}
	}
	for _, tok := range argv {
		switch tok {
		case "&&", "||", "|", ";", "&":
			flush()
			continue
		}
		if trimmed := strings.TrimRight(tok, ";"); trimmed != tok {
			if trimmed != "" {
				current = append(current, trimmed)
			}
			flush()
			continue
		}
		current = append(current, tok)
	}
	flush()
	return segments
}

// operationRank orders exec operations by how much a mixed command reveals
// about the work: a chained `cd core && go test ./...` is a test run, not a
// directory change.
func operationRank(op schema.Operation) int {
	switch op {
	case schema.OperationTest:
		return 4
	case schema.OperationLint:
		return 3
	case schema.OperationFormat:
		return 2
	case schema.OperationInspect:
		return 1
	default:
		return 0
	}
}

func segmentOperation(argv []string) schema.Operation {
	for len(argv) > 0 && envAssignRe.MatchString(argv[0]) {
		argv = argv[1:]
	}
	if len(argv) == 0 {
		return schema.OperationExecute
	}

	program := programName(argv[0])
	subcommand := ""
	if len(argv) > 1 {
		subcommand = strings.ToLower(argv[1])
	}

	switch {
	case program == "go" && subcommand == "test",
		program == "pytest",
		program == "cargo" && subcommand == "test",
		program == "dotnet" && subcommand == "test",
		isPackageScript(argv, "test"):
		return schema.OperationTest
	case program == "go" && subcommand == "vet",
		program == "golangci-lint",
		program == "eslint",
		program == "ruff" && subcommand != "format",
		program == "cargo" && subcommand == "clippy",
		isPackageScript(argv, "lint"):
		return schema.OperationLint
	case program == "gofmt",
		program == "goimports",
		program == "prettier",
		program == "black",
		program == "ruff" && subcommand == "format",
		program == "cargo" && subcommand == "fmt",
		isPackageScript(argv, "format"):
		return schema.OperationFormat
	case program == "git" && (subcommand == "diff" || subcommand == "status" || subcommand == "show" || subcommand == "log"):
		return schema.OperationInspect
	default:
		return schema.OperationExecute
	}
}

func isPackageScript(argv []string, script string) bool {
	if len(argv) < 2 {
		return false
	}
	program := programName(argv[0])
	if program != "npm" && program != "pnpm" && program != "yarn" && program != "bun" {
		return false
	}
	if strings.EqualFold(argv[1], script) {
		return true
	}
	return len(argv) > 2 && strings.EqualFold(argv[1], "run") && strings.EqualFold(argv[2], script)
}
