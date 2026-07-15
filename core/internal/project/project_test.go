package project

import (
	"os"
	"path/filepath"
	"testing"
)

func writeGitConfig(t *testing.T, root, url string) {
	t.Helper()
	gitDir := filepath.Join(root, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "[core]\n\trepositoryformatversion = 0\n[remote \"origin\"]\n\turl = " + url +
		"\n\tfetch = +refs/heads/*:refs/remotes/origin/*\n"
	if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestIdentify_RemoteWinsAndAgreesAcrossSubdirs(t *testing.T) {
	root := t.TempDir()
	writeGitConfig(t, root, "git@github.com:acme/app.git")
	sub := filepath.Join(root, "pkg", "svc")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	_, idRoot := Identify(root)
	_, idSub := Identify(sub)
	if idRoot != idSub {
		t.Errorf("subdir id %q must equal root id %q", idSub, idRoot)
	}
	if idRoot != shortHash("github.com/acme/app") {
		t.Errorf("id = %q, want hash of normalized remote", idRoot)
	}
}

func TestIdentify_SameRemoteDifferentPathsMatch(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	writeGitConfig(t, a, "https://github.com/acme/app.git")
	writeGitConfig(t, b, "https://github.com/acme/app") // same repo, moved/cloned elsewhere
	_, idA := Identify(a)
	_, idB := Identify(b)
	if idA != idB {
		t.Errorf("same remote at different paths gave %q vs %q", idA, idB)
	}
}

func TestIdentify_NoRemoteFallsBackToPath(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil { // repo, no config
		t.Fatal(err)
	}
	_, id := Identify(root)
	if id != shortHash(canonical(root)) {
		t.Errorf("no-remote repo id = %q, want hash of canonical root", id)
	}
}

func TestIdentify_NonRepoUsesCwdPath(t *testing.T) {
	dir := t.TempDir()
	root, id := Identify(dir)
	if root != dir {
		t.Errorf("root = %q, want startDir %q", root, dir)
	}
	if id != shortHash(canonical(dir)) {
		t.Errorf("non-repo id = %q, want hash of canonical dir", id)
	}
}

func TestIdentify_Empty(t *testing.T) {
	if r, id := Identify(""); r != "" || id != "" {
		t.Errorf(`Identify("") = (%q,%q), want empty`, r, id)
	}
}

func TestIdentify_WorktreeConvergesToMainRepo(t *testing.T) {
	main := t.TempDir()
	writeGitConfig(t, main, "https://github.com/acme/app.git")
	wtGitDir := filepath.Join(main, ".git", "worktrees", "wt")
	if err := os.MkdirAll(wtGitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtGitDir, "commondir"), []byte("../..\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wt := t.TempDir()
	if err := os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: "+wtGitDir+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, idMain := Identify(main)
	_, idWt := Identify(wt)
	if idWt != idMain {
		t.Errorf("worktree id %q must converge to main repo id %q", idWt, idMain)
	}
	if idWt != shortHash("github.com/acme/app") {
		t.Errorf("worktree id = %q, want hash of normalized remote", idWt)
	}
}

func TestIdentify_ConfigWithoutOriginFallsBackToPath(t *testing.T) {
	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte("[core]\n\tbare = false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, id := Identify(root)
	if id != shortHash(canonical(root)) {
		t.Errorf("no-origin config id = %q, want canonical path hash", id)
	}
}

func TestRepositoryRejectsNonRepoAndResolvesRepo(t *testing.T) {
	nonRepo := t.TempDir()
	if root, id, ok := Repository(nonRepo); ok || root != "" || id != "" {
		t.Fatalf("Repository(non-repo) = (%q, %q, %v), want empty", root, id, ok)
	}

	repo := t.TempDir()
	writeGitConfig(t, repo, "git@github.com:acme/app.git")
	sub := filepath.Join(repo, "pkg")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	root, id, ok := Repository(sub)
	if !ok || root != repo || id != shortHash("github.com/acme/app") {
		t.Fatalf("Repository(repo child) = (%q, %q, %v)", root, id, ok)
	}
}
