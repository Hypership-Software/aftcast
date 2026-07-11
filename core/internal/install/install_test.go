package install

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitWritesHooksAndBacksUp(t *testing.T) {
	dir := t.TempDir()
	settings := filepath.Join(dir, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settings), 0o755); err != nil {
		t.Fatal(err)
	}
	orig := `{"permissions":{"allow":["Bash(node:*)"]}}`
	if err := os.WriteFile(settings, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}

	// Pin the self-verify probe at a dead port (nothing binds :1 without root) so
	// the "no daemon" assertion is deterministic even if a real daemon is running.
	home := filepath.Join(dir, "gate-home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "daemon.json"),
		[]byte(`{"http_port":1,"http_url":"http://127.0.0.1:1/hook"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	opts := Options{
		Home:         home,
		SettingsPath: settings,
		BinaryPath:   "C:/opt/gated.exe",
	}
	var out bytes.Buffer
	if err := Init(opts, &out); err != nil {
		t.Fatalf("Init: %v", err)
	}

	got, _ := os.ReadFile(settings)
	hasHTTP, hasSession := hooksPresent(got)
	if !hasHTTP || !hasSession {
		t.Fatalf("Init did not wire both transports: http=%v session=%v\n%s", hasHTTP, hasSession, got)
	}
	if !strings.Contains(string(got), `"allow"`) {
		t.Error("Init dropped the user's permissions")
	}

	// Backup preserves the pre-init settings verbatim.
	bak, err := os.ReadFile(settings + backupSuffix)
	if err != nil {
		t.Fatalf("no backup written: %v", err)
	}
	if string(bak) != orig {
		t.Errorf("backup = %s, want verbatim original %s", bak, orig)
	}

	// With no daemon running, Init reports the verification gap rather than failing.
	if !strings.Contains(out.String(), "could not verify a running daemon") {
		t.Errorf("expected a self-verify gap note, got:\n%s", out.String())
	}
}

func TestInitThenUninstallRestores(t *testing.T) {
	dir := t.TempDir()
	settings := filepath.Join(dir, "settings.json")
	orig := `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"my-linter"}]}]}}`
	if err := os.WriteFile(settings, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	opts := Options{Home: filepath.Join(dir, "h"), SettingsPath: settings, BinaryPath: "C:/opt/gated.exe"}

	if err := Init(opts, new(bytes.Buffer)); err != nil {
		t.Fatal(err)
	}
	if err := Uninstall(opts, new(bytes.Buffer)); err != nil {
		t.Fatal(err)
	}

	got, _ := os.ReadFile(settings)
	if !strings.Contains(string(got), "my-linter") {
		t.Errorf("uninstall dropped the user's hook:\n%s", got)
	}
	if strings.Contains(string(got), "/hook") || strings.Contains(string(got), "hook claudecode") {
		t.Errorf("uninstall left gate hooks behind:\n%s", got)
	}
}

func TestResolveSettingsPathHonorsEnv(t *testing.T) {
	want := filepath.Join(t.TempDir(), "proj", "settings.json")
	t.Setenv("GATED_SETTINGS", want)
	got, err := resolveSettingsPath("")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("resolveSettingsPath = %q, want %q (GATED_SETTINGS override)", got, want)
	}
}

func TestDoctorReportsWiringGaps(t *testing.T) {
	dir := t.TempDir()
	settings := filepath.Join(dir, "settings.json")
	// A settings file with no gate hooks and no running daemon: doctor must fail.
	if err := os.WriteFile(settings, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	ok := Doctor(Options{Home: filepath.Join(dir, "h"), SettingsPath: settings}, &out)
	if ok {
		t.Error("Doctor reported healthy for an un-wired, daemon-less install")
	}
	if !strings.Contains(out.String(), "FAIL") {
		t.Errorf("Doctor output missing any FAIL line:\n%s", out.String())
	}
}
