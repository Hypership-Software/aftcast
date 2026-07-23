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
	if !strings.Contains(string(out), "aftcast") {
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

func TestCoachUnknownSubcommand(t *testing.T) {
	if code := run([]string{"coach", "bogus"}); code != 2 {
		t.Fatalf("coach bogus: exit = %d, want 2", code)
	}
}

func TestEvidenceBadSince(t *testing.T) {
	if code := run([]string{"evidence", "--since", "bogus"}); code != 2 {
		t.Fatalf("evidence --since bogus: exit = %d, want 2", code)
	}
}

func TestEvidenceSinceMissingValue(t *testing.T) {
	if code := run([]string{"evidence", "--since"}); code != 2 {
		t.Fatalf("evidence --since (no value): exit = %d, want 2", code)
	}
}

func TestHelpMentionsCoach(t *testing.T) {
	if !strings.Contains(helpText(), "coach") {
		t.Fatalf("help text should list the coach command")
	}
}

func TestHelpMentionsEvidence(t *testing.T) {
	if !strings.Contains(helpText(), "evidence") {
		t.Fatalf("help text should list the evidence command")
	}
}
