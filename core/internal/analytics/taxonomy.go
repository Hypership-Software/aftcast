package analytics

import (
	"path"
	"path/filepath"
	"strings"

	"github.com/Hypership-Software/atlas/internal/schema"
)

// Taxonomy classifies a session's task from the files it touched and the commands
// it ran — deterministic, most-specific category first. confidence is the share of
// touched files supporting the winner (a coarse verb-only signal defaults to 0.5).
func Taxonomy(evts []schema.TelemetryEvent) (taskType string, confidence float64) {
	var written, read []string
	ranTest, failedBeforeWrite, sawFailure := false, false, false
	for _, e := range evts {
		switch e.ToolClass {
		case schema.ClassFileWrite:
			written = append(written, e.Files...)
			if sawFailure {
				failedBeforeWrite = true
			}
		case schema.ClassFileRead:
			read = append(read, e.Files...)
		case schema.ClassExec:
			if hasVerb(e.Verbs, testVerbs) || (hasVerb(e.Verbs, map[string]bool{"go": true}) && e.ToolOK != "") {
				ranTest = true
			}
		}
		if e.ToolOK == schema.OutcomeFailed {
			sawFailure = true
		}
	}
	written, read = dedup(written), dedup(read)
	total := len(written)

	if n := count(written, isMigration); n > 0 {
		return TaskMigration, ratio(n, total)
	}
	if n := count(written, isTestFile); n > 0 {
		return TaskTesting, ratio(n, total)
	}
	if ranTest && total == 0 {
		return TaskTesting, 0.5
	}
	if n := count(written, isInfra); n > 0 {
		return TaskInfra, ratio(n, total)
	}
	if n := count(written, isConfig); n > 0 {
		return TaskConfig, ratio(n, total)
	}
	if total > 0 && count(written, isDocs) == total {
		return TaskDocs, 1.0
	}
	if failedBeforeWrite && total > 0 {
		return TaskBugfix, ratio(total, total)
	}
	if total > 0 {
		return TaskFeature, ratio(total, total)
	}
	return TaskExploration, ratio(len(read), len(read))
}

func isTestFile(f string) bool {
	b := base(f)
	return strings.Contains(b, "_test.") || strings.Contains(b, ".test.") || strings.Contains(b, ".spec.")
}

func isMigration(f string) bool {
	s := slash(f)
	return strings.Contains(s, "/migrations/") || strings.HasPrefix(s, "migrations/")
}

func isInfra(f string) bool {
	b, e, s := base(f), ext(f), slash(f)
	return b == "Dockerfile" || e == ".tf" ||
		strings.Contains(s, "/terraform/") || strings.HasPrefix(s, "terraform/") ||
		strings.Contains(s, "/k8s/") || strings.Contains(s, "/helm/")
}

func isConfig(f string) bool {
	switch base(f) {
	case "package.json", "tsconfig.json", "go.mod", "go.sum", "Cargo.toml", "pyproject.toml":
		return true
	}
	switch ext(f) {
	case ".toml", ".yaml", ".yml", ".ini", ".cfg":
		return true
	}
	return strings.Contains(slash(f), "/.github/workflows/")
}

func isDocs(f string) bool {
	return ext(f) == ".md" || strings.Contains(slash(f), "/docs/") || strings.HasPrefix(slash(f), "docs/")
}

func slash(f string) string { return filepath.ToSlash(f) }
func base(f string) string  { return path.Base(slash(f)) }
func ext(f string) string   { return strings.ToLower(path.Ext(slash(f))) }

func count(files []string, pred func(string) bool) int {
	n := 0
	for _, f := range files {
		if pred(f) {
			n++
		}
	}
	return n
}

func dedup(xs []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, x := range xs {
		if x == "" {
			continue
		}
		if _, ok := seen[x]; ok {
			continue
		}
		seen[x] = struct{}{}
		out = append(out, x)
	}
	return out
}

// ratio is n/total clamped to [0,1]; 0 total yields 0.
func ratio(n, total int) float64 {
	if total <= 0 {
		return 0
	}
	r := float64(n) / float64(total)
	if r > 1 {
		return 1
	}
	return r
}
