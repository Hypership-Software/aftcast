package handoff

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func gitRepo(t *testing.T, commits int) string {
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
	for i := 0; i < commits; i++ {
		path := filepath.Join(dir, "f.txt")
		if err := os.WriteFile(path, []byte{byte('a' + i)}, 0o600); err != nil {
			t.Fatal(err)
		}
		run("add", "f.txt")
		run("commit", "-m", "c")
	}
	return dir
}

func TestResolveSHAsNewestFirst(t *testing.T) {
	dir := gitRepo(t, 3)
	shas, err := ResolveSHAs(dir, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(shas) != 3 {
		t.Fatalf("got %d SHAs, want 3", len(shas))
	}
	for _, s := range shas {
		if len(s) != 40 {
			t.Errorf("SHA %q is not full-length", s)
		}
	}
}

func TestResolveSHAsRespectsLimit(t *testing.T) {
	dir := gitRepo(t, 3)
	shas, err := ResolveSHAs(dir, "main", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(shas) != 2 {
		t.Fatalf("got %d SHAs, want 2", len(shas))
	}
}

func TestResolveSHAsUnknownRefErrors(t *testing.T) {
	dir := gitRepo(t, 1)
	if _, err := ResolveSHAs(dir, "no-such-ref", 10); err == nil {
		t.Fatal("want error for unknown ref")
	}
}

func TestResolveSHAsOptionShapedRefErrors(t *testing.T) {
	dir := gitRepo(t, 1)
	if _, err := ResolveSHAs(dir, "--all", 10); err == nil {
		t.Fatal("want error for option-shaped ref")
	}
}

func TestMatchesAny(t *testing.T) {
	full := []string{"bb16536aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	if !MatchesAny("bb16536", full) {
		t.Error("abbreviated SHA should prefix-match")
	}
	if MatchesAny("bb16537", full) {
		t.Error("non-prefix must not match")
	}
	if MatchesAny("", full) {
		t.Error("empty captured SHA must never match")
	}
}
