package adapter

import (
	"encoding/json"
	"regexp"
	"strings"
)

// commitLineRe matches git commit's summary line: "[branch sha] subject", with
// "(root-commit)" / "detached HEAD" variants — the SHA is the last hex token
// before the closing bracket.
var commitLineRe = regexp.MustCompile(`^\[[^\]\n]+ ([0-9a-f]{7,40})\](?: |$)`)

// commitSHA extracts the created commit's SHA from a successful shell call, the
// join key from this session's conduct to the git record of its outcome. The
// stdout is spoofable (any segment can echo a commit-shaped line), so a match is
// trusted only when the whole command proves the last bracket line is git's own:
// no `||` anywhere (a masked failure makes execution unprovable), a literal
// `git commit` segment present, only git segments after it (git prints no
// commit-summary look-alikes, so nothing downstream can outrank the real line;
// spoof lines from earlier segments lose to it because output follows execution
// order), and the commit not silenced by --quiet (a quiet commit prints no
// honest line, leaving only spoofs to match). Last match wins so an --amend
// chain yields the SHA that exists. Best-effort per ADR-006: an unparseable
// command extracts nothing.
func commitSHA(tool, command, stdout string) string {
	if stdout == "" {
		return ""
	}
	dialect, ok := deliveryDialect(tool)
	if !ok {
		return ""
	}
	segments, operators, ok := parseShellCommand(command, dialect)
	if !ok {
		return ""
	}
	for _, operator := range operators {
		if operator == "||" {
			return ""
		}
	}
	last := -1
	for i, segment := range segments {
		if isGitCommit(segment.argv) {
			last = i
		}
	}
	if last < 0 || quietCommit(segments[last].argv) {
		return ""
	}
	for _, segment := range segments[last+1:] {
		if !isGitSegment(segment.argv) {
			return ""
		}
	}

	sha := ""
	for _, line := range strings.Split(stdout, "\n") {
		if m := commitLineRe.FindStringSubmatch(strings.TrimRight(line, "\r")); m != nil {
			sha = m[1]
		}
	}
	return sha
}

// isGitCommit walks the same global-option prefix as isGitPush and reports
// whether the segment's subcommand is commit.
func isGitCommit(segment []string) bool {
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
			return tok == "commit"
		}
	}
	return false
}

func isGitSegment(segment []string) bool {
	i := 0
	for i < len(segment) && envAssignRe.MatchString(segment[i]) {
		i++
	}
	return i < len(segment) && programName(segment[i]) == "git"
}

// quietCommit is deliberately over-broad: any short-flag cluster containing q
// (or --quiet) counts, and a false positive only costs one uncaptured SHA.
func quietCommit(segment []string) bool {
	for _, tok := range segment {
		if tok == "--quiet" {
			return true
		}
		if len(tok) > 1 && tok[0] == '-' && tok[1] != '-' && strings.ContainsRune(tok, 'q') {
			return true
		}
	}
	return false
}

// responseStdout pulls only the stdout field from a tool_response object; a
// string-shaped response (skill launches) has no stdout and yields nothing.
func responseStdout(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var r struct {
		Stdout string `json:"stdout"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return ""
	}
	return r.Stdout
}
