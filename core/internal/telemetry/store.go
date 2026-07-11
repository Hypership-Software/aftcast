// Package telemetry projects the tamper-evident audit log into a SQLite
// read-model — the query surface behind the insights TUI and the local
// dashboard. The event log remains the single source of truth; this store is a
// rebuildable projection of it (Project is idempotent and can be dropped and
// rebuilt at any time). Uses the pure-Go modernc.org/sqlite driver so the whole
// binary stays CGO-free and cross-compiles statically.
package telemetry

import (
	"database/sql"

	_ "modernc.org/sqlite"
)

// Session is one row of the read-model's sessions table: a folded summary of a
// single agent session. The structural columns (identity, counts, timing,
// taint, skills) are computed by Task 16's Project; the analytical columns
// (Outcome, OneShot, CorrectionTurns, TaskType) are populated by Task 17.
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
	OneShot         bool
	CorrectionTurns int
	TaskType        string
	SkillsUsed      string
	DurationMS      int64
}

// Store is a handle to the SQLite read-model database.
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
	one_shot         INTEGER,
	correction_turns INTEGER,
	task_type        TEXT,
	skills_used      TEXT,
	duration_ms      INTEGER
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

// OpenStore opens (creating if needed) the read-model at path and ensures the
// schema exists.
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

// Close releases the underlying database.
func (s *Store) Close() error { return s.db.Close() }

// Sessions returns every folded session row, ordered by session_id.
func (s *Store) Sessions() ([]Session, error) {
	rows, err := s.db.Query(`SELECT session_id, user, org, harness, started, ended, exit_reason,
		turn_count, tool_calls, danger_detected, taint,
		outcome, one_shot, correction_turns, task_type, skills_used, duration_ms
		FROM sessions ORDER BY session_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Session
	for rows.Next() {
		var s Session
		var taint, oneShot int
		if err := rows.Scan(&s.SessionID, &s.User, &s.Org, &s.Harness, &s.Started, &s.Ended, &s.ExitReason,
			&s.TurnCount, &s.ToolCalls, &s.DangerDetected, &taint,
			&s.Outcome, &oneShot, &s.CorrectionTurns, &s.TaskType, &s.SkillsUsed, &s.DurationMS); err != nil {
			return nil, err
		}
		s.Taint = taint != 0
		s.OneShot = oneShot != 0
		out = append(out, s)
	}
	return out, rows.Err()
}
