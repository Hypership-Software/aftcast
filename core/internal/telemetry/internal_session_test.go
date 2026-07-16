package telemetry

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Hypership-Software/aftcast/internal/audit"
	"github.com/Hypership-Software/aftcast/internal/project"
	"github.com/Hypership-Software/aftcast/internal/schema"
)

func TestProjectExcludesInternalSessions(t *testing.T) {
	dir := t.TempDir()
	key := []byte("0123456789abcdef0123456789abcdef")
	log, err := audit.NewLog(filepath.Join(dir, "log"), key)
	if err != nil {
		t.Fatalf("NewLog: %v", err)
	}
	defer log.Close()

	// A real session, then the init self-check marker as the LAST (highest-seq)
	// event — so the watermark must still advance past it even though it is filtered.
	for _, e := range []schema.TelemetryEvent{
		{SessionID: "real-1", EventType: schema.EventUserPrompt},
		{SessionID: "real-1", EventType: schema.EventPreTool},
		{SessionID: schema.SelfCheckSessionID, EventType: schema.EventPreTool},
	} {
		if err := log.Record(e); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}

	store, err := OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()
	if err := store.Project(log); err != nil {
		t.Fatalf("Project: %v", err)
	}

	sessions, err := store.Sessions()
	if err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	if len(sessions) != 1 || sessions[0].SessionID != "real-1" {
		t.Fatalf("read-model should hold only the real session, got %+v", sessions)
	}

	// The marker's events must not be queryable either.
	if evs, _ := store.EventsForSession(schema.SelfCheckSessionID); len(evs) != 0 {
		t.Fatalf("self-check events leaked into the read-model: %d", len(evs))
	}

	// Watermark advanced past the filtered marker: a re-project is a no-op and the
	// real session is unchanged.
	if err := store.Project(log); err != nil {
		t.Fatalf("re-Project: %v", err)
	}
	sessions2, _ := store.Sessions()
	if len(sessions2) != 1 {
		t.Fatalf("re-project changed the read-model: %+v", sessions2)
	}
}

func TestProjectExcludesEmptyShellSessions(t *testing.T) {
	dir := t.TempDir()
	key := []byte("0123456789abcdef0123456789abcdef")
	log, err := audit.NewLog(filepath.Join(dir, "log"), key)
	if err != nil {
		t.Fatalf("NewLog: %v", err)
	}
	defer log.Close()

	// A real session (prompt + tool call), then an empty shell — a Claude Code
	// session that opened and closed with no interaction (session_start + stop,
	// zero prompts, zero tool calls). The shell is recorded LAST so the watermark
	// must still advance past it even though it is dropped from the read-model.
	for _, e := range []schema.TelemetryEvent{
		{SessionID: "real-1", EventType: schema.EventUserPrompt},
		{SessionID: "real-1", EventType: schema.EventPreTool},
		{SessionID: "shell-1", EventType: schema.EventSessionStart},
		{SessionID: "shell-1", EventType: schema.EventStop},
	} {
		if err := log.Record(e); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}

	store, err := OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()
	if err := store.Project(log); err != nil {
		t.Fatalf("Project: %v", err)
	}

	sessions, err := store.Sessions()
	if err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	if len(sessions) != 1 || sessions[0].SessionID != "real-1" {
		t.Fatalf("read-model should hold only the session with activity, got %+v", sessions)
	}
	// The shell's events must not be queryable either — a phantom session with no
	// row to open would still leak into any events consumer.
	if evs, _ := store.EventsForSession("shell-1"); len(evs) != 0 {
		t.Fatalf("empty-shell events leaked into the read-model: %d", len(evs))
	}
}

// A prompt-only session (the user asked something, Claude answered in prose with
// no tool call) IS a real session and must be kept — it is distinct from an empty
// shell. The insights table may hide it by default (0 tool calls), but analytics
// still counts it.
func TestProjectKeepsPromptOnlySession(t *testing.T) {
	evs := []schema.TelemetryEvent{
		{SessionID: "qa", EventType: schema.EventSessionStart, TS: "2026-07-14T00:00:00Z"},
		{SessionID: "qa", EventType: schema.EventUserPrompt, TS: "2026-07-14T00:00:01Z"},
		{SessionID: "qa", EventType: schema.EventStop, TS: "2026-07-14T00:00:02Z"},
	}
	got := foldSessions(evs)
	if len(got) != 1 || got[0].SessionID != "qa" {
		t.Fatalf("prompt-only session must be kept, got %+v", got)
	}
}

