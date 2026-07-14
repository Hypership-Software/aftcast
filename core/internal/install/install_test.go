package install

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Hypership-Software/atlas/internal/svc"
)

// Force plain output so assertions match regardless of whether the test runner's
// stdout is a terminal.
func TestMain(m *testing.M) {
	os.Setenv("NO_COLOR", "1")
	os.Exit(m.Run())
}

func TestDoctorReportsStaleDaemonAsDown(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "h")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	// A stale record pointing at a port nothing listens on must read as down.
	if err := os.WriteFile(filepath.Join(home, "daemon.json"),
		[]byte(`{"pid":999999,"http_port":1,"http_url":"http://127.0.0.1:1/hook"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	settings := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(settings, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if Doctor(Options{Home: home, SettingsPath: settings}, &out) {
		t.Error("doctor reported healthy despite a stale daemon record")
	}
	if !strings.Contains(out.String(), "[FAIL] daemon running") {
		t.Errorf("stale daemon record not reported as down:\n%s", out.String())
	}
}

func TestStatusFlagsPortMismatch(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "h")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	// A bare listener stands in for a live daemon on an ephemeral port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	if err := os.WriteFile(filepath.Join(home, "daemon.json"),
		[]byte(fmt.Sprintf(`{"pid":1,"http_port":%d,"http_url":"http://127.0.0.1:%d/hook"}`, port, port)), 0o600); err != nil {
		t.Fatal(err)
	}
	// Settings wired at a different port than the daemon actually bound.
	settings := filepath.Join(dir, "settings.json")
	body := fmt.Sprintf(`{"hooks":{"PreToolUse":[{"matcher":"*","hooks":[{"type":"http","url":"http://127.0.0.1:%d/hook"}]}],"SessionStart":[{"hooks":[{"type":"command","command":"gated hook claudecode"}]}]}}`, port+1)
	if err := os.WriteFile(settings, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if Status(Options{Home: home, SettingsPath: settings}, &out) {
		t.Error("status reported healthy despite settings pointing at the wrong port")
	}
	if !strings.Contains(out.String(), "port") {
		t.Errorf("status did not flag the port mismatch:\n%s", out.String())
	}
}

func TestInitStartsDaemonAndPointsHooksAtBoundPort(t *testing.T) {
	dir := t.TempDir()
	settings := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(settings, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	prev := ensureDaemon
	ensureDaemon = func(svc.EnsureOptions) (svc.Info, bool, error) {
		return svc.Info{HTTPPort: 47105, HTTPURL: "http://127.0.0.1:47105/hook"}, true, nil
	}
	t.Cleanup(func() { ensureDaemon = prev })

	var out bytes.Buffer
	opts := Options{Home: filepath.Join(dir, "h"), SettingsPath: settings, BinaryPath: "C:/opt/gated.exe"}
	if err := Init(opts, &out); err != nil {
		t.Fatalf("Init: %v", err)
	}

	got, _ := os.ReadFile(settings)
	if !strings.Contains(string(got), "http://127.0.0.1:47105/hook") {
		t.Errorf("hooks not pointed at the daemon's actually-bound port:\n%s", got)
	}
	if !strings.Contains(out.String(), "started the Atlas daemon") {
		t.Errorf("Init did not report starting the daemon:\n%s", out.String())
	}
}

func TestUninstallStopsDaemon(t *testing.T) {
	dir := t.TempDir()
	settings := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(settings, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	stopped := false
	prev := stopDaemon
	stopDaemon = func(string) (bool, error) { stopped = true; return true, nil }
	t.Cleanup(func() { stopDaemon = prev })

	if err := Uninstall(Options{Home: filepath.Join(dir, "h"), SettingsPath: settings}, new(bytes.Buffer)); err != nil {
		t.Fatal(err)
	}
	if !stopped {
		t.Error("Uninstall did not stop the daemon")
	}
}

func TestStatusReportsDownAndUnwired(t *testing.T) {
	dir := t.TempDir()
	settings := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(settings, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	ok := Status(Options{Home: filepath.Join(dir, "h"), SettingsPath: settings}, &out)
	if ok {
		t.Error("Status reported healthy for a down, un-wired install")
	}
	if !strings.Contains(out.String(), "daemon") || !strings.Contains(out.String(), "hooks") {
		t.Errorf("Status output missing daemon/hooks lines:\n%s", out.String())
	}
}

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

func TestHooksWired_AbsentSettings(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.json")
	if HooksWired(Options{SettingsPath: missing}) {
		t.Fatal("no settings file → not wired")
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
