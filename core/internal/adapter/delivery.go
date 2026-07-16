package adapter

import (
	"errors"
	"strings"

	"github.com/Hypership-Software/aftcast/internal/schema"
	"github.com/google/shlex"
)

type shellDialect int

const (
	dialectPOSIX shellDialect = iota
	dialectPowerShell
)

func (d shellDialect) escapeChar() byte {
	if d == dialectPowerShell {
		return '`'
	}
	return '\\'
}

func successfulDeliverySignal(tool, command string) schema.DeliverySignal {
	dialect, ok := deliveryDialect(tool)
	if !ok {
		return ""
	}
	segments, operators, ok := parseShellCommand(command, dialect)
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
			if hasDynamicShellSyntax(segment.raw, dialect) {
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

func deliveryDialect(tool string) (shellDialect, bool) {
	switch programName(tool) {
	case "bash", "sh", "dash", "zsh", "ksh":
		return dialectPOSIX, true
	case "powershell", "pwsh":
		return dialectPowerShell, true
	default:
		return dialectPOSIX, false
	}
}

type shellSegment struct {
	raw  string
	argv []string
}

func parseShellCommand(command string, dialect shellDialect) ([]shellSegment, []string, bool) {
	rawSegments, operators, ok := splitShellCommand(command, dialect)
	if !ok {
		return nil, nil, false
	}
	segments := make([]shellSegment, 0, len(rawSegments))
	for _, raw := range rawSegments {
		argv, err := segmentFields(raw, dialect)
		if err != nil || len(argv) == 0 {
			return nil, nil, false
		}
		segments = append(segments, shellSegment{raw: raw, argv: argv})
	}
	return segments, operators, true
}

func segmentFields(raw string, dialect shellDialect) ([]string, error) {
	if dialect == dialectPowerShell {
		return powerShellFields(raw)
	}
	return shlex.Split(raw)
}

// hasDynamicShellSyntax reports content whose runtime value cannot be read from
// the text: expansions, substitutions, globs — and unquoted parens, whose
// grouping this parser does not model. Single quotes protect literally in both
// dialects; the dialect's escape char protects the next character.
func hasDynamicShellSyntax(raw string, dialect shellDialect) bool {
	var quote byte
	escaped := false
	escape := dialect.escapeChar()
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
		if ch == escape {
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
		if ch == '$' {
			return true
		}
		if dialect == dialectPOSIX && ch == '`' {
			return true
		}
		if quote == 0 && strings.ContainsRune("*?[{}()", rune(ch)) {
			return true
		}
	}
	return false
}

func splitShellCommand(command string, dialect shellDialect) ([]string, []string, bool) {
	var segments []string
	var operators []string
	var quote byte
	escaped := false
	escape := dialect.escapeChar()
	start := 0

scan:
	for i := 0; i < len(command); i++ {
		ch := command[i]
		if escaped {
			escaped = false
			continue
		}
		if quote != 0 {
			if quote == '"' && ch == escape {
				escaped = true
				continue
			}
			if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == escape {
			escaped = true
			continue
		}
		if ch == '\'' || ch == '"' {
			quote = ch
			continue
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
			switch {
			case i+1 < len(command) && command[i+1] == '&':
				operator = "&&"
				width = 2
			case i > 0 && command[i-1] == '>':
				continue // stream duplication: 2>&1, >&2
			case i+1 < len(command) && command[i+1] == '>':
				continue // redirect-all: &> log
			default:
				// Backgrounding (POSIX) or the call operator (PowerShell):
				// either way the push's completion is unprovable from exit 0.
				return nil, nil, false
			}
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

var errUnterminatedToken = errors.New("adapter: unterminated powershell token")

func powerShellFields(raw string) ([]string, error) {
	var fields []string
	var current strings.Builder
	var quote byte
	escaped := false
	inToken := false
	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		switch {
		case escaped:
			current.WriteByte(ch)
			escaped = false
			inToken = true
		case quote == '\'':
			if ch == '\'' {
				quote = 0
			} else {
				current.WriteByte(ch)
			}
		case ch == '`':
			escaped = true
			inToken = true
		case quote == '"':
			if ch == '"' {
				quote = 0
			} else {
				current.WriteByte(ch)
			}
		case ch == '\'' || ch == '"':
			quote = ch
			inToken = true
		case ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n':
			if inToken {
				fields = append(fields, current.String())
				current.Reset()
				inToken = false
			}
		default:
			current.WriteByte(ch)
			inToken = true
		}
	}
	if quote != 0 || escaped {
		return nil, errUnterminatedToken
	}
	if inToken {
		fields = append(fields, current.String())
	}
	return fields, nil
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
	return strings.TrimSuffix(strings.ToLower(pathBase(tok)), ".exe")
}