func TestFoldSessions_ProjectID(t *testing.T) {
	evs := []schema.TelemetryEvent{
		{SessionID: "s1", EventType: schema.EventSessionStart, TS: "2026-07-14T00:00:00Z"},
		{SessionID: "s1", EventType: schema.EventPreTool, Project: "proj123", TS: "2026-07-14T00:00:01Z"},
	}
	got := foldSessions(evs)
	if len(got) != 1 || got[0].ProjectID != "proj123" {
		t.Fatalf("ProjectID = %q, want proj123", got[0].ProjectID)
	}
}

func TestFoldSessionsInfersLocalProjectName(t *testing.T) {
	repoA := makeTestRepo(t, "zeta", "git@github.com:acme/zeta.git")
	repoB := makeTestRepo(t, "alpha", "git@github.com:acme/alpha.git")
	_, idA := project.Identify(repoA)
	missing := filepath.Join(t.TempDir(), "deleted", "gone.go")

	events := []schema.TelemetryEvent{
		fileEvent("current", idA, filepath.Join(repoA, "current.go")),
		fileEvent("legacy", "", filepath.Join(repoB, "legacy.go")),
		fileEvent("mixed", "", filepath.Join(repoA, "one.go"), filepath.Join(repoB, "one.go"), filepath.Join(repoB, "two.go")),
		fileEvent("tie", "", filepath.Join(repoA, "tie.go"), filepath.Join(repoB, "tie.go")),
		fileEvent("mismatch", idA, filepath.Join(repoB, "wrong.go")),
		fileEvent("deleted", "", missing),
	}

	got := map[string]Session{}
	for _, session := range foldSessions(events) {
		got[session.SessionID] = session
	}
	if got["current"].ProjectName != filepath.Base(repoA) {
		t.Fatalf("current ProjectName = %q, want %q", got["current"].ProjectName, filepath.Base(repoA))
	}
	if got["legacy"].ProjectName != filepath.Base(repoB) {
		t.Fatalf("legacy ProjectName = %q, want %q", got["legacy"].ProjectName, filepath.Base(repoB))
	}
	if got["mixed"].ProjectName != filepath.Base(repoB) {
		t.Fatalf("mixed ProjectName = %q, want majority %q", got["mixed"].ProjectName, filepath.Base(repoB))
	}
	wantTie := filepath.Base(repoA)
	if filepath.Base(repoB) < wantTie {
		wantTie = filepath.Base(repoB)
	}
	if got["tie"].ProjectName != wantTie {
		t.Fatalf("tie ProjectName = %q, want lexicographic %q", got["tie"].ProjectName, wantTie)
	}
	// A session id that no observed repository proves falls back to where the
	// files demonstrably live — a remote rename kills the old id forever, and
	// orphaning that history into "other project" is worse than trusting the
	// session's own file evidence.
	if got["mismatch"].ProjectName != filepath.Base(repoB) {
		t.Fatalf("mismatch ProjectName = %q, want file-evidence fallback %q", got["mismatch"].ProjectName, filepath.Base(repoB))
	}
	if got["deleted"].ProjectName != "" {
		t.Fatalf("deleted ProjectName = %q, want empty — no repository on disk proves anything", got["deleted"].ProjectName)
	}
}

func TestFoldSessionsProjectNameSurvivesRemoteRename(t *testing.T) {
	repo := makeTestRepo(t, "atlas", "git@github.com:acme/atlas-old.git")
	_, oldID := project.Identify(repo)

	config := "[remote \"origin\"]\n\turl = git@github.com:acme/aftcast-new.git\n"
	if err := os.WriteFile(filepath.Join(repo, ".git", "config"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, newID := project.Identify(repo); newID == oldID {
		t.Fatalf("fixture broken: rename must change the project id")
	}

	got := foldSessions([]schema.TelemetryEvent{
		fileEvent("renamed", oldID, filepath.Join(repo, "main.go")),
	})
	if len(got) != 1 || got[0].ProjectName != filepath.Base(repo) {
		t.Fatalf("renamed-remote session ProjectName = %q, want %q", got[0].ProjectName, filepath.Base(repo))
	}
}

func makeTestRepo(t *testing.T, name, remote string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), name)
	gitDir := filepath.Join(root, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	config := "[remote \"origin\"]\n\turl = " + remote + "\n"
	if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func fileEvent(sessionID, projectID string, files ...string) schema.TelemetryEvent {
	return schema.TelemetryEvent{
		SessionID: sessionID,
		EventType: schema.EventPreTool,
		ToolClass: schema.ClassFileRead,
		Project:   projectID,
		Files:     files,
	}
}
