// Package project resolves a working directory to a stable, opaque project
// identity shared by the capture hook and the `aftcast` client. Only the hash is
// ever persisted; neither the path nor the remote URL leaves this process.
package project

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const idLen = 12

// Identify resolves startDir to (root, id). id tracks the repo, not the folder it
// lives in — normalized origin remote when present, else the canonical repo/cwd
// path — so a folder move or a second clone of the same repo keeps one identity.
func Identify(startDir string) (root, id string) {
	if startDir == "" {
		return "", ""
	}
	root, isRepo := findRoot(startDir)
	return root, shortHash(projectKey(root, isRepo))
}

// Repository resolves startDir only when it belongs to a Git repository. It is
// used by local, rebuildable projections that may show a repository's basename;
// callers must not persist root or derive wire data from it.
func Repository(startDir string) (root, id string, ok bool) {
	if startDir == "" {
		return "", "", false
	}
	root, isRepo := findRoot(startDir)
	if !isRepo {
		return "", "", false
	}
	return root, shortHash(projectKey(root, true)), true
}

func findRoot(startDir string) (root string, isRepo bool) {
	dir := startDir
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return startDir, false
		}
		dir = parent
	}
}

func projectKey(root string, isRepo bool) string {
	if isRepo {
		if remote := originRemote(root); remote != "" {
			return remote
		}
	}
	return canonical(root)
}

func shortHash(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])[:idLen]
}

// canonical normalizes a path so the hook side and client side hash identical
// bytes: absolute, symlinks resolved (best-effort), cleaned, and case-folded on
// Windows (its filesystem is case-insensitive, so C:\p and c:\P are one project).
func canonical(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		abs = p
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	abs = filepath.Clean(abs)
	if runtime.GOOS == "windows" {
		abs = strings.ToLower(abs)
	}
	return abs
}
