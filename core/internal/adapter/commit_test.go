package adapter

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNormalizeCapturesPermissionModeAndEffort(t *testing.T) {
	_, pre := normalize(t, "pretooluse-bash.json")
	if pre.PermissionMode != "bypassPermissions" {
		t.Errorf("permission_mode = %q, want bypassPermissions", pre.PermissionMode)
	}
	if pre.Effort != "xhigh" {
		t.Errorf("effort = %q, want xhigh", pre.Effort)
	}
	_, post := normalize(t, "posttooluse-git-commit.json")
	if post.PermissionMode != "default" {
		t.Errorf("permission_mode = %q, want default", post.PermissionMode)
	}
	if post.Effort != "high" {
		t.Errorf("effort = %q, want high", post.Effort)
	}
}

func TestNormalizeGitCommitCapturesSHA(t *testing.T) {
	_, e := normalize(t, "posttooluse-git-commit.json")
	if e.CommitSHA != "bb16536" {
		t.Errorf("commit_sha = %q, want bb16536", e.CommitSHA)
	}
	if e.DeliverySignal != "" {
		t.Errorf("delivery_signal = %q, want empty (a commit is not a push)", e.DeliverySignal)
	}
}

// The SHA is the only byte sequence extracted from tool output: the rest of
// stdout is content and must never reach the recorded event (ADR-011).
func TestNormalizeGitCommitNoStdoutLeak(t *testing.T) {
	_, e := normalize(t, "posttooluse-git-commit.json")
	blob, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"capture the join key", "5 files changed", "insertions"} {
		if strings.Contains(string(blob), fragment) {
			t.Errorf("stdout content leaked into recorded event: found %q in %s", fragment, blob)
		}
	}
}

func TestCommitSHA(t *testing.T) {
	tests := []struct {
		name    string
		tool    string
		command string
		stdout  string
		want    string
	}{
		{
			name:    "plain commit",
			tool:    "Bash",
			command: `git commit -m "fix: thing"`,
			stdout:  "[main 4a5b6c7] fix: thing\n 2 files changed",
			want:    "4a5b6c7",
		},
		{
			name:    "root commit format",
			tool:    "Bash",
			command: `git commit -m "initial"`,
			stdout:  "[main (root-commit) abc1234] initial\n 1 file changed",
			want:    "abc1234",
		},
		{
			name:    "detached head format",
			tool:    "Bash",
			command: `git commit -m "hotfix"`,
			stdout:  "[detached HEAD f00dbead] hotfix",
			want:    "f00dbead",
		},
		{
			name:    "compound with test run before commit",
			tool:    "Bash",
			command: `go test ./... && git commit -am "green"`,
			stdout:  "ok  \tpkg\t0.5s\n[main 1234abcd] green\n 3 files changed",
			want:    "1234abcd",
		},
		{
			name:    "commit then push takes the commit line",
			tool:    "Bash",
			command: `git commit -m "ship" && git push`,
			stdout:  "[main cafe123] ship\n 1 file changed\nTo github.com:org/repo.git\n   abc..def  main -> main",
			want:    "cafe123",
		},
		{
			name:    "amend after commit takes the last sha",
			tool:    "Bash",
			command: `git commit -m "v1" && git commit --amend --no-edit`,
			stdout:  "[main 1111aaa] v1\n 1 file changed\n[main 2222bbb] v1\n Date: now\n 1 file changed",
			want:    "2222bbb",
		},
		{
			name:    "powershell commit",
			tool:    "PowerShell",
			command: `git add -A; git commit -m "windows path"`,
			stdout:  "[main dead007] windows path\n 4 files changed",
			want:    "dead007",
		},
		{
			name:    "non-git segment after the commit cannot spoof the sha",
			tool:    "Bash",
			command: `git commit -m "real" && echo "[main 0000000] fake"`,
			stdout:  "[main abc1234] real\n 1 file changed\n[main 0000000] fake",
			want:    "",
		},
		{
			name:    "or-masked failure with trailing echo stamps nothing",
			tool:    "Bash",
			command: `git commit -m "x" || true; echo "[main 0000000] fake"`,
			stdout:  "On branch main\nnothing to commit\n[main 0000000] fake",
			want:    "",
		},
		{
			name:    "or-operator makes commit execution unprovable",
			tool:    "Bash",
			command: `go test ./... || git commit -am "green"`,
			stdout:  "[main 9999fff] spoofed by test output",
			want:    "",
		},
		{
			name:    "quiet commit prints no honest line so nothing is trusted",
			tool:    "Bash",
			command: `echo "[main 0000000] fake" && git commit -q -m "silent"`,
			stdout:  "[main 0000000] fake",
			want:    "",
		},
		{
			name:    "quiet flag in a short cluster is still quiet",
			tool:    "Bash",
			command: `git commit -aqm "silent"`,
			stdout:  "[main 0000000] fake",
			want:    "",
		},
		{
			name:    "no commit segment means no sha even if stdout matches",
			tool:    "Bash",
			command: `cat release-notes.txt`,
			stdout:  "[main 9876fed] looks like a commit line",
			want:    "",
		},
		{
			name:    "commit segment but no bracket line",
			tool:    "Bash",
			command: `git commit -m "nothing staged"`,
			stdout:  "On branch main\nnothing to commit, working tree clean",
			want:    "",
		},
		{
			name:    "git subcommand is not commit",
			tool:    "Bash",
			command: `git status`,
			stdout:  "[main 5555ccc] stale reflog text",
			want:    "",
		},
		{
			name:    "non-shell tool never parses",
			tool:    "Read",
			command: `git commit -m "x"`,
			stdout:  "[main 6666ddd] x",
			want:    "",
		},
		{
			name:    "unparseable command stays conservative",
			tool:    "Bash",
			command: `git commit -m "unterminated`,
			stdout:  "[main 7777eee] unterminated",
			want:    "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := commitSHA(tt.tool, tt.command, tt.stdout)
			if got != tt.want {
				t.Errorf("commitSHA(%q, %q) = %q, want %q", tt.command, tt.stdout, got, tt.want)
			}
		})
	}
}

// transcript_path rides the eval-only Descriptor so the daemon can sample
// context usage at a stop — it must never reach the persisted event.
func TestNormalizeCarriesTranscriptPathOnDescriptorOnly(t *testing.T) {
	d, e := normalize(t, "pretooluse-bash.json")
	if d.TranscriptPath == "" {
		t.Error("descriptor transcript_path is empty, want the payload's path")
	}
	blob, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(blob), "spike-transcript-0001") {
		t.Errorf("transcript path leaked into recorded event: %s", blob)
	}
}
