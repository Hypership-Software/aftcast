package main

import (
	"fmt"
	"os"

	"github.com/Hypership-Software/atlas/internal/hookcmd"
	"github.com/Hypership-Software/atlas/internal/meta"
)

const usage = "usage: gated <daemon|hook|init|policy|approvals|audit|insights|status|doctor|off|version>"

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
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n%s\n", args[0], usage)
		return 2
	}
}
