package ui

import (
	"strings"
	"testing"
)

// Helpers only wrap text in styling; they must never drop or mangle it, so piped
// output stays greppable and callers' substrings survive.
func TestHelpersPreserveText(t *testing.T) {
	helpers := map[string]func(string) string{
		"OK": OK, "Bad": Bad, "Warn": Warn, "Hint": Hint, "Bold": Bold,
	}
	for _, s := range []string{"ok", "FAIL", "daemon running", ""} {
		for name, f := range helpers {
			if got := f(s); !strings.Contains(got, s) {
				t.Errorf("%s(%q) = %q — text not preserved", name, s, got)
			}
		}
	}
}

// NO_COLOR yields exactly the input, no escape codes.
func TestNoColorIsPlain(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	if got := OK("hello"); got != "hello" {
		t.Errorf("OK with NO_COLOR = %q, want plain %q", got, "hello")
	}
}
