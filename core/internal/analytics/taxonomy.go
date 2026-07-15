package analytics

import (
	"path"
	"path/filepath"
	"strings"

	"github.com/Hypership-Software/aftcast/internal/schema"
)

// write categories. Every written file maps to exactly one, most-specific first;
// anything that is not a recognized specialized artifact is implementation source.
const (
	catImpl      = "impl"
	catTest      = "test"
	catMigration = "migration"
	catInfra     = "infra"
	catConfig    = "config"
	catDocs      = "docs"
)

// Taxonomy classifies a session's task from the files it wrote and the commands it
// ran. The governing rule: if the session wrote implementation source, it is feature
// work — tests, docs, and config written alongside are in service of the feature,
// not the task's identity. This is deliberate: a TDD session writes tests with its
// code, and writing tests must not make it a "testing" task. Only a session with no
// implementation source takes its identity from its dominant specialized output.
// confidence is the winning category's share of the written files.
//
// (There is intentionally no automatic "bugfix" class: a red→green cycle is
// indistinguishable from ordinary TDD with metadata alone — ADR-011 — so guessing
// bugfix from tool failures mislabels the disciplined workflow it should reward.)
func Taxonomy(evts []schema.TelemetryEvent) (taskType string, confidence float64) {
	var written, read []string
	ranTest := false
	for _, e := range evts {
		switch e.ToolClass {
		case schema.ClassFileWrite:
			written = append(written, e.Files...)
		case schema.ClassFileRead:
			read = append(read, e.Files...)
		case schema.ClassExec:
			if hasVerb(e.Verbs, testVerbs) || (hasVerb(e.Verbs, map[string]bool{"go": true}) && e.ToolOK != "") {
				ranTest = true
			}
		}
	}
	written, read = dedup(written), dedup(read)
	total := len(written)
	if total == 0 {
		if ranTest {
			return TaskTesting, 0.5
		}
		return TaskExploration, ratio(len(read), len(read))
	}

	buckets := map[string]int{}
	for _, f := range written {
		buckets[fileCategory(f)]++
	}
	if impl := buckets[catImpl]; impl > 0 {
		return TaskFeature, ratio(impl, total)
	}
	// No implementation source: identity is the dominant specialized output; ties
	// resolve by the specificity order below.
	for _, o := range []struct{ cat, task string }{
		{catMigration, TaskMigration},
		{catInfra, TaskInfra},
		{catConfig, TaskConfig},
		{catTest, TaskTesting},
		{catDocs, TaskDocs},
	} {
		if buckets[o.cat] == maxBucket(buckets) {
			return o.task, ratio(buckets[o.cat], total)
		}
	}
	return TaskFeature, ratio(total, total)
}

// fileCategory maps a written file to exactly one category, most-specific first.
func fileCategory(f string) string {
	switch {
	case isMigration(f):
		return catMigration
	case isInfra(f):
		return catInfra
	case isTestFile(f):
		return catTest
	case isConfig(f):
		return catConfig
	case isDocs(f):
		return catDocs
	default:
		return catImpl
	}
}

func maxBucket(buckets map[string]int) int {
	max := 0
	for _, n := range buckets {
		if n > max {
			max = n
		}
	}
	return max
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
