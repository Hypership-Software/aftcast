// Package telemetry projects the audit log into a SQLite read-model — the query
// surface behind the insights TUI and dashboard. The event log stays the single
// source of truth; this store is a rebuildable projection (Project is
// idempotent). Uses the pure-Go modernc.org/sqlite driver so the binary stays
// CGO-free.
package telemetry

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/Hypership-Software/atlas/internal/schema"

	_ "modernc.org/sqlite"
)

// Session is one folded summary row of the sessions table. Structural columns
// (identity, counts, timing, taint, skills) are computed by Project; the
// analytical columns (Outcome, CleanDelivery, CorrectionTurns, TaskType) are
// populated separately.
type Session struct {
	SessionID       string
	User            string
	Org             string
	Harness         string
	Started         string
	Ended           string
	ExitReason      string
	TurnCount       int
	ToolCalls       int
	DangerDetected  int
	Taint           bool
	Outcome         string
	CleanDelivery   bool
	CorrectionTurns int
	TaskType        string
	SkillsUsed      string
	DurationMS      int64
	FilesTouched    int
	CaptureVersion  int
	FilesChanged    int
	Shipped         bool
	ProjectID       string
}

type Store struct {
	db *sql.DB
}

const schemaDDL = `
CREATE TABLE IF NOT EXISTS sessions (
	session_id       TEXT PRIMARY KEY,
	user             TEXT,
	org              TEXT,
	harness          TEXT,
	started          TEXT,
	ended            TEXT,
	exit_reason      TEXT,
	turn_count       INTEGER,
	tool_calls       INTEGER,
	danger_detected  INTEGER,
	taint            INTEGER,
	outcome          TEXT,
	clean_delivery   INTEGER,
	correction_turns INTEGER,
	task_type        TEXT,
	skills_used      TEXT,
	duration_ms      INTEGER,
	files_touched    INTEGER,
	files_changed    INTEGER,
	shipped          INTEGER,
	capture_version  INTEGER,
	project_id       TEXT
);
CREATE TABLE IF NOT EXISTS events (
	seq         INTEGER PRIMARY KEY,
	session_id  TEXT,
	turn_index  INTEGER,
	event_type  TEXT,
	ts          TEXT,
	raw         TEXT
);
CREATE INDEX IF NOT EXISTS idx_events_session ON events(session_id, seq);
CREATE TABLE IF NOT EXISTS meta (
	key   TEXT PRIMARY KEY,
	value TEXT
);
`

func OpenStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// SQLite is single-writer; one connection avoids "database is locked" when a
	// projection tick overlaps a read from the insights surface.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schemaDDL); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// Sessions returns every folded session row, ordered by session_id.
func (s *Store) Sessions() ([]Session, error) {
	rows, err := s.db.Query(`SELECT session_id, user, org, harness, started, ended, exit_reason,
		turn_count, tool_calls, danger_detected, taint,
		outcome, clean_delivery, correction_turns, task_type, skills_used, duration_ms,
		files_touched, files_changed, shipped, capture_version, project_id
		FROM sessions ORDER BY session_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Session
	for rows.Next() {
		var session Session
		var taint, clean, shipped int
		if err := rows.Scan(&session.SessionID, &session.User, &session.Org, &session.Harness, &session.Started, &session.Ended, &session.ExitReason,
			&session.TurnCount, &session.ToolCalls, &session.DangerDetected, &taint,
			&session.Outcome, &clean, &session.CorrectionTurns, &session.TaskType, &session.SkillsUsed, &session.DurationMS,
			&session.FilesTouched, &session.FilesChanged, &shipped, &session.CaptureVersion, &session.ProjectID); err != nil {
			return nil, err
		}
		session.Taint = taint != 0
		session.CleanDelivery = clean != 0
		session.Shipped = shipped != 0
		out = append(out, session)
	}
	return out, rows.Err()
}

// EventsForSession returns a session's events in seq order, decoded from the
// events table's raw column (the marshaled TelemetryEvent Project stored).
func (s *Store) EventsForSession(id string) ([]schema.TelemetryEvent, error) {
	rows, err := s.db.Query(`SELECT raw FROM events WHERE session_id = ? ORDER BY seq`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []schema.TelemetryEvent
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var e schema.TelemetryEvent
		if err := json.Unmarshal([]byte(raw), &e); err != nil {
			return nil, fmt.Errorf("telemetry: decode event: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
