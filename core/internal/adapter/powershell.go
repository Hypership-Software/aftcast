package adapter

import (
	"path"
	"path/filepath"
	"strings"
)

// powerShellCommandVerb resolves the first invoked program from a PowerShell
// command. POSIX tokenization reads PowerShell noise — `&`, `$errs`,
// `$ErrorActionPreference` — as the program, so this walks statements with
// PowerShell rules instead: assignments are skipped (a spaced `$var = ...`
// consumes its whole statement, since its value expression is not the session's
// verb until proven), the `&` call operator resolves to its target, and a bare
// variable in program position is unknowable rather than guessable.
func powerShellCommandVerb(command string) string {
	for _, line := range strings.Split(command, "\n") {
		if verb := powerShellLineVerb(line); verb != "" {
			return verb
		}
	}
	return ""
}

func powerShellLineVerb(line string) string {
	fields, err := powerShellFields(line)
	if err != nil {
		fields = strings.Fields(line)
	}
	skipping := false
	for i := 0; i < len(fields); i++ {
		raw := fields[i]
		endsStatement := strings.HasSuffix(raw, ";")
		if skipping {
			skipping = !endsStatement
			continue
		}
		tok := strings.TrimLeft(strings.TrimRight(raw, ";"), "(")
		switch {
		case tok == "" || tok == "&" || tok == "|" || tok == "{" || tok == "}" || tok == ")":
			continue
		case strings.HasPrefix(tok, "$"):
			if strings.Contains(tok, "=") {
				continue
			}
			if i+1 < len(fields) && strings.HasPrefix(fields[i+1], "=") {
				next := fields[i+1]
				i++
				skipping = !endsStatement && !strings.HasSuffix(next, ";")
				continue
			}
			return ""
		case isPowerShellKeyword(tok):
			continue
		default:
			base := strings.TrimSuffix(path.Base(filepath.ToSlash(tok)), ".exe")
			if base != "" && base != "." {
				return base
			}
		}
	}
	return ""
}

func isPowerShellKeyword(tok string) bool {
	switch strings.ToLower(tok) {
	case "if", "elseif", "else", "foreach", "for", "while", "do", "until",
		"switch", "try", "catch", "finally", "function", "param", "return",
		"throw", "begin", "process", "end":
		return true
	default:
		return false
	}
}
