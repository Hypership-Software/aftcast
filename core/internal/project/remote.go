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
