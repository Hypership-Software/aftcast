package install

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Hypership-Software/atlas/internal/svc"
	"github.com/Hypership-Software/atlas/internal/ui"
)

// Daemon lifecycle seams, injectable in tests so init/uninstall are exercised
// without spawning a real detached process.
var (
	ensureDaemon = svc.Ensure
	stopDaemon   = svc.Stop
)

const (
	defaultTimeout  = 30 // seconds, per hook
	defaultHookPort = 47100
	backupSuffix    = ".gated-backup"
	probeTimeout    = 2 * time.Second
)

// Options configures init/uninstall/doctor. The zero value targets the current
// user's Claude Code settings and ~/.gated (or $GATED_HOME).
type Options struct {
	Home         string // gate state dir; "" => $GATED_HOME, else ~/.gated
	SettingsPath string // "" => ~/.claude/settings.json
	BinaryPath   string // gated binary for the SessionStart shim; "" => os.Executable
	Timeout      int    // per-hook timeout (seconds); 0 => defaultTimeout
}

// daemonInfo is the subset of daemon.json init/doctor read to find the live
// hook endpoint the daemon actually bound.
type daemonInfo struct {
	HTTPURL  string `json:"http_url"`
	HTTPPort int    `json:"http_port"`
}

// Init merges the gate's hook entries into Claude Code's settings (backing up
// the original first) so the next session is gated and observed. It self-verifies
// the wiring against a running daemon if one is reachable, and prints guidance if
// not. Writing hooks does not require the daemon to be running.
func Init(opts Options, w io.Writer) error {
	settingsPath, err := resolveSettingsPath(opts.SettingsPath)
	if err != nil {
		return err
	}
	cfg, err := hookConfig(opts)
	if err != nil {
		return err
	}

	// Start (or detect) the daemon first so the hooks point at the port it
	// actually bound — self-healing the baked-in port — and the next session is
	// observed immediately. Best-effort: if it can't start, still write the hooks
	// and report the gap; they reach the daemon once it is up.
	if info, started, eerr := ensureDaemon(svc.EnsureOptions{Home: resolveHome(opts.Home), Bin: opts.BinaryPath}); eerr != nil {
		fmt.Fprintf(w, "note: could not start the Atlas daemon (%v)\n", eerr)
	} else {
		cfg.HTTPURL = info.HTTPURL
		if started {
			fmt.Fprintf(w, "started the Atlas daemon in the background (port %d); stop it with `gated stop`\n", info.HTTPPort)
		} else {
			fmt.Fprintf(w, "Atlas daemon already running (port %d)\n", info.HTTPPort)
		}
	}

	orig, err := readSettings(settingsPath)
	if err != nil {
		return err
	}
	if err := backup(settingsPath, orig); err != nil {
		return fmt.Errorf("back up settings: %w", err)
	}
	merged, err := MergeHooks(orig, cfg)
	if err != nil {
		return fmt.Errorf("merge hooks: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(settingsPath, merged, 0o644); err != nil {
		return fmt.Errorf("write settings: %w", err)
	}
	// Ensure the user policy dir exists (the starter pack is embedded and always
	// active via policy.LoadWithStarter; this is where user/org policies land).
	_ = os.MkdirAll(filepath.Join(resolveHome(opts.Home), "policies"), 0o700)

	fmt.Fprintf(w, "wrote gate hooks to %s (backup: %s%s)\n", settingsPath, settingsPath, backupSuffix)
	fmt.Fprintf(w, "hook endpoint: %s\n", cfg.HTTPURL)

	if err := selfVerify(cfg.HTTPURL); err != nil {
		fmt.Fprintf(w, "%s could not verify a running daemon (%v)\n", ui.Warn("note:"), err)
		fmt.Fprint(w, ui.Hint("      start it with `gated daemon run`, then `gated doctor`.\n"))
	} else {
		fmt.Fprintf(w, "%s the daemon answered a probe over the hook endpoint.\n", ui.OK("verified:"))
	}
	return nil
}

// Uninstall removes only the gate's own hook entries, leaving the user's hooks
// and every other setting intact.
func Uninstall(opts Options, w io.Writer) error {
	settingsPath, err := resolveSettingsPath(opts.SettingsPath)
	if err != nil {
		return err
	}
	if stopped, serr := stopDaemon(resolveHome(opts.Home)); serr != nil {
		fmt.Fprintf(w, "note: could not stop the Atlas daemon (%v)\n", serr)
	} else if stopped {
		fmt.Fprintln(w, "stopped the Atlas daemon")
	}
	orig, err := readSettings(settingsPath)
	if err != nil {
		return err
	}
	cleaned, err := RemoveHooks(orig)
	if err != nil {
		return fmt.Errorf("remove hooks: %w", err)
	}
	if err := os.WriteFile(settingsPath, cleaned, 0o644); err != nil {
		return fmt.Errorf("write settings: %w", err)
	}
	fmt.Fprintf(w, "removed gate hooks from %s\n", settingsPath)
	return nil
}

// Doctor prints a pass/fail line per wiring check and reports whether everything
// is healthy. (Charm styling arrives with the Task 25 DX pass; the checks are
// the substance.)
func Doctor(opts Options, w io.Writer) bool {
	ok := true
	pass := func(label string, good bool, detail string) {
		mark, color := "FAIL", ui.Bad
		if good {
			mark, color = "ok", ui.OK
		} else {
			ok = false
		}
		fmt.Fprintf(w, "%s %s%s\n", color(fmt.Sprintf("[%-4s]", mark)), label, ui.Hint(detail))
	}

	settingsPath, err := resolveSettingsPath(opts.SettingsPath)
	if err != nil {
		pass("settings path", false, ": "+err.Error())
		return false
	}

	info, up := svc.Running(resolveHome(opts.Home))
	if up {
		pass("daemon running", true, fmt.Sprintf(" (port %d)", info.HTTPPort))
	} else {
		pass("daemon running", false, " — start it with `gated init` or `gated daemon run`")
	}

	orig, serr := readSettings(settingsPath)
	hasHTTP, hasSession := hooksPresent(orig)
	pass("http hooks in settings", serr == nil && hasHTTP, ": "+settingsPath)
	pass("SessionStart shim in settings", serr == nil && hasSession, "")

	if up {
		match := settingsPointAt(orig, info.HTTPURL)
		detail := " (" + info.HTTPURL + ")"
		if !match {
			detail += " — re-run `gated init` to repoint the hooks"
		}
		pass("settings port matches daemon", match, detail)
	}
	return ok
}

// Status prints a one-glance summary — is the daemon up, are the hooks wired —
// and returns whether both hold. The quick check; `doctor` is the detailed
// per-check breakdown.
func Status(opts Options, w io.Writer) bool {
	info, up := svc.Running(resolveHome(opts.Home))

	wired, portMatch := false, true
	if settingsPath, err := resolveSettingsPath(opts.SettingsPath); err == nil {
		if orig, rerr := readSettings(settingsPath); rerr == nil {
			http, session := hooksPresent(orig)
			wired = http && session
			if up && wired {
				portMatch = settingsPointAt(orig, info.HTTPURL)
			}
		}
	}

	if up {
		fmt.Fprintf(w, "daemon:  %s\n", ui.OK(fmt.Sprintf("running (port %d)", info.HTTPPort)))
	} else {
		fmt.Fprintf(w, "daemon:  %s%s\n", ui.Bad("not running"), ui.Hint(" — start it with `gated init` or `gated daemon run`"))
	}
	if wired {
		fmt.Fprintf(w, "hooks:   %s\n", ui.OK("wired into Claude Code settings"))
	} else {
		fmt.Fprintf(w, "hooks:   %s%s\n", ui.Bad("not wired"), ui.Hint(" — run `gated init`"))
	}
	if !portMatch {
		fmt.Fprintf(w, "port:    %s\n", ui.Warn(fmt.Sprintf("settings point at a different port than the daemon bound (%d) — re-run `gated init`", info.HTTPPort)))
	}
	return up && wired && portMatch
}

// --- helpers ---

func hookConfig(opts Options) (HookConfig, error) {
	bin := opts.BinaryPath
	if bin == "" {
		exe, err := os.Executable()
		if err != nil {
			return HookConfig{}, fmt.Errorf("locate gated binary: %w", err)
		}
		bin = exe
	}
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	// Prefer the URL the running daemon actually bound; fall back to the default
	// port when no daemon record exists yet (init can run before the daemon).
	hookURL := defaultHookURL()
	if info, err := readDaemonInfo(resolveHome(opts.Home)); err == nil && info.HTTPURL != "" {
		hookURL = info.HTTPURL
	}
	return HookConfig{
		HTTPURL: hookURL,
		Command: filepath.ToSlash(bin) + " hook claudecode",
		Timeout: timeout,
	}, nil
}

// selfVerify posts a benign PreToolUse probe to the hook endpoint and confirms
// the daemon answers 200, proving the settings URL reaches a live daemon. Atlas
// observes, so the response carries no decision — a clean 200 is the signal. The
// probe also records one event (a visible marker that init ran).
func selfVerify(hookURL string) error {
	payload := `{"hook_event_name":"PreToolUse","session_id":"gated-init-selfcheck","tool_name":"Read","tool_input":{"file_path":"/tmp/gated-selfcheck"}}`
	client := &http.Client{Timeout: probeTimeout}
	resp, err := client.Post(hookURL, "application/json", strings.NewReader(payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected response (status %d)", resp.StatusCode)
	}
	return nil
}

func hooksPresent(settings []byte) (http, session bool) {
	_, hooks, err := parse(settings)
	if err != nil {
		return false, false
	}
	for _, groups := range hooks {
		for _, g := range groups {
			for _, raw := range g.Hooks {
				var p hookProbe
				if json.Unmarshal(raw, &p) != nil {
					continue
				}
				if p.Type == "http" && loopbackHookURL(p.URL) {
					http = true
				}
				if p.Type == "command" && strings.Contains(p.Command, "hook claudecode") {
					session = true
				}
			}
		}
	}
	return http, session
}

func settingsPointAt(settings []byte, url string) bool {
	if url == "" {
		return false
	}
	_, hooks, err := parse(settings)
	if err != nil {
		return false
	}
	for _, groups := range hooks {
		for _, g := range groups {
			for _, raw := range g.Hooks {
				var p hookProbe
				if json.Unmarshal(raw, &p) == nil && p.Type == "http" && p.URL == url {
					return true
				}
			}
		}
	}
	return false
}

func readSettings(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return []byte("{}"), nil
	}
	return b, err
}

func backup(path string, content []byte) error {
	if len(content) == 0 || string(content) == "{}" {
		return nil // nothing meaningful to preserve
	}
	return os.WriteFile(path+backupSuffix, content, 0o644)
}

func readDaemonInfo(home string) (daemonInfo, error) {
	b, err := os.ReadFile(filepath.Join(home, "daemon.json"))
	if err != nil {
		return daemonInfo{}, err
	}
	var info daemonInfo
	if err := json.Unmarshal(b, &info); err != nil {
		return daemonInfo{}, err
	}
	return info, nil
}

func resolveSettingsPath(path string) (string, error) {
	if path != "" {
		return path, nil
	}
	// GATED_SETTINGS redirects the target (a project-level .claude/settings.json,
	// or a sandbox during testing) so init/doctor never touch the wrong file.
	if env := os.Getenv("GATED_SETTINGS"); env != "" {
		return env, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "settings.json"), nil
}

func resolveHome(home string) string {
	if home != "" {
		return home
	}
	if env := os.Getenv("GATED_HOME"); env != "" {
		return env
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".gated")
}

func defaultHookURL() string {
	return fmt.Sprintf("http://127.0.0.1:%d/hook", defaultHookPort)
}
