package project

import "strings"

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
	} else if at := strings.Index(s, "@"); at >= 0 {
		// scp-like: git@host:org/repo.git — turn the first ':' into '/'.
		s = s[at+1:]
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
