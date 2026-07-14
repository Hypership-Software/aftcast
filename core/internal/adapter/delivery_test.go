package adapter

import (
	"testing"

	"github.com/Hypership-Software/atlas/internal/schema"
)

func detectCommand(t *testing.T, command string) schema.DeliverySignal {
	t.Helper()
	return deliverySignal(command)
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
		{"dry run short", `git push -n origin main`, ""},
		{"delete flag", `git push --delete origin old`, ""},
		{"delete refspec", `git push origin :old`, ""},
		{"force delete refspec", `git push origin +:old`, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detectCommand(t, tt.command); got != tt.want {
				t.Fatalf("deliverySignal(%q) = %q, want %q", tt.command, got, tt.want)
			}
		})
	}
}
