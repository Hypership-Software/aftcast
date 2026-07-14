package adapter

import (
	"path"
	"path/filepath"
	"strings"

	"github.com/Hypership-Software/atlas/internal/schema"
)

var shellSeparator = map[string]bool{
	"&&": true,
	"||": true,
	";":  true,
	"|":  true,
}

func deliverySignal(argv []string) schema.DeliverySignal {
	for _, segment := range commandSegments(argv) {
		if isGitPush(segment) {
			return schema.DeliveryGitPush
		}
	}
	return ""
}

func commandSegments(argv []string) [][]string {
	var out [][]string
	var current []string
	for _, tok := range argv {
		if shellSeparator[tok] {
			if len(current) > 0 {
				out = append(out, current)
				current = nil
			}
			continue
		}
		current = append(current, tok)
	}
	if len(current) > 0 {
		out = append(out, current)
	}
	return out
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
	for _, arg := range segment[i+1:] {
		if arg == "-n" || arg == "-d" || strings.HasPrefix(arg, "--dry-run") || strings.HasPrefix(arg, "--delete") || strings.HasPrefix(arg, ":") {
			return false
		}
	}
	return true
}

func programName(tok string) string {
	base := path.Base(filepath.ToSlash(tok))
	return strings.TrimSuffix(strings.ToLower(base), ".exe")
}
