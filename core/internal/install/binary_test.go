package install

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Hypership-Software/atlas/internal/svc"
)

func TestInstallBinaryCopiesIntoHomeBin(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	src := filepath.Join(dir, "build", "gated-fresh")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	want := []byte("fresh-binary-bytes")
	if err := os.WriteFile(src, want, 0o755); err != nil {
		t.Fatal(err)
	}

	installed, replaced, err := installBinary(home, src)
	if err != nil {
		t.Fatalf("installBinary: %v", err)
	}
	if !replaced {
		t.Error("expected replaced=true for a fresh install")
	}
	if got := filepath.Dir(installed); got != filepath.Join(home, "bin") {
		t.Errorf("installed under %s, want %s", got, filepath.Join(home, "bin"))
	}
	got, err := os.ReadFile(installed)
	if err != nil {
		t.Fatalf("read installed: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("installed bytes = %q, want %q", got, want)
	}
}

func TestInstallBinaryNoOpWhenSourceIsInstalled(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	dest := filepath.Join(home, "bin", binaryName())
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	want := []byte("already-installed")
	if err := os.WriteFile(dest, want, 0o755); err != nil {
		t.Fatal(err)
	}

	installed, replaced, err := installBinary(home, dest)
	if err != nil {
		t.Fatalf("installBinary: %v", err)
	}
	if replaced {
		t.Error("expected replaced=false when the source is already the installed binary")
	}
	if installed != dest {
		t.Errorf("installed = %s, want %s", installed, dest)
	}
	// A copy-onto-itself must never truncate the running binary.
	got, _ := os.ReadFile(dest)
	if string(got) != string(want) {
		t.Errorf("installed binary corrupted: %q, want %q", got, want)
	}
}

func TestInitInstallsBinaryAndPointsSettingsAtIt(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	settings := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(settings, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// A real source binary outside home, so the install actually copies.
	src := filepath.Join(dir, "build", "gated-fresh.exe")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("srcbin"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Fake the daemon at a dead port so self-verify never touches a real daemon.
	prev := ensureDaemon
	ensureDaemon = func(svc.EnsureOptions) (svc.Info, bool, error) {
		return svc.Info{HTTPPort: 1, HTTPURL: "http://127.0.0.1:1/hook"}, true, nil
	}
	t.Cleanup(func() { ensureDaemon = prev })

	if err := Init(Options{Home: home, SettingsPath: settings, BinaryPath: src}, new(bytes.Buffer)); err != nil {
		t.Fatalf("Init: %v", err)
	}

	installed := filepath.Join(home, "bin", binaryName())
	if _, err := os.Stat(installed); err != nil {
		t.Fatalf("binary not installed into home: %v", err)
	}
	got, _ := os.ReadFile(settings)
	if wantCmd := filepath.ToSlash(installed); !strings.Contains(string(got), wantCmd) {
		t.Errorf("SessionStart hook does not point at the installed binary %q:\n%s", wantCmd, got)
	}
	if strings.Contains(string(got), filepath.ToSlash(src)) {
		t.Errorf("settings still reference the source binary path %q:\n%s", src, got)
	}
}
