package project

import (
	"os"
	"path/filepath"
	"strings"
)

// normalizeRemote reduces a git remote URL to a stable host/org/repo key, lowercased,
// so the same repo reached over SSH or HTTPS (or cloned to different paths) hashes to
// one identity. Returns "" when raw isn't a recognizable remote URL.
func normalizeRemote(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
		if at := strings.Index(s, "@"); at >= 0 {
			s = s[at+1:]
		}
		// drop a :port sitting between host and the first '/'
		if slash := strings.Index(s, "/"); slash >= 0 {
			if colon := strings.IndexByte(s[:slash], ':'); colon >= 0 {
				s = s[:colon] + s[slash:]
			}
		}
	} else {
		if at := strings.Index(s, "@"); at >= 0 {
			s = s[at+1:]
		}
		if colon := strings.Index(s, ":"); colon >= 0 {
			s = s[:colon] + "/" + s[colon+1:]
		}
	}
	s = strings.Trim(s, "/")
	s = strings.TrimSuffix(s, ".git")
	if !strings.Contains(s, "/") {
		return ""
	}
	return strings.ToLower(s)
}

// originRemote returns the normalized origin remote for the repo at root, or "".
// It reads .git/config directly — no `git` subprocess — so it stays cheap on the
// capture hot path.
func originRemote(root string) string {
	cfg := gitConfigPath(root)
	if cfg == "" {
		return ""
	}
	data, err := os.ReadFile(cfg)
	if err != nil {
		return ""
	}
	return normalizeRemote(originURL(string(data)))
}

// gitConfigPath returns the path to the config holding remotes. For a normal repo
// that is <root>/.git/config; for a linked worktree, .git is a file pointing at
// <main>/.git/worktrees/<name>, and the shared config lives in the common dir
// (found via the commondir file).
func gitConfigPath(root string) string {
	dotgit := filepath.Join(root, ".git")
	info, err := os.Stat(dotgit)
	if err != nil {
		return ""
	}
	if info.IsDir() {
		return filepath.Join(dotgit, "config")
	}
	data, err := os.ReadFile(dotgit)
	if err != nil {
		return ""
	}
	gitdir := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(data)), "gitdir:"))
	if gitdir == "" {
		return ""
	}
	if !filepath.IsAbs(gitdir) {
		gitdir = filepath.Join(root, gitdir)
	}
	common := gitdir
	if cd, err := os.ReadFile(filepath.Join(gitdir, "commondir")); err == nil {
		rel := strings.TrimSpace(string(cd))
		if filepath.IsAbs(rel) {
			common = rel
		} else {
			common = filepath.Join(gitdir, rel)
		}
	}
	return filepath.Join(common, "config")
}

// originURL extracts the [remote "origin"] url from git config text.
func originURL(cfg string) string {
	inOrigin := false
	for _, line := range strings.Split(cfg, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "[") {
			inOrigin = strings.HasPrefix(t, `[remote "origin"]`)
			continue
		}
		if inOrigin {
			if k, v, ok := strings.Cut(t, "="); ok && strings.TrimSpace(k) == "url" {
				return strings.TrimSpace(v)
			}
		}
	}
	return ""
}
