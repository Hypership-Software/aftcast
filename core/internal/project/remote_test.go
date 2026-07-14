package project

import "testing"

func TestNormalizeRemote(t *testing.T) {
	const want = "github.com/acme/app"
	cases := map[string]string{
		"git@github.com:Acme/App.git":            want,
		"https://github.com/acme/app":            want,
		"https://github.com/acme/app.git":        want,
		"ssh://git@github.com/acme/app/":         want,
		"github.com:acme/app.git":                want, // userless scp
		"ssh://git@github.com:2222/acme/app.git": want, // ssh with port
		"https://GitHub.com/Acme/App":            want, // host/case variants converge
		"":                                       "",
		"https://github.com":                     "", // host only, no org/repo
		"not-a-url":                              "",
	}
	for in, exp := range cases {
		if got := normalizeRemote(in); got != exp {
			t.Errorf("normalizeRemote(%q) = %q, want %q", in, got, exp)
		}
	}
}
