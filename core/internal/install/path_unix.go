//go:build !windows

package install

import (
	"fmt"
	"os"
	"strings"
)

const (
	pathBlockStart = "# >>> aftcast >>>"
	pathBlockEnd   = "# <<< aftcast <<<"
)

// ensurePathEntry appends an idempotent, marked block to the user's shell profile
// exporting dir onto PATH. Returns (false,nil) when the block is already present.
func ensurePathEntry(dir string) (bool, error) {
	profile, err := shellProfile()
	if err != nil {
		return false, err
	}
	data, err := os.ReadFile(profile)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	if strings.Contains(string(data), pathBlockStart) {
		return false, nil
	}
	block := fmt.Sprintf("\n%s\nexport PATH=\"%s:$PATH\"\n%s\n", pathBlockStart, dir, pathBlockEnd)
	f, err := os.OpenFile(profile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return false, err
	}
	defer f.Close()
	if _, err := f.WriteString(block); err != nil {
		return false, err
	}
	return true, nil
}

func removePathEntry(_ string) error {
	profile, err := shellProfile()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(profile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	s := string(data)
	start := strings.Index(s, pathBlockStart)
	end := strings.Index(s, pathBlockEnd)
	if start < 0 || end < 0 || end < start {
		return nil
	}
	end += len(pathBlockEnd)
	if start > 0 && s[start-1] == '\n' {
		start--
	}
	return os.WriteFile(profile, []byte(s[:start]+s[end:]), 0o644)
}

func shellProfile() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return shellProfilePath(home, os.Getenv("SHELL"), fileExists), nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
