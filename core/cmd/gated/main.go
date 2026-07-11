package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Hypership-Software/atlas/internal/hookcmd"
	"github.com/Hypership-Software/atlas/internal/install"
	"github.com/Hypership-Software/atlas/internal/meta"
	"github.com/Hypership-Software/atlas/internal/svc"
	"github.com/Hypership-Software/atlas/internal/ui"
)

func helpText() string {
	return ui.Bold("gated — local observability for AI coding agents") + `

usage: gated <command>

commands:
  init         wire Claude Code hooks and start the observer daemon
  status       daemon + hook health at a glance
  doctor       detailed wiring checks
  stop         stop the background daemon
  uninstall    remove hooks and stop the daemon
  daemon run   run the daemon in the foreground
  version      print version`
}

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, helpText())
		return 2
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
			fmt.Fprintln(os.Stdout, ui.OK("stopped the Atlas daemon"))
		} else {
			fmt.Fprintln(os.Stdout, ui.Hint("no Atlas daemon was running"))
		}
		return 0
	case "doctor":
		if install.Doctor(install.Options{}, os.Stdout) {
			return 0
		}
		return 1
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s\n", args[0], helpText())
		return 2
	}
}

// fail prints a styled error line to stderr and returns exit code 1.
func fail(cmd string, err error) int {
	fmt.Fprintf(os.Stderr, "%s gated %s: %v\n", ui.Bad("error:"), cmd, err)
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
		fmt.Fprintf(os.Stderr, "%s OS-service registration is not wired yet — run `gated daemon run` in the foreground for now.\n", ui.Warn("gated daemon "+sub+":"))
		return 2
	default:
		fmt.Fprintln(os.Stderr, "usage: gated daemon <run>")
		return 2
	}
}
