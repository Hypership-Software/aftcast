// Package ui is the CLI's shared presentation layer: small colour helpers that
// style text on a colour-capable terminal and return it untouched otherwise
// (NO_COLOR, or output piped/redirected), so the CLI stays greppable and
// copy-pasteable. Colours are the terminal's own ANSI palette, so they track the
// user's light/dark theme.
package ui

import (
	"os"

	"github.com/charmbracelet/lipgloss"
)

var (
	okStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	badStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	warnStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	hintStyle = lipgloss.NewStyle().Faint(true)
	boldStyle = lipgloss.NewStyle().Bold(true)
)

func OK(s string) string   { return render(okStyle, s) }
func Bad(s string) string  { return render(badStyle, s) }
func Warn(s string) string { return render(warnStyle, s) }
func Hint(s string) string { return render(hintStyle, s) }
func Bold(s string) string { return render(boldStyle, s) }

func render(style lipgloss.Style, s string) string {
	if s == "" || !colorEnabled() {
		return s
	}
	return style.Render(s)
}

// colorEnabled honours NO_COLOR and only styles when stdout is a terminal.
// Checked per call so it tracks the environment (including in tests).
func colorEnabled() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
