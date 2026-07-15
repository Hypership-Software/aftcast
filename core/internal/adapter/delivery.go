package adapter

import (
	"path"
	"path/filepath"
	"strings"

	"github.com/Hypership-Software/atlas/internal/schema"
	"github.com/google/shlex"
)

func successfulDeliverySignal(tool, command string) schema.DeliverySignal {
	if !isPOSIXShell(tool) {
		return ""
	}
	segments, operators, ok := parseShellCommand(command)
	if !ok {
		return ""
	}

	start := 0
	for i, operator := range operators {
		if operator == ";" {
			start = i + 1
		}
	}
	for _, operator := range operators[start:] {
		if operator != "&&" {
			return ""
		}
	}

	found := false
	for _, segment := range segments[start:] {
		if alwaysFails(segment.argv) {
			return ""
		}
		if isGitPush(segment.argv) {
			if hasDynamicShellSyntax(segment.raw) {
				return ""
			}
			found = true
		}
	}
	if found {
		return schema.DeliveryGitPush
	}
	return ""
}

func isPOSIXShell(tool string) bool {
	switch programName(tool) {
	case "bash", "sh", "dash", "zsh", "ksh":
		return true
	default:
		return false
	}
}

type shellSegment struct {
	raw  string
	argv []string
}

func parseShellCommand(command string) ([]shellSegment, []string, bool) {
	rawSegments, operators, ok := splitShellCommand(command)
	if !ok {
		return nil, nil, false
	}
	segments := make([]shellSegment, 0, len(rawSegments))
	for _, raw := range rawSegments {
		argv, err := shlex.Split(raw)
		if err != nil || len(argv) == 0 {
			return nil, nil, false
		}
		segments = append(segments, shellSegment{raw: raw, argv: argv})
	}
	return segments, operators, true
}

func hasDynamicShellSyntax(raw string) bool {
	var quote byte
	escaped := false
	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		if escaped {
			escaped = false
			continue
		}
		if quote == '\'' {
			if ch == '\'' {
				quote = 0
			}
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if ch == '\'' {
			if quote == 0 {
				quote = '\''
			}
			continue
		}
		if ch == '"' {
			if quote == '"' {
				quote = 0
			} else if quote == 0 {
				quote = '"'
			}
			continue
		}
		if ch == '$' || ch == '`' {
			return true
		}
		if quote == 0 && strings.ContainsRune("*?[{}", rune(ch)) {
			return true
		}
	}
	return false
}

func splitShellCommand(command string) ([]string, []string, bool) {
	var segments []string
	var operators []string
	var quote byte
	escaped := false
	start := 0

scan:
	for i := 0; i < len(command); i++ {
		ch := command[i]
		if escaped {
			escaped = false
			continue
		}
		if quote != 0 {
			if quote == '"' && ch == '\\' {
				escaped = true
				continue
			}
			if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if ch == '\'' || ch == '"' {
			quote = ch
			continue
		}
		if ch == '(' || ch == ')' || ch == '`' {
			return nil, nil, false
		}
		if ch == '#' && startsShellComment(command, i) {
			newline := strings.IndexByte(command[i:], '\n')
			if newline < 0 {
				command = command[:i]
				break scan
			}
			i += newline - 1
			continue
		}

		operator := ""
		width := 1
		switch ch {
		case '&':
			if i+1 >= len(command) || command[i+1] != '&' {
				return nil, nil, false
			}
			operator = "&&"
			width = 2
		case '|':
			operator = "|"
			if i+1 < len(command) && command[i+1] == '|' {
				operator = "||"
				width = 2
			}
		case ';', '\n':
			operator = ";"
		}
		if operator == "" {
			continue
		}
		segment := strings.TrimSpace(command[start:i])
		if segment == "" {
			return nil, nil, false
		}
		segments = append(segments, segment)
		operators = append(operators, operator)
		i += width - 1
		start = i + 1
	}

	if quote != 0 || escaped {
		return nil, nil, false
	}
	tail := strings.TrimSpace(command[start:])
	if tail == "" {
		if len(operators) == 0 || operators[len(operators)-1] != ";" {
			return nil, nil, false
		}
		operators = operators[:len(operators)-1]
	} else {
		segments = append(segments, tail)
	}
	if len(segments) != len(operators)+1 {
		return nil, nil, false
	}
	return segments, operators, true
}

func startsShellComment(command string, i int) bool {
	if i == 0 {
		return true
	}
	switch command[i-1] {
	case ' ', '\t', '\r', '\n', ';', '&', '|':
		return true
	default:
		return false
	}
}

func alwaysFails(segment []string) bool {
	i := 0
	for i < len(segment) && envAssignRe.MatchString(segment[i]) {
		i++
	}
	return i < len(segment) && programName(segment[i]) == "false"
}

func isGitPush(segment []string) bool {
	i := 0
	for i < len(segment) && envAssignRe.MatchString(segment[i]) {
		i++
	}
	if i >= len(segment) || programName(segment[i]) != "git" {
		return false
	}
	i++

	for i < len(segment) {
		tok := segment[i]
		switch {
		case tok == "-C" || tok == "-c" || tok == "--git-dir" || tok == "--work-tree":
			if i+1 >= len(segment) {
				return false
			}
			i += 2
		case strings.HasPrefix(tok, "-C") && len(tok) > 2:
			i++
		case strings.HasPrefix(tok, "--git-dir=") || strings.HasPrefix(tok, "--work-tree="):
			i++
		case tok == "--no-pager" || tok == "--bare":
			i++
		default:
			goto subcommand
		}
	}

subcommand:
	if i >= len(segment) || segment[i] != "push" {
		return false
	}
	options := true
	for i = i + 1; i < len(segment); i++ {
		arg := segment[i]
		if options && arg == "--" {
			options = false
			continue
		}
		if options {
			unsafe, consumesNext := classifyPushOption(arg)
			if unsafe {
				return false
			}
			if consumesNext {
				if i+1 >= len(segment) {
					return false
				}
				i++
				continue
			}
		}
		if strings.HasPrefix(arg, ":") || strings.HasPrefix(arg, "+:") {
			return false
		}
	}
	return true
}

func classifyPushOption(arg string) (unsafe, consumesNext bool) {
	if strings.HasPrefix(arg, "--") {
		name, _, hasValue := strings.Cut(strings.TrimPrefix(arg, "--"), "=")
		switch {
		case strings.HasPrefix(name, "d"):
			return true, false
		case name == "help":
			return true, false
		case isAcceptedLongOption(name, "mirror", 1):
			return true, false
		case isAcceptedLongOption(name, "prune", 3):
			return true, false
		case isAcceptedLongOption(name, "push-option", 2):
			return false, !hasValue
		default:
			return false, false
		}
	}
	if len(arg) < 2 || arg[0] != '-' {
		return false, false
	}
	for i := 1; i < len(arg); i++ {
		switch arg[i] {
		case 'n', 'd', 'h':
			return true, false
		case 'o':
			return false, i == len(arg)-1
		}
	}
	return false, false
}

func isAcceptedLongOption(name, full string, minimum int) bool {
	return len(name) >= minimum && strings.HasPrefix(full, name)
}

func programName(tok string) string {
	base := path.Base(filepath.ToSlash(tok))
	return strings.TrimSuffix(strings.ToLower(base), ".exe")
}
