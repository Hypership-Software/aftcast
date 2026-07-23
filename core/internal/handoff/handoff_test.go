package handoff_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Hypership-Software/aftcast/internal/handoff"
)

// gitRepoForE2E is a package-boundary duplicate of refs_test.go's gitRepo:
// this file is package handoff_test (an external test package) so it cannot
// see the internal-test helper. DRY yields to the package boundary here.
func gitRepoForE2E(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "main")
	for i := 0; i < 2; i++ {
		path := filepath.Join(dir, "f.txt")
		if err := os.WriteFile(path, []byte{byte('a' + i)}, 0o600); err != nil {
			t.Fatal(err)
		}
		run("add", "f.txt")
		run("commit", "-m", "c")
	}
	return dir
}

// Run wires ref resolution, the read-model join, facts, verification, and
// rendering. With an empty home (no log) and a repo whose commits nothing
// captured, the digest must still render — with the honesty lines, not an error.
func TestRunEmptyRecordIsHonest(t *testing.T) {
	home := t.TempDir()
	repo := gitRepoForE2E(t)
	out, err := handoff.Run(home, repo, "")
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)
	if !strings.Contains(text, "No captured session") {
		t.Errorf("empty record must render the honesty line, got:\n%s", text)
	}
	if !strings.Contains(text, "## Attestation") {
		t.Error("attestation section missing")
	}
}
