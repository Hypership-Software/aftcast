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

func TestDaemonUnknownSubcommand(t *testing.T) {
	if code := run([]string{"daemon", "bogus"}); code != 2 {
		t.Fatalf("daemon bogus: exit = %d, want 2", code)
	}
}

func TestDaemonInstallNotWiredYet(t *testing.T) {
	if code := run([]string{"daemon", "install"}); code != 2 {
		t.Fatalf("daemon install: exit = %d, want 2 (deferred to install sprint)", code)
	}
}

func TestOffNotWiredYet(t *testing.T) {
	if code := run([]string{"off"}); code != 2 {
		t.Fatalf("off: exit = %d, want 2 (deferred; guidance printed)", code)
	}
}
