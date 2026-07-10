package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestVersionSubcommand(t *testing.T) {
	out, err := exec.Command("go", "run", ".", "version").CombinedOutput()
	if err != nil {
		t.Fatalf("version failed: %v: %s", err, out)
	}
	if !strings.Contains(string(out), "gated") {
		t.Fatalf("want name, got: %s", out)
	}
}
