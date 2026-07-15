package adapter

import (
	"testing"

	"github.com/Hypership-Software/aftcast/internal/schema"
)

func detectCommand(t *testing.T, command string) schema.DeliverySignal {
	t.Helper()
	return detectToolCommand(t, "Bash", command)
}

func detectToolCommand(t *testing.T, tool, command string) schema.DeliverySignal {
	t.Helper()
	return successfulDeliverySignal(tool, command)
}

func TestDeliverySignalGitPushVariants(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    schema.DeliverySignal
	}{
		{"bare", `git push`, schema.DeliveryGitPush},
		{"origin branch", `git push origin feature/coach`, schema.DeliveryGitPush},
		{"upstream flag", `git push --set-upstream origin feature/coach`, schema.DeliveryGitPush},
		{"git cwd", `git -C repo push origin main`, schema.DeliveryGitPush},
		{"environment prefix", `GIT_SSH_COMMAND="ssh -i key" git push`, schema.DeliveryGitPush},
		{"chained", `git commit -m coach && git push`, schema.DeliveryGitPush},
		{"later real segment", `echo preparing && git push`, schema.DeliveryGitPush},
		{"or fallback masks failure", `git push || true`, ""},
		{"or may skip push", `true || git push`, ""},
		{"failed suffix contradicts success", `git push && false`, ""},
		{"quoted separator", `echo "&&" git push`, ""},
		{"commented push", `echo ok # && git push`, ""},
		{"nested push", `echo $(true && git push )`, ""},
		{"unspaced separator", `echo ok;git push`, schema.DeliveryGitPush},
		{"status", `git status`, ""},
		{"quoted mention", `echo "git push"`, ""},
		{"unquoted echo", `echo git push`, ""},
		{"dry run long", `git push --dry-run origin main`, ""},
		{"dry run abbreviated long", `git push --dry-r origin main`, ""},
		{"dry run short", `git push -n origin main`, ""},
		{"dry run bundled after verbose", `git push -vn origin main`, ""},
		{"delete bundled after verbose", `git push -vd origin main`, ""},
		{"dry run delete bundle", `git push -nd origin main`, ""},
		{"delete flag", `git push --delete origin old`, ""},
		{"delete abbreviated long", `git push --del origin old`, ""},
		{"mirror flag", `git push --mirror origin`, ""},
		{"mirror minimum abbreviation", `git push --m origin`, ""},
		{"prune flag", `git push --prune origin`, ""},
		{"prune minimum abbreviation", `git push --pru origin`, ""},
		{"delete refspec", `git push origin :old`, ""},
		{"force delete refspec", `git push origin +:old`, ""},
		{"help long", `git push --help`, ""},
		{"help short", `git push -h`, ""},
		{"signed long option", `git push --signed origin main`, schema.DeliveryGitPush},
		{"porcelain long option", `git push --porcelain origin main`, schema.DeliveryGitPush},
		{"progress long option", `git push --progress origin main`, schema.DeliveryGitPush},
		{"attached push option", `git push -osend origin main`, schema.DeliveryGitPush},
		{"separate push option with flag-like value", `git push -o -nd origin main`, schema.DeliveryGitPush},
		{"long push option with flag-like value", `git push --push-option -nd origin main`, schema.DeliveryGitPush},
		{"abbreviated push option with flag-like value", `git push --pu -nd origin main`, schema.DeliveryGitPush},
		{"refspec containing bundle letters", `git push origin topic:refs/heads/send`, schema.DeliveryGitPush},
		{"option terminator protects repository", `git push -- -nd`, schema.DeliveryGitPush},
		{"hidden dry run variable", `PUSH_ARGS=--dry-run; git push $PUSH_ARGS`, ""},
		{"hidden quoted deletion refspec variable", `REF=:old; git push origin "$REF"`, ""},
		{"command substitution", `git push origin "$(printf main)"`, ""},
		{"wildcard glob", `git push origin refs/heads/*`, ""},
		{"brace expansion", `git push origin {main,release}`, ""},
		{"single quoted parameter literal", `git push origin '$REF'`, schema.DeliveryGitPush},
		{"escaped parameter literal", `git push origin \$REF`, schema.DeliveryGitPush},
		{"escaped command substitution literal", `git push origin \$\(literal\)`, schema.DeliveryGitPush},
		{"escaped glob literal", `git push origin refs/heads/\*`, schema.DeliveryGitPush},
		{"escaped brace literal", `git push origin \{main,release\}`, schema.DeliveryGitPush},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detectCommand(t, tt.command); got != tt.want {
				t.Fatalf("successfulDeliverySignal(Bash, %q) = %q, want %q", tt.command, got, tt.want)
			}
		})
	}
}

func TestSuccessfulDeliverySignalShellDialects(t *testing.T) {
	for _, shell := range []string{"Bash", "sh", "/bin/dash", "zsh", "ksh"} {
		t.Run(shell, func(t *testing.T) {
			if got := detectToolCommand(t, shell, "git push"); got != schema.DeliveryGitPush {
				t.Fatalf("successfulDeliverySignal(%q, git push) = %q, want git_push", shell, got)
			}
		})
	}

	for _, shell := range []string{"PowerShell", "pwsh", "cmd"} {
		t.Run(shell, func(t *testing.T) {
			command := `git push definitely-not-a-remote\; exit 0`
			if got := detectToolCommand(t, shell, command); got != "" {
				t.Fatalf("successfulDeliverySignal(%q, %q) = %q, want empty", shell, command, got)
			}
		})
	}
}
