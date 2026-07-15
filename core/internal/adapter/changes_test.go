package adapter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Hypership-Software/aftcast/internal/schema"
)

func TestChangeStatsEditAndMultiEdit(t *testing.T) {
	for _, tt := range []struct {
		name    string
		fixture string
		added   int
		removed int
	}{
		{"edit", "pretooluse-edit.json", 2, 1},
		{"multi edit", "pretooluse-multiedit.json", 2, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var h ccHook
			if err := json.Unmarshal(fixture(t, tt.fixture), &h); err != nil {
				t.Fatal(err)
			}
			got := changeStats(h)
			if got == nil || got.LinesAdded != tt.added || got.LinesRemoved != tt.removed {
				t.Fatalf("changeStats = %+v, want +%d/-%d", got, tt.added, tt.removed)
			}
		})
	}
}

func TestChangeStatsWriteNewExistingAndUnchanged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.go")

	newFile := writeHook(dir, path, "one\ntwo\n")
	if got := changeStats(newFile); got == nil || got.LinesAdded != 2 || got.LinesRemoved != 0 {
		t.Fatalf("new write = %+v", got)
	}

	if err := os.WriteFile(path, []byte("one\nold\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	existing := writeHook(dir, path, "one\nnew\nextra\n")
	if got := changeStats(existing); got == nil || got.LinesAdded != 2 || got.LinesRemoved != 1 {
		t.Fatalf("existing write = %+v", got)
	}

	unchanged := writeHook(dir, path, "one\nold\n")
	if got := changeStats(unchanged); got == nil || *got != (schema.ChangeStats{}) {
		t.Fatalf("unchanged write = %+v", got)
	}
}

func TestChangeStatsRejectsOversizedInput(t *testing.T) {
	var h ccHook
	h.ToolName = "Edit"
	h.ToolInput = json.RawMessage(`{"file_path":"x","old_string":"a","new_string":"` + strings.Repeat("x", maxDiffBytes+1) + `"}`)
	if got := changeStats(h); got != nil {
		t.Fatalf("oversized change = %+v", got)
	}
}

func TestNormalizeChangeStatsAndOperationDoNotPersistContent(t *testing.T) {
	_, pre := normalize(t, "pretooluse-edit.json")
	if pre.Operation != schema.OperationEdit || pre.ChangeStats == nil || pre.ChangeStats.LinesAdded != 2 || pre.ChangeStats.LinesRemoved != 1 {
		t.Fatalf("pre event = %+v", pre)
	}
	raw, err := pre.Canonical()
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"DO-NOT-PERSIST-OLD", "DO-NOT-PERSIST-NEW", "old_string", "new_string"} {
		if strings.Contains(string(raw), marker) {
			t.Fatalf("event leaked %q: %s", marker, raw)
		}
	}

	postRaw := strings.ReplaceAll(string(fixture(t, "pretooluse-edit.json")), "PreToolUse", "PostToolUse")
	_, post, err := cc(t).Normalize("", []byte(postRaw))
	if err != nil {
		t.Fatal(err)
	}
	if post.ChangeStats != nil || post.Operation != schema.OperationEdit {
		t.Fatalf("post event = %+v", post)
	}
}

func writeHook(cwd, path, content string) ccHook {
	input, _ := json.Marshal(map[string]string{"file_path": path, "content": content})
	return ccHook{Cwd: cwd, ToolName: "Write", ToolInput: input}
}
