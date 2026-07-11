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
)

const usage = "usage: gated <daemon|hook|init|uninstall|doctor|version>"

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, usage)
		return 2
	}
	switch args[0] {
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
			fmt.Fprintf(os.Stderr, "gated init: %v\n", err)
			return 1
		}
		return 0
	case "uninstall":
		if err := install.Uninstall(install.Options{}, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "gated uninstall: %v\n", err)
			return 1
		}
		return 0
	case "doctor":
		if install.Doctor(install.Options{}, os.Stdout) {
			return 0
		}
		return 1
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n%s\n", args[0], usage)
		return 2
	}
}

// daemon runs and manages the resident gate. `run` serves in the foreground
// until interrupted (Ctrl+C / SIGTERM). OS-service registration (install/
// uninstall for auto-start) is deferred to the install sprint — foreground run
// is a complete, dogfoodable gate today.
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
			fmt.Fprintf(os.Stderr, "gated daemon: %v\n", err)
			return 1
		}
		return 0
	case "install", "uninstall":
		fmt.Fprintf(os.Stderr, "gated daemon %s: OS-service registration is not wired yet — run `gated daemon run` in the foreground for now.\n", sub)
		return 2
	default:
		fmt.Fprintf(os.Stderr, "usage: gated daemon <run>\n")
		return 2
	}
}
