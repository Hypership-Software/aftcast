package adapter

import (
	"testing"

	"github.com/Hypership-Software/atlas/internal/schema"
	"github.com/google/shlex"
)

func commandTokens(t *testing.T, command string) []string {
	t.Helper()
	toks, err := shlex.Split(command)
	if err != nil {
		t.Fatal(err)
	}
	return toks
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
		{"status", `git status`, ""},
		{"quoted mention", `echo "git push"`, ""},
		{"unquoted echo", `echo git push`, ""},
		{"dry run long", `git push --dry-run origin main`, ""},
		{"dry run short", `git push -n origin main`, ""},
		{"delete flag", `git push --delete origin old`, ""},
		{"delete refspec", `git push origin :old`, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deliverySignal(commandTokens(t, tt.command)); got != tt.want {
				t.Fatalf("deliverySignal(%q) = %q, want %q", tt.command, got, tt.want)
			}
		})
	}
}
