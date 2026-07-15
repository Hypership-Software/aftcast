package install

import (
	"path/filepath"
	"strings"
)

// addToPath returns pathVal with dir appended (';'-separated) when absent; changed
// is false if dir was already present (case-insensitive, trailing-slash-insensitive).
func addToPath(pathVal, dir string) (next string, changed bool) {
	if pathListContains(pathVal, dir) {
		return pathVal, false
	}
	if pathVal != "" && !strings.HasSuffix(pathVal, ";") {
		pathVal += ";"
	}
	return pathVal + dir, true
}

func removeFromPath(pathVal, dir string) string {
	parts := strings.Split(pathVal, ";")
	kept := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" && !pathEntryEqual(p, dir) {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, ";")
}

func pathListContains(pathVal, dir string) bool {
	for _, p := range strings.Split(pathVal, ";") {
		if pathEntryEqual(p, dir) {
			return true
		}
	}
	return false
}

func pathEntryEqual(a, b string) bool {
	return strings.EqualFold(strings.TrimRight(a, `\`), strings.TrimRight(b, `\`))
}

func shellProfilePath(home, shell string, exists func(string) bool) string {
	profile := func(name string) string { return filepath.Join(home, name) }
	switch strings.ToLower(filepath.Base(shell)) {
	case "zsh":
		return profile(".zshrc")
	case "bash":
		for _, name := range []string{".bashrc", ".bash_profile"} {
			if path := profile(name); exists(path) {
				return path
			}
		}
		return profile(".profile")
	}
	for _, name := range []string{".zshrc", ".bashrc", ".bash_profile", ".profile"} {
		if path := profile(name); exists(path) {
			return path
		}
	}
	return profile(".profile")
}
