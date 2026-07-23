package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Hypership-Software/aftcast/internal/handoff"
	"github.com/Hypership-Software/aftcast/internal/hookcmd"
	"github.com/Hypership-Software/aftcast/internal/insights"
	"github.com/Hypership-Software/aftcast/internal/install"
	"github.com/Hypership-Software/aftcast/internal/meta"
	"github.com/Hypership-Software/aftcast/internal/project"
	"github.com/Hypership-Software/aftcast/internal/svc"
	"github.com/Hypership-Software/aftcast/internal/ui"
)

func helpText() string {
	return ui.Bold("aftcast — local observability for AI coding agents") + `

usage: aftcast <command>

commands:
  init         wire Claude Code hooks and start the observer daemon
  status       daemon + hook health at a glance
  doctor       detailed wiring checks
  insights     browse captured sessions and analytics
  coach        what keeps failing and is worth a permanent fix (coach export <id>, coach distill <id>)
  handoff      digest skeleton for a branch or commit (how it came to exist)
  stop         stop the background daemon
  uninstall    remove hooks and stop the daemon
  daemon run   run the daemon in the foreground
  version      print version`
}

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	if len(args) == 0 {
		return insightsCmd(nil)
	}
	switch args[0] {
	case "help", "-h", "--help":
		fmt.Fprintln(os.Stdout, helpText())
		return 0
	case "version":
		fmt.Printf("%s %s\n", meta.BinaryName(), meta.Version())
		return 0
	case "hook":
		harness := "claudecode"
		if len(args) > 1 {
			harness = args[1]
		}
		return hookcmd.Run(harness, os.Stdin, os.Stdout, os.Stderr)
	case "daemon":
		return daemon(args[1:])
	case "init":
		if err := install.Init(install.Options{}, os.Stdout); err != nil {
			return fail("init", err)
		}
		return 0
	case "uninstall":
		if err := install.Uninstall(install.Options{}, os.Stdout); err != nil {
			return fail("uninstall", err)
		}
		return 0
	case "status":
		if install.Status(install.Options{}, os.Stdout) {
			return 0
		}
		return 1
	case "stop":
		stopped, err := svc.Stop("")
		if err != nil {
			return fail("stop", err)
		}
		if stopped {
			fmt.Fprintln(os.Stdout, ui.OK("stopped the Aftcast daemon"))
		} else {
			fmt.Fprintln(os.Stdout, ui.Hint("no Aftcast daemon was running"))
		}
		return 0
	case "doctor":
		if install.Doctor(install.Options{}, os.Stdout) {
			return 0
		}
		return 1
	case "insights":
		return insightsCmd(args[1:])
	case "coach":
		return coachCmd(args[1:])
	case "handoff":
		return handoffCmd(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s\n", args[0], helpText())
		return 2
	}
}

// insightsCmd opens the project-scoped insights TUI, falling back to the global
// view with --all. The "not set up" hint only shows when hooks are unwired AND
// there is no captured data, so history stays reachable after `aftcast uninstall`
// and a settings-read error can't hide a populated dashboard. This is also what
// bare `aftcast` dispatches to.
func insightsCmd(args []string) int {
	global := false
	for _, a := range args {
		if a == "--all" {
			global = true
		}
	}
	wired := install.HooksWired(install.Options{})
	notSetUp := ui.Hint("Aftcast isn't set up yet — run `aftcast init`.")
	store, err := svc.OpenReadModel("")
	if err != nil {
		if !wired {
			fmt.Fprintln(os.Stdout, notSetUp)
			return 0
		}
		return fail("insights", err)
	}
	defer store.Close()
	if !wired {
		if sessions, serr := store.Sessions(); serr == nil && len(sessions) == 0 {
			fmt.Fprintln(os.Stdout, notSetUp)
			return 0
		}
	}
	wd, _ := os.Getwd()
	root, id := project.Identify(wd)
	name := ""
	if root != "" {
		name = filepath.Base(root)
	}
	if err := insights.Run(store, insights.Scope{ProjectID: id, Name: name, StartGlobal: global}); err != nil {
		return fail("insights", err)
	}
	return 0
}

// coachCmd surfaces recurring friction worth a permanent fix; `export <id>`
// writes one fingerprint's evidence bundle to stdout for an agent to encode.
// `distill <id>` writes a skill-drafting bundle instead — transcript
// coordinates only, taint-gated, attested against the audit chain.
func coachCmd(args []string) int {
	usage := func() int {
		fmt.Fprintln(os.Stderr, "usage: aftcast coach [export <id>|distill <id>]")
		return 2
	}
	if len(args) > 0 && ((args[0] != "export" && args[0] != "distill") || len(args) != 2) {
		return usage()
	}
	store, err := svc.OpenReadModel("")
	if err != nil {
		return fail("coach", err)
	}
	defer store.Close()
	if len(args) == 0 {
		if err := insights.CoachReport(store, os.Stdout, time.Now()); err != nil {
			return fail("coach", err)
		}
		return 0
	}
	if args[0] == "distill" {
		rep, err := svc.VerifyLog("")
		if err != nil {
			return fail("coach", err)
		}
		if err := insights.CoachDistill(store, args[1], rep, os.Stdout, time.Now()); err != nil {
			return fail("coach", err)
		}
		return 0
	}
	if err := insights.CoachExport(store, args[1], os.Stdout, time.Now()); err != nil {
		return fail("coach", err)
	}
	return 0
}

// handoffCmd assembles the digest skeleton for a ref (branch or commit,
// defaulting to HEAD) and writes it to the working directory.
func handoffCmd(args []string) int {
	ref := ""
	if len(args) > 0 {
		ref = args[0]
	}
	cwd, err := os.Getwd()
	if err != nil {
		return fail("handoff", err)
	}
	out, err := handoff.Run("", cwd, ref)
	if err != nil {
		return fail("handoff", err)
	}
	name := ref
	if name == "" {
		name = "HEAD"
	}
	path := "aftcast-handoff-" + sanitizeRef(name) + ".md"
	if err := os.WriteFile(path, out, 0o600); err != nil {
		return fail("handoff", err)
	}
	fmt.Printf("wrote %s — the narrative sections carry instructions for your own Claude; review before sharing.\n", path)
	return 0
}

// sanitizeRef makes a ref safe for use in a filename: every byte outside
// [A-Za-z0-9._-] becomes a hyphen.
func sanitizeRef(ref string) string {
	b := []byte(ref)
	for i, c := range b {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '.', c == '_', c == '-':
		default:
			b[i] = '-'
		}
	}
	return string(b)
}

// fail prints a styled error line to stderr and returns exit code 1.
func fail(cmd string, err error) int {
	fmt.Fprintf(os.Stderr, "%s aftcast %s: %v\n", ui.Bad("error:"), cmd, err)
	return 1
}

// daemon runs the resident observer. `run` serves in the foreground until
// interrupted; OS-service registration for auto-start is deferred.
func daemon(args []string) int {
	sub := "run"
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "run":
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		if err := svc.Run(ctx, svc.Options{}); err != nil {
			return fail("daemon", err)
		}
		return 0
	case "install", "uninstall":
		fmt.Fprintf(os.Stderr, "%s OS-service registration is not wired yet — run `aftcast daemon run` in the foreground for now.\n", ui.Warn("aftcast daemon "+sub+":"))
		return 2
	default:
		fmt.Fprintln(os.Stderr, "usage: aftcast daemon <run>")
		return 2
	}
}
